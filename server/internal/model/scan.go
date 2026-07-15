package model

// ============================================================================
// POST /scans 统一扫描请求（对齐 client/docs/api-scans.md）
// 客户端采集原始信号，服务端分析。以下只声明服务端实际消费的字段。
// ============================================================================

// CollectorResult 通用采集结果包装
type CollectorResult[T any] struct {
	Status string  `json:"status"` // success / error / unsupported / denied
	Data   *T      `json:"data,omitempty"`
	Error  string  `json:"error,omitempty"`
	Timing float64 `json:"timing,omitempty"`
}

type ScanRequest struct {
	ScanID      string          `json:"scan_id"`
	Timestamp   int64           `json:"timestamp"`
	Scenario    string          `json:"scenario"` // general / ecommerce / social
	Fingerprint ScanFingerprint `json:"fingerprint"`
	Environment ScanEnvironment `json:"environment"`
	Leaks       ScanLeaks       `json:"leaks"`
	Network     *ScanNetwork    `json:"network,omitempty"` // 客户端上报的网络物理层信号（RTT，Phase 1 只采集不计分）
	FPJSAudit   *FPJSAudit      `json:"fpjs_audit,omitempty"`
}

// ---- 指纹 ----

type ScanFingerprint struct {
	Canvas *CollectorResult[CanvasData] `json:"canvas,omitempty"`
	Audio  *CollectorResult[AudioData]  `json:"audio,omitempty"`
	WebGL  *CollectorResult[WebGLData]  `json:"webgl,omitempty"`
}

type CanvasData struct {
	HashSampleA string `json:"hash_sample_a"`
	HashSampleB string `json:"hash_sample_b"`
	NativeCode  string `json:"native_code"`
	DataLength  int    `json:"data_length"`
}

type AudioData struct {
	HashSample string  `json:"hash_sample"`
	EntropyRaw float64 `json:"entropy_raw"`
	NativeCode string  `json:"native_code"`
}

type WebGLData struct {
	ContextName      string `json:"context_name"`
	BaseVendor       string `json:"base_vendor"`
	BaseRenderer     string `json:"base_renderer"`
	UnmaskedVendor   string `json:"unmasked_vendor"`
	UnmaskedRenderer string `json:"unmasked_renderer"`
	NativeCode       string `json:"native_code"`
}

// ---- 环境 ----

type ScanEnvironment struct {
	Navigator     *CollectorResult[NavigatorData]     `json:"navigator,omitempty"`
	UserAgentData *CollectorResult[UserAgentDataInfo] `json:"userAgentData,omitempty"`
	Screen        *CollectorResult[ScreenData]        `json:"screen,omitempty"`
	Timezone      *CollectorResult[TimezoneData]      `json:"timezone,omitempty"`
}

type NavigatorData struct {
	UserAgent           string   `json:"userAgent"`
	Platform            string   `json:"platform"`
	Language            string   `json:"language"`
	Languages           []string `json:"languages"`
	HardwareConcurrency int      `json:"hardwareConcurrency"`
	DeviceMemory        *float64 `json:"deviceMemory"`
	MaxTouchPoints      int      `json:"maxTouchPoints"`
	Webdriver           *bool    `json:"webdriver"`
	Vendor              string   `json:"vendor"`
	PluginsLength       int      `json:"plugins_length"`
}

type UserAgentDataInfo struct {
	Brands   []UABrand `json:"brands"`
	Mobile   bool      `json:"mobile"`
	Platform string    `json:"platform"`
}

type UABrand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

type ScreenData struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	DevicePixelRatio float64 `json:"devicePixelRatio"`
}

type TimezoneData struct {
	Timezone       string `json:"timezone"`
	TimezoneOffset int    `json:"timezoneOffset"`
}

// ---- 泄露（客户端已采集的结果，服务端复核）----

type ScanLeaks struct {
	WebRTC *CollectorResult[WebRTCLeakData] `json:"webrtc,omitempty"`
	DNS    *CollectorResult[DNSLeakData]    `json:"dns,omitempty"`
	IPv6   *CollectorResult[IPv6LeakData]   `json:"ipv6,omitempty"`
}

type WebRTCLeakData struct {
	Leaked    bool     `json:"leaked"`
	LocalIPs  []string `json:"local_ips"`
	PublicIPs []string `json:"public_ips"`
	STUNIPs   []string `json:"stun_ips"`
}

type DNSLeakData struct {
	Leaked    bool     `json:"leaked"`
	TestID    string   `json:"test_id,omitempty"`
	Resolvers []string `json:"resolvers"`
}

type IPv6LeakData struct {
	Leaked      bool   `json:"leaked"`
	IPv6Address string `json:"ipv6_address"`
}

// ---- 网络物理层（RTT，Phase 1 影子采集：只落库不计分）----
// 用物理延迟识别代理：代理在客户端与服务器之间插入一段物理链路，往返时延会暴露它。
// IP 层分类有信息论天花板，物理测量是绕不过的补充。client 端严格对齐以下形状，勿改。

// ScanNetwork 客户端上报的网络物理层信号
type ScanNetwork struct {
	RTT *CollectorResult[RTTData] `json:"rtt,omitempty"`
}

// RTTData 浏览器测得的应用层往返时延（含代理腿）
type RTTData struct {
	Samples    []float64 `json:"samples"` // 每次探测的应用层往返 ms
	MinMS      float64   `json:"min_ms"`
	MedianMS   float64   `json:"median_ms"`
	JitterMS   float64   `json:"jitter_ms"` // max-min
	Count      int       `json:"count"`
	ConnectMS  *float64  `json:"connect_ms,omitempty"` // Resource Timing connectEnd-connectStart，H2 复用时不可得
	ConnReused *bool     `json:"conn_reused,omitempty"`
}

// ---- FingerprintJS 审计 ----

type FPJSAudit struct {
	VisitorID string        `json:"visitorId,omitempty"`
	Hook      FPJSHookAudit `json:"hook"`
}

type FPJSHookAudit struct {
	Status  string   `json:"status"` // SAFE / DANGER
	Message string   `json:"message"`
	Methods []string `json:"methods"`
}

// ============================================================================
// POST /scans 响应
// ============================================================================

type ScanResponse struct {
	ScanID           string           `json:"scan_id"`
	Status           string           `json:"status"`
	Score            int              `json:"score"`
	RiskLevel        string           `json:"risk_level"` // safe / low / medium / high / critical
	Diagnosis        []DiagnosisItem  `json:"diagnosis"`
	Dimensions       []DimensionScore `json:"dimensions"` // 四轴雷达分（纯后端评分，前端直接消费）
	Analysis         ScanAnalysis     `json:"analysis"`
	ServerData       ScanServerData   `json:"server_data"`
	ProcessedAt      string           `json:"processed_at"`
	ProcessingTimeMs int64            `json:"processing_time_ms"`
}

type DiagnosisItem struct {
	Status string `json:"status"` // success / warning / error
	Text   string `json:"text"`
	Code   string `json:"code,omitempty"`
}

// DimensionScore 单个维度（雷达轴）的评分与语义状态，由后端统一算出。
type DimensionScore struct {
	Key    string `json:"key"` // identity / consistency / leak / environment
	Label  string `json:"label"`
	Score  int    `json:"score"` // 0-100
	State  string `json:"state"` // coherent / contradiction / leak / unknown
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
}

type ScanAnalysis struct {
	Identity    IdentityAnalysis    `json:"identity"`
	Fingerprint FingerprintAnalysis `json:"fingerprint"`
	Privacy     PrivacyAnalysis     `json:"privacy"`
	Consistency ConsistencyChecks   `json:"consistency"`
	Automation  AutomationAnalysis  `json:"automation"`
	// RTT 物理探测（Phase 1 只采集不计分）。刻意放进 ScanAnalysis：journal 会整块
	// dump analysis，RTT 随之自动落遥测流水，且影子回放能读到它——但 rules.go 的
	// score() 绝不读 a.RTT（无规则、无扣分、无维度）。计分留待 Phase 3 标定后。
	RTT RTTAnalysis `json:"rtt"`
}

// RTTAnalysis 由客户端 RTTData、nginx 头和 IP 信息派生出的 journal-ready RTT 视图。
// 三个测量：客户端应用层 RTT（含代理腿）/ nginx 出口↔服务器 TCP RTT / 地理-延迟下限校验。
type RTTAnalysis struct {
	Status         string   `json:"status"`                  // ok / partial / unavailable
	ClientMinMS    float64  `json:"client_min_ms,omitempty"` // 浏览器测得（含代理腿）
	ClientMedianMS float64  `json:"client_median_ms,omitempty"`
	ClientJitterMS float64  `json:"client_jitter_ms,omitempty"`
	ServerTCPMS    float64  `json:"server_tcp_ms,omitempty"` // nginx 测得：出口 IP ↔ 服务器
	ServerRTTVarMS float64  `json:"server_rtt_var_ms,omitempty"`
	DeltaMS        *float64 `json:"delta_ms,omitempty"`   // client_min - server_tcp，≈ 浏览器→出口 这一腿
	GeoMinMS       *float64 `json:"geo_min_ms,omitempty"` // IP 声称位置到本机的物理最小 RTT
	GeoDistanceKM  *float64 `json:"geo_distance_km,omitempty"`
	GeoViolation   *bool    `json:"geo_violation,omitempty"` // server_tcp < geo_min（IP 位置与延迟矛盾）
	// GeoSource 声明 geo 字段用了哪路坐标：intel（intel 自带精确坐标）/
	// cn_province_centroid（CN 权威层清空坐标后用省会质心兜底，含 800km 保守 slack）。
	// 仅在算出 geo 字段时设置，供 Phase 2 标定按来源分权（质心近似需放宽）。
	GeoSource  string   `json:"geo_source,omitempty"`
	ConnectMS  *float64 `json:"connect_ms,omitempty"`
	ConnReused *bool    `json:"conn_reused,omitempty"`
}

type IdentityAnalysis struct {
	IPType           string   `json:"ip_type"` // residential / datacenter / mobile / unknown
	IPTypeConfidence int      `json:"ip_type_confidence"`
	Verdict          string   `json:"verdict"` // residential_pass / datacenter_fail / gray_unknown
	FraudScore       int      `json:"fraud_score"`
	IsProxy          bool     `json:"is_proxy"`
	IsVPN            bool     `json:"is_vpn"`
	IsTor            bool     `json:"is_tor"`
	IsHosting        bool     `json:"is_hosting"`
	HumanRatio       *float64 `json:"human_ratio,omitempty"`

	// 出口 IP 信誉/暴露（P5，来自 ReputationService）
	BlacklistHits []string `json:"blacklist_hits"`
	OpenPorts     []int    `json:"open_ports,omitempty"`
	OpenProxyPort bool     `json:"open_proxy_port,omitempty"` // 强代理端口（socks/tor/openvpn/wireguard）
	WeakProxyPort bool     `json:"weak_proxy_port,omitempty"` // 弱代理端口（http-proxy），仅机房出口下计分
	IsTorExit     bool     `json:"is_tor_exit,omitempty"`
	ExposureTags  []string `json:"exposure_tags,omitempty"`
}

type FingerprintAnalysis struct {
	Canvas        FPCanvasAnalysis `json:"canvas"`
	Audio         FPAudioAnalysis  `json:"audio"`
	WebGL         FPWebGLAnalysis  `json:"webgl"`
	OverallStatus string           `json:"overall_status"` // native / protected / conflict / unknown
}

type FPCanvasAnalysis struct {
	Status   string `json:"status"` // native / noise / blocked
	IsStable bool   `json:"is_stable"`
	IsHooked bool   `json:"is_hooked"`
}

type FPAudioAnalysis struct {
	Status   string `json:"status"`
	IsHooked bool   `json:"is_hooked"`
}

type FPWebGLAnalysis struct {
	Status         string `json:"status"` // match / mismatch / unknown
	MismatchReason string `json:"mismatch_reason,omitempty"`
	VendorMatchsUA bool   `json:"vendor_matches_ua"`
}

type PrivacyAnalysis struct {
	WebRTC        PrivacyWebRTC `json:"webrtc"`
	DNS           PrivacyDNS    `json:"dns"`
	IPv6          PrivacyIPv6   `json:"ipv6"`
	OverallStatus string        `json:"overall_status"` // safe / warning / danger / disabled
}

type PrivacyWebRTC struct {
	Status        string   `json:"status"` // safe / warning / danger / blocked
	LeakedIPs     []string `json:"leaked_ips"`
	MatchesExitIP bool     `json:"matches_exit_ip"`
}

type PrivacyDNS struct {
	Status        string `json:"status"`
	MatchesExitIP bool   `json:"matches_exit_ip"`
}

type PrivacyIPv6 struct {
	Status  string `json:"status"`
	Address string `json:"address,omitempty"`
}

type AutomationAnalysis struct {
	WebdriverDetected bool     `json:"webdriver_detected"`
	AutomationTraces  []string `json:"automation_traces"`
	OverallStatus     string   `json:"overall_status"` // safe / suspicious / detected
}

type ScanServerData struct {
	ClientIP    string   `json:"client_ip"`
	IPIntel     *IPIntel `json:"ip_intel,omitempty"`
	JA3Hash     string   `json:"ja3_hash,omitempty"` // 预留：TLS 指纹（P3）
	HTTPVersion string   `json:"http_version,omitempty"`
}
