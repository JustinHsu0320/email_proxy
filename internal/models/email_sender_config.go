// internal/models/email_sender_config.go
// Email Sender Config 資料模型

package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailSenderConfig Email Sender Config 資料模型
// 儲存每個 Client 的 Microsoft OAuth 配置
type EmailSenderConfig struct {
	ID                      uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ClientTokenID           uuid.UUID `json:"client_token_id" gorm:"type:uuid;not null"`
	SenderEmail             string    `json:"sender_email" gorm:"not null"`
	MSTenantID              string    `json:"ms_tenant_id" gorm:"not null"`
	MSClientID              string    `json:"ms_client_id" gorm:"not null"`
	MSClientSecretEncrypted string    `json:"-" gorm:"column:ms_client_secret_encrypted;not null"`
	IsActive                bool      `json:"is_active" gorm:"default:true"`
	CreatedAt               time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt               time.Time `json:"updated_at" gorm:"autoUpdateTime"`

	// 關聯
	ClientToken *ClientToken `json:"client_token,omitempty" gorm:"foreignKey:ClientTokenID"`
}

// TableName 指定資料表名稱
func (EmailSenderConfig) TableName() string {
	return "email_sender_configs"
}

// CreateSenderConfigRequest 建立 Sender Config 請求
type CreateSenderConfigRequest struct {
	SenderEmail    string `json:"sender_email" binding:"required,email"`
	MSTenantID     string `json:"ms_tenant_id" binding:"required"`
	MSClientID     string `json:"ms_client_id" binding:"required"`
	MSClientSecret string `json:"ms_client_secret" binding:"required"`
}

// UpdateSenderConfigRequest 更新 Sender Config 請求
type UpdateSenderConfigRequest struct {
	MSTenantID     string `json:"ms_tenant_id"`
	MSClientID     string `json:"ms_client_id"`
	MSClientSecret string `json:"ms_client_secret"`
	IsActive       *bool  `json:"is_active"`
}

// SenderConfigResponse Sender Config 回應
type SenderConfigResponse struct {
	ID             uuid.UUID `json:"id"`
	ClientTokenID  uuid.UUID `json:"client_token_id"`
	SenderEmail    string    `json:"sender_email"`
	MSTenantID     string    `json:"ms_tenant_id"`
	MSClientID     string    `json:"ms_client_id"`
	MSClientSecret string    `json:"ms_client_secret"` // 遮罩後的值
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ToResponse 轉換為回應結構 (secret 遮罩)
func (e *EmailSenderConfig) ToResponse(decryptedSecret string) SenderConfigResponse {
	maskedSecret := maskSecret(decryptedSecret)
	return SenderConfigResponse{
		ID:             e.ID,
		ClientTokenID:  e.ClientTokenID,
		SenderEmail:    e.SenderEmail,
		MSTenantID:     e.MSTenantID,
		MSClientID:     e.MSClientID,
		MSClientSecret: maskedSecret,
		IsActive:       e.IsActive,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// maskSecret 遮罩 secret (顯示前4後4碼)
func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}
