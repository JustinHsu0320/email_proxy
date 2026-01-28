// internal/smtp/session.go
// SMTP Session 處理 - 接收郵件並解析 MIME 格式

package smtp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
	"mail-proxy/internal/services"
)

// Session 實作 smtp.Session 介面
// 處理單一 SMTP 連線的郵件接收
type Session struct {
	cfg          *config.Config
	db           *gorm.DB
	queueService *services.QueueService
	keydbService *services.KeyDBService

	from string   // 寄件者地址
	to   []string // 收件者地址列表
}

// NewSession 建立新的 Session
func NewSession(cfg *config.Config, db *gorm.DB, queueService *services.QueueService, keydbService *services.KeyDBService) *Session {
	return &Session{
		cfg:          cfg,
		db:           db,
		queueService: queueService,
		keydbService: keydbService,
		to:           make([]string, 0),
	}
}

// AuthPlain 處理 PLAIN 認證
// 若 SMTPAuthRequired 為 false，則直接接受所有連線
func (s *Session) AuthPlain(username, password string) error {
	if !s.cfg.SMTPAuthRequired {
		// 不需要認證，直接接受
		return nil
	}

	// TODO: 可擴展認證邏輯，例如驗證 API Token
	log.Printf("[SMTP] 認證嘗試: username=%s", username)

	// 目前簡單實作：若有設定 JWT Secret 則比對密碼
	if password == s.cfg.JWTSecret {
		return nil
	}

	return fmt.Errorf("invalid credentials")
}

// Mail 處理 MAIL FROM 指令
// 使用 go-smtp 的 MailOptions 結構
func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error {
	// 移除可能的角括號
	from = cleanEmail(from)
	log.Printf("[SMTP] MAIL FROM: %s", from)

	// 檢查是否在允許的網域清單中
	if len(s.cfg.SMTPAllowedDomains) > 0 {
		allowed := false
		for _, domain := range s.cfg.SMTPAllowedDomains {
			if strings.HasSuffix(strings.ToLower(from), strings.ToLower(domain)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("sender domain not allowed: %s", from)
		}
	}

	s.from = from
	return nil
}

// Rcpt 處理 RCPT TO 指令
// 使用 go-smtp 的 RcptOptions 結構
func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	// 移除可能的角括號
	to = cleanEmail(to)
	log.Printf("[SMTP] RCPT TO: %s", to)

	s.to = append(s.to, to)
	return nil
}

// Data 處理 DATA 指令，接收郵件內容
func (s *Session) Data(r io.Reader) error {
	log.Printf("[SMTP] 開始接收郵件資料 (from=%s, to=%v)", s.from, s.to)

	// 讀取完整的郵件內容
	buf := new(bytes.Buffer)
	size, err := buf.ReadFrom(r)
	if err != nil {
		log.Printf("[SMTP] 讀取郵件資料失敗: %v", err)
		return fmt.Errorf("failed to read mail data: %w", err)
	}

	// 檢查郵件大小
	maxSizeBytes := int64(s.cfg.SMTPMaxMessageSize) * 1024 * 1024
	if size > maxSizeBytes {
		return fmt.Errorf("message too large: %d bytes (max: %d bytes)", size, maxSizeBytes)
	}

	log.Printf("[SMTP] 收到郵件: %d bytes", size)

	// 解析 MIME 郵件並創建資料庫記錄
	mail, err := s.parseMailData(buf)
	if err != nil {
		log.Printf("[SMTP] 解析郵件失敗: %v", err)
		return fmt.Errorf("failed to parse mail: %w", err)
	}

	// 儲存到資料庫
	if err := s.db.Create(&mail).Error; err != nil {
		log.Printf("[SMTP] 儲存郵件記錄失敗: %v", err)
		return fmt.Errorf("failed to create mail record: %w", err)
	}

	log.Printf("[SMTP] 郵件記錄已建立: mail_id=%s", mail.ID.String())

	// 建立附件資訊列表（用於 RabbitMQ）
	var attachmentInfos []models.AttachmentInfo
	for _, att := range mail.Attachments {
		attachmentInfos = append(attachmentInfos, models.AttachmentInfo{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			SizeBytes:   att.SizeBytes,
			StoragePath: att.StoragePath,
		})
	}

	// 建立 RabbitMQ 訊息
	mailJob := &models.MailJob{
		MailID:       mail.ID.String(),
		FromAddress:  mail.FromAddress,
		ToAddresses:  mail.ToAddresses,
		CCAddresses:  mail.CCAddresses,
		BCCAddresses: mail.BCCAddresses,
		Subject:      mail.Subject,
		Body:         mail.Body,
		HTML:         mail.HTML,
		Attachments:  attachmentInfos,
		Metadata:     mail.Metadata,
		RetryCount:   0,
	}

	// 發送到 RabbitMQ 佇列
	if err := s.queueService.PublishMail(mailJob); err != nil {
		log.Printf("[SMTP] 發送到佇列失敗: %v", err)
		// 更新狀態為失敗
		s.db.Model(&mail).Update("status", models.MailStatusFailed)
		return fmt.Errorf("failed to queue mail: %w", err)
	}

	// 更新 KeyDB 狀態
	ctx := context.Background()
	s.keydbService.SetStatus(ctx, mail.ID.String(), "queued", 0, "")

	log.Printf("[SMTP] 郵件已排入佇列: mail_id=%s", mail.ID.String())
	return nil
}

// parseMailData 解析 MIME 郵件資料並創建資料庫模型
func (s *Session) parseMailData(buf *bytes.Buffer) (*models.Mail, error) {
	// 使用 go-message 解析郵件
	mr, err := mail.CreateReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		// 若解析失敗，嘗試將整個內容作為純文字處理
		return s.createSimpleMailJob(buf.String())
	}
	defer mr.Close()

	// 取得郵件標頭
	header := mr.Header

	// 取得主旨
	subject, _ := header.Subject()

	// 取得寄件者（優先使用 MAIL FROM）
	from := s.from
	if from == "" {
		if addrs, err := header.AddressList("From"); err == nil && len(addrs) > 0 {
			from = addrs[0].Address
		}
	}

	// 取得收件者（優先使用 RCPT TO）
	toAddresses := s.to
	if len(toAddresses) == 0 {
		if addrs, err := header.AddressList("To"); err == nil {
			for _, addr := range addrs {
				toAddresses = append(toAddresses, addr.Address)
			}
		}
	}

	// 取得 CC
	var ccAddresses []string
	if addrs, err := header.AddressList("Cc"); err == nil {
		for _, addr := range addrs {
			ccAddresses = append(ccAddresses, addr.Address)
		}
	}

	// 取得 BCC
	var bccAddresses []string
	if addrs, err := header.AddressList("Bcc"); err == nil {
		for _, addr := range addrs {
			bccAddresses = append(bccAddresses, addr.Address)
		}
	}

	// 解析郵件內容（純文字與 HTML）
	var bodyText, bodyHTML string
	var attachments []models.Attachment
	var attachmentDataList [][]byte // 儲存附件二進制資料

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[SMTP] 解析郵件部分失敗: %v", err)
			continue
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			// 內嵌內容（純文字或 HTML）
			contentType, _, _ := h.ContentType()
			content, _ := io.ReadAll(part.Body)

			if strings.HasPrefix(contentType, "text/plain") {
				bodyText = string(content)
			} else if strings.HasPrefix(contentType, "text/html") {
				bodyHTML = string(content)
			}

		case *mail.AttachmentHeader:
			// 附件處理
			filename, _ := h.Filename()
			contentType, params, _ := h.ContentType()

			// 詳細日誌：輸出所有標頭以便除錯
			log.Printf("[SMTP] 發現 AttachmentHeader - filename=%q, contentType=%s", filename, contentType)

			// 列出所有標頭以便除錯
			headerFields := h.Fields()
			for headerFields.Next() {
				log.Printf("[SMTP] Header: %s = %s", headerFields.Key(), headerFields.Value())
			}

			// 修復：嘗試從多個來源獲取檔名
			if filename == "" {
				// 1. 從 Content-Disposition 獲取
				disp := h.Get("Content-Disposition")
				if disp != "" {
					log.Printf("[SMTP] Content-Disposition: %s", disp)
					// 解析 filename 參數（可能用 ; 分隔）
					parts := strings.Split(disp, ";")
					for _, part := range parts {
						part = strings.TrimSpace(part)
						if strings.HasPrefix(part, "filename=") {
							filename = strings.TrimPrefix(part, "filename=")
							filename = strings.Trim(filename, `"`)
							break
						}
					}
				}
			}

			// 2. 如果還是沒有檔名，從 Content-Type 的 name 參數獲取
			if filename == "" && params != nil {
				if name, ok := params["name"]; ok {
					filename = name
					log.Printf("[SMTP] 從 Content-Type name 參數取得檔名: %s", filename)
				}
			}

			// 3. 嘗試從其他可能的標頭獲取
			if filename == "" {
				// 檢查 X-Attachment-Id 或其他自定義標頭
				if xFilename := h.Get("X-Attachment-Name"); xFilename != "" {
					filename = xFilename
					log.Printf("[SMTP] 從 X-Attachment-Name 取得檔名: %s", filename)
				}
			}

			// 4. 如果仍然沒有檔名，生成一個預設檔名
			if filename == "" {
				// 根據 content type 生成副檔名
				ext := ".bin"
				if strings.HasPrefix(contentType, "image/") {
					parts := strings.Split(contentType, "/")
					if len(parts) == 2 {
						ext = "." + parts[1]
					}
				} else if strings.HasPrefix(contentType, "application/pdf") {
					ext = ".pdf"
				} else if strings.HasPrefix(contentType, "text/") {
					ext = ".txt"
				}

				filename = fmt.Sprintf("attachment_%d%s", time.Now().Unix(), ext)
				log.Printf("[SMTP] 無法取得附件檔名，使用預設檔名: %s", filename)
			} else {
				log.Printf("[SMTP] 解析附件檔名: %s", filename)
			}

			// 讀取附件內容
			attachmentData, err := io.ReadAll(part.Body)
			if err != nil {
				log.Printf("[SMTP] 讀取附件失敗 %s: %v", filename, err)
				continue
			}

			sizeBytes := int64(len(attachmentData))

			// 暫時儲存附件資料，稍後在建立 mailID 後才寫入檔案
			attachments = append(attachments, models.Attachment{
				ID:          uuid.New(),
				Filename:    filename,
				ContentType: contentType,
				SizeBytes:   sizeBytes,
				// StoragePath 和 MailID 會在後面設定（需要先有 mailID）
			})

			// 暫存附件內容（稍後儲存）
			attachmentDataList = append(attachmentDataList, attachmentData)
		}
	}

	// 若沒有解析到內容，使用原始資料
	if bodyText == "" && bodyHTML == "" {
		bodyText = buf.String()
	}

	// 建立 Mail 資料庫模型
	mailID := uuid.New()
	mail := &models.Mail{
		ID:           mailID,
		FromAddress:  from,
		ToAddresses:  pq.StringArray(toAddresses),
		CCAddresses:  pq.StringArray(ccAddresses),
		BCCAddresses: pq.StringArray(bccAddresses),
		Subject:      subject,
		Body:         bodyText,
		HTML:         bodyHTML,
		Status:       models.MailStatusQueued,
		ClientID:     "smtp-inbound", // SMTP 來源使用固定 client_id
		ClientName:   "SMTP Receiver",
		Metadata: map[string]string{
			"source":      "smtp-inbound",
			"received_at": time.Now().Format(time.RFC3339),
		},
		RetryCount: 0,
	}

	// 儲存附件並設定 MailID 和 StoragePath
	for i := range attachments {
		attachments[i].MailID = mailID

		// 儲存附件到檔案系統（現在有 mailID 了）
		if i < len(attachmentDataList) {
			storagePath, err := s.saveAttachmentWithMailID(mailID, attachments[i].Filename, attachmentDataList[i])
			if err != nil {
				log.Printf("[SMTP] 儲存附件失敗 %s: %v", attachments[i].Filename, err)
				continue
			}
			attachments[i].StoragePath = storagePath
			log.Printf("[SMTP] 儲存附件: %s -> %s", attachments[i].Filename, storagePath)
		}
	}
	mail.Attachments = attachments

	return mail, nil
}

// createSimpleMailJob 建立簡單的 Mail（無法解析 MIME 時使用）
func (s *Session) createSimpleMailJob(rawContent string) (*models.Mail, error) {
	mailID := uuid.New()
	return &models.Mail{
		ID:          mailID,
		FromAddress: s.from,
		ToAddresses: pq.StringArray(s.to),
		Subject:     "(No Subject)",
		Body:        rawContent,
		Status:      models.MailStatusQueued,
		ClientID:    "smtp-inbound",
		ClientName:  "SMTP Receiver",
		Metadata: map[string]string{
			"source":      "smtp-inbound",
			"raw_content": "true",
			"received_at": time.Now().Format(time.RFC3339),
		},
		RetryCount: 0,
	}, nil
}

// Reset 重置 Session 狀態
func (s *Session) Reset() {
	s.from = ""
	s.to = make([]string, 0)
}

// saveAttachmentWithMailID 儲存附件到檔案系統（使用 API handler 相同的目錄結構）
// 返回完整的絕對儲存路徑
func (s *Session) saveAttachmentWithMailID(mailID uuid.UUID, filename string, data []byte) (string, error) {
	// 使用與 API handler 相同的路徑結構：AttachmentPath/YYYY/MM/DD/mailID/filename
	storagePath := filepath.Join(
		s.cfg.AttachmentPath,
		time.Now().Format("2006/01/02"),
		mailID.String(),
		filepath.Base(filename), // 清理檔名，避免路徑穿越攻擊
	)

	// 建立目錄
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create attachment directory: %w", err)
	}

	// 寫入檔案
	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write attachment file: %w", err)
	}

	// 返回完整的絕對路徑
	return storagePath, nil
}

// Logout 處理 QUIT 指令
func (s *Session) Logout() error {
	log.Printf("[SMTP] Session 結束")
	return nil
}

// cleanEmail 清理郵件地址（移除角括號）
func cleanEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.TrimPrefix(email, "<")
	email = strings.TrimSuffix(email, ">")
	return email
}
