// internal/models/client.go
// Client Token 資料模型

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ClientToken Client Token 資料模型
type ClientToken struct {
	ID          uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ClientID    string         `json:"client_id" gorm:"uniqueIndex;not null"`
	ClientName  string         `json:"client_name" gorm:"not null"`
	Department  string         `json:"department,omitempty"`
	Permissions pq.StringArray `json:"permissions" gorm:"type:text[];not null"`
	TokenHash   string         `json:"-" gorm:"not null"`
	CreatedAt   time.Time      `json:"created_at" gorm:"autoCreateTime"`
	RevokedAt   *time.Time     `json:"revoked_at,omitempty"`
	IsActive    bool           `json:"is_active" gorm:"default:true"`
}

// TableName 指定資料表名稱
func (ClientToken) TableName() string {
	return "client_tokens"
}

// APILog API 請求日誌
type APILog struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientID       string    `json:"client_id" gorm:"not null"`
	ClientName     string    `json:"client_name,omitempty"`
	RequestIP      string    `json:"request_ip,omitempty"`
	Endpoint       string    `json:"endpoint" gorm:"not null"`
	Method         string    `json:"method" gorm:"not null"`
	MailID         *string   `json:"mail_id,omitempty"`
	StatusCode     int       `json:"status_code,omitempty"`
	ResponseTimeMs int       `json:"response_time_ms,omitempty"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定資料表名稱
func (APILog) TableName() string {
	return "api_logs"
}

// JWTClaims JWT Token Claims
type JWTClaims struct {
	Issuer      string   `json:"iss"`
	Subject     string   `json:"sub"`
	IssuedAt    int64    `json:"iat"`
	ClientID    string   `json:"client_id"`
	ClientName  string   `json:"client_name"`
	Department  string   `json:"department,omitempty"`
	Permissions []string `json:"permissions"`
}

// CreateTokenRequest 建立 Token 請求
type CreateTokenRequest struct {
	ClientName  string   `json:"client_name" binding:"required"`
	Department  string   `json:"department"`
	Permissions []string `json:"permissions" binding:"required,min=1"`
}

// CreateTokenResponse 建立 Token 回應
type CreateTokenResponse struct {
	Token     string    `json:"token"`
	ClientID  string    `json:"client_id"`
	CreatedAt time.Time `json:"created_at"`
}
