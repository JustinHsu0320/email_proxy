// internal/config/config.go
// 設定模組 - 載入環境變數

package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config 應用程式設定
type Config struct {
	// 環境
	Env     string
	APIPort string

	// 資料庫
	DatabaseURL string

	// RabbitMQ
	RabbitMQURL     string
	MailQueueName   string
	RetryQueueName  string
	FailedQueueName string
	MaxRetryCount   int

	// KeyDB
	KeyDBURL       string
	KeyDBPassword  string
	KeyDBStatusTTL time.Duration

	// Microsoft OAuth 2.0
	MicrosoftTenantID     string
	MicrosoftClientID     string
	MicrosoftClientSecret string

	// SendGrid
	SendGridAPIKey string
	OrgEmailDomain string

	// 附件
	AttachmentPath      string
	MaxAttachmentSizeMB int

	// Worker
	WorkerConcurrency int
	WorkerPrefetch    int

	// JWT
	JWTSecret string

	// Encryption
	EncryptionKey string

	// Admin Token 初始化
	InitAdminToken bool
	AdminTokenName string

	// SMTP Inbound Server 設定
	SMTPInboundPort    string   // SMTP 監聽埠號 (預設: 2525)
	SMTPInboundTLSPort string   // SMTP TLS 監聽埠號 (預設: 1587)
	SMTPTLSEnabled     bool     // 是否啟用 TLS
	SMTPAuthRequired   bool     // 是否需要認證
	SMTPAllowedDomains []string // 允許的寄件網域 (空白表示允許全部)
	SMTPMaxMessageSize int      // 最大訊息大小 (MB)
}

// Load 載入設定
func Load() *Config {
	// 嘗試載入 .env 檔案 (開發環境)
	_ = godotenv.Load()

	return &Config{
		// 環境
		Env:     getEnv("APP_ENV", "development"),
		APIPort: getEnv("API_PORT", "8080"),

		// 資料庫
		DatabaseURL: getEnv("DATABASE_URL", "postgres://smtp_user:password@localhost:5432/smtp_service?sslmode=disable"),

		// RabbitMQ
		RabbitMQURL:     getEnv("RABBITMQ_URL", "amqp://admin:password@localhost:5672/"),
		MailQueueName:   getEnv("MAIL_QUEUE_NAME", "mail-queue"),
		RetryQueueName:  getEnv("RETRY_QUEUE_NAME", "retry-queue"),
		FailedQueueName: getEnv("FAILED_QUEUE_NAME", "failed-mails"),
		MaxRetryCount:   getEnvAsInt("MAX_RETRY_COUNT", 5),

		// KeyDB
		KeyDBURL:       getEnv("KEYDB_URL", "localhost:6379"),
		KeyDBPassword:  getEnv("KEYDB_PASSWORD", ""),
		KeyDBStatusTTL: time.Duration(getEnvAsInt("KEYDB_STATUS_TTL_DAYS", 14)) * 24 * time.Hour,

		// Microsoft OAuth 2.0
		MicrosoftTenantID:     getEnv("MICROSOFT_TENANT_ID", ""),
		MicrosoftClientID:     getEnv("MICROSOFT_CLIENT_ID", ""),
		MicrosoftClientSecret: getEnv("MICROSOFT_CLIENT_SECRET", ""),

		// SendGrid
		SendGridAPIKey: getEnv("SENDGRID_API_KEY", ""),
		OrgEmailDomain: getEnv("ORG_EMAIL_DOMAIN", "@ptc-nec.com.tw"),

		// 附件
		AttachmentPath:      getEnv("ATTACHMENT_VOLUME_PATH", "/app/attachments"),
		MaxAttachmentSizeMB: getEnvAsInt("MAX_ATTACHMENT_SIZE_MB", 25),

		// Worker
		WorkerConcurrency: getEnvAsInt("WORKER_CONCURRENCY", 10),
		WorkerPrefetch:    getEnvAsInt("WORKER_PREFETCH", 10),

		// JWT
		JWTSecret: getEnv("JWT_SECRET", "change-this-secret"),

		// Encryption (32 bytes for AES-256)
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),

		// Admin Token 初始化
		InitAdminToken: getEnvAsBool("INIT_ADMIN_TOKEN", true),
		AdminTokenName: getEnv("ADMIN_TOKEN_NAME", "MIS Admin"),

		// SMTP Inbound Server
		SMTPInboundPort:    getEnv("SMTP_INBOUND_PORT", "2525"),
		SMTPInboundTLSPort: getEnv("SMTP_INBOUND_TLS_PORT", "1587"),
		SMTPTLSEnabled:     getEnvAsBool("SMTP_TLS_ENABLED", false),
		SMTPAuthRequired:   getEnvAsBool("SMTP_AUTH_REQUIRED", false),
		SMTPAllowedDomains: getEnvAsSlice("SMTP_ALLOWED_DOMAINS", []string{}),
		SMTPMaxMessageSize: getEnvAsInt("SMTP_MAX_MESSAGE_SIZE_MB", 25),
	}
}

// getEnv 取得環境變數，若不存在則回傳預設值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt 取得環境變數並轉換為整數
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsBool 取得環境變數並轉換為布林值
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

// getEnvAsSlice 取得環境變數並轉換為字串切片（以逗號分隔）
func getEnvAsSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultValue
}
