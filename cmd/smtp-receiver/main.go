// cmd/smtp-receiver/main.go
// SMTP Inbound Server 入口程式
// 接收外部 SMTP 郵件並轉發到 RabbitMQ 佇列

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
	"mail-proxy/internal/services"
	"mail-proxy/internal/smtp"
)

func main() {
	log.Println("========================================")
	log.Println("    Mail Proxy - SMTP Inbound Server")
	log.Println("========================================")
	log.Println("啟動 SMTP 接收服務...")

	// 載入設定
	cfg := config.Load()

	// 初始化資料庫
	log.Println("連接資料庫...")
	db := initDatabase(cfg)
	log.Println("資料庫連接成功")

	// 初始化 KeyDB
	log.Println("連接 KeyDB...")
	keydbService, err := services.NewKeyDBService(cfg)
	if err != nil {
		log.Fatalf("無法連接 KeyDB: %v", err)
	}
	log.Println("KeyDB 連接成功")

	// 初始化 RabbitMQ 佇列服務
	log.Println("連接 RabbitMQ...")
	queueService, err := services.NewQueueService(cfg)
	if err != nil {
		log.Fatalf("無法連接 RabbitMQ: %v", err)
	}
	defer queueService.Close()
	log.Println("RabbitMQ 連接成功")

	// 建立 SMTP 伺服器
	smtpServer := smtp.NewServer(cfg, db, queueService, keydbService)

	// 啟動 SMTP 伺服器（非同步）
	go func() {
		if err := smtpServer.Start(); err != nil {
			log.Fatalf("SMTP 伺服器錯誤: %v", err)
		}
	}()

	log.Println("========================================")
	log.Printf("SMTP 伺服器已啟動，監聽埠號: %s", cfg.SMTPInboundPort)
	log.Println("按 Ctrl+C 停止服務")
	log.Println("========================================")

	// 等待中斷信號
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在關閉 SMTP 伺服器...")

	// 優雅關機
	if err := smtpServer.Shutdown(); err != nil {
		log.Printf("關閉 SMTP 伺服器時發生錯誤: %v", err)
	}

	log.Println("SMTP 伺服器已停止")
}

// initDatabase 初始化資料庫連線
func initDatabase(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("無法連接資料庫: %v", err)
	}

	// 自動遷移（確保資料表存在）
	if err := db.AutoMigrate(&models.Mail{}, &models.Attachment{}); err != nil {
		log.Fatalf("資料庫遷移失敗: %v", err)
	}

	return db
}
