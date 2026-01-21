// internal/services/keydb_service.go
// KeyDB 狀態快取服務

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"smtp-service/internal/config"
	"smtp-service/internal/models"
)

// KeyDBService KeyDB 服務
type KeyDBService struct {
	cfg    *config.Config
	client *redis.Client
}

// NewKeyDBService 建立 KeyDB 服務
func NewKeyDBService(cfg *config.Config) (*KeyDBService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.KeyDBURL,
		Password: cfg.KeyDBPassword,
		DB:       0,
	})

	// 測試連接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to KeyDB: %w", err)
	}

	return &KeyDBService{
		cfg:    cfg,
		client: client,
	}, nil
}

// SetStatus 設定郵件狀態
func (s *KeyDBService) SetStatus(ctx context.Context, mailID, status string, retryCount int, errorMsg string) error {
	key := fmt.Sprintf("mail:status:%s", mailID)

	statusCache := models.MailStatusCache{
		MailID:       mailID,
		Status:       status,
		RetryCount:   retryCount,
		LastUpdated:  time.Now().UTC().Format(time.RFC3339),
		ErrorMessage: errorMsg,
	}

	data, err := json.Marshal(statusCache)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	return s.client.Set(ctx, key, data, s.cfg.KeyDBStatusTTL).Err()
}

// GetStatus 取得郵件狀態
func (s *KeyDBService) GetStatus(ctx context.Context, mailID string) (*models.MailStatusCache, error) {
	key := fmt.Sprintf("mail:status:%s", mailID)

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("status not found")
		}
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	var status models.MailStatusCache
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status: %w", err)
	}

	return &status, nil
}

// Ping 檢查連接
func (s *KeyDBService) Ping(ctx context.Context) bool {
	return s.client.Ping(ctx).Err() == nil
}

// Close 關閉連接
func (s *KeyDBService) Close() error {
	return s.client.Close()
}
