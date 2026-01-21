// internal/api/handlers/mail_handler.go
// 郵件 API Handler

package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"smtp-service/internal/config"
	"smtp-service/internal/models"
	"smtp-service/internal/services"
)

// MailHandler 郵件 Handler
type MailHandler struct {
	cfg          *config.Config
	db           *gorm.DB
	queueService *services.QueueService
	keydbService *services.KeyDBService
}

// NewMailHandler 建立 Mail Handler
func NewMailHandler(cfg *config.Config, db *gorm.DB, queueService *services.QueueService, keydbService *services.KeyDBService) *MailHandler {
	return &MailHandler{
		cfg:          cfg,
		db:           db,
		queueService: queueService,
		keydbService: keydbService,
	}
}

// SendRequest 發送郵件請求
type SendRequest struct {
	From        string              `json:"from" binding:"required,email"`
	To          []string            `json:"to" binding:"required,min=1,dive,email"`
	CC          []string            `json:"cc,omitempty" binding:"omitempty,dive,email"`
	BCC         []string            `json:"bcc,omitempty" binding:"omitempty,dive,email"`
	Subject     string              `json:"subject" binding:"required"`
	Body        string              `json:"body,omitempty"`
	HTML        string              `json:"html,omitempty"`
	Attachments []AttachmentRequest `json:"attachments,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
}

// AttachmentRequest 附件請求
type AttachmentRequest struct {
	Filename    string `json:"filename" binding:"required"`
	Content     string `json:"content" binding:"required"`
	ContentType string `json:"content_type"`
}

// Send 發送單封郵件
func (h *MailHandler) Send(c *gin.Context) {
	var req SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// 取得 client 資訊
	clientID, _ := c.Get("client_id")
	clientName, _ := c.Get("client_name")

	// 建立郵件記錄
	mail := models.Mail{
		ID:           uuid.New(),
		FromAddress:  req.From,
		ToAddresses:  pq.StringArray(req.To),
		CCAddresses:  pq.StringArray(req.CC),
		BCCAddresses: pq.StringArray(req.BCC),
		Subject:      req.Subject,
		Body:         req.Body,
		HTML:         req.HTML,
		Status:       models.MailStatusQueued,
		ClientID:     clientID.(string),
		ClientName:   clientName.(string),
		Metadata:     req.Metadata,
	}

	// 處理附件
	var attachments []models.AttachmentInfo
	for _, att := range req.Attachments {
		// 檢查附件大小
		content, err := base64.StdEncoding.DecodeString(att.Content)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "invalid_attachment",
				"message": fmt.Sprintf("Invalid base64 content for %s", att.Filename),
			})
			return
		}

		sizeMB := float64(len(content)) / 1024 / 1024
		if sizeMB > float64(h.cfg.MaxAttachmentSizeMB) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "attachment_too_large",
				"message": fmt.Sprintf("%s exceeds maximum size of %dMB", att.Filename, h.cfg.MaxAttachmentSizeMB),
			})
			return
		}

		// 儲存附件
		storagePath := filepath.Join(
			h.cfg.AttachmentPath,
			time.Now().Format("2006/01/02"),
			mail.ID.String(),
			att.Filename,
		)

		if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "storage_error",
				"message": "Failed to create attachment directory",
			})
			return
		}

		if err := os.WriteFile(storagePath, content, 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "storage_error",
				"message": "Failed to save attachment",
			})
			return
		}

		// 記錄附件
		attachment := models.Attachment{
			ID:          uuid.New(),
			MailID:      mail.ID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   int64(len(content)),
			StoragePath: storagePath,
		}
		mail.Attachments = append(mail.Attachments, attachment)

		attachments = append(attachments, models.AttachmentInfo{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			StoragePath: storagePath,
		})
	}

	// 儲存到資料庫
	if err := h.db.Create(&mail).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "database_error",
			"message": "Failed to create mail record",
		})
		return
	}

	// 建立 RabbitMQ 訊息
	job := models.MailJob{
		MailID:       mail.ID.String(),
		FromAddress:  mail.FromAddress,
		ToAddresses:  req.To,
		CCAddresses:  req.CC,
		BCCAddresses: req.BCC,
		Subject:      mail.Subject,
		Body:         mail.Body,
		HTML:         mail.HTML,
		Attachments:  attachments,
		Metadata:     req.Metadata,
		RetryCount:   0,
	}

	// 發送到 RabbitMQ
	if err := h.queueService.PublishMail(&job); err != nil {
		// 更新狀態為失敗
		h.db.Model(&mail).Update("status", models.MailStatusFailed)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "queue_error",
			"message": "Failed to queue mail",
		})
		return
	}

	// 更新 KeyDB 狀態
	h.keydbService.SetStatus(c.Request.Context(), mail.ID.String(), "queued", 0, "")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"mail_id": mail.ID.String(),
		"status":  "queued",
		"message": "郵件已加入發送隊列",
	})
}

// SendBatch 批次發送郵件
func (h *MailHandler) SendBatch(c *gin.Context) {
	var req struct {
		Mails []SendRequest `json:"mails" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// 取得 client 資訊
	clientID, _ := c.Get("client_id")
	clientName, _ := c.Get("client_name")

	batchID := uuid.New().String()
	results := make([]gin.H, 0, len(req.Mails))

	for _, mailReq := range req.Mails {
		result := h.processSingleMail(c, mailReq, clientID.(string), clientName.(string))
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"batch_id": batchID,
		"results":  results,
	})
}

// processSingleMail 處理單封郵件 (批次發送內部使用)
func (h *MailHandler) processSingleMail(c *gin.Context, req SendRequest, clientID, clientName string) gin.H {
	// 建立郵件記錄
	mail := models.Mail{
		ID:           uuid.New(),
		FromAddress:  req.From,
		ToAddresses:  pq.StringArray(req.To),
		CCAddresses:  pq.StringArray(req.CC),
		BCCAddresses: pq.StringArray(req.BCC),
		Subject:      req.Subject,
		Body:         req.Body,
		HTML:         req.HTML,
		Status:       models.MailStatusQueued,
		ClientID:     clientID,
		ClientName:   clientName,
		Metadata:     req.Metadata,
	}

	// 處理附件
	var attachments []models.AttachmentInfo
	for _, att := range req.Attachments {
		// 解碼 Base64
		content, err := base64.StdEncoding.DecodeString(att.Content)
		if err != nil {
			return gin.H{
				"mail_id": nil,
				"status":  "failed",
				"error":   fmt.Sprintf("Invalid base64 content for %s", att.Filename),
			}
		}

		// 檢查大小
		sizeMB := float64(len(content)) / 1024 / 1024
		if sizeMB > float64(h.cfg.MaxAttachmentSizeMB) {
			return gin.H{
				"mail_id": nil,
				"status":  "failed",
				"error":   fmt.Sprintf("%s exceeds maximum size of %dMB", att.Filename, h.cfg.MaxAttachmentSizeMB),
			}
		}

		// 儲存附件
		storagePath := filepath.Join(
			h.cfg.AttachmentPath,
			time.Now().Format("2006/01/02"),
			mail.ID.String(),
			att.Filename,
		)

		if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
			return gin.H{
				"mail_id": nil,
				"status":  "failed",
				"error":   "Failed to create attachment directory",
			}
		}

		if err := os.WriteFile(storagePath, content, 0644); err != nil {
			return gin.H{
				"mail_id": nil,
				"status":  "failed",
				"error":   "Failed to save attachment",
			}
		}

		// 記錄附件
		attachment := models.Attachment{
			ID:          uuid.New(),
			MailID:      mail.ID,
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   int64(len(content)),
			StoragePath: storagePath,
		}
		mail.Attachments = append(mail.Attachments, attachment)

		attachments = append(attachments, models.AttachmentInfo{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			StoragePath: storagePath,
		})
	}

	// 儲存到資料庫
	if err := h.db.Create(&mail).Error; err != nil {
		return gin.H{
			"mail_id": nil,
			"status":  "failed",
			"error":   "Failed to create mail record",
		}
	}

	// 建立 RabbitMQ 訊息
	job := models.MailJob{
		MailID:       mail.ID.String(),
		FromAddress:  mail.FromAddress,
		ToAddresses:  req.To,
		CCAddresses:  req.CC,
		BCCAddresses: req.BCC,
		Subject:      mail.Subject,
		Body:         mail.Body,
		HTML:         mail.HTML,
		Attachments:  attachments,
		Metadata:     req.Metadata,
		RetryCount:   0,
	}

	// 發送到 RabbitMQ
	if err := h.queueService.PublishMail(&job); err != nil {
		// 更新狀態為失敗
		h.db.Model(&mail).Update("status", models.MailStatusFailed)
		return gin.H{
			"mail_id": mail.ID.String(),
			"status":  "failed",
			"error":   "Failed to queue mail",
		}
	}

	// 更新 KeyDB 狀態
	h.keydbService.SetStatus(c.Request.Context(), mail.ID.String(), "queued", 0, "")

	return gin.H{
		"mail_id": mail.ID.String(),
		"status":  "queued",
	}
}

// GetStatus 查詢郵件狀態
func (h *MailHandler) GetStatus(c *gin.Context) {
	mailID := c.Param("id")

	// 先查 KeyDB
	status, err := h.keydbService.GetStatus(c.Request.Context(), mailID)
	if err == nil && status != nil {
		c.JSON(http.StatusOK, status)
		return
	}

	// 查資料庫
	var mail models.Mail
	if err := h.db.Where("id = ?", mailID).First(&mail).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "not_found",
			"message": "Mail not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"mail_id":       mail.ID.String(),
		"status":        mail.Status,
		"retry_count":   mail.RetryCount,
		"created_at":    mail.CreatedAt,
		"sent_at":       mail.SentAt,
		"error_message": mail.ErrorMessage,
	})
}

// GetHistory 查詢郵件歷史
func (h *MailHandler) GetHistory(c *gin.Context) {
	clientID, _ := c.Get("client_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var total int64
	var mails []models.Mail

	query := h.db.Model(&models.Mail{}).Where("client_id = ?", clientID)

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&mails)

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"page":  page,
		"limit": limit,
		"data":  mails,
	})
}

// Cancel 取消郵件
func (h *MailHandler) Cancel(c *gin.Context) {
	mailID := c.Param("id")
	clientID, _ := c.Get("client_id")

	var mail models.Mail
	if err := h.db.Where("id = ? AND client_id = ?", mailID, clientID).First(&mail).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "not_found",
			"message": "Mail not found",
		})
		return
	}

	// 只能取消 queued 狀態的郵件
	if mail.Status != models.MailStatusQueued {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "cannot_cancel",
			"message": "Only queued mails can be cancelled",
		})
		return
	}

	// 更新狀態
	h.db.Model(&mail).Update("status", models.MailStatusCancelled)
	h.keydbService.SetStatus(c.Request.Context(), mailID, "cancelled", 0, "")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"mail_id": mailID,
		"status":  "cancelled",
		"message": "郵件已取消",
	})
}
