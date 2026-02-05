// internal/services/email_sender_config_service.go
// Email Sender Config 服務 - 管理 Microsoft OAuth 配置

package services

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// EmailSenderConfigService Email Sender Config 服務
type EmailSenderConfigService struct {
	db         *gorm.DB
	cfg        *config.Config
	encryption *EncryptionService
}

// NewEmailSenderConfigService 建立 Email Sender Config 服務
func NewEmailSenderConfigService(cfg *config.Config, db *gorm.DB, encryption *EncryptionService) *EmailSenderConfigService {
	return &EmailSenderConfigService{
		db:         db,
		cfg:        cfg,
		encryption: encryption,
	}
}

// Create 建立 sender config
func (s *EmailSenderConfigService) Create(clientTokenID uuid.UUID, req *models.CreateSenderConfigRequest) (*models.EmailSenderConfig, error) {
	// 驗證 sender_email 是否為組織網域
	if !strings.HasSuffix(strings.ToLower(req.SenderEmail), strings.ToLower(s.cfg.OrgEmailDomain)) {
		return nil, errors.New("sender_email must be organization domain")
	}

	// 加密 client secret
	encryptedSecret, err := s.encryption.Encrypt(req.MSClientSecret)
	if err != nil {
		return nil, errors.New("failed to encrypt client secret")
	}

	config := &models.EmailSenderConfig{
		ID:                      uuid.New(),
		ClientTokenID:           clientTokenID,
		SenderEmail:             strings.ToLower(req.SenderEmail),
		MSTenantID:              req.MSTenantID,
		MSClientID:              req.MSClientID,
		MSClientSecretEncrypted: encryptedSecret,
		IsActive:                true,
	}

	if err := s.db.Create(config).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, errors.New("sender_email already exists for this client")
		}
		return nil, err
	}

	return config, nil
}

// GetByID 根據 ID 查詢
func (s *EmailSenderConfigService) GetByID(id uuid.UUID) (*models.EmailSenderConfig, error) {
	var config models.EmailSenderConfig
	if err := s.db.First(&config, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

// GetBySenderEmail 根據 client_token_id + sender_email 查詢
func (s *EmailSenderConfigService) GetBySenderEmail(clientTokenID uuid.UUID, senderEmail string) (*models.EmailSenderConfig, error) {
	var config models.EmailSenderConfig
	err := s.db.First(&config, "client_token_id = ? AND sender_email = ? AND is_active = true", clientTokenID, strings.ToLower(senderEmail)).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetBySenderEmailOnly 只根據 sender_email 查詢 (用於 mail handler)
func (s *EmailSenderConfigService) GetBySenderEmailOnly(senderEmail string) (*models.EmailSenderConfig, error) {
	var config models.EmailSenderConfig
	err := s.db.First(&config, "sender_email = ? AND is_active = true", strings.ToLower(senderEmail)).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// ListByClientTokenID 列出 Client 的所有配置
func (s *EmailSenderConfigService) ListByClientTokenID(clientTokenID uuid.UUID) ([]models.EmailSenderConfig, error) {
	var configs []models.EmailSenderConfig
	if err := s.db.Where("client_token_id = ?", clientTokenID).Order("created_at DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

// Update 更新配置
func (s *EmailSenderConfigService) Update(id uuid.UUID, req *models.UpdateSenderConfigRequest) (*models.EmailSenderConfig, error) {
	var config models.EmailSenderConfig
	if err := s.db.First(&config, "id = ?", id).Error; err != nil {
		return nil, err
	}

	// 更新欄位
	if req.MSTenantID != "" {
		config.MSTenantID = req.MSTenantID
	}
	if req.MSClientID != "" {
		config.MSClientID = req.MSClientID
	}
	if req.MSClientSecret != "" {
		encryptedSecret, err := s.encryption.Encrypt(req.MSClientSecret)
		if err != nil {
			return nil, errors.New("failed to encrypt client secret")
		}
		config.MSClientSecretEncrypted = encryptedSecret
	}
	if req.IsActive != nil {
		config.IsActive = *req.IsActive
	}

	if err := s.db.Save(&config).Error; err != nil {
		return nil, err
	}

	return &config, nil
}

// Delete 刪除配置
func (s *EmailSenderConfigService) Delete(id uuid.UUID) error {
	result := s.db.Delete(&models.EmailSenderConfig{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return errors.New("config not found")
	}
	return result.Error
}

// DecryptSecret 解密 client secret
func (s *EmailSenderConfigService) DecryptSecret(config *models.EmailSenderConfig) (string, error) {
	return s.encryption.Decrypt(config.MSClientSecretEncrypted)
}

// GetDecryptedConfig 取得解密後的完整配置 (用於 Worker)
func (s *EmailSenderConfigService) GetDecryptedConfig(id uuid.UUID) (*models.EmailSenderConfig, string, error) {
	config, err := s.GetByID(id)
	if err != nil {
		return nil, "", err
	}

	secret, err := s.DecryptSecret(config)
	if err != nil {
		return nil, "", err
	}

	return config, secret, nil
}
