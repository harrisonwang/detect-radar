package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"detect-radar/internal/model"
)

type IP2LocationAdapter struct {
	RemoteBase
}

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
	AS          string  `json:"as"`
	ISP         string  `json:"isp"`
	IsProxy     bool    `json:"is_proxy"`

	TimeZoneInfo struct {
		Olson string `json:"olson"`
	} `json:"time_zone_info"`

	Country struct {
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
	} `json:"country"`

	UsageType string `json:"usage_type"`

	NetSpeed string `json:"net_speed"`

	FraudScore int `json:"fraud_score"`

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

func NewIP2LocationAdapter(apiKey string) *IP2LocationAdapter {
	return &IP2LocationAdapter{
		RemoteBase: NewRemoteBase(
			"ip2location",
			"https://api.ip2location.io",
			apiKey,
		),
	}
}

func (a *IP2LocationAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	url := fmt.Sprintf("%s/?key=%s&ip=%s&format=json", a.BaseURL(), a.APIKey(), ip)

	var resp ip2lResponse
	if err := a.DoGet(ctx, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("ip2location lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

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

		IsVPN:     p.IsVPN,
		IsProxy:   p.IsPublicProxy || p.IsWebProxy || resp.IsProxy,
		IsTor:     p.IsTor,
		IsHosting: p.IsDataCenter,

		FraudScore: resp.FraudScore,
	}

	if resp.TimeZoneInfo.Olson != "" {
		intel.Timezone = resp.TimeZoneInfo.Olson
	} else {
		intel.Timezone = resp.TimeZone
	}

	intel.UsageTypeRaw = a.mapUsageTypeRaw(resp.UsageType, resp.NetSpeed)
	intel.UsageType = a.mapUsageType(resp.UsageType)

	if p.IsResidentialProxy {
		intel.IsProxy = true
	}

	intel.IsAnonymous = intel.IsVPN || intel.IsProxy || intel.IsTor

	intel.UsageType = intel.InferUsageType()

	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ip2location"
	}

	return intel
}

func (a *IP2LocationAdapter) mapUsageType(code string) string {

	if strings.Contains(code, "/") {
		code = strings.Split(code, "/")[0]
	}

	switch code {
	case "ISP", "MOB", "RES", "COM", "ORG", "EDU", "GOV", "MIL", "LIB":
		return "isp"
	case "DCH", "CDN", "SES":
		return "hosting"
	default:
		return "unknown"
	}
}

func (a *IP2LocationAdapter) mapUsageTypeRaw(code string, netSpeed string) string {
	if netSpeed == "SAT" {
		return "satellite"
	}

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
