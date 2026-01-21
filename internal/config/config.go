// internal/config/config.go
// 設定模組 - 載入環境變數

package config

import (
	"os"
	"strconv"
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

	// 附件
	AttachmentPath      string
	MaxAttachmentSizeMB int

	// Worker
	WorkerConcurrency int
	WorkerPrefetch    int

	// JWT
	JWTSecret string

	// Admin Token 初始化
	InitAdminToken bool
	AdminTokenName string
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

		// 附件
		AttachmentPath:      getEnv("ATTACHMENT_PATH", "/app/attachments"),
		MaxAttachmentSizeMB: getEnvAsInt("MAX_ATTACHMENT_SIZE_MB", 25),

		// Worker
		WorkerConcurrency: getEnvAsInt("WORKER_CONCURRENCY", 10),
		WorkerPrefetch:    getEnvAsInt("WORKER_PREFETCH", 10),

		// JWT
		JWTSecret: getEnv("JWT_SECRET", "change-this-secret"),

		// Admin Token 初始化
		InitAdminToken: getEnvAsBool("INIT_ADMIN_TOKEN", true),
		AdminTokenName: getEnv("ADMIN_TOKEN_NAME", "MIS Admin"),
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
