// internal/services/mail_router.go
// 郵件路由服務 - 根據寄件者網域選擇對應的郵件服務

package services

import (
	"fmt"
	"log"
	"strings"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// MailRouter 郵件路由服務
// 根據 from_address 網域判斷使用 Graph API 或 SendGrid
type MailRouter struct {
	graphService    MailSender
	sendgridService MailSender
	orgDomain       string
}

// NewMailRouter 建立郵件路由服務
func NewMailRouter(cfg *config.Config, graphService MailSender, sendgridService MailSender) *MailRouter {
	return &MailRouter{
		graphService:    graphService,
		sendgridService: sendgridService,
		orgDomain:       strings.ToLower(cfg.OrgEmailDomain),
	}
}

// Route 根據寄件者網域選擇對應的郵件服務
func (r *MailRouter) Route(job *models.MailJob) MailSender {
	fromAddress := strings.ToLower(job.FromAddress)

	// 若寄件者為組織網域，使用 Graph API
	if strings.HasSuffix(fromAddress, r.orgDomain) {
		return r.graphService
	}

	// 否則使用 SendGrid
	return r.sendgridService
}

// SendMail 發送郵件 (自動路由到對應服務)
func (r *MailRouter) SendMail(job *models.MailJob) error {
	sender := r.Route(job)
	log.Printf("Using %s for sender: %s", sender.Name(), job.FromAddress)
	return sender.SendMail(job)
}

// Name 回傳服務名稱
func (r *MailRouter) Name() string {
	return "MailRouter"
}

// ValidateConfiguration 驗證郵件服務設定
func (r *MailRouter) ValidateConfiguration() error {
	if r.graphService == nil {
		return fmt.Errorf("Graph API service is not configured")
	}
	if r.sendgridService == nil {
		return fmt.Errorf("SendGrid service is not configured")
	}
	if r.orgDomain == "" {
		return fmt.Errorf("organization email domain is not configured")
	}
	return nil
}
