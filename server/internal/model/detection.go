package model

import (
	"time"

	"detect-radar/dnsassessment"
)

type DNSLeakTestRequest struct {
	ScanID string `json:"scan_id"`
}

type DNSLeakTest struct {
	TestID          string              `json:"-"`
	TestDomain      string              `json:"-"`
	ScanID          string              `json:"-"`
	ScanInstanceID  string              `json:"-"`
	ExpectedCountry string              `json:"-"`
	Exit            *dnsassessment.Exit `json:"-"`
	ExpiresAt       int64               `json:"-"`
	CreatedAt       time.Time           `json:"-"`
	DNSQueries      []DNSQuery          `json:"-"`
	Journaled       bool                `json:"-"`
}

type DNSLeakTestResponse struct {
	ID         string `json:"id"`
	TestDomain string `json:"test_domain"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
}

type DNSQuery struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	ISP         string `json:"isp"`
	ASN         string `json:"asn,omitempty"`
	NetworkType string `json:"network_type,omitempty"`
	QueriedAt   string `json:"queried_at"`
}

type DNSLeakResult struct {
	ID              string                 `json:"id"`
	Leaked          bool                   `json:"leaked"`
	Level           string                 `json:"level"`
	DNSServers      []DNSQuery             `json:"dns_servers"`
	ExpectedCountry string                 `json:"expected_country"`
	ActualCountries []string               `json:"actual_countries"`
	Recommendation  string                 `json:"recommendation"`
	Assessment      *dnsassessment.Verdict `json:"assessment,omitempty"`
}

type BrowserInfo struct {
	Timezone  string   `json:"timezone"`
	Language  string   `json:"language"`
	Languages []string `json:"languages"`
}

type TimezoneCheck struct {
	Passed      bool   `json:"passed"`
	Browser     string `json:"browser"`
	Expected    string `json:"expected"`
	OffsetHours int    `json:"offset_hours,omitempty"`
}

type LanguageCheck struct {
	Passed    bool     `json:"passed"`
	Browser   string   `json:"browser"`
	Expected  []string `json:"expected"`
	IPCountry string   `json:"ip_country"`
}

type ConsistencyChecks struct {
	Timezone TimezoneCheck `json:"timezone"`
	Language LanguageCheck `json:"language"`
}
