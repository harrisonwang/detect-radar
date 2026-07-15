package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"detect-radar/internal/model"
)

// ScanService 汇总一次扫描的服务端分析：IP 信息、一致性、泄露复核、指纹和自动化
type ScanService struct {
	ipIntel     *IPIntelService
	consistency *ConsistencyService
	reputation  *ReputationService
	rttCoords   rttServerCoords // 服务器坐标，用于 RTT 地理-延迟下限校验（Phase 1）
	store       sync.Map        // scanID -> storedScan
	ttl         time.Duration
}

type storedScan struct {
	resp      *model.ScanResponse
	createdAt time.Time
}

func NewScanService(ipIntel *IPIntelService, consistency *ConsistencyService, reputation *ReputationService, rttLat, rttLon float64) *ScanService {
	s := &ScanService{
		ipIntel:     ipIntel,
		consistency: consistency,
		reputation:  reputation,
		rttCoords:   rttServerCoords{Lat: rttLat, Lon: rttLon},
		ttl:         30 * time.Minute,
	}
	go s.cleanup()
	return s
}

// Analyze 执行一次完整扫描分析。
// tcpRTT/tcpRTTVar 是 nginx 透传的 X-TCP-RTT / X-TCP-RTTVAR 头（µs 整数字符串，
// 描述「出口 IP↔服务器」这一腿；本地开发无 nginx 时为空，优雅降级）。
func (s *ScanService) Analyze(ctx context.Context, req *model.ScanRequest, clientIP, httpVersion, tcpRTT, tcpRTTVar string) *model.ScanResponse {
	start := time.Now()

	// 1. 出口 IP 信息（深度查询）
	intel, err := s.ipIntel.Lookup(ctx, clientIP, LookupOptions{DeepScan: true})
	if err != nil || intel == nil {
		intel = &model.IPIntel{IP: clientIP}
	}

	resp := &model.ScanResponse{
		ScanID: req.ScanID,
		Status: "success",
		ServerData: model.ScanServerData{
			ClientIP:    clientIP,
			IPIntel:     intel,
			HTTPVersion: httpVersion,
		},
	}

	// 2. 各维度分析
	resp.Analysis.Identity = buildIdentity(intel)

	// 2.1 出口 IP 信誉/暴露（P5）——硬超时，避免 DNSBL/Shodan 慢查询拖垮整次扫描
	if s.reputation != nil {
		repCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		rep := s.reputation.Check(repCtx, clientIP)
		cancel()
		resp.Analysis.Identity.BlacklistHits = rep.BlacklistSources
		resp.Analysis.Identity.OpenPorts = rep.OpenPorts
		resp.Analysis.Identity.OpenProxyPort = rep.OpenProxyPort
		resp.Analysis.Identity.WeakProxyPort = rep.WeakProxyPort
		resp.Analysis.Identity.IsTorExit = rep.IsTorExit
		resp.Analysis.Identity.ExposureTags = rep.ExposureTags
	}

	resp.Analysis.Consistency = s.consistency.Evaluate(browserFrom(req), intel)
	resp.Analysis.Privacy = buildPrivacy(req, clientIP, intel)
	resp.Analysis.Fingerprint = buildFingerprint(req)
	resp.Analysis.Automation = buildAutomation(req)

	// 2.2 RTT 物理探测（Phase 1 影子采集：只落库不计分，score() 不读 a.RTT）
	resp.Analysis.RTT = deriveRTT(clientRTTData(req), tcpRTT, tcpRTTVar, intel, s.rttCoords)

	// 3. 评分 + 诊断 + 四维雷达分（rules.go 是唯一评分入口）
	resp.Score, resp.RiskLevel, resp.Diagnosis, resp.Dimensions = score(resp.Analysis, intel)

	resp.ProcessedAt = time.Now().UTC().Format(time.RFC3339)
	resp.ProcessingTimeMs = time.Since(start).Milliseconds()

	if req.ScanID != "" {
		s.store.Store(req.ScanID, storedScan{resp: resp, createdAt: time.Now()})
	}
	return resp
}

// Get 按 scan_id 取回历史结果
func (s *ScanService) Get(scanID string) (*model.ScanResponse, bool) {
	v, ok := s.store.Load(scanID)
	if !ok {
		return nil, false
	}
	return v.(storedScan).resp, true
}

// UpdateDNSLeak 用异步返回的 DNS 泄露结果更新已存扫描并重新评分。
// DNS 走独立慢路径（自建权威 NS + DNSTap，约 4s），初次扫描时 DNS 尚未定；
// 结果到位后回传后端由此方法重算，保证评分始终纯后端产出。
func (s *ScanService) UpdateDNSLeak(scanID string, leaked bool) (*model.ScanResponse, bool) {
	v, ok := s.store.Load(scanID)
	if !ok {
		return nil, false
	}
	stored := v.(storedScan)
	resp := stored.resp

	// 就地改 resp（只动 Privacy.DNS 再重算评分）：resp.Analysis.RTT 原样保留，
	// 不会在 DNS 重评分路径里被丢掉（RTT Phase 1 采集数据须随 post-DNS 判定一起留存）。
	if leaked {
		resp.Analysis.Privacy.DNS = model.PrivacyDNS{Status: "danger", MatchesExitIP: false}
	} else {
		resp.Analysis.Privacy.DNS = model.PrivacyDNS{Status: "safe", MatchesExitIP: true}
	}

	// 重算 privacy 综合状态（与 buildPrivacy 同口径）
	pv := &resp.Analysis.Privacy
	switch {
	case pv.WebRTC.Status == "danger" || pv.DNS.Status == "danger" || pv.IPv6.Status == "danger":
		pv.OverallStatus = "danger"
	case pv.WebRTC.Status == "warning":
		pv.OverallStatus = "warning"
	default:
		pv.OverallStatus = "safe"
	}

	resp.Score, resp.RiskLevel, resp.Diagnosis, resp.Dimensions = score(resp.Analysis, resp.ServerData.IPIntel)
	s.store.Store(scanID, storedScan{resp: resp, createdAt: stored.createdAt})
	return resp, true
}

func (s *ScanService) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.store.Range(func(key, value any) bool {
			if now.Sub(value.(storedScan).createdAt) > s.ttl {
				s.store.Delete(key)
			}
			return true
		})
	}
}

// ---- identity ----

func buildIdentity(intel *model.IPIntel) model.IdentityAnalysis {
	ia := model.IdentityAnalysis{
		IPTypeConfidence: intel.Confidence,
		FraudScore:       intel.FraudScore,
		IsProxy:          intel.IsProxy,
		IsVPN:            intel.IsVPN,
		IsTor:            intel.IsTor,
		IsHosting:        intel.IsHosting,
		HumanRatio:       intel.HumanRatio,
	}

	usage := intel.InferUsageType()
	switch {
	case intel.IsHosting || usage == "hosting":
		ia.IPType = "datacenter"
		ia.Verdict = "datacenter_fail"
	case intel.IsMobile:
		ia.IPType = "mobile"
		ia.Verdict = "residential_pass"
	case usage == "isp":
		ia.IPType = "residential"
		ia.Verdict = "residential_pass"
	default:
		ia.IPType = "unknown"
		ia.Verdict = "gray_unknown"
	}
	return ia
}

// ---- consistency inputs ----

// clientRTTData 取出客户端上报的 RTT 采样（缺失/未采集时返回 nil，deriveRTT 会优雅降级）
func clientRTTData(req *model.ScanRequest) *model.RTTData {
	if req.Network == nil || req.Network.RTT == nil {
		return nil
	}
	return req.Network.RTT.Data
}

func browserFrom(req *model.ScanRequest) model.BrowserInfo {
	b := model.BrowserInfo{}
	if tz := req.Environment.Timezone; tz != nil && tz.Data != nil {
		b.Timezone = tz.Data.Timezone
	}
	if nav := req.Environment.Navigator; nav != nil && nav.Data != nil {
		b.Language = nav.Data.Language
		b.Languages = nav.Data.Languages
	}
	return b
}

// ---- privacy ----

func buildPrivacy(req *model.ScanRequest, exitIP string, intel *model.IPIntel) model.PrivacyAnalysis {
	pa := model.PrivacyAnalysis{
		WebRTC: model.PrivacyWebRTC{Status: "blocked", LeakedIPs: []string{}, MatchesExitIP: true},
		DNS:    model.PrivacyDNS{Status: "blocked", MatchesExitIP: true},
		IPv6:   model.PrivacyIPv6{Status: "disabled"},
	}

	exitHasProxyRisk := intel != nil && (intel.HasProxy() || intel.IsHosting || intel.InferUsageType() == "hosting")

	// WebRTC：普通双栈公网地址只提示；明确代理/机房出口出现额外公网地址才判高风险。
	if w := req.Leaks.WebRTC; w != nil && w.Data != nil {
		var extra []string
		for _, ip := range w.Data.PublicIPs {
			if ip != exitIP {
				extra = append(extra, ip)
			}
		}
		switch {
		case len(extra) > 0 && exitHasProxyRisk:
			pa.WebRTC = model.PrivacyWebRTC{Status: "danger", LeakedIPs: extra, MatchesExitIP: false}
		case len(extra) > 0:
			pa.WebRTC = model.PrivacyWebRTC{Status: "warning", LeakedIPs: extra, MatchesExitIP: true}
		case len(w.Data.LocalIPs) > 0 && len(w.Data.PublicIPs) == 0:
			pa.WebRTC = model.PrivacyWebRTC{Status: "warning", LeakedIPs: []string{}, MatchesExitIP: true}
		default:
			pa.WebRTC = model.PrivacyWebRTC{Status: "safe", LeakedIPs: []string{}, MatchesExitIP: true}
		}
	}

	// DNS：采用客户端复核后的 leaked 标志
	if d := req.Leaks.DNS; d != nil && d.Data != nil {
		if d.Data.Leaked {
			pa.DNS = model.PrivacyDNS{Status: "danger", MatchesExitIP: false}
		} else if len(d.Data.Resolvers) > 0 {
			pa.DNS = model.PrivacyDNS{Status: "safe", MatchesExitIP: true}
		}
	}

	// IPv6：普通双栈不算泄露；明确代理/机房出口下出现 IPv6 才提示可能旁路。
	if v := req.Leaks.IPv6; v != nil && v.Data != nil {
		if v.Data.Leaked && v.Data.IPv6Address != "" {
			if exitHasProxyRisk {
				pa.IPv6 = model.PrivacyIPv6{Status: "danger", Address: v.Data.IPv6Address}
			} else {
				pa.IPv6 = model.PrivacyIPv6{Status: "safe", Address: v.Data.IPv6Address}
			}
		} else {
			pa.IPv6 = model.PrivacyIPv6{Status: "safe"}
		}
	}

	// 综合
	switch {
	case pa.WebRTC.Status == "danger" || pa.DNS.Status == "danger" || pa.IPv6.Status == "danger":
		pa.OverallStatus = "danger"
	case pa.WebRTC.Status == "warning":
		pa.OverallStatus = "warning"
	default:
		pa.OverallStatus = "safe"
	}
	return pa
}

// ---- fingerprint ----

func buildFingerprint(req *model.ScanRequest) model.FingerprintAnalysis {
	fa := model.FingerprintAnalysis{
		Canvas: model.FPCanvasAnalysis{Status: "unknown"},
		Audio:  model.FPAudioAnalysis{Status: "unknown"},
		WebGL:  model.FPWebGLAnalysis{Status: "unknown", VendorMatchsUA: true},
	}

	if c := req.Fingerprint.Canvas; c != nil && c.Data != nil {
		stable := c.Data.HashSampleA == c.Data.HashSampleB
		hooked := !isNativeCode(c.Data.NativeCode)
		fa.Canvas.IsStable = stable
		fa.Canvas.IsHooked = hooked
		switch {
		case hooked:
			fa.Canvas.Status = "blocked"
		case !stable:
			fa.Canvas.Status = "noise"
		default:
			fa.Canvas.Status = "native"
		}
	}

	if a := req.Fingerprint.Audio; a != nil && a.Data != nil {
		hooked := !isNativeCode(a.Data.NativeCode)
		fa.Audio.IsHooked = hooked
		if hooked {
			fa.Audio.Status = "blocked"
		} else {
			fa.Audio.Status = "native"
		}
	}

	// WebGL 渲染器 vs 声称的平台
	platform := ""
	if nav := req.Environment.Navigator; nav != nil && nav.Data != nil {
		platform = nav.Data.Platform
	}
	if g := req.Fingerprint.WebGL; g != nil && g.Data != nil {
		match, reason := webglMatchesPlatform(g.Data.UnmaskedRenderer, platform)
		fa.WebGL.VendorMatchsUA = match
		if match {
			fa.WebGL.Status = "match"
		} else {
			fa.WebGL.Status = "mismatch"
			fa.WebGL.MismatchReason = reason
		}
	}

	// 综合
	switch {
	case fa.WebGL.Status == "mismatch":
		fa.OverallStatus = "conflict"
	case fa.Canvas.IsHooked || fa.Audio.IsHooked || fa.Canvas.Status == "noise":
		fa.OverallStatus = "protected"
	case fa.Canvas.Status == "native":
		fa.OverallStatus = "native"
	default:
		fa.OverallStatus = "unknown"
	}
	return fa
}

// ---- automation ----

func buildAutomation(req *model.ScanRequest) model.AutomationAnalysis {
	aa := model.AutomationAnalysis{AutomationTraces: []string{}, OverallStatus: "safe"}

	nav := req.Environment.Navigator
	if nav == nil || nav.Data == nil {
		return aa
	}

	if nav.Data.Webdriver != nil && *nav.Data.Webdriver {
		aa.WebdriverDetected = true
		aa.AutomationTraces = append(aa.AutomationTraces, "navigator.webdriver=true")
	}

	mobile := false
	if ua := req.Environment.UserAgentData; ua != nil && ua.Data != nil {
		mobile = ua.Data.Mobile
	}
	if nav.Data.PluginsLength == 0 && !mobile {
		aa.AutomationTraces = append(aa.AutomationTraces, "plugins_length=0 (桌面浏览器无插件，可疑)")
	}

	// FingerprintJS hook 审计
	if req.FPJSAudit != nil && req.FPJSAudit.Hook.Status == "DANGER" {
		aa.AutomationTraces = append(aa.AutomationTraces, "检测到原生方法被 Hook: "+strings.Join(req.FPJSAudit.Hook.Methods, ","))
	}

	switch {
	case aa.WebdriverDetected:
		aa.OverallStatus = "detected"
	case len(aa.AutomationTraces) > 0:
		aa.OverallStatus = "suspicious"
	}
	return aa
}

// ---- helpers ----

func isNativeCode(code string) bool {
	return strings.Contains(code, "[native code]")
}

// webglMatchesPlatform 只对「物理不可能」的组合判 mismatch，避免假阳性
func webglMatchesPlatform(renderer, platform string) (bool, string) {
	r := strings.ToLower(renderer)
	p := strings.ToLower(platform)
	if r == "" || p == "" {
		return true, ""
	}

	isMac := strings.Contains(p, "mac")
	isIOS := strings.Contains(p, "iphone") || strings.Contains(p, "ipad")

	// Apple 平台不可能出现 Direct3D / NVIDIA 桌面卡
	if isMac || isIOS {
		if strings.Contains(r, "direct3d") || strings.Contains(r, "d3d") || strings.Contains(r, "nvidia") {
			return false, "Apple 平台不应出现 Direct3D/NVIDIA 渲染器"
		}
	}

	// iOS 不可能是桌面 GPU（ANGLE Direct3D 等）
	if isIOS && strings.Contains(r, "angle") {
		return false, "iOS 不应出现桌面 ANGLE 渲染器"
	}

	return true, ""
}
