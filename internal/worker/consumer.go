// internal/worker/consumer.go
// RabbitMQ Worker Consumer

package worker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
	"mail-proxy/internal/services"
	"mail-proxy/pkg/microsoft"
)

// Consumer RabbitMQ Consumer
type Consumer struct {
	cfg                 *config.Config
	db                  *gorm.DB
	conn                *amqp.Connection
	channel             *amqp.Channel
	oauthService        *microsoft.OAuthService
	mailRouter          *services.MailRouter
	keydbService        *services.KeyDBService
	senderConfigService *services.EmailSenderConfigService
	graphMailService    *services.GraphMailService

	isShutdown bool
	activeJobs int
	mu         sync.Mutex
	wg         sync.WaitGroup
}

// NewConsumer 建立 Consumer
func NewConsumer(
	cfg *config.Config,
	db *gorm.DB,
	oauthService *microsoft.OAuthService,
	mailRouter *services.MailRouter,
	keydbService *services.KeyDBService,
	senderConfigService *services.EmailSenderConfigService,
	graphMailService *services.GraphMailService,
) *Consumer {
	return &Consumer{
		cfg:                 cfg,
		db:                  db,
		oauthService:        oauthService,
		mailRouter:          mailRouter,
		keydbService:        keydbService,
		senderConfigService: senderConfigService,
		graphMailService:    graphMailService,
	}
}

// Start 啟動 Consumer
func (c *Consumer) Start() error {
	var err error

	// 連接 RabbitMQ
	c.conn, err = amqp.Dial(c.cfg.RabbitMQURL)
	if err != nil {
		return err
	}

	c.channel, err = c.conn.Channel()
	if err != nil {
		return err
	}

	// 宣告隊列
	_, err = c.channel.QueueDeclare(
		c.cfg.MailQueueName,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange": "dlx",
		},
	)
	if err != nil {
		return err
	}

	// 設定 prefetch
	err = c.channel.Qos(c.cfg.WorkerPrefetch, 0, false)
	if err != nil {
		return err
	}

	// 開始消費主隊列
	msgs, err := c.channel.Consume(
		c.cfg.MailQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	log.Printf("Worker started, consuming from queue: %s", c.cfg.MailQueueName)

	// 處理訊息
	for i := 0; i < c.cfg.WorkerConcurrency; i++ {
		c.wg.Add(1)
		go c.processMessages(msgs)
	}

	return nil
}

// processMessages 處理訊息
func (c *Consumer) processMessages(msgs <-chan amqp.Delivery) {
	defer c.wg.Done()

	for msg := range msgs {
		if c.isShutdown {
			msg.Nack(false, true) // 重新排隊
			continue
		}

		c.handleMessage(msg)
	}
}

// handleMessage 處理單一訊息
func (c *Consumer) handleMessage(msg amqp.Delivery) {
	c.mu.Lock()
	c.activeJobs++
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.activeJobs--
		c.mu.Unlock()
	}()

	ctx := context.Background()

	// 解析訊息
	var job models.MailJob
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		log.Printf("Failed to parse message: %v", err)
		msg.Nack(false, false)
		return
	}

	log.Printf("Processing mail: %s (retry: %d)", job.MailID, job.RetryCount)

	// 檢查郵件是否已被取消
	var mail models.Mail
	if err := c.db.Where("id = ?", job.MailID).First(&mail).Error; err == nil {
		if mail.Status == models.MailStatusCancelled {
			log.Printf("Mail %s has been cancelled, skipping", job.MailID)
			msg.Ack(false)
			return
		}
	}

	// 更新狀態為 processing
	c.keydbService.SetStatus(ctx, job.MailID, "processing", job.RetryCount, "")
	c.db.Model(&models.Mail{}).Where("id = ?", job.MailID).Update("status", models.MailStatusProcessing)

	// 檢查是否有 SenderConfigID (來自 API 請求)
	var sendErr error
	if job.SenderConfigID != "" && c.senderConfigService != nil {
		// 使用資料庫配置發送
		senderConfigUUID, err := uuid.Parse(job.SenderConfigID)
		if err != nil {
			log.Printf("Invalid sender config ID for mail %s: %v", job.MailID, err)
			c.handleRetry(ctx, msg, &job, err)
			return
		}

		config, secret, err := c.senderConfigService.GetDecryptedConfig(senderConfigUUID)
		if err != nil {
			log.Printf("Failed to get sender config for mail %s: %v", job.MailID, err)
			c.handleRetry(ctx, msg, &job, err)
			return
		}

		log.Printf("Using database OAuth config for sender: %s", job.FromAddress)
		sendErr = c.graphMailService.SendMailWithConfig(&job, config.MSTenantID, config.MSClientID, secret)
	} else {
		// 使用環境變數配置 (組織網域) 或 SendGrid (非組織網域)
		if strings.HasSuffix(strings.ToLower(job.FromAddress), strings.ToLower(c.cfg.OrgEmailDomain)) {
			log.Printf("Using environment OAuth config for sender: %s (SMTP Receiver source)", job.FromAddress)
		}
		sendErr = c.mailRouter.SendMail(&job)
	}

	if sendErr != nil {
		log.Printf("Failed to send mail %s: %v", job.MailID, sendErr)
		c.handleRetry(ctx, msg, &job, sendErr)
		return
	}

	// 發送成功
	log.Printf("Mail sent successfully: %s", job.MailID)
	now := time.Now()
	c.db.Model(&models.Mail{}).Where("id = ?", job.MailID).Updates(map[string]interface{}{
		"status":  models.MailStatusSent,
		"sent_at": now,
	})
	c.keydbService.SetStatus(ctx, job.MailID, "sent", job.RetryCount, "")

	msg.Ack(false)
}

// handleRetry 處理重試
func (c *Consumer) handleRetry(ctx context.Context, msg amqp.Delivery, job *models.MailJob, sendErr error) {
	job.RetryCount++
	errorMsg := sendErr.Error()

	if job.RetryCount >= c.cfg.MaxRetryCount {
		// 達到最大重試次數
		log.Printf("Mail %s failed after %d retries", job.MailID, job.RetryCount)

		// 發送到失敗隊列
		body, _ := json.Marshal(job)
		c.channel.PublishWithContext(
			ctx,
			"",
			c.cfg.FailedQueueName,
			false,
			false,
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         body,
			},
		)

		// 更新資料庫
		c.db.Model(&models.Mail{}).Where("id = ?", job.MailID).Updates(map[string]interface{}{
			"status":        models.MailStatusFailed,
			"retry_count":   job.RetryCount,
			"error_message": errorMsg,
		})
		c.keydbService.SetStatus(ctx, job.MailID, "failed", job.RetryCount, errorMsg)

		msg.Ack(false)
		return
	}

	// 計算延遲時間 (指數退避)
	delay := time.Duration(1<<uint(job.RetryCount)) * time.Second
	log.Printf("Retrying mail %s in %v (attempt %d)", job.MailID, delay, job.RetryCount)

	// 更新狀態
	c.db.Model(&models.Mail{}).Where("id = ?", job.MailID).Update("retry_count", job.RetryCount)
	c.keydbService.SetStatus(ctx, job.MailID, "queued", job.RetryCount, "")

	// 延遲後重新發送
	go func() {
		time.Sleep(delay)
		body, _ := json.Marshal(job)
		c.channel.PublishWithContext(
			context.Background(),
			"",
			c.cfg.MailQueueName,
			false,
			false,
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         body,
				Headers: amqp.Table{
					"x-retry-count": job.RetryCount,
				},
			},
		)
	}()

	msg.Ack(false)
}

// GracefulShutdown 優雅關機
func (c *Consumer) GracefulShutdown() {
	log.Println("Initiating graceful shutdown...")
	c.isShutdown = true

	// 停止接收新訊息
	if c.channel != nil {
		c.channel.Cancel("", false)
	}

	// 等待所有進行中的任務完成
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Println("Shutdown timeout, forcing close")
			goto cleanup
		case <-ticker.C:
			c.mu.Lock()
			active := c.activeJobs
			c.mu.Unlock()
			if active == 0 {
				goto cleanup
			}
		}
	}

cleanup:
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	log.Println("Worker shutdown complete")
}
