package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"detect-radar/internal/model"
)

// BigDataCloudAdapter BigDataCloud API 适配器
// 免费额度：每月 10,000 次
// API 文档：https://www.bigdatacloud.com/docs/api/ip-geolocation-full
type BigDataCloudAdapter struct {
	RemoteBase
}

// BigDataCloud API 响应结构
type bdcResponse struct {
	IP      string `json:"ip"`
	Country struct {
		IsoAlpha2 string `json:"isoAlpha2"`
		Name      string `json:"name"`
	} `json:"country"`
	Location struct {
		City                 string  `json:"city"`
		PrincipalSubdivision string  `json:"principalSubdivision"`
		Latitude             float64 `json:"latitude"`
		Longitude            float64 `json:"longitude"`
		TimeZone             struct {
			IanaTimeId string `json:"ianaTimeId"`
		} `json:"timeZone"`
	} `json:"location"`
	Network struct {
		Organisation string `json:"organisation"`
		Carriers     []struct {
			ASN string `json:"asn"`
		} `json:"carriers"`
	} `json:"network"`
	SecurityThreat string `json:"securityThreat"`
	HazardReport   struct {
		IsKnownAsTorServer bool `json:"isKnownAsTorServer"`
		IsKnownAsVpn       bool `json:"isKnownAsVpn"`
		IsKnownAsProxy     bool `json:"isKnownAsProxy"`
		IsSpamhausDrop     bool `json:"isSpamhausDrop"`
		HostingLikelihood  int  `json:"hostingLikelihood"` // 0-10
		IsHostingAsn       bool `json:"isHostingAsn"`
		IsCellular         bool `json:"isCellular"`
		ICloudPrivateRelay bool `json:"iCloudPrivateRelay"`
	} `json:"hazardReport"`
}

// NewBigDataCloudAdapter 创建 BigDataCloud 适配器
func NewBigDataCloudAdapter(apiKey string) *BigDataCloudAdapter {
	return &BigDataCloudAdapter{
		RemoteBase: NewRemoteBase(
			"bigdatacloud",
			"https://api.bigdatacloud.net/data/ip-geolocation-full",
			apiKey,
		),
	}
}

// Capabilities 返回适配器能力
func (a *BigDataCloudAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true, // 支持 VPN/Proxy/Tor 检测
		HasUsageType: true, // 支持 hosting/cellular 检测
		HasRiskScore: true, // 有 hostingLikelihood
	}
}

// Lookup 查询 IP 信息
func (a *BigDataCloudAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	// 构建请求 URL
	url := fmt.Sprintf("%s?ip=%s&key=%s", a.BaseURL(), ip, a.APIKey())

	var resp bdcResponse
	if err := a.DoGet(ctx, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("bigdatacloud lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

// transform 将 BigDataCloud 响应转换为统一模型
func (a *BigDataCloudAdapter) transform(resp *bdcResponse) *model.IPIntel {
	h := resp.HazardReport

	intel := &model.IPIntel{
		IP:          resp.IP,
		Country:     resp.Country.IsoAlpha2,
		CountryName: resp.Country.Name,
		City:        resp.Location.City,
		Region:      resp.Location.PrincipalSubdivision,
		Timezone:    resp.Location.TimeZone.IanaTimeId,
		Latitude:    resp.Location.Latitude,
		Longitude:   resp.Location.Longitude,
		Org:         resp.Network.Organisation,
		Source:      "bigdatacloud",
		Tier:        "L3",
		FetchedAt:   time.Now(),

		// 风险标识
		IsVPN:     h.IsKnownAsVpn,
		IsProxy:   h.IsKnownAsProxy,
		IsTor:     h.IsKnownAsTorServer,
		IsRelay:   h.ICloudPrivateRelay,
		IsHosting: h.IsHostingAsn || h.HostingLikelihood >= 8,
		IsMobile:  h.IsCellular,
	}

	// 提取 ASN
	if len(resp.Network.Carriers) > 0 {
		intel.ASN = resp.Network.Carriers[0].ASN
	}

	// 推断使用类型
	intel.UsageType, intel.UsageTypeRaw = a.inferUsageType(resp)

	// 设置匿名标识
	intel.IsAnonymous = intel.IsVPN || intel.IsProxy || intel.IsTor || intel.IsRelay

	// 使用类型归一化
	intel.UsageType = intel.InferUsageType()

	// 设置置信度：将 hostingLikelihood (0-10) 转换为 confidence (0-100)
	// 但 API 确认的置信度不低于 80
	intel.Confidence = h.HostingLikelihood * 10
	if intel.Confidence < 80 && (intel.IsHosting || intel.UsageTypeRaw != "") {
		intel.Confidence = 80
	}
	intel.DetectMethod = "api_bigdatacloud"

	return intel
}

// inferUsageType 根据 BigDataCloud 响应推断使用类型
// 返回 (统一类型, 原始细分类型)
func (a *BigDataCloudAdapter) inferUsageType(resp *bdcResponse) (string, string) {
	h := resp.HazardReport
	if h.IsHostingAsn || h.HostingLikelihood >= 8 {
		return "hosting", "hosting"
	}
	if h.IsCellular {
		return "isp", "mobile"
	}
	if strings.Contains(strings.ToLower(resp.SecurityThreat), "hosting") {
		return "hosting", "hosting"
	}
	if h.HostingLikelihood <= 2 {
		// 低 likelihood 视为 isp，但无法确定是 residential 还是 business
		return "isp", ""
	}
	return "unknown", "" // 灰区
}
