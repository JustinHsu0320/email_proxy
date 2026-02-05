// internal/api/handlers/sender_config_handler.go
// Sender Config 管理 API Handler

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mail-proxy/internal/models"
	"mail-proxy/internal/services"
)

// SenderConfigHandler Sender Config 管理 Handler
type SenderConfigHandler struct {
	senderConfigService *services.EmailSenderConfigService
}

// NewSenderConfigHandler 建立 Sender Config Handler
func NewSenderConfigHandler(senderConfigService *services.EmailSenderConfigService) *SenderConfigHandler {
	return &SenderConfigHandler{
		senderConfigService: senderConfigService,
	}
}

// CreateSenderConfig 建立新的 sender config
// POST /api/v1/auth/sender-config
func (h *SenderConfigHandler) CreateSenderConfig(c *gin.Context) {
	// 從 context 取得 client_token_id
	clientTokenIDStr, exists := c.Get("client_token_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "unauthorized",
			"message": "Client token ID not found",
		})
		return
	}

	clientTokenID, err := uuid.Parse(clientTokenIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_client_token_id",
			"message": "Invalid client token ID",
		})
		return
	}

	var req models.CreateSenderConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	config, err := h.senderConfigService.Create(clientTokenID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "create_error",
			"message": err.Error(),
		})
		return
	}

	// 回傳遮罩後的 response
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    config.ToResponse(req.MSClientSecret),
	})
}

// ListSenderConfigs 列出當前 Client 的所有 sender configs
// GET /api/v1/auth/sender-configs
func (h *SenderConfigHandler) ListSenderConfigs(c *gin.Context) {
	clientTokenIDStr, exists := c.Get("client_token_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "unauthorized",
			"message": "Client token ID not found",
		})
		return
	}

	clientTokenID, err := uuid.Parse(clientTokenIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_client_token_id",
			"message": "Invalid client token ID",
		})
		return
	}

	configs, err := h.senderConfigService.ListByClientTokenID(clientTokenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "list_error",
			"message": err.Error(),
		})
		return
	}

	// 轉換為遮罩後的 response
	responses := make([]models.SenderConfigResponse, len(configs))
	for i, cfg := range configs {
		responses[i] = cfg.ToResponse("") // 空字串會被遮罩為 ****
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"total":   len(responses),
		"data":    responses,
	})
}

// GetSenderConfig 查詢單一 sender config
// GET /api/v1/auth/sender-config/:id
func (h *SenderConfigHandler) GetSenderConfig(c *gin.Context) {
	configID := c.Param("id")

	id, err := uuid.Parse(configID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_id",
			"message": "Invalid config ID",
		})
		return
	}

	config, err := h.senderConfigService.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "not_found",
			"message": "Sender config not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config.ToResponse(""),
	})
}

// UpdateSenderConfig 更新 sender config
// PUT /api/v1/auth/sender-config/:id
func (h *SenderConfigHandler) UpdateSenderConfig(c *gin.Context) {
	configID := c.Param("id")

	id, err := uuid.Parse(configID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_id",
			"message": "Invalid config ID",
		})
		return
	}

	var req models.UpdateSenderConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	config, err := h.senderConfigService.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "update_error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config.ToResponse(""),
	})
}

// DeleteSenderConfig 刪除 sender config
// DELETE /api/v1/auth/sender-config/:id
func (h *SenderConfigHandler) DeleteSenderConfig(c *gin.Context) {
	configID := c.Param("id")

	id, err := uuid.Parse(configID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_id",
			"message": "Invalid config ID",
		})
		return
	}

	if err := h.senderConfigService.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "delete_error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Sender config 已刪除",
	})
}
