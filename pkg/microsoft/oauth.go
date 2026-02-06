// pkg/microsoft/oauth.go
// Microsoft OAuth 2.0 Token 取得與快取

package microsoft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthService Microsoft OAuth 2.0 服務
type OAuthService struct {
	tenantID     string
	clientID     string
	clientSecret string

	accessToken string
	expiresAt   time.Time
	mu          sync.RWMutex
}

// tokenResponse OAuth 2.0 Token 回應
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// NewOAuthService 建立 OAuth 服務
func NewOAuthService(tenantID, clientID, clientSecret string) *OAuthService {
	return &OAuthService{
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// GetAccessToken 取得 Access Token (帶快取)
func (s *OAuthService) GetAccessToken() (string, error) {
	s.mu.RLock()
	// 檢查快取是否有效 (提前 60 秒更新)
	if s.accessToken != "" && time.Now().Add(60*time.Second).Before(s.expiresAt) {
		token := s.accessToken
		s.mu.RUnlock()
		return token, nil
	}
	s.mu.RUnlock()

	// 需要更新 Token
	return s.refreshToken()
}

// refreshToken 刷新 Access Token
func (s *OAuthService) refreshToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check: 可能其他 goroutine 已經更新
	if s.accessToken != "" && time.Now().Add(60*time.Second).Before(s.expiresAt) {
		return s.accessToken, nil
	}

	// 建立 Token 請求
	tokenURL := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		s.tenantID,
	)

	data := url.Values{}
	data.Set("client_id", s.clientID)
	data.Set("client_secret", s.clientSecret)
	data.Set("scope", "https://graph.microsoft.com/.default")
	data.Set("grant_type", "client_credentials")

	// 發送請求
	resp, err := http.Post(
		tokenURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	// 檢查回應狀態
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status: %d", resp.StatusCode)
	}

	// 解析回應
	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	// 更新快取
	s.accessToken = tokenResp.AccessToken
	s.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return s.accessToken, nil
}

// IsConfigured 檢查 OAuth 是否已設定
func (s *OAuthService) IsConfigured() bool {
	return s.tenantID != "" && s.clientID != "" && s.clientSecret != ""
}

// OAuthManager 多租戶 OAuth 管理器
// 不使用快取，每次都建立新的 OAuthService 以確保使用最新的憑證
type OAuthManager struct{}

// NewOAuthManager 建立 OAuth 管理器
func NewOAuthManager() *OAuthManager {
	return &OAuthManager{}
}

// GetOrCreateService 建立 OAuthService
// 每次都建立新的 OAuthService，不使用快取
func (m *OAuthManager) GetOrCreateService(tenantID, clientID, clientSecret string) *OAuthService {
	// 每次都建立新的 OAuthService，確保使用傳入的最新憑證
	return NewOAuthService(tenantID, clientID, clientSecret)
}

// GetAccessToken 根據配置取得 Access Token
func (m *OAuthManager) GetAccessToken(tenantID, clientID, clientSecret string) (string, error) {
	service := m.GetOrCreateService(tenantID, clientID, clientSecret)
	return service.GetAccessToken()
}

// DefaultOAuthManager 全域預設管理器
var DefaultOAuthManager = NewOAuthManager()

// GetAccessTokenFromManager 從預設管理器取得 Token
func GetAccessTokenFromManager(tenantID, clientID, clientSecret string) (string, error) {
	return DefaultOAuthManager.GetAccessToken(tenantID, clientID, clientSecret)
}
