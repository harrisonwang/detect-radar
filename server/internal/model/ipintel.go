package model

import (
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

func (i *IPIntel) HasProxy() bool {
	return i.IsVPN || i.IsProxy || i.IsTor || i.IsRelay || i.IsAnonymous
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
