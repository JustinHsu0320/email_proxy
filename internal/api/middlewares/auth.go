// internal/api/middlewares/auth.go
// JWT 認證中介軟體

package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// JWTAuth JWT 認證中介軟體
func JWTAuth(cfg *config.Config, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 取得 Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "missing_token",
				"message": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// 解析 Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "invalid_token_format",
				"message": "Authorization header must be Bearer token",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析 JWT Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// 確認簽名方法
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "invalid_token",
				"message": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// 取得 Claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "invalid_claims",
				"message": "Invalid token claims",
			})
			c.Abort()
			return
		}

		// 取得 client_id
		clientID, ok := claims["client_id"].(string)
		if !ok || clientID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "invalid_client",
				"message": "Token missing client_id",
			})
			c.Abort()
			return
		}

		// 驗證 Token 是否有效 (未撤銷)
		var clientToken models.ClientToken
		if err := db.Where("client_id = ? AND is_active = ?", clientID, true).First(&clientToken).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "token_revoked",
				"message": "Token has been revoked or is inactive",
			})
			c.Abort()
			return
		}

		// 設定 context
		c.Set("client_id", clientID)
		c.Set("client_name", claims["client_name"])
		c.Set("department", claims["department"])
		c.Set("permissions", claims["permissions"])

		c.Next()
	}
}

// RequirePermission 權限檢查中介軟體
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		permsInterface, exists := c.Get("permissions")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "no_permissions",
				"message": "No permissions found",
			})
			c.Abort()
			return
		}

		// 轉換權限列表
		var permissions []string
		switch v := permsInterface.(type) {
		case []interface{}:
			for _, p := range v {
				if s, ok := p.(string); ok {
					permissions = append(permissions, s)
				}
			}
		case []string:
			permissions = v
		}

		// 檢查權限
		hasPermission := false
		for _, p := range permissions {
			if p == permission || p == "admin" {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "permission_denied",
				"message": "You don't have permission to access this resource",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
