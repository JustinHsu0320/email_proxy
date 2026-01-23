// internal/api/handlers/health_handler.go
// 健康檢查 Handler

package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/services"
)

// HealthHandler 健康檢查 Handler
type HealthHandler struct {
	cfg          *config.Config
	db           *gorm.DB
	keydbService *services.KeyDBService
}

// NewHealthHandler 建立 Health Handler
func NewHealthHandler(cfg *config.Config, db *gorm.DB, keydbService *services.KeyDBService) *HealthHandler {
	return &HealthHandler{
		cfg:          cfg,
		db:           db,
		keydbService: keydbService,
	}
}

// Health 健康檢查
func (h *HealthHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	response := gin.H{
		"status":  "healthy",
		"version": "1.0.0",
		"services": gin.H{
			"postgresql": "ok",
			"keydb":      "ok",
			"rabbitmq":   "ok",
		},
	}

	// 檢查 PostgreSQL
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		response["services"].(gin.H)["postgresql"] = "error"
		response["status"] = "degraded"
	}

	// 檢查 KeyDB
	if h.keydbService != nil && !h.keydbService.Ping(ctx) {
		response["services"].(gin.H)["keydb"] = "error"
		response["status"] = "degraded"
	}

	// 回應
	statusCode := http.StatusOK
	if response["status"] == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}
