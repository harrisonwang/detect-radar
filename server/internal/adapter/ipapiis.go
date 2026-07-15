package adapter

import (
	"context"
	"fmt"
	"time"

	"detect-radar/internal/model"
)

// IPAPIISAdapter ipapi.is API 适配器
// 每天免费 1000 次查询
// API 文档：https://ipapi.is/documentation.html
type IPAPIISAdapter struct {
	RemoteBase
}

// ipapi.is API 响应结构
type ipapisResponse struct {
	IP      string `json:"ip"`
	RIR     string `json:"rir"` // 区域互联网注册机构
	IsBogon bool   `json:"is_bogon"`
	IsIPv4  bool   `json:"is_ipv4"`
	IsIPv6  bool   `json:"is_ipv6"`

	// ASN 信息
	ASN *struct {
		ASN         int    `json:"asn"`
		Descr       string `json:"descr"`
		Country     string `json:"country"`
		Active      bool   `json:"active"`
		Org         string `json:"org"`
		Domain      string `json:"domain"`
		Abuse       string `json:"abuse"`
		Type        string `json:"type"` // hosting, isp, business, education
		Created     string `json:"created"`
		Updated     string `json:"updated"`
		RIR         string `json:"rir"`
		WhoisServer string `json:"whois_server"`
	} `json:"asn,omitempty"`

	// 位置信息
	Location *struct {
		Continent     string  `json:"continent"`
		Country       string  `json:"country"`
		CountryCode   string  `json:"country_code"`
		State         string  `json:"state"`
		City          string  `json:"city"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Timezone      string  `json:"timezone"`
		LocalTime     string  `json:"local_time"`
		LocalTimeUnix int64   `json:"local_time_unix"`
		IsInEU        bool    `json:"is_in_european_union"`
	} `json:"location,omitempty"`

	// 隐私/风险检测
	IsDatacenter bool `json:"is_datacenter"`
	IsIsp        bool `json:"is_isp"`
	IsMobile     bool `json:"is_mobile"`
	IsVPN        bool `json:"is_vpn"`
	IsProxy      bool `json:"is_proxy"`
	IsTor        bool `json:"is_tor"`
	IsRelay      bool `json:"is_relay"`
	IsHosting    bool `json:"is_hosting"`

	// 公司信息
	Company *struct {
		Name    string `json:"name"`
		Domain  string `json:"domain"`
		Network string `json:"network"`
		Type    string `json:"type"` // hosting, isp, business, education
	} `json:"company,omitempty"`

	// 数据中心信息
	Datacenter *struct {
		Name    string `json:"name"`
		Network string `json:"network"`
	} `json:"datacenter,omitempty"`

	// VPN 信息
	VPN *struct {
		Name    string `json:"name"`
		Network string `json:"network"`
	} `json:"vpn,omitempty"`

	// 代理信息
	Proxy *struct {
		Type string `json:"type"` // http, socks, etc.
	} `json:"proxy,omitempty"`

	// Tor 信息
	Tor *struct {
		IsRelay bool   `json:"is_relay"`
		IsExit  bool   `json:"is_exit"`
		Name    string `json:"name,omitempty"`
	} `json:"tor,omitempty"`

	// 滥用信息
	Abuse *struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
		Network string `json:"network"`
	} `json:"abuse,omitempty"`
}

// NewIPAPIISAdapter 创建 ipapi.is 适配器
func NewIPAPIISAdapter(apiKey string) *IPAPIISAdapter {
	return &IPAPIISAdapter{
		RemoteBase: NewRemoteBase("ipapi.is", "https://api.ipapi.is", apiKey),
	}
}

// Capabilities 返回适配器能力
func (a *IPAPIISAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true, // VPN/Proxy/Tor/Datacenter 检测
		HasUsageType: true, // hosting/isp/mobile
		HasRiskScore: false,
	}
}

// Lookup 查询 IP 信息
func (a *IPAPIISAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	url := fmt.Sprintf("%s?q=%s", a.BaseURL(), ip)

	var headers map[string]string
	if a.APIKey() != "" {
		headers = map[string]string{
			"X-API-Key": a.APIKey(),
		}
	}

	var resp ipapisResponse
	if err := a.DoGet(ctx, url, headers, &resp); err != nil {
		return nil, fmt.Errorf("ipapi.is lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

// transform 将 ipapi.is 响应转换为统一模型
func (a *IPAPIISAdapter) transform(resp *ipapisResponse) *model.IPIntel {
	intel := &model.IPIntel{
		IP:          resp.IP,
		Source:      "ipapi.is",
		Tier:        "L3",
		FetchedAt:   time.Now(),
		IsVPN:       resp.IsVPN,
		IsProxy:     resp.IsProxy,
		IsTor:       resp.IsTor,
		IsRelay:     resp.IsRelay,
		IsHosting:   resp.IsHosting || resp.IsDatacenter,
		IsMobile:    resp.IsMobile,
		IsAnonymous: resp.IsVPN || resp.IsProxy || resp.IsTor || resp.IsRelay,
	}

	// 位置信息
	if resp.Location != nil {
		intel.Country = resp.Location.CountryCode
		intel.CountryName = resp.Location.Country
		intel.City = resp.Location.City
		intel.Region = resp.Location.State
		intel.Timezone = resp.Location.Timezone
		intel.Latitude = resp.Location.Latitude
		intel.Longitude = resp.Location.Longitude
	}

	// ASN 信息
	if resp.ASN != nil {
		intel.ASN = fmt.Sprintf("AS%d", resp.ASN.ASN)
		intel.Org = resp.ASN.Org
		if intel.Org == "" {
			intel.Org = resp.ASN.Descr
		}
		intel.UsageTypeRaw = resp.ASN.Type // 保存原始类型
		intel.UsageType = a.mapUsageType(resp.ASN.Type)
	}

	// 公司信息（补充）
	if resp.Company != nil && intel.Org == "" {
		intel.Org = resp.Company.Name
		if intel.UsageTypeRaw == "" {
			intel.UsageTypeRaw = resp.Company.Type // 保存原始类型
		}
		if intel.UsageType == "" {
			intel.UsageType = a.mapUsageType(resp.Company.Type)
		}
	}

	// 推断 ISP
	if resp.IsIsp && intel.ISP == "" {
		intel.ISP = intel.Org
	}

	// 使用类型归一化
	intel.UsageType = intel.InferUsageType()

	// 设置 API 检测置信度
	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ipapis"
	}

	return intel
}

// mapUsageType 映射使用类型（统一口径）
func (a *IPAPIISAdapter) mapUsageType(t string) string {
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
