package adapter

import (
	"context"
	"fmt"
	"time"

	"detect-radar/internal/model"
)

// IPRegistryAdapter IPRegistry API 适配器
// 免费额度：累计 100,000 次（用完即止）
// API 文档：https://ipregistry.co/docs/
type IPRegistryAdapter struct {
	RemoteBase
}

// IPRegistry API 响应结构
type iprResponse struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Type     string `json:"type"` // ipv4 / ipv6

	Company struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Type   string `json:"type"` // isp / hosting / business / education
	} `json:"company"`

	Connection struct {
		ASN    int    `json:"asn"`
		Domain string `json:"domain"`
		Org    string `json:"organization"`
		Route  string `json:"route"`
		Type   string `json:"type"` // isp / hosting / business / education
	} `json:"connection"`

	Location struct {
		City    string `json:"city"`
		Country struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"country"`
		Region struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"region"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Postal    string  `json:"postal"`
		Language  struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"language"`
	} `json:"location"`

	Security struct {
		IsAbuser        bool `json:"is_abuser"`
		IsAnonymous     bool `json:"is_anonymous"`
		IsAttacker      bool `json:"is_attacker"`
		IsBogon         bool `json:"is_bogon"`
		IsCloudProvider bool `json:"is_cloud_provider"`
		IsProxy         bool `json:"is_proxy"`
		IsRelay         bool `json:"is_relay"`
		IsThreat        bool `json:"is_threat"`
		IsTor           bool `json:"is_tor"`
		IsTorExitNode   bool `json:"is_tor_exit_node"`
		IsVPN           bool `json:"is_vpn"`
	} `json:"security"`

	TimeZone struct {
		ID string `json:"id"`
	} `json:"time_zone"`

	Carrier struct {
		Name string `json:"name"`
		MCC  string `json:"mcc"`
		MNC  string `json:"mnc"`
	} `json:"carrier"`
}

// NewIPRegistryAdapter 创建 IPRegistry 适配器
func NewIPRegistryAdapter(apiKey string) *IPRegistryAdapter {
	return &IPRegistryAdapter{
		RemoteBase: NewRemoteBase(
			"ipregistry",
			"https://api.ipregistry.co",
			apiKey,
		),
	}
}

// Capabilities 返回适配器能力
func (a *IPRegistryAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true, // 完整的安全检测
		HasUsageType: true, // connection.type
		HasRiskScore: false,
	}
}

// Lookup 查询 IP 信息
func (a *IPRegistryAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	// 构建请求 URL
	url := fmt.Sprintf("%s/%s?key=%s", a.BaseURL(), ip, a.APIKey())

	var resp iprResponse
	if err := a.DoGet(ctx, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("ipregistry lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

// transform 将 IPRegistry 响应转换为统一模型
func (a *IPRegistryAdapter) transform(resp *iprResponse) *model.IPIntel {
	s := resp.Security

	intel := &model.IPIntel{
		IP:          resp.IP,
		Hostname:    resp.Hostname,
		Country:     resp.Location.Country.Code,
		CountryName: resp.Location.Country.Name,
		City:        resp.Location.City,
		Region:      resp.Location.Region.Name,
		Postal:      resp.Location.Postal,
		Timezone:    resp.TimeZone.ID,
		Latitude:    resp.Location.Latitude,
		Longitude:   resp.Location.Longitude,
		ASN:         fmt.Sprintf("AS%d", resp.Connection.ASN),
		Org:         resp.Company.Name,
		ISP:         resp.Connection.Org,
		Source:      "ipregistry",
		Tier:        "L3",
		FetchedAt:   time.Now(),

		// 风险标识
		IsVPN:       s.IsVPN,
		IsProxy:     s.IsProxy,
		IsTor:       s.IsTor || s.IsTorExitNode,
		IsRelay:     s.IsRelay,
		IsHosting:   s.IsCloudProvider,
		IsAnonymous: s.IsAnonymous,
	}

	// 使用类型（优先使用 connection.type，其次 company.type）
	connType := resp.Connection.Type
	if connType == "" {
		connType = resp.Company.Type
	}
	intel.UsageTypeRaw = connType // 保存原始类型
	intel.UsageType = a.mapUsageType(connType)

	// 移动网络检测
	if resp.Carrier.Name != "" {
		intel.IsMobile = true
		if intel.UsageTypeRaw == "" {
			intel.UsageTypeRaw = "mobile"
		}
		if intel.UsageType == "" || intel.UsageType == "unknown" {
			intel.UsageType = "isp"
		}
	}

	// 使用类型归一化
	intel.UsageType = intel.InferUsageType()

	// 设置 API 检测置信度
	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ipregistry"
	}

	return intel
}

// mapUsageType 映射 IPRegistry 的 type 字段（统一口径）
func (a *IPRegistryAdapter) mapUsageType(t string) string {
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
