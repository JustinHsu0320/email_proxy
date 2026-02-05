// internal/services/smtp_service.go
// Microsoft Graph API 郵件發送服務

package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
	"mail-proxy/pkg/microsoft"
)

// GraphMailService Microsoft Graph API 郵件發送服務
// 實作 MailSender interface
type GraphMailService struct {
	cfg          *config.Config
	oauthService *microsoft.OAuthService // 用於 SMTP Receiver (環境變數配置)
	oauthManager *microsoft.OAuthManager // 用於 API 請求 (資料庫配置)
	httpClient   *http.Client
}

// NewGraphMailService 建立 Graph API 郵件服務
func NewGraphMailService(cfg *config.Config, oauthService *microsoft.OAuthService) *GraphMailService {
	return &GraphMailService{
		cfg:          cfg,
		oauthService: oauthService,
		oauthManager: microsoft.DefaultOAuthManager,
		httpClient:   &http.Client{},
	}
}

// Name 回傳服務名稱
func (s *GraphMailService) Name() string {
	return "Microsoft Graph API"
}

// GraphMailRequest Graph API 郵件請求結構
type GraphMailRequest struct {
	Message         GraphMessage `json:"message"`
	SaveToSentItems bool         `json:"saveToSentItems"`
}

// GraphMessage Graph API 郵件訊息結構
type GraphMessage struct {
	Subject       string            `json:"subject"`
	Body          GraphBody         `json:"body"`
	ToRecipients  []GraphRecipient  `json:"toRecipients"`
	CcRecipients  []GraphRecipient  `json:"ccRecipients,omitempty"`
	BccRecipients []GraphRecipient  `json:"bccRecipients,omitempty"`
	Attachments   []GraphAttachment `json:"attachments,omitempty"`
}

// GraphBody Graph API 郵件內容結構
type GraphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// GraphRecipient Graph API 收件人結構
type GraphRecipient struct {
	EmailAddress GraphEmailAddress `json:"emailAddress"`
}

// GraphEmailAddress Graph API 電子郵件地址結構
type GraphEmailAddress struct {
	Address string `json:"address"`
}

// GraphAttachment Graph API 附件結構
type GraphAttachment struct {
	ODataType    string `json:"@odata.type"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	ContentBytes string `json:"contentBytes"`
}

// GraphErrorResponse Graph API 錯誤回應
type GraphErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SendMail 發送郵件 (使用 Microsoft Graph API)
func (s *GraphMailService) SendMail(job *models.MailJob) error {
	// 取得 OAuth 2.0 Access Token
	accessToken, err := s.oauthService.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// 建立 Graph API 請求
	mailRequest := s.buildGraphRequest(job)

	// 讀取附件
	if err := s.loadAttachments(job, &mailRequest.Message); err != nil {
		return fmt.Errorf("failed to load attachments: %w", err)
	}

	// 序列化請求
	jsonBody, err := json.Marshal(mailRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Graph API 端點
	graphURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/sendMail",
		job.FromAddress,
	)

	// 建立 HTTP 請求
	req, err := http.NewRequest("POST", graphURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	// 發送請求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 檢查回應 (202 Accepted 表示成功)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		var errResp GraphErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return fmt.Errorf("Graph API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}

		return fmt.Errorf("Graph API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildGraphRequest 建立 Graph API 請求結構
func (s *GraphMailService) buildGraphRequest(job *models.MailJob) *GraphMailRequest {
	// 決定內容類型
	contentType := "text"
	content := job.Body
	if job.HTML != "" {
		contentType = "html"
		content = job.HTML
	}

	// 建立收件人列表
	toRecipients := make([]GraphRecipient, len(job.ToAddresses))
	for i, addr := range job.ToAddresses {
		toRecipients[i] = GraphRecipient{
			EmailAddress: GraphEmailAddress{Address: addr},
		}
	}

	// CC 收件人
	ccRecipients := make([]GraphRecipient, len(job.CCAddresses))
	for i, addr := range job.CCAddresses {
		ccRecipients[i] = GraphRecipient{
			EmailAddress: GraphEmailAddress{Address: addr},
		}
	}

	// BCC 收件人
	bccRecipients := make([]GraphRecipient, len(job.BCCAddresses))
	for i, addr := range job.BCCAddresses {
		bccRecipients[i] = GraphRecipient{
			EmailAddress: GraphEmailAddress{Address: addr},
		}
	}

	return &GraphMailRequest{
		Message: GraphMessage{
			Subject: job.Subject,
			Body: GraphBody{
				ContentType: contentType,
				Content:     content,
			},
			ToRecipients:  toRecipients,
			CcRecipients:  ccRecipients,
			BccRecipients: bccRecipients,
		},
		SaveToSentItems: true,
	}
}

// loadAttachments 載入附件
func (s *GraphMailService) loadAttachments(job *models.MailJob, message *GraphMessage) error {
	if len(job.Attachments) == 0 {
		return nil
	}

	attachments := make([]GraphAttachment, 0, len(job.Attachments))

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

		attachments = append(attachments, GraphAttachment{
			ODataType:    "#microsoft.graph.fileAttachment",
			Name:         filepath.Base(att.Filename),
			ContentType:  contentType,
			ContentBytes: base64.StdEncoding.EncodeToString(content),
		})
	}

	message.Attachments = attachments
	return nil
}

// SendMailWithConfig 使用指定的 OAuth 配置發送郵件 (用於 API 請求)
func (s *GraphMailService) SendMailWithConfig(job *models.MailJob, tenantID, clientID, clientSecret string) error {
	// 從 OAuthManager 取得 Access Token
	accessToken, err := s.oauthManager.GetAccessToken(tenantID, clientID, clientSecret)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// 建立 Graph API 請求
	mailRequest := s.buildGraphRequest(job)

	// 讀取附件
	if err := s.loadAttachments(job, &mailRequest.Message); err != nil {
		return fmt.Errorf("failed to load attachments: %w", err)
	}

	// 序列化請求
	jsonBody, err := json.Marshal(mailRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Graph API 端點
	graphURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/sendMail",
		job.FromAddress,
	)

	// 建立 HTTP 請求
	req, err := http.NewRequest("POST", graphURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	// 發送請求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 檢查回應 (202 Accepted 表示成功)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		var errResp GraphErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return fmt.Errorf("Graph API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}

		return fmt.Errorf("Graph API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
