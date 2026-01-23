// internal/services/admin_token_service.go
// MIS Admin Token 初始化服務

package services

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

const (
	// MISAdminClientID 固定的 MIS Admin Client ID
	MISAdminClientID = "mis-admin"
	// MISAdminDepartment MIS 部門名稱
	MISAdminDepartment = "MIS"
)

// AdminTokenService Admin Token 初始化服務
type AdminTokenService struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewAdminTokenService 建立 Admin Token 服務
func NewAdminTokenService(cfg *config.Config, db *gorm.DB) *AdminTokenService {
	return &AdminTokenService{
		cfg: cfg,
		db:  db,
	}
}

// InitializeAdminToken 初始化 MIS Admin Token
// 若 Token 不存在則建立並輸出到 logs
// 若 Token 已存在且有效則跳過
// 若 Token 已存在但已撤銷則重新啟用並生成新 Token
func (s *AdminTokenService) InitializeAdminToken() error {
	if !s.cfg.InitAdminToken {
		log.Println("[Admin Token] INIT_ADMIN_TOKEN=false, skipping initialization")
		return nil
	}

	log.Println("[Admin Token] Checking for existing MIS admin token...")

	// 查詢是否已存在 mis-admin token
	var existingToken models.ClientToken
	err := s.db.Where("client_id = ?", MISAdminClientID).First(&existingToken).Error

	if err == nil {
		// Token 已存在
		if existingToken.IsActive {
			log.Println("[Admin Token] ✓ MIS admin token already exists and is active")
			log.Printf("[Admin Token]   Client ID: %s", existingToken.ClientID)
			log.Printf("[Admin Token]   Client Name: %s", existingToken.ClientName)
			log.Printf("[Admin Token]   Created At: %s", existingToken.CreatedAt.Format(time.RFC3339))
			log.Println("[Admin Token]   (Token value is not stored, only hash is retained)")
			return nil
		}

		// Token 已撤銷，重新啟用並生成新 Token
		log.Println("[Admin Token] Found revoked MIS admin token, regenerating...")
		return s.regenerateToken(&existingToken)
	}

	if err != gorm.ErrRecordNotFound {
		return err
	}

	// Token 不存在，建立新 Token
	log.Println("[Admin Token] No existing MIS admin token found, creating new one...")
	return s.createNewToken()
}

// createNewToken 建立新的 MIS Admin Token
func (s *AdminTokenService) createNewToken() error {
	// 建立 JWT Token (永久有效)
	tokenString, err := s.generateJWTToken()
	if err != nil {
		return err
	}

	// 計算 token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := hex.EncodeToString(hash[:])

	// 儲存到資料庫
	clientToken := models.ClientToken{
		ID:          uuid.New(),
		ClientID:    MISAdminClientID,
		ClientName:  s.cfg.AdminTokenName,
		Department:  MISAdminDepartment,
		Permissions: pq.StringArray{"admin"},
		TokenHash:   tokenHash,
		IsActive:    true,
	}

	if err := s.db.Create(&clientToken).Error; err != nil {
		return err
	}

	// 輸出 Token 到 logs (只有首次建立時輸出)
	s.printTokenToLogs(tokenString, &clientToken)

	return nil
}

// regenerateToken 重新生成 Token (針對已撤銷的 Token)
func (s *AdminTokenService) regenerateToken(existingToken *models.ClientToken) error {
	// 建立 JWT Token (永久有效)
	tokenString, err := s.generateJWTToken()
	if err != nil {
		return err
	}

	// 計算 token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := hex.EncodeToString(hash[:])

	// 更新資料庫
	existingToken.TokenHash = tokenHash
	existingToken.IsActive = true
	existingToken.RevokedAt = nil
	existingToken.ClientName = s.cfg.AdminTokenName

	if err := s.db.Save(existingToken).Error; err != nil {
		return err
	}

	// 輸出 Token 到 logs
	s.printTokenToLogs(tokenString, existingToken)

	return nil
}

// generateJWTToken 生成 JWT Token
func (s *AdminTokenService) generateJWTToken() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":         "mail-proxy-system",
		"sub":         uuid.New().String(),
		"iat":         now.Unix(),
		"client_id":   MISAdminClientID,
		"client_name": s.cfg.AdminTokenName,
		"department":  MISAdminDepartment,
		"permissions": []string{"admin"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

// printTokenToLogs 輸出 Token 到 logs
func (s *AdminTokenService) printTokenToLogs(tokenString string, clientToken *models.ClientToken) {
	separator := strings.Repeat("=", 80)

	log.Println("")
	log.Println(separator)
	log.Println("🔐 MIS ADMIN TOKEN CREATED SUCCESSFULLY")
	log.Println(separator)
	log.Println("")
	log.Printf("  Client ID:    %s", clientToken.ClientID)
	log.Printf("  Client Name:  %s", clientToken.ClientName)
	log.Printf("  Department:   %s", clientToken.Department)
	log.Printf("  Permissions:  %v", clientToken.Permissions)
	log.Printf("  Created At:   %s", clientToken.CreatedAt.Format(time.RFC3339))
	log.Println("")
	log.Println("  ⚠️  IMPORTANT: Copy the token below immediately!")
	log.Println("  ⚠️  This token will NOT be shown again.")
	log.Println("")
	log.Println("  Token:")
	log.Println("")
	log.Printf("  %s", tokenString)
	log.Println("")
	log.Println(separator)
	log.Println("🔒 建議 MIS 立即記錄此 Token 後清除 logs")
	log.Println("🔒 若有偵聽 Docker logs 的容器，請確保在 mail-proxy-api 啟動後才建立")
	log.Println(separator)
	log.Println("")
}
