// internal/api/handlers/auth_handler.go
// Token 管理 API Handler

package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// AuthHandler Token 管理 Handler
type AuthHandler struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewAuthHandler 建立 Auth Handler
func NewAuthHandler(cfg *config.Config, db *gorm.DB) *AuthHandler {
	return &AuthHandler{
		cfg: cfg,
		db:  db,
	}
}

// CreateToken 建立新 Token
func (h *AuthHandler) CreateToken(c *gin.Context) {
	var req models.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// 產生唯一 client_id
	clientID := fmt.Sprintf("client_%s", uuid.New().String()[:8])

	// 建立 JWT Token (永久有效)
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":         "mail-proxy-system",
		"sub":         uuid.New().String(),
		"iat":         now.Unix(),
		"client_id":   clientID,
		"client_name": req.ClientName,
		"department":  req.Department,
		"permissions": req.Permissions,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "token_generation_error",
			"message": "Failed to generate token",
		})
		return
	}

	// 計算 token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := hex.EncodeToString(hash[:])

	// 儲存到資料庫
	clientToken := models.ClientToken{
		ID:          uuid.New(),
		ClientID:    clientID,
		ClientName:  req.ClientName,
		Department:  req.Department,
		Permissions: pq.StringArray(req.Permissions),
		TokenHash:   tokenHash,
		IsActive:    true,
	}

	if err := h.db.Create(&clientToken).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "database_error",
			"message": "Failed to save token",
		})
		return
	}

	c.JSON(http.StatusCreated, models.CreateTokenResponse{
		Token:     tokenString,
		ClientID:  clientID,
		CreatedAt: clientToken.CreatedAt,
	})
}

// GetToken 查詢 Token 資訊
func (h *AuthHandler) GetToken(c *gin.Context) {
	tokenID := c.Param("id")

	var clientToken models.ClientToken
	// 先嘗試用 client_id 查詢，再嘗試用 UUID 查詢
	query := h.db.Where("client_id = ?", tokenID)
	if _, err := uuid.Parse(tokenID); err == nil {
		query = query.Or("id = ?", tokenID)
	}

	if err := query.First(&clientToken).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "not_found",
			"message": "Token not found",
		})
		return
	}

	c.JSON(http.StatusOK, clientToken)
}

// RevokeToken 撤銷 Token
func (h *AuthHandler) RevokeToken(c *gin.Context) {
	tokenID := c.Param("id")

	// 建立查詢條件
	query := h.db.Model(&models.ClientToken{}).Where("client_id = ?", tokenID)
	if _, err := uuid.Parse(tokenID); err == nil {
		query = query.Or("id = ?", tokenID)
	}

	now := time.Now()
	result := query.Updates(map[string]interface{}{
		"is_active":  false,
		"revoked_at": now,
	})

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "not_found",
			"message": "Token not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token 已撤銷",
	})
}

// ListTokens 列出所有 Token
func (h *AuthHandler) ListTokens(c *gin.Context) {
	var tokens []models.ClientToken
	h.db.Order("created_at DESC").Find(&tokens)

	c.JSON(http.StatusOK, gin.H{
		"total": len(tokens),
		"data":  tokens,
	})
}
