package token

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// TokenManager 管理与中心服务器的认证令牌
type TokenManager struct {
	clusterID     string
	clusterSecret string
	token         string
	mu            sync.RWMutex
	client        *http.Client
	serverURL     string
}

// ChallengeResponse 挑战认证响应结构
type ChallengeResponse struct {
	Challenge string `json:"challenge"`
}

// TokenResponse 令牌响应结构
type TokenResponse struct {
	Token string `json:"token"`
	TTL   int64  `json:"ttl"`
}

// NewTokenManager 创建新的令牌管理器
func NewTokenManager(clusterID, clusterSecret, serverURL string) *TokenManager {
	return &TokenManager{
		clusterID:     clusterID,
		clusterSecret: clusterSecret,
		client:        &http.Client{},
		serverURL:     serverURL,
	}
}

// GetToken 获取当前有效的令牌
func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.RLock()
	token := tm.token
	tm.mu.RUnlock()

	if token != "" {
		return token, nil
	}

	// 获取新令牌
	return tm.fetchToken()
}

// fetchToken 从中心服务器获取新令牌
func (tm *TokenManager) fetchToken() (string, error) {
	// 请求挑战
	challengeURL := fmt.Sprintf("%s/openbmclapi-agent/challenge?clusterId=%s", tm.serverURL, tm.clusterID)
	resp, err := tm.client.Get(challengeURL)
	if err != nil {
		return "", fmt.Errorf("无法获取挑战: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取挑战失败，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("无法读取挑战响应: %w", err)
	}

	var challengeResp ChallengeResponse
	if err := json.Unmarshal(body, &challengeResp); err != nil {
		return "", fmt.Errorf("无法解析挑战响应: %w", err)
	}

	// 签名挑战
	signature := tm.signChallenge(challengeResp.Challenge)

	// 请求令牌
	tokenURL := fmt.Sprintf("%s/openbmclapi-agent/token", tm.serverURL)
	tokenReq := map[string]interface{}{
		"clusterId": tm.clusterID,
		"challenge": challengeResp.Challenge,
		"signature": signature,
	}

	tokenReqBytes, err := json.Marshal(tokenReq)
	if err != nil {
		return "", fmt.Errorf("无法序列化令牌请求: %w", err)
	}

	tokenResp, err := tm.client.Post(tokenURL, "application/json", bytes.NewBuffer(tokenReqBytes))
	if err != nil {
		return "", fmt.Errorf("无法获取令牌: %w", err)
	}
	defer tokenResp.Body.Close()

	// 修改状态码检查：201才是正确的状态码
	if tokenResp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("获取令牌失败，状态码: %d", tokenResp.StatusCode)
	}

	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", fmt.Errorf("无法读取令牌响应: %w", err)
	}

	var tokenRespData TokenResponse
	if err := json.Unmarshal(tokenBody, &tokenRespData); err != nil {
		return "", fmt.Errorf("无法解析令牌响应: %w", err)
	}
	// 保存令牌
	tm.mu.Lock()
	tm.token = tokenRespData.Token
	tm.mu.Unlock()

	// 安排令牌刷新
	go tm.scheduleRefresh(tokenRespData.TTL)

	return tokenRespData.Token, nil
}

// signChallenge 使用HMAC-SHA256签名挑战
func (tm *TokenManager) signChallenge(challenge string) string {
	key := []byte(tm.clusterSecret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(challenge))
	return hex.EncodeToString(h.Sum(nil))
}

// scheduleRefresh 安排令牌刷新
func (tm *TokenManager) scheduleRefresh(ttl int64) {
	// 在令牌过期前10分钟刷新，或者在TTL的一半时间刷新（取较大值）
	refreshTime := ttl / 2
	if refreshTime < 600 { // 最少10分钟
		refreshTime = 600
	}

	time.Sleep(time.Duration(refreshTime) * time.Second)
	tm.refreshToken()
}

// refreshToken 刷新令牌
func (tm *TokenManager) refreshToken() {
	tm.mu.RLock()
	currentToken := tm.token
	tm.mu.RUnlock()

	// 使用当前令牌获取新令牌
	tokenURL := fmt.Sprintf("%s/openbmclapi-agent/token", tm.serverURL)
	tokenReq := map[string]interface{}{
		"clusterId": tm.clusterID,
		"token":     currentToken,
	}

	tokenReqBytes, err := json.Marshal(tokenReq)
	if err != nil {
		fmt.Printf("无法序列化令牌刷新请求: %v\n", err)
		return
	}

	tokenResp, err := tm.client.Post(tokenURL, "application/json", bytes.NewBuffer(tokenReqBytes))
	if err != nil {
		fmt.Printf("无法刷新令牌: %v\n", err)
		return
	}
	defer tokenResp.Body.Close()

	// 修改状态码检查：201才是正确的状态码
	if tokenResp.StatusCode != http.StatusCreated {
		fmt.Printf("刷新令牌失败，状态码: %d\n", tokenResp.StatusCode)
		return
	}

	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		fmt.Printf("无法读取令牌刷新响应: %v\n", err)
		return
	}

	var tokenRespData TokenResponse
	if err := json.Unmarshal(tokenBody, &tokenRespData); err != nil {
		fmt.Printf("无法解析令牌刷新响应: %v\n", err)
		return
	}

	// 更新令牌
	tm.mu.Lock()
	tm.token = tokenRespData.Token
	tm.mu.Unlock()

	// 安排下次刷新
	go tm.scheduleRefresh(tokenRespData.TTL)
}
