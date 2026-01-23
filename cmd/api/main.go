// cmd/api/main.go
// Gin RESTful API 入口

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mail-proxy/internal/api/routes"
	"mail-proxy/internal/config"
	"mail-proxy/internal/services"
	"mail-proxy/pkg/microsoft"
)

func main() {
	log.Println("Starting Mail Proxy API Server...")

	// 載入設定
	cfg := config.Load()

	// 初始化資料庫
	db, err := initDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 初始化 MIS Admin Token
	adminTokenService := services.NewAdminTokenService(cfg, db)
	if err := adminTokenService.InitializeAdminToken(); err != nil {
		log.Printf("Warning: Failed to initialize admin token: %v", err)
	}

	// 初始化 KeyDB
	keydbService, err := services.NewKeyDBService(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to KeyDB: %v", err)
	}
	defer keydbService.Close()

	// 初始化 RabbitMQ
	queueService, err := services.NewQueueService(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer queueService.Close()

	// 初始化 OAuth 服務
	oauthService := microsoft.NewOAuthService(
		cfg.MicrosoftTenantID,
		cfg.MicrosoftClientID,
		cfg.MicrosoftClientSecret,
	)

	// 初始化 Gin
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// 註冊路由
	routes.RegisterRoutes(router, &routes.Dependencies{
		Config:       cfg,
		DB:           db,
		OAuthService: oauthService,
		QueueService: queueService,
		KeyDBService: keydbService,
	})

	// 建立 HTTP Server
	srv := &http.Server{
		Addr:    ":" + cfg.APIPort,
		Handler: router,
	}

	// 優雅關機
	go func() {
		log.Printf("API Server listening on port %s", cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中斷信號
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down API server...")

	// 優雅關閉
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("API Server stopped")
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

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database connected successfully")
	return db, nil
}
