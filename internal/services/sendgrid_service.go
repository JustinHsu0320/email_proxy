// internal/services/sendgrid_service.go
// SendGrid 郵件發送服務

package services

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// SendGridService SendGrid 郵件發送服務
// 實作 MailSender interface
type SendGridService struct {
	cfg    *config.Config
	client *sendgrid.Client
}

// NewSendGridService 建立 SendGrid 服務
func NewSendGridService(cfg *config.Config) *SendGridService {
	client := sendgrid.NewSendClient(cfg.SendGridAPIKey)
	return &SendGridService{
		cfg:    cfg,
		client: client,
	}
}

// Name 回傳服務名稱
func (s *SendGridService) Name() string {
	return "SendGrid"
}

// IsConfigured 檢查 SendGrid 是否已設定
func (s *SendGridService) IsConfigured() bool {
	return s.cfg.SendGridAPIKey != ""
}

// SendMail 發送郵件 (使用 SendGrid API)
func (s *SendGridService) SendMail(job *models.MailJob) error {
	// 建立寄件人
	from := mail.NewEmail("", job.FromAddress)

	// 建立郵件
	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = job.Subject

	// 建立個人化設定 (收件人)
	personalization := mail.NewPersonalization()

	// To 收件人
	for _, addr := range job.ToAddresses {
		personalization.AddTos(mail.NewEmail("", addr))
	}

	// CC 收件人
	for _, addr := range job.CCAddresses {
		personalization.AddCCs(mail.NewEmail("", addr))
	}

	// BCC 收件人
	for _, addr := range job.BCCAddresses {
		personalization.AddBCCs(mail.NewEmail("", addr))
	}

	message.AddPersonalizations(personalization)

	// 設定郵件內容 (SendGrid 要求順序: text/plain 必須在 text/html 之前)
	if job.Body != "" {
		message.AddContent(mail.NewContent("text/plain", job.Body))
	}
	if job.HTML != "" {
		message.AddContent(mail.NewContent("text/html", job.HTML))
	}

	// 載入附件
	if err := s.loadAttachments(job, message); err != nil {
		return fmt.Errorf("failed to load attachments: %w", err)
	}

	// 發送郵件
	response, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email via SendGrid: %w", err)
	}

	// 檢查回應狀態 (2xx 表示成功)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("SendGrid API error (status %d): %s", response.StatusCode, response.Body)
	}

	return nil
}

// loadAttachments 載入附件
func (s *SendGridService) loadAttachments(job *models.MailJob, message *mail.SGMailV3) error {
	if len(job.Attachments) == 0 {
		return nil
	}

	for _, att := range job.Attachments {
		// 讀取附件檔案
		content, err := os.ReadFile(att.StoragePath)
		if err != nil {
			return fmt.Errorf("failed to read attachment %s: %w", att.Filename, err)
		}

		// 取得 content type
		contentType := att.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// 建立附件
		attachment := mail.NewAttachment()
		attachment.SetContent(base64.StdEncoding.EncodeToString(content))
		attachment.SetType(contentType)
		attachment.SetFilename(filepath.Base(att.Filename))
		attachment.SetDisposition("attachment")

		message.AddAttachment(attachment)
	}

	return nil
}
