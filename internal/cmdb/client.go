package cmdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client 抽象 CMDB 数据源。
type Client interface {
	FetchSnapshot(ctx context.Context) (Snapshot, error)
}

// StaticClient 用于测试或最小实现，直接返回内存中的快照。
type StaticClient struct {
	Snapshot Snapshot
}

// FetchSnapshot 返回预设快照。
func (c *StaticClient) FetchSnapshot(context.Context) (Snapshot, error) {
	return c.Snapshot, nil
}

// TokenSource 用于提供调用 CMDB 接口所需的 Token。
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource 返回固定 Token，适用于测试或简易场景。
type StaticTokenSource struct {
	Value string
}

// Token 返回固定值。
func (s *StaticTokenSource) Token(context.Context) (string, error) {
	return s.Value, nil
}

// PasswordTokenSource 通过用户名/密码调用认证接口换取 Token，并带简单缓存。
type PasswordTokenSource struct {
	endpoint   string
	username   string
	password   string
	httpClient *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// PasswordTokenConfig 配置基于用户名/密码的 TokenSource。
type PasswordTokenConfig struct {
	Endpoint   string
	Username   string
	Password   string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewPasswordTokenSource 创建一个 PasswordTokenSource。
func NewPasswordTokenSource(cfg PasswordTokenConfig) (*PasswordTokenSource, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("token endpoint 不能为空")
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &PasswordTokenSource{
		endpoint:   cfg.Endpoint,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: client,
	}, nil
}

// Token 实现 TokenSource 接口，必要时刷新 Token。
func (s *PasswordTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Until(s.expiry) > 30*time.Second {
		return s.token, nil
	}
	return s.refresh(ctx)
}

func (s *PasswordTokenSource) refresh(ctx context.Context) (string, error) {
	body := map[string]string{
		"username": s.username,
		"password": s.password,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("编码 token 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("构建 token 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取 token 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token 接口返回状态码 %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("token 响应中缺少 access_token")
	}
	expires := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn == 0 {
		expires = time.Now().Add(30 * time.Minute)
	}
	s.token = tokenResp.AccessToken
	s.expiry = expires
	return s.token, nil
}

// HTTPClient 实现 Client，通过 HTTP 与 CMDB 通信。
type HTTPClient struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource TokenSource
	snapshotAPI string
	authHeader  string
}

// HTTPConfig 配置 HTTP 客户端。
type HTTPConfig struct {
	BaseURL        string
	TokenSource    TokenSource
	Timeout        time.Duration
	CustomClient   *http.Client
	SnapshotAPI    string
	AuthHeaderName string
}

// NewHTTPClient 根据配置创建 CMDB HTTP 客户端。
func NewHTTPClient(cfg HTTPConfig) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("cmdb base url 不能为空")
	}
	client := cfg.CustomClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	endpoint := cfg.SnapshotAPI
	if endpoint == "" {
		endpoint = "/api/v1/snapshot"
	}
	authHeader := cfg.AuthHeaderName
	if strings.TrimSpace(authHeader) == "" {
		authHeader = "Authorization"
	}

	return &HTTPClient{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		httpClient:  client,
		tokenSource: cfg.TokenSource,
		snapshotAPI: endpoint,
		authHeader:  authHeader,
	}, nil
}

// FetchSnapshot 调用 CMDB HTTP 接口，获取最新的拓扑快照。
func (c *HTTPClient) FetchSnapshot(ctx context.Context) (Snapshot, error) {
	if c == nil {
		return Snapshot{}, errors.New("cmdb http client 未初始化")
	}
	var snapshot Snapshot
	if err := c.getJSON(ctx, c.snapshotAPI, &snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, out any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.tokenSource != nil {
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return fmt.Errorf("获取 token 失败: %w", err)
		}
		if token != "" {
			req.Header.Set(c.authHeader, "Bearer "+token)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 CMDB 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CMDB 返回状态码 %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析 CMDB 响应失败: %w", err)
	}
	return nil
}
