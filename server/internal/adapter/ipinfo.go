package adapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"detect-radar/internal/model"
)

// IPInfoAdapter IPInfo.io API 适配器
// 支持 Core API（$49/月）/ Business API（$117/月）
// API 文档：https://ipinfo.io/developers
type IPInfoAdapter struct {
	RemoteBase
}

// IPInfo API 响应结构（兼容 Core/Business/Widget）
type ipinfoResponse struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"` // "lat,lng" 格式
	Org      string `json:"org"` // "AS25820 IT7 Networks Inc" 格式
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`

	// ASN 对象（Core 及以上）
	ASN *struct {
		ASN    string `json:"asn"`
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Route  string `json:"route"`
		Type   string `json:"type"` // hosting / isp / business / education
	} `json:"asn,omitempty"`

	// Company 对象（Business 及以上）
	Company *struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Type   string `json:"type"`
	} `json:"company,omitempty"`

	// Privacy 对象（Core 及以上）
	Privacy *struct {
		VPN     bool   `json:"vpn"`
		Proxy   bool   `json:"proxy"`
		Tor     bool   `json:"tor"`
		Relay   bool   `json:"relay"`
		Hosting bool   `json:"hosting"`
		Service string `json:"service"` // 具体服务名，如 "NordVPN"
	} `json:"privacy,omitempty"`

	// 顶层布尔标识（Core 及以上）
	IsAnycast   bool `json:"is_anycast"`
	IsMobile    bool `json:"is_mobile"`
	IsAnonymous bool `json:"is_anonymous"`
	IsSatellite bool `json:"is_satellite"`
	IsHosting   bool `json:"is_hosting"`
}

// NewIPInfoAdapter 创建 IPInfo 适配器
// 需要提供 API Key（Core/Business/Enterprise）
func NewIPInfoAdapter(apiKey string) *IPInfoAdapter {
	return &IPInfoAdapter{
		RemoteBase: NewRemoteBase("ipinfo", "https://ipinfo.io", apiKey),
	}
}

// Capabilities 返回适配器能力
func (a *IPInfoAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true, // Core 及以上有 privacy 对象
		HasUsageType: true, // asn.type
		HasRiskScore: false,
	}
}

// Lookup 查询 IP 信息
func (a *IPInfoAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	url := fmt.Sprintf("%s/%s", a.BaseURL(), ip)

	var headers map[string]string
	if a.APIKey() != "" {
		headers = map[string]string{
			"Authorization": "Bearer " + a.APIKey(),
		}
	}

	var resp ipinfoResponse
	if err := a.DoGet(ctx, url, headers, &resp); err != nil {
		return nil, fmt.Errorf("ipinfo lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

// transform 将 IPInfo 响应转换为统一模型
func (a *IPInfoAdapter) transform(resp *ipinfoResponse) *model.IPIntel {
	intel := &model.IPIntel{
		IP:        resp.IP,
		Hostname:  resp.Hostname,
		Country:   resp.Country,
		City:      resp.City,
		Region:    resp.Region,
		Postal:    resp.Postal,
		Timezone:  resp.Timezone,
		Source:    "ipinfo",
		Tier:      "L3",
		FetchedAt: time.Now(),

		// 顶层标识
		IsMobile:    resp.IsMobile,
		IsHosting:   resp.IsHosting,
		IsAnonymous: resp.IsAnonymous,
	}

	// 解析经纬度
	if resp.Loc != "" {
		parts := strings.Split(resp.Loc, ",")
		if len(parts) == 2 {
			intel.Latitude, _ = strconv.ParseFloat(parts[0], 64)
			intel.Longitude, _ = strconv.ParseFloat(parts[1], 64)
		}
	}

	// 解析 ASN（兼容 org 字段和 asn 对象）
	if resp.ASN != nil {
		intel.ASN = resp.ASN.ASN
		intel.Org = resp.ASN.Name
		intel.UsageTypeRaw = resp.ASN.Type // 保存原始类型
		intel.UsageType = a.mapUsageType(resp.ASN.Type)
	} else if resp.Org != "" {
		intel.ASN, intel.Org = a.parseOrgField(resp.Org)
	}

	// Company 信息
	if resp.Company != nil {
		if intel.Org == "" {
			intel.Org = resp.Company.Name
		}
		if intel.UsageTypeRaw == "" {
			intel.UsageTypeRaw = resp.Company.Type // 保存原始类型
		}
		if intel.UsageType == "" {
			intel.UsageType = a.mapUsageType(resp.Company.Type)
		}
	}

	// Privacy 信息
	if resp.Privacy != nil {
		intel.IsVPN = resp.Privacy.VPN
		intel.IsProxy = resp.Privacy.Proxy
		intel.IsTor = resp.Privacy.Tor
		intel.IsRelay = resp.Privacy.Relay
		if resp.Privacy.Hosting {
			intel.IsHosting = true
		}
	}

	// 更新匿名标识
	if intel.IsVPN || intel.IsProxy || intel.IsTor || intel.IsRelay {
		intel.IsAnonymous = true
	}

	// 使用类型归一化
	intel.UsageType = intel.InferUsageType()

	// 设置 API 检测置信度
	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ipinfo"
	}

	return intel
}

// parseOrgField 解析 "AS25820 IT7 Networks Inc" 格式
func (a *IPInfoAdapter) parseOrgField(org string) (asn, name string) {
	parts := strings.SplitN(org, " ", 2)
	if len(parts) == 2 && strings.HasPrefix(parts[0], "AS") {
		return parts[0], parts[1]
	}
	return "", org
}

// mapUsageType 映射 IPInfo 的 type 字段（统一口径）
func (a *IPInfoAdapter) mapUsageType(t string) string {
	// 统一映射：isp/hosting/unknown
	// business/education/government 属于 ISP 分支，不是机房
	switch t {
	case "isp", "business", "education", "government":
		return "isp"
	case "hosting", "datacenter":
		return "hosting"
	default:
		return "unknown"
	}
}
