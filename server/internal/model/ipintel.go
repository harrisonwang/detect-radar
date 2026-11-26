package model

import (
	"fmt"
	"time"
)

type IPIntel struct {
	IP          string  `json:"ip"`
	Hostname    string  `json:"hostname,omitempty"`
	Country     string  `json:"country"`
	CountryName string  `json:"country_name,omitempty"`
	City        string  `json:"city"`
	Region      string  `json:"region,omitempty"`
	Postal      string  `json:"postal,omitempty"`
	Timezone    string  `json:"timezone"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`

	ASN string `json:"asn"`
	Org string `json:"org"`
	ISP string `json:"isp,omitempty"`

	IsVPN       bool `json:"is_vpn"`
	IsProxy     bool `json:"is_proxy"`
	IsTor       bool `json:"is_tor"`
	IsRelay     bool `json:"is_relay"`
	IsHosting   bool `json:"is_hosting"`
	IsMobile    bool `json:"is_mobile"`
	IsAnonymous bool `json:"is_anonymous"`

	UsageType     string   `json:"usage_type"`
	UsageTypeRaw  string   `json:"usage_type_raw,omitempty"`
	Confidence    int      `json:"confidence,omitempty"`
	DetectMethod  string   `json:"detect_method,omitempty"`
	NeedsDeepScan bool     `json:"needs_deep_scan,omitempty"`
	RDNS          string   `json:"rdns,omitempty"`
	FraudScore    int      `json:"fraud_score"`
	HumanRatio    *float64 `json:"human_ratio,omitempty"`
	Tip           string   `json:"tip,omitempty"`

	Source    string    `json:"source"`
	Tier      string    `json:"tier"`
	FetchedAt time.Time `json:"fetched_at"`
}

type IPInfo struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"`
	Country  string `json:"country,omitempty"`
	Loc      string `json:"loc,omitempty"`
	Org      string `json:"org,omitempty"`
	Postal   string `json:"postal,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Source   string `json:"source,omitempty"`
}

func (i *IPIntel) HasProxy() bool {
	return i.IsVPN || i.IsProxy || i.IsTor || i.IsRelay || i.IsAnonymous
}

func (i *IPIntel) HasRisk() bool {
	return i.HasProxy() || i.IsHosting
}

func (i *IPIntel) IsClean() bool {
	return !i.HasRisk() && (i.UsageType == "isp" || i.IsMobile)
}

func (i *IPIntel) InferUsageType() string {
	if i.IsHosting {
		return "hosting"
	}
	if i.IsMobile {
		return "isp"
	}

	if i.UsageTypeRaw != "" {
		switch i.UsageTypeRaw {
		case "isp", "mobile", "residential", "satellite", "business", "education", "government":
			return "isp"
		case "hosting", "datacenter":
			return "hosting"
		}
	}

	if i.UsageType != "" {
		switch i.UsageType {
		case "isp", "mobile", "residential", "satellite", "business", "education", "government":
			return "isp"
		case "hosting", "datacenter":
			return "hosting"
		}
	}

	return "unknown"
}

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
