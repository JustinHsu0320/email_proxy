// internal/services/mail_sender.go
// 郵件發送服務共用介面

package services

import "mail-proxy/internal/models"

// MailSender 郵件發送服務介面
// 所有郵件發送服務（Graph API、SendGrid 等）都需實作此介面
type MailSender interface {
	// SendMail 發送郵件
	SendMail(job *models.MailJob) error

	// Name 回傳服務名稱，用於 logging
	Name() string
}
