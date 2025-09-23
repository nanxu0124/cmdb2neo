package cmdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

type AppObject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type DataContent struct {
	Id               int         `json:"id"`
	Idc              string      `json:"idc"`
	NetworkPartition string      `json:"network_partition"`
	ServerType       int         `json:"server_type"`
	Ip               string      `json:"ip"`
	HostName         string      `json:"host_name"`
	HostIp           string      `json:"host_ip"`
	AppObj           []AppObject `json:"app_obj"`
}

type ResponseData struct {
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
	Total int           `json:"total"`
	Data  []DataContent `json:"data"`
}

type Request struct {
	Code int          `json:"code"`
	Data ResponseData `json:"data"`
	Msg  string       `json:"msg"`
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
	if snapshotPtr, ok := out.(*Snapshot); ok {
		snap, err := c.fetchSnapshot(ctx, path)
		if err != nil {
			return err
		}
		*snapshotPtr = snap
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
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

func (c *HTTPClient) fetchSnapshot(ctx context.Context, path string) (Snapshot, error) {
	idcs := []string{"M5", "IDC1", "IDC2"}
	snapshot := Snapshot{RunID: time.Now().UTC().Format("20060102T150405Z")}

	hostSeen := make(map[int]bool)
	vmSeen := make(map[int]bool)
	physicalSeen := make(map[int]bool)
	appSeen := make(map[int]bool)
	npIDs := make(map[string]int)
	npCounter := 1

	for idx, idcName := range idcs {
		snapshot.IDCs = append(snapshot.IDCs, IDC{Id: idx + 1, Name: idcName, Location: idcName})

		contents, err := c.fetchAllPagesForIDC(ctx, path, idcName)
		if err != nil {
			return Snapshot{}, err
		}

		for _, item := range contents {
			npKey := idcName + ":" + item.NetworkPartition
			if item.NetworkPartition != "" {
				if _, exists := npIDs[npKey]; !exists {
					snapshot.NetworkPartitions = append(snapshot.NetworkPartitions, NetworkPartition{
						Id:   npCounter,
						Idc:  idcName,
						Name: item.NetworkPartition,
						CIDR: "",
					})
					npIDs[npKey] = npCounter
					npCounter++
				}
			}

			switch item.ServerType {
			case 1:
				if !hostSeen[item.Id] {
					snapshot.HostMachines = append(snapshot.HostMachines, HostMachine{
						Id:             item.Id,
						Idc:            idcName,
						NetworkPartion: item.NetworkPartition,
						ServerType:     strconv.Itoa(item.ServerType),
						Ip:             item.Ip,
						Hostname:       item.HostName,
					})
					hostSeen[item.Id] = true
				}
			case 2:
				if !vmSeen[item.Id] {
					snapshot.VirtualMachines = append(snapshot.VirtualMachines, VirtualMachine{
						Id:             item.Id,
						Idc:            idcName,
						NetworkPartion: item.NetworkPartition,
						ServerType:     strconv.Itoa(item.ServerType),
						Ip:             item.Ip,
						Hostname:       item.HostName,
						HostIp:         item.HostIp,
					})
					vmSeen[item.Id] = true
				}
			case 3:
				if !physicalSeen[item.Id] {
					snapshot.PhysicalMachines = append(snapshot.PhysicalMachines, PhysicalMachine{
						Id:             item.Id,
						Idc:            idcName,
						NetworkPartion: item.NetworkPartition,
						ServerType:     strconv.Itoa(item.ServerType),
						Ip:             item.Ip,
						Hostname:       item.HostName,
					})
					physicalSeen[item.Id] = true
				}
			}

			if len(item.AppObj) > 0 {
				for idxApp, appInfo := range item.AppObj {
					appID := appInfo.ID
					if appID == 0 {
						appID = item.Id*100 + idxApp + 1
					}
					if appSeen[appID] {
						continue
					}
					name := appInfo.Name
					if strings.TrimSpace(name) == "" {
						name = fmt.Sprintf("app-%d", appID)
					}
					snapshot.Apps = append(snapshot.Apps, App{
						Id:         appID,
						Ip:         item.Ip,
						Name:       name,
						ServerType: strconv.Itoa(item.ServerType),
					})
					appSeen[appID] = true
				}
			}
		}
	}

	return snapshot, nil
}

func (c *HTTPClient) fetchAllPagesForIDC(ctx context.Context, path, idc string) ([]DataContent, error) {
	endpoint := c.baseURL + path
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("解析请求地址失败: %w", err)
	}
	query := parsed.Query()
	if query.Get("limit") == "" {
		query.Set("limit", "20")
	}
	if strings.TrimSpace(idc) != "" {
		query.Set("idc", idc)
	}

	var (
		allData    []DataContent
		page       = 1
		pageLimit  = 0
		totalItems = 0
	)

	for {
		query.Set("page", strconv.Itoa(page))
		parsed.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("构建请求失败: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if c.tokenSource != nil {
			token, err := c.tokenSource.Token(ctx)
			if err != nil {
				return nil, fmt.Errorf("获取 token 失败: %w", err)
			}
			if token != "" {
				req.Header.Set(c.authHeader, "Bearer "+token)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 CMDB 失败: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取 CMDB 响应失败: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("CMDB 返回状态码 %d", resp.StatusCode)
		}

		var payload Request
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("解析 CMDB 响应失败: %w", err)
		}

		if len(payload.Data.Data) == 0 {
			break
		}
		allData = append(allData, payload.Data.Data...)

		pageLimit = payload.Data.Limit
		totalItems = payload.Data.Total
		if pageLimit > 0 && totalItems > 0 && page*pageLimit >= totalItems {
			break
		}

		page++
	}

	return allData, nil
}
