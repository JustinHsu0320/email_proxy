// internal/smtp/backend.go
// SMTP Backend 介面實作 - 處理 SMTP 連線認證與 Session 建立

package smtp

import (
	"log"

	gosmtp "github.com/emersion/go-smtp"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/services"
)

// Backend 實作 smtp.Backend 介面
// 負責處理 SMTP 連線並建立 Session
type Backend struct {
	cfg          *config.Config         // 應用程式設定
	db           *gorm.DB               // 資料庫連線
	queueService *services.QueueService // RabbitMQ 佇列服務
	keydbService *services.KeyDBService // KeyDB 快取服務
}

// NewBackend 建立 SMTP Backend
func NewBackend(cfg *config.Config, db *gorm.DB, queueService *services.QueueService, keydbService *services.KeyDBService) *Backend {
	return &Backend{
		cfg:          cfg,
		db:           db,
		queueService: queueService,
		keydbService: keydbService,
	}
}

// NewSession 建立新的 SMTP Session
// 實作 smtp.Backend 介面
func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	log.Printf("[SMTP] 新連線來自: %s", c.Hostname())

	return NewSession(b.cfg, b.db, b.queueService, b.keydbService), nil
}
