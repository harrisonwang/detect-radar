package adapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"detect-radar/internal/model"
)

type IPInfoAdapter struct {
	RemoteBase
}

type ipinfoResponse struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"`
	Org      string `json:"org"`
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`

	ASN *struct {
		ASN    string `json:"asn"`
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Route  string `json:"route"`
		Type   string `json:"type"`
	} `json:"asn,omitempty"`

	Company *struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Type   string `json:"type"`
	} `json:"company,omitempty"`

	Privacy *struct {
		VPN     bool   `json:"vpn"`
		Proxy   bool   `json:"proxy"`
		Tor     bool   `json:"tor"`
		Relay   bool   `json:"relay"`
		Hosting bool   `json:"hosting"`
		Service string `json:"service"`
	} `json:"privacy,omitempty"`

	IsAnycast   bool `json:"is_anycast"`
	IsMobile    bool `json:"is_mobile"`
	IsAnonymous bool `json:"is_anonymous"`
	IsSatellite bool `json:"is_satellite"`
	IsHosting   bool `json:"is_hosting"`
}

func NewIPInfoAdapter(apiKey string) *IPInfoAdapter {
	return &IPInfoAdapter{
		RemoteBase: NewRemoteBase("ipinfo", "https://ipinfo.io", apiKey),
	}
}

func (a *IPInfoAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       true,
		HasASN:       true,
		HasTimezone:  true,
		HasPrivacy:   true,
		HasUsageType: true,
		HasRiskScore: false,
	}
}

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

		IsMobile:    resp.IsMobile,
		IsHosting:   resp.IsHosting,
		IsAnonymous: resp.IsAnonymous,
	}

	if resp.Loc != "" {
		parts := strings.Split(resp.Loc, ",")
		if len(parts) == 2 {
			intel.Latitude, _ = strconv.ParseFloat(parts[0], 64)
			intel.Longitude, _ = strconv.ParseFloat(parts[1], 64)
		}
	}

	if resp.ASN != nil {
		intel.ASN = resp.ASN.ASN
		intel.Org = resp.ASN.Name
		intel.UsageTypeRaw = resp.ASN.Type
		intel.UsageType = a.mapUsageType(resp.ASN.Type)
	} else if resp.Org != "" {
		intel.ASN, intel.Org = a.parseOrgField(resp.Org)
	}

	if resp.Company != nil {
		if intel.Org == "" {
			intel.Org = resp.Company.Name
		}
		if intel.UsageTypeRaw == "" {
			intel.UsageTypeRaw = resp.Company.Type
		}
		if intel.UsageType == "" {
			intel.UsageType = a.mapUsageType(resp.Company.Type)
		}
	}

	if resp.Privacy != nil {
		intel.IsVPN = resp.Privacy.VPN
		intel.IsProxy = resp.Privacy.Proxy
		intel.IsTor = resp.Privacy.Tor
		intel.IsRelay = resp.Privacy.Relay
		if resp.Privacy.Hosting {
			intel.IsHosting = true
		}
	}

	if intel.IsVPN || intel.IsProxy || intel.IsTor || intel.IsRelay {
		intel.IsAnonymous = true
	}

	intel.UsageType = intel.InferUsageType()

	if intel.IsHosting || intel.UsageTypeRaw != "" {
		intel.Confidence = 85
		intel.DetectMethod = "api_ipinfo"
	}

	return intel
}

func (a *IPInfoAdapter) parseOrgField(org string) (asn, name string) {
	parts := strings.SplitN(org, " ", 2)
	if len(parts) == 2 && strings.HasPrefix(parts[0], "AS") {
		return parts[0], parts[1]
	}
	return "", org
}

func (a *IPInfoAdapter) mapUsageType(t string) string {
	switch t {
	case "isp", "business", "education", "government":
		return "isp"
	case "hosting", "datacenter":
		return "hosting"
	default:
		return "unknown"
	}
}
