// internal/smtp/server.go
// SMTP Server 核心 - 啟動與管理 SMTP 伺服器

package smtp

import (
	"fmt"
	"log"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/services"
)

// Server SMTP 伺服器
type Server struct {
	cfg          *config.Config
	db           *gorm.DB
	queueService *services.QueueService
	keydbService *services.KeyDBService
	smtpServer   *gosmtp.Server
}

// NewServer 建立 SMTP 伺服器
func NewServer(cfg *config.Config, db *gorm.DB, queueService *services.QueueService, keydbService *services.KeyDBService) *Server {
	return &Server{
		cfg:          cfg,
		db:           db,
		queueService: queueService,
		keydbService: keydbService,
	}
}

// Start 啟動 SMTP 伺服器
func (s *Server) Start() error {
	// 建立 Backend
	backend := NewBackend(s.cfg, s.db, s.queueService, s.keydbService)

	// 設定 SMTP 伺服器
	s.smtpServer = gosmtp.NewServer(backend)
	s.smtpServer.Addr = fmt.Sprintf(":%s", s.cfg.SMTPInboundPort)
	s.smtpServer.Domain = "mail-proxy.local"
	s.smtpServer.ReadTimeout = 30 * time.Second
	s.smtpServer.WriteTimeout = 30 * time.Second
	s.smtpServer.MaxMessageBytes = int64(s.cfg.SMTPMaxMessageSize) * 1024 * 1024
	s.smtpServer.MaxRecipients = 50
	s.smtpServer.AllowInsecureAuth = true // 開發環境允許，生產環境應使用 TLS

	log.Printf("[SMTP] 伺服器啟動中... 監聽埠號: %s", s.cfg.SMTPInboundPort)
	log.Printf("[SMTP] 認證需求: %v", s.cfg.SMTPAuthRequired)
	log.Printf("[SMTP] 最大訊息大小: %d MB", s.cfg.SMTPMaxMessageSize)

	if len(s.cfg.SMTPAllowedDomains) > 0 {
		log.Printf("[SMTP] 允許的寄件網域: %v", s.cfg.SMTPAllowedDomains)
	} else {
		log.Printf("[SMTP] 允許所有寄件網域")
	}

	// 啟動伺服器（阻塞式）
	if err := s.smtpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("SMTP server error: %w", err)
	}

	return nil
}

// Shutdown 優雅關機
func (s *Server) Shutdown() error {
	if s.smtpServer != nil {
		log.Println("[SMTP] 正在關閉伺服器...")
		return s.smtpServer.Close()
	}
	return nil
}
