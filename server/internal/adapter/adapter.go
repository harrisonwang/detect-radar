package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"detect-radar/internal/model"
)

// Adapter IP 数据源适配器接口
type Adapter interface {
	// Name 适配器标识名称
	Name() string

	// Tier 层级: "L1" (本地) 或 "L3" (远程 API)
	Tier() string

	// Lookup 查询 IP 信息
	Lookup(ctx context.Context, ip string) (*model.IPIntel, error)

	// Capabilities 支持的能力
	Capabilities() Capabilities
}

// Capabilities 适配器能力声明
type Capabilities struct {
	HasGeo       bool // 地理位置 (国家/城市)
	HasASN       bool // ASN 信息
	HasTimezone  bool // 时区
	HasPrivacy   bool // 隐私检测 (VPN/Proxy/Tor)
	HasUsageType bool // 使用类型 (residential/datacenter)
	HasRiskScore bool // 风险评分
}

// Registry 适配器注册表
type Registry struct {
	local   Adapter   // L1 本地库
	remotes []Adapter // L3 远程 API（按优先级排序）
}

// NewRegistry 创建新的注册表
func NewRegistry() *Registry {
	return &Registry{
		remotes: make([]Adapter, 0),
	}
}

// SetLocal 设置本地适配器
func (r *Registry) SetLocal(a Adapter) {
	r.local = a
}

// AddRemote 添加远程适配器（按添加顺序作为优先级）
func (r *Registry) AddRemote(a Adapter) {
	r.remotes = append(r.remotes, a)
}

// Local 获取本地适配器
func (r *Registry) Local() Adapter {
	return r.local
}

// Remotes 获取所有远程适配器
func (r *Registry) Remotes() []Adapter {
	return r.remotes
}

// HasLocal 是否有本地适配器
func (r *Registry) HasLocal() bool {
	return r.local != nil
}

// RemoteCount 远程适配器数量
func (r *Registry) RemoteCount() int {
	return len(r.remotes)
}

// ===== 远程适配器基类 =====

// RemoteBase 远程适配器基类，提供通用的 HTTP 客户端功能
type RemoteBase struct {
	name       string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewRemoteBase 创建远程适配器基类
func NewRemoteBase(name, baseURL, apiKey string) RemoteBase {
	return RemoteBase{
		name:    name,
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name 返回适配器名称
func (b *RemoteBase) Name() string { return b.name }

// Tier 返回层级
func (b *RemoteBase) Tier() string { return "L3" }

// DoGet 执行 GET 请求
func (b *RemoteBase) DoGet(ctx context.Context, url string, headers map[string]string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	// 设置默认 headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "DetectRadar/1.0")

	// 设置自定义 headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}

	return nil
}

// APIKey 获取 API Key
func (b *RemoteBase) APIKey() string {
	return b.apiKey
}

// BaseURL 获取基础 URL
func (b *RemoteBase) BaseURL() string {
	return b.baseURL
}
