// cmd/worker/main.go
// RabbitMQ Worker 入口

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mail-proxy/internal/config"
	"mail-proxy/internal/services"
	"mail-proxy/internal/worker"
	"mail-proxy/pkg/microsoft"
)

func main() {
	log.Println("Starting Mail Proxy Worker...")

	// 載入設定
	cfg := config.Load()

	// 初始化資料庫
	db, err := initDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 初始化 KeyDB
	keydbService, err := services.NewKeyDBService(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to KeyDB: %v", err)
	}
	defer keydbService.Close()

	// 初始化 OAuth 服務
	oauthService := microsoft.NewOAuthService(
		cfg.MicrosoftTenantID,
		cfg.MicrosoftClientID,
		cfg.MicrosoftClientSecret,
	)

	if !oauthService.IsConfigured() {
		log.Println("WARNING: Microsoft OAuth not configured, Graph API mail sending will fail")
	}

	// 初始化 Graph API 郵件服務
	graphMailService := services.NewGraphMailService(cfg, oauthService)

	// 初始化 SendGrid 郵件服務
	sendgridService := services.NewSendGridService(cfg)
	if !sendgridService.IsConfigured() {
		log.Println("WARNING: SendGrid API Key not configured, SendGrid mail sending will fail")
	}

	// 初始化郵件路由服務
	mailRouter := services.NewMailRouter(cfg, graphMailService, sendgridService)
	if err := mailRouter.ValidateConfiguration(); err != nil {
		log.Printf("WARNING: Mail router configuration issue: %v", err)
	}

	// 初始化 Consumer
	consumer := worker.NewConsumer(cfg, db, oauthService, mailRouter, keydbService)

	// 啟動 Consumer
	go func() {
		if err := consumer.Start(); err != nil {
			log.Fatalf("Failed to start worker: %v", err)
		}
	}()

	log.Printf("Worker started with concurrency: %d", cfg.WorkerConcurrency)

	// 等待中斷信號
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker...")
	consumer.GracefulShutdown()

	log.Println("Worker stopped")
}

// initDatabase 初始化資料庫連接
func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	gormLogger := logger.Default
	if cfg.Env == "production" {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}

	// 設定連接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database connected successfully")
	return db, nil
}
