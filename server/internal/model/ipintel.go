package model

import (
	"fmt"
	"time"
)

// IPIntel 统一 IP 信息模型（与数据源无关）
type IPIntel struct {
	// === 基础信息 ===
	IP          string  `json:"ip"`
	Hostname    string  `json:"hostname,omitempty"`
	Country     string  `json:"country"`                // ISO 3166-1 alpha-2
	CountryName string  `json:"country_name,omitempty"` // 国家全称
	City        string  `json:"city"`
	Region      string  `json:"region,omitempty"` // 省/州
	Postal      string  `json:"postal,omitempty"` // 邮编
	Timezone    string  `json:"timezone"`         // IANA 时区
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`

	// === 网络信息 ===
	ASN string `json:"asn"`           // AS 号，如 "AS4134"
	Org string `json:"org"`           // 组织名称
	ISP string `json:"isp,omitempty"` // ISP 名称

	// === 风险标识（布尔） ===
	IsVPN       bool `json:"is_vpn"`
	IsProxy     bool `json:"is_proxy"`
	IsTor       bool `json:"is_tor"`
	IsRelay     bool `json:"is_relay"`     // iCloud Private Relay 等
	IsHosting   bool `json:"is_hosting"`   // 机房/托管 IP
	IsMobile    bool `json:"is_mobile"`    // 移动网络
	IsAnonymous bool `json:"is_anonymous"` // 任何匿名化服务

	// === 检测结果 ===
	UsageType     string   `json:"usage_type"`                // isp/hosting/unknown（统一口径）
	UsageTypeRaw  string   `json:"usage_type_raw,omitempty"`  // 原始细分类型: residential/mobile/business/hosting 等
	Confidence    int      `json:"confidence,omitempty"`      // 检测置信度 0-100
	DetectMethod  string   `json:"detect_method,omitempty"`   // 检测方法
	NeedsDeepScan bool     `json:"needs_deep_scan,omitempty"` // 是否建议深度扫描
	RDNS          string   `json:"rdns,omitempty"`            // rDNS 记录
	FraudScore    int      `json:"fraud_score"`               // 欺诈评分 (0-100)
	HumanRatio    *float64 `json:"human_ratio,omitempty"`     // 真实用户占比 (0-100)，来自 Cloudflare Radar
	Tip           string   `json:"tip,omitempty"`             // 提示文案

	// === 元数据 ===
	Source    string    `json:"source"`     // 数据来源: local/bigdatacloud/ip2location/ipregistry/ipinfo
	Tier      string    `json:"tier"`       // 命中的查询层级，按命中顺序累加: L1(本地mmdb) / L2(HostingDetector实时免费信号: CIDR/ASN黑名单/Cymru/rDNS) / L3(远程付费API)，如 L1+L2、L1+L2+L3
	FetchedAt time.Time `json:"fetched_at"` // 获取时间
}

// IPInfo 旧版 IP 信息结构（兼容现有代码）
type IPInfo struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"`
	Country  string `json:"country,omitempty"`
	Loc      string `json:"loc,omitempty"` // "lat,lng" 格式
	Org      string `json:"org,omitempty"`
	Postal   string `json:"postal,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Source   string `json:"source,omitempty"`
}

// HasProxy 是否使用了代理（合并 vpn/proxy/tor/relay）
func (i *IPIntel) HasProxy() bool {
	return i.IsVPN || i.IsProxy || i.IsTor || i.IsRelay || i.IsAnonymous
}

// HasRisk 是否存在任何风险标识
func (i *IPIntel) HasRisk() bool {
	return i.HasProxy() || i.IsHosting
}

// IsClean 是否为纯净 IP（无任何风险标识，且为正常 ISP 用户）
func (i *IPIntel) IsClean() bool {
	return !i.HasRisk() && (i.UsageType == "isp" || i.IsMobile)
}

// InferUsageType 根据标识推断使用类型
// 统一为 3 种：isp / hosting / unknown
// - isp: 所有非机房 IP（residential, mobile, business, education, government）
// - hosting: 仅 datacenter/hosting（机房 IP）
// - unknown: 无法确定（灰区），建议深度扫描
//
// 优先级：布尔标识 > UsageTypeRaw > UsageType 字符串
func (i *IPIntel) InferUsageType() string {
	// 1. 优先检查布尔标识（本地检测结果，更可靠）
	if i.IsHosting {
		return "hosting"
	}
	if i.IsMobile {
		return "isp"
	}

	// 2. 检查 UsageTypeRaw（远程 API 返回的原始类型）
	if i.UsageTypeRaw != "" {
		switch i.UsageTypeRaw {
		case "isp", "mobile", "residential", "satellite", "business", "education", "government":
			return "isp"
		case "hosting", "datacenter":
			return "hosting"
		}
	}

	// 3. 再检查 UsageType 字符串
	if i.UsageType != "" {
		switch i.UsageType {
		case "isp", "mobile", "residential", "satellite", "business", "education", "government":
			return "isp"
		case "hosting", "datacenter":
			return "hosting"
		}
	}

	// 4. 无法确定
	return "unknown"
}

// ToIPInfo 转换为旧版 IPInfo 结构（兼容现有代码）
func (i *IPIntel) ToIPInfo() *IPInfo {
	loc := ""
	if i.Latitude != 0 || i.Longitude != 0 {
		loc = fmt.Sprintf("%.4f,%.4f", i.Latitude, i.Longitude)
	}
	return &IPInfo{
		IP:       i.IP,
		Hostname: i.Hostname,
		City:     i.City,
		Region:   i.Region,
		Country:  i.Country,
		Loc:      loc,
		Org:      i.Org,
		Postal:   i.Postal,
		Timezone: i.Timezone,
		Source:   i.Source,
	}
}
