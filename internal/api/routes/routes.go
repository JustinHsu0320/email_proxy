// internal/api/routes/routes.go
// Gin 路由註冊

package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smtp-service/internal/api/handlers"
	"smtp-service/internal/api/middlewares"
	"smtp-service/internal/config"
	"smtp-service/internal/services"
	"smtp-service/pkg/microsoft"
)

// Dependencies 路由依賴
type Dependencies struct {
	Config       *config.Config
	DB           *gorm.DB
	OAuthService *microsoft.OAuthService
	SMTPService  *services.SMTPService
	QueueService *services.QueueService
	KeyDBService *services.KeyDBService
}

// RegisterRoutes 註冊所有路由
func RegisterRoutes(router *gin.Engine, deps *Dependencies) {
	// 初始化 Handlers
	healthHandler := handlers.NewHealthHandler(deps.Config, deps.DB, deps.KeyDBService)
	mailHandler := handlers.NewMailHandler(deps.Config, deps.DB, deps.QueueService, deps.KeyDBService)
	authHandler := handlers.NewAuthHandler(deps.Config, deps.DB)

	// 公開路由
	router.GET("/health", healthHandler.Health)

	// API v1 路由群組
	v1 := router.Group("/api/v1")
	{
		// 郵件相關 API (需認證)
		mail := v1.Group("/mail")
		mail.Use(middlewares.JWTAuth(deps.Config, deps.DB))
		{
			mail.POST("/send", mailHandler.Send)
			mail.POST("/send/batch", mailHandler.SendBatch)
			mail.GET("/status/:id", mailHandler.GetStatus)
			mail.GET("/history", mailHandler.GetHistory)
			mail.DELETE("/cancel/:id", mailHandler.Cancel)
		}

		// Token 管理 API (需 admin 權限)
		auth := v1.Group("/auth")
		auth.Use(middlewares.JWTAuth(deps.Config, deps.DB))
		auth.Use(middlewares.RequirePermission("admin"))
		{
			auth.POST("/token", authHandler.CreateToken)
			auth.GET("/token/:id", authHandler.GetToken)
			auth.DELETE("/token/:id", authHandler.RevokeToken)
			auth.GET("/tokens", authHandler.ListTokens)
		}
	}
}
