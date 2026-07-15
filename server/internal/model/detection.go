package model

import "time"

// ============================================================================
// DNS 泄露检测（自建权威 NS / DNSTap 记录 resolver）
// ============================================================================

// DNSLeakTestRequest 创建测试的请求体（scan_id 可选，用于遥测流水关联到所属扫描）
type DNSLeakTestRequest struct {
	ScanID string `json:"scan_id"`
}

// DNSLeakTest 一次 DNS 泄露测试的内部记录
type DNSLeakTest struct {
	TestID     string     `json:"-"`
	TestDomain string     `json:"-"`
	ScanID     string     `json:"-"`
	ExpiresAt  int64      `json:"-"`
	CreatedAt  time.Time  `json:"-"`
	DNSQueries []DNSQuery `json:"-"`
	Journaled  bool       `json:"-"` // 观测结果是否已写入遥测流水（只写一次）
}

// DNSLeakTestResponse 创建测试后返回给客户端的信息
type DNSLeakTestResponse struct {
	ID         string `json:"id"`
	TestDomain string `json:"test_domain"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
}

// DNSQuery 一条被记录到的 resolver 查询
type DNSQuery struct {
	IP        string `json:"ip"`
	Country   string `json:"country"`
	ISP       string `json:"isp"`
	QueriedAt string `json:"queried_at"`
}

// DNSLeakResult DNS 泄露分析结果
type DNSLeakResult struct {
	ID              string     `json:"id"`
	Leaked          bool       `json:"leaked"`
	Level           string     `json:"level"` // safe / warning / danger
	DNSServers      []DNSQuery `json:"dns_servers"`
	ExpectedCountry string     `json:"expected_country"`
	ActualCountries []string   `json:"actual_countries"`
	Recommendation  string     `json:"recommendation"`
}

// ============================================================================
// 环境一致性检测（IP ↔ 时区 / 语言）
// ============================================================================

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
