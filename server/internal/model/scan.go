package model

import "detect-radar/dnsassessment"

type CollectorResult[T any] struct {
	Status string  `json:"status"`
	Data   *T      `json:"data,omitempty"`
	Error  string  `json:"error,omitempty"`
	Timing float64 `json:"timing,omitempty"`
}

type ScanRequest struct {
	ScanID      string          `json:"scan_id"`
	Scenario    string          `json:"scenario"`
	Fingerprint ScanFingerprint `json:"fingerprint"`
	Environment ScanEnvironment `json:"environment"`
	Leaks       ScanLeaks       `json:"leaks"`
	Network     *ScanNetwork    `json:"network,omitempty"`
	FPJSAudit   *FPJSAudit      `json:"fpjs_audit,omitempty"`
}

type ScanFingerprint struct {
	Canvas *CollectorResult[CanvasData] `json:"canvas,omitempty"`
	Audio  *CollectorResult[AudioData]  `json:"audio,omitempty"`
	WebGL  *CollectorResult[WebGLData]  `json:"webgl,omitempty"`
}

type CanvasData struct {
	HashSampleA string `json:"hash_sample_a"`
	HashSampleB string `json:"hash_sample_b"`
	NativeCode  string `json:"native_code"`
}

type AudioData struct {
	NativeCode string `json:"native_code"`
}

type WebGLData struct {
	UnmaskedRenderer string `json:"unmasked_renderer"`
}

type ScanEnvironment struct {
	Navigator     *CollectorResult[NavigatorData]     `json:"navigator,omitempty"`
	UserAgentData *CollectorResult[UserAgentDataInfo] `json:"userAgentData,omitempty"`
	Timezone      *CollectorResult[TimezoneData]      `json:"timezone,omitempty"`
}

type NavigatorData struct {
	Platform      string   `json:"platform"`
	Language      string   `json:"language"`
	Languages     []string `json:"languages"`
	Webdriver     *bool    `json:"webdriver"`
	PluginsLength int      `json:"plugins_length"`
}

type UserAgentDataInfo struct {
	Mobile bool `json:"mobile"`
}

type TimezoneData struct {
	Timezone string `json:"timezone"`
}

type ScanLeaks struct {
	WebRTC *CollectorResult[WebRTCLeakData] `json:"webrtc,omitempty"`
	IPv6   *CollectorResult[IPv6LeakData]   `json:"ipv6,omitempty"`
}

type WebRTCLeakData struct {
	LocalIPs  []string `json:"local_ips"`
	PublicIPs []string `json:"public_ips"`
}

type IPv6LeakData struct {
	Leaked      bool   `json:"leaked"`
	IPv6Address string `json:"ipv6_address"`
}

type ScanNetwork struct {
	RTT *CollectorResult[RTTData] `json:"rtt,omitempty"`
}

type RTTData struct {
	ProbeID    string    `json:"probe_id,omitempty"`
	Samples    []float64 `json:"samples"`
	MinMS      float64   `json:"min_ms"`
	MedianMS   float64   `json:"median_ms"`
	JitterMS   float64   `json:"jitter_ms"`
	Count      int       `json:"count"`
	ConnectMS  *float64  `json:"connect_ms,omitempty"`
	ConnReused *bool     `json:"conn_reused,omitempty"`
}

type FPJSAudit struct {
	Hook FPJSHookAudit `json:"hook"`
}

type FPJSHookAudit struct {
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Methods []string `json:"methods"`
}

type ScanResponse struct {
	ScanID           string           `json:"scan_id"`
	Status           string           `json:"status"`
	Scenario         string           `json:"scenario,omitempty"`
	Score            int              `json:"score"`
	RiskLevel        string           `json:"risk_level"`
	Diagnosis        []DiagnosisItem  `json:"diagnosis"`
	Dimensions       []DimensionScore `json:"dimensions"`
	Analysis         ScanAnalysis     `json:"analysis"`
	ServerData       ScanServerData   `json:"server_data"`
	ProcessedAt      string           `json:"processed_at"`
	ProcessingTimeMs int64            `json:"processing_time_ms"`
}

type DiagnosisItem struct {
	Status string `json:"status"`
	Text   string `json:"text"`
	Code   string `json:"code,omitempty"`
}

type DimensionScore struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Score  int    `json:"score"`
	State  string `json:"state"`
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
}

type ScanAnalysis struct {
	Identity    IdentityAnalysis    `json:"identity"`
	Fingerprint FingerprintAnalysis `json:"fingerprint"`
	Privacy     PrivacyAnalysis     `json:"privacy"`
	Consistency ConsistencyChecks   `json:"consistency"`
	Automation  AutomationAnalysis  `json:"automation"`
	RTT         RTTAnalysis         `json:"rtt"`
}

type RTTAnalysis struct {
	Status         string   `json:"status"`
	ClientMinMS    float64  `json:"client_min_ms,omitempty"`
	ClientMedianMS float64  `json:"client_median_ms,omitempty"`
	ClientJitterMS float64  `json:"client_jitter_ms,omitempty"`
	ServerTCPMS    float64  `json:"server_tcp_ms,omitempty"`
	ServerTCPMinMS float64  `json:"server_tcp_min_ms,omitempty"`
	ServerRTTVarMS float64  `json:"server_rtt_var_ms,omitempty"`
	DeltaMinMS     *float64 `json:"delta_min_ms,omitempty"`
	GeoMinMS       *float64 `json:"geo_min_ms,omitempty"`
	GeoDistanceKM  *float64 `json:"geo_distance_km,omitempty"`
	GeoViolation   *bool    `json:"geo_violation,omitempty"`
	GeoSource      string   `json:"geo_source,omitempty"`
	ConnectMS      *float64 `json:"connect_ms,omitempty"`
	ConnReused     *bool    `json:"conn_reused,omitempty"`
}

type IdentityAnalysis struct {
	IPType           string   `json:"ip_type"`
	IPTypeConfidence int      `json:"ip_type_confidence"`
	Verdict          string   `json:"verdict"`
	FraudScore       int      `json:"fraud_score"`
	IsProxy          bool     `json:"is_proxy"`
	IsVPN            bool     `json:"is_vpn"`
	IsTor            bool     `json:"is_tor"`
	IsHosting        bool     `json:"is_hosting"`
	HumanRatio       *float64 `json:"human_ratio,omitempty"`

	BlacklistHits []string `json:"blacklist_hits"`
	OpenPorts     []int    `json:"open_ports,omitempty"`
	OpenProxyPort bool     `json:"open_proxy_port,omitempty"`
	WeakProxyPort bool     `json:"weak_proxy_port,omitempty"`
	IsTorExit     bool     `json:"is_tor_exit,omitempty"`
	ExposureTags  []string `json:"exposure_tags,omitempty"`
}

type FingerprintAnalysis struct {
	Canvas        FPCanvasAnalysis `json:"canvas"`
	Audio         FPAudioAnalysis  `json:"audio"`
	WebGL         FPWebGLAnalysis  `json:"webgl"`
	OverallStatus string           `json:"overall_status"`
}

type FPCanvasAnalysis struct {
	Status   string `json:"status"`
	IsStable bool   `json:"is_stable"`
	IsHooked bool   `json:"is_hooked"`
}

type FPAudioAnalysis struct {
	Status   string `json:"status"`
	IsHooked bool   `json:"is_hooked"`
}

type FPWebGLAnalysis struct {
	Status         string `json:"status"`
	MismatchCode   string `json:"mismatch_code,omitempty"`
	MismatchReason string `json:"mismatch_reason,omitempty"`
	VendorMatchsUA bool   `json:"vendor_matches_ua"`
}

type PrivacyAnalysis struct {
	WebRTC        PrivacyWebRTC `json:"webrtc"`
	DNS           PrivacyDNS    `json:"dns"`
	IPv6          PrivacyIPv6   `json:"ipv6"`
	OverallStatus string        `json:"overall_status"`
}

type PrivacyWebRTC struct {
	Status        string   `json:"status"`
	LeakedIPs     []string `json:"leaked_ips"`
	MatchesExitIP bool     `json:"matches_exit_ip"`
}

type PrivacyDNS struct {
	Status        string                 `json:"status"`
	MatchesExitIP bool                   `json:"matches_exit_ip"`
	Country       string                 `json:"country,omitempty"`
	ISP           string                 `json:"isp,omitempty"`
	Assessment    *dnsassessment.Verdict `json:"assessment,omitempty"`
}

type PrivacyIPv6 struct {
	Status  string `json:"status"`
	Address string `json:"address,omitempty"`
}

type AutomationAnalysis struct {
	WebdriverDetected bool     `json:"webdriver_detected"`
	NoDesktopPlugins  bool     `json:"no_desktop_plugins,omitempty"`
	HookedMethods     []string `json:"hooked_methods,omitempty"`
	AutomationTraces  []string `json:"automation_traces"`
	OverallStatus     string   `json:"overall_status"`
}

type ScanServerData struct {
	ClientIP    string   `json:"client_ip"`
	IPIntel     *IPIntel `json:"ip_intel,omitempty"`
	HTTPVersion string   `json:"http_version,omitempty"`
}
