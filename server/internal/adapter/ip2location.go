package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"detect-radar/internal/model"
)

// IP2LocationAdapter IP2Location.io API 适配器
// 免费额度：每月 50,000 次
// API 文档：https://www.ip2location.io/ip2location-documentation
type IP2LocationAdapter struct {
	RemoteBase
}

// IP2Location API 响应结构
type ip2lResponse struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	RegionName  string  `json:"region_name"`
	CityName    string  `json:"city_name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	ZipCode     string  `json:"zip_code"`
	TimeZone    string  `json:"time_zone"`
	ASN         string  `json:"asn"`
	AS          string  `json:"as"`  // 组织名称
	ISP         string  `json:"isp"` // ISP 名称（比 AS 更具体）
	IsProxy     bool    `json:"is_proxy"`

	// 时区信息（嵌套结构）
	TimeZoneInfo struct {
		Olson string `json:"olson"`
	} `json:"time_zone_info"`

	// 国家信息（嵌套结构）
	Country struct {
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
	} `json:"country"`

	// 使用类型
	UsageType string `json:"usage_type"`

	// 网络速度类型（DSL/T1/SAT 等，SAT 表示卫星）
	NetSpeed string `json:"net_speed"`

	// 欺诈评分
	FraudScore int `json:"fraud_score"`

	// 代理检测详情
	Proxy struct {
		IsVPN              bool   `json:"is_vpn"`
		IsTor              bool   `json:"is_tor"`
		IsDataCenter       bool   `json:"is_data_center"`
		IsPublicProxy      bool   `json:"is_public_proxy"`
		IsWebProxy         bool   `json:"is_web_proxy"`
		IsWebCrawler       bool   `json:"is_web_crawler"`
		IsResidentialProxy bool   `json:"is_residential_proxy"`
		IsSpammer          bool   `json:"is_spammer"`
		IsScanner          bool   `json:"is_scanner"`
		IsBotnet           bool   `json:"is_botnet"`
		ProxyType          string `json:"proxy_type"`
		Threat             string `json:"threat"`
	} `json:"proxy"`
}

// NewIP2LocationAdapter 创建 IP2Location 适配器
func NewIP2LocationAdapter(apiKey string) *IP2LocationAdapter {
	return &IP2LocationAdapter{
		RemoteBase: NewRemoteBase(
			"ip2location",
			"https://api.ip2location.io",
			apiKey,
		),
	}
}

// Capabilities 返回适配器能力
func (a *IP2LocationAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true, // 支持完整的代理检测
		HasUsageType: true, // 有 usage_type 字段
		HasRiskScore: true, // 有 fraud_score
	}
}

// Lookup 查询 IP 信息
func (a *IP2LocationAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	// 构建请求 URL
	url := fmt.Sprintf("%s/?key=%s&ip=%s&format=json", a.BaseURL(), a.APIKey(), ip)

	var resp ip2lResponse
	if err := a.DoGet(ctx, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("ip2location lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

// transform 将 IP2Location 响应转换为统一模型
func (a *IP2LocationAdapter) transform(resp *ip2lResponse) *model.IPIntel {
	p := resp.Proxy

	intel := &model.IPIntel{
		IP:          resp.IP,
		Country:     resp.CountryCode,
		CountryName: resp.CountryName,
		City:        resp.CityName,
		Region:      resp.RegionName,
		Postal:      resp.ZipCode,
		Latitude:    resp.Latitude,
		Longitude:   resp.Longitude,
		ASN:         resp.ASN,
		Org:         resp.AS,
		ISP:         resp.ISP,
		Source:      "ip2location",
		Tier:        "L3",
		FetchedAt:   time.Now(),

		// 风险标识
		IsVPN:     p.IsVPN,
		IsProxy:   p.IsPublicProxy || p.IsWebProxy || resp.IsProxy,
		IsTor:     p.IsTor,
		IsHosting: p.IsDataCenter,

		// 风险评分
		FraudScore: resp.FraudScore,
	}

	// 时区（优先使用嵌套结构）
	if resp.TimeZoneInfo.Olson != "" {
		intel.Timezone = resp.TimeZoneInfo.Olson
	} else {
		intel.Timezone = resp.TimeZone
	}

	// 使用类型映射
	intel.UsageTypeRaw = a.mapUsageTypeRaw(resp.UsageType, resp.NetSpeed) // 保存原始细分类型
	intel.UsageType = a.mapUsageType(resp.UsageType)

	// 住宅代理处理
	if p.IsResidentialProxy {
		intel.IsProxy = true
	}

	// 设置匿名标识
	intel.IsAnonymous = intel.IsVPN || intel.IsProxy || intel.IsTor

	// 使用类型归一化
	intel.UsageType = intel.InferUsageType()

	// 设置 API 检测置信度
	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ip2location"
	}

	return intel
}

// mapUsageType 映射 IP2Location 的 usage_type 字段（统一口径）
func (a *IP2LocationAdapter) mapUsageType(code string) string {
	// IP2Location usage_type 代码说明：
	// COM - Commercial（企业）
	// ORG - Organization（机构）
	// GOV - Government（政府）
	// MIL - Military（军方）
	// EDU - University/College/School（教育）
	// LIB - Library（图书馆）
	// CDN - Content Delivery Network
	// ISP - Fixed Line ISP
	// MOB - Mobile ISP
	// DCH - Data Center/Web Hosting/Transit
	// SES - Search Engine Spider
	// RSV - Reserved
	// 注意：可能出现混合类型如 "ISP/MOB"

	// 处理混合类型（如 ISP/MOB），取第一个
	if strings.Contains(code, "/") {
		code = strings.Split(code, "/")[0]
	}

	// 统一映射：isp/hosting/unknown
	// COM/ORG/EDU/GOV/MIL/LIB 都属于 ISP 分支，不是机房
	switch code {
	case "ISP", "MOB", "RES", "COM", "ORG", "EDU", "GOV", "MIL", "LIB":
		return "isp"
	case "DCH", "CDN", "SES":
		return "hosting"
	default:
		return "unknown"
	}
}

// mapUsageTypeRaw 映射 IP2Location 的 usage_type 为细分类型
func (a *IP2LocationAdapter) mapUsageTypeRaw(code string, netSpeed string) string {
	// 卫星网络优先判断（通过 net_speed 字段识别）
	if netSpeed == "SAT" {
		return "satellite"
	}

	// 处理混合类型（如 ISP/MOB），取第一个
	if strings.Contains(code, "/") {
		code = strings.Split(code, "/")[0]
	}

	switch code {
	case "ISP":
		return "isp"
	case "MOB":
		return "mobile"
	case "RES":
		return "residential"
	case "COM":
		return "business"
	case "ORG":
		return "business"
	case "EDU":
		return "education"
	case "GOV":
		return "government"
	case "MIL":
		return "government"
	case "LIB":
		return "education"
	case "DCH", "CDN":
		return "hosting"
	case "SES":
		return "hosting"
	default:
		return ""
	}
}
