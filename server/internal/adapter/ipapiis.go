package adapter

import (
	"context"
	"fmt"
	"time"

	"detect-radar/internal/model"
)

type IPAPIISAdapter struct {
	RemoteBase
}

type ipapisResponse struct {
	IP      string `json:"ip"`
	RIR     string `json:"rir"`
	IsBogon bool   `json:"is_bogon"`
	IsIPv4  bool   `json:"is_ipv4"`
	IsIPv6  bool   `json:"is_ipv6"`

	ASN *struct {
		ASN         int    `json:"asn"`
		Descr       string `json:"descr"`
		Country     string `json:"country"`
		Active      bool   `json:"active"`
		Org         string `json:"org"`
		Domain      string `json:"domain"`
		Abuse       string `json:"abuse"`
		Type        string `json:"type"`
		Created     string `json:"created"`
		Updated     string `json:"updated"`
		RIR         string `json:"rir"`
		WhoisServer string `json:"whois_server"`
	} `json:"asn,omitempty"`

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

	IsDatacenter bool `json:"is_datacenter"`
	IsIsp        bool `json:"is_isp"`
	IsMobile     bool `json:"is_mobile"`
	IsVPN        bool `json:"is_vpn"`
	IsProxy      bool `json:"is_proxy"`
	IsTor        bool `json:"is_tor"`
	IsRelay      bool `json:"is_relay"`
	IsHosting    bool `json:"is_hosting"`

	Company *struct {
		Name    string `json:"name"`
		Domain  string `json:"domain"`
		Network string `json:"network"`
		Type    string `json:"type"`
	} `json:"company,omitempty"`

	Datacenter *struct {
		Name    string `json:"name"`
		Network string `json:"network"`
	} `json:"datacenter,omitempty"`

	VPN *struct {
		Name    string `json:"name"`
		Network string `json:"network"`
	} `json:"vpn,omitempty"`

	Proxy *struct {
		Type string `json:"type"`
	} `json:"proxy,omitempty"`

	Tor *struct {
		IsRelay bool   `json:"is_relay"`
		IsExit  bool   `json:"is_exit"`
		Name    string `json:"name,omitempty"`
	} `json:"tor,omitempty"`

	Abuse *struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
		Network string `json:"network"`
	} `json:"abuse,omitempty"`
}

func NewIPAPIISAdapter(apiKey string) *IPAPIISAdapter {
	return &IPAPIISAdapter{
		RemoteBase: NewRemoteBase("ipapi.is", "https://api.ipapi.is", apiKey),
	}
}

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

	if resp.Location != nil {
		intel.Country = resp.Location.CountryCode
		intel.CountryName = resp.Location.Country
		intel.City = resp.Location.City
		intel.Region = resp.Location.State
		intel.Timezone = resp.Location.Timezone
		intel.Latitude = resp.Location.Latitude
		intel.Longitude = resp.Location.Longitude
	}

	if resp.ASN != nil {
		intel.ASN = fmt.Sprintf("AS%d", resp.ASN.ASN)
		intel.Org = resp.ASN.Org
		if intel.Org == "" {
			intel.Org = resp.ASN.Descr
		}
		intel.UsageTypeRaw = resp.ASN.Type
		intel.UsageType = a.mapUsageType(resp.ASN.Type)
	}

	if resp.Company != nil && intel.Org == "" {
		intel.Org = resp.Company.Name
		if intel.UsageTypeRaw == "" {
			intel.UsageTypeRaw = resp.Company.Type
		}
		if intel.UsageType == "" {
			intel.UsageType = a.mapUsageType(resp.Company.Type)
		}
	}

	if resp.IsIsp && intel.ISP == "" {
		intel.ISP = intel.Org
	}

	intel.UsageType = intel.InferUsageType()

	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ipapis"
	}

	return intel
}

func (a *IPAPIISAdapter) mapUsageType(t string) string {
	switch t {
	case "isp", "business", "education", "government":
		return "isp"
	case "hosting", "datacenter":
		return "hosting"
	default:
		return "unknown"
	}
}
