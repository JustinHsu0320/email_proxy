// internal/models/mail.go
// 郵件資料模型

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MailStatus 郵件狀態
type MailStatus string

const (
	MailStatusQueued     MailStatus = "queued"
	MailStatusProcessing MailStatus = "processing"
	MailStatusSent       MailStatus = "sent"
	MailStatusFailed     MailStatus = "failed"
	MailStatusCancelled  MailStatus = "cancelled"
)

// Mail 郵件資料模型
type Mail struct {
	ID           uuid.UUID         `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	FromAddress  string            `json:"from" gorm:"column:from_address;not null"`
	ToAddresses  pq.StringArray    `json:"to" gorm:"column:to_addresses;type:text[];not null"`
	CCAddresses  pq.StringArray    `json:"cc,omitempty" gorm:"column:cc_addresses;type:text[]"`
	BCCAddresses pq.StringArray    `json:"bcc,omitempty" gorm:"column:bcc_addresses;type:text[]"`
	Subject      string            `json:"subject" gorm:"not null"`
	Body         string            `json:"body,omitempty"`
	HTML         string            `json:"html,omitempty"`
	Status       MailStatus        `json:"status" gorm:"not null;default:'queued'"`
	RetryCount   int               `json:"retry_count" gorm:"default:0"`
	ErrorMessage string            `json:"error_message,omitempty"`
	SentAt       *time.Time        `json:"sent_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time         `json:"updated_at" gorm:"autoUpdateTime"`
	ClientID     string            `json:"client_id" gorm:"not null"`
	ClientName   string            `json:"client_name,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty" gorm:"type:jsonb"`

	// 關聯
	Attachments []Attachment `json:"attachments,omitempty" gorm:"foreignKey:MailID"`
}

// TableName 指定資料表名稱
func (Mail) TableName() string {
	return "mails"
}

// Attachment 附件資料模型
type Attachment struct {
	ID          uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	MailID      uuid.UUID `json:"mail_id" gorm:"type:uuid;not null"`
	Filename    string    `json:"filename" gorm:"not null"`
	ContentType string    `json:"content_type,omitempty"`
	SizeBytes   int64     `json:"size_bytes,omitempty"`
	StoragePath string    `json:"storage_path" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定資料表名稱
func (Attachment) TableName() string {
	return "attachments"
}

// MailJob RabbitMQ 訊息格式
type MailJob struct {
	MailID       string            `json:"mail_id"`
	FromAddress  string            `json:"from"`
	ToAddresses  []string          `json:"to"`
	CCAddresses  []string          `json:"cc,omitempty"`
	BCCAddresses []string          `json:"bcc,omitempty"`
	Subject      string            `json:"subject"`
	Body         string            `json:"body,omitempty"`
	HTML         string            `json:"html,omitempty"`
	Attachments  []AttachmentInfo  `json:"attachments,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RetryCount   int               `json:"retry_count"`
}

// AttachmentInfo 附件資訊
type AttachmentInfo struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	StoragePath string `json:"storage_path"`
}

// MailStatusCache KeyDB 快取格式
type MailStatusCache struct {
	MailID       string `json:"mail_id"`
	Status       string `json:"status"`
	RetryCount   int    `json:"retry_count"`
	LastUpdated  string `json:"last_updated"`
	ErrorMessage string `json:"error_message,omitempty"`
}
