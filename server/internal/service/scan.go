package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"detect-radar/internal/model"
	"github.com/google/uuid"
)

// ErrAmbiguousScan 表示客户端扫描标识对应多个历史实例。
var ErrAmbiguousScan = errors.New("扫描标识对应多个扫描实例")

// ScanService 汇总一次扫描的服务端分析：IP 信息、一致性、泄露复核、指纹和自动化
type ScanService struct {
	ipIntel     *IPIntelService
	consistency *ConsistencyService
	reputation  *ReputationService
	rttCoords   rttServerCoords // 服务器坐标，用于 RTT 地理-延迟下限校验（Phase 1）
	mu          sync.RWMutex
	store       map[string]storedScan // 扫描实例标识 -> 不可变的响应快照
	latest      map[string]string     // 客户端扫描标识 -> 最新实例
	journal     *Journal
	ttl         time.Duration
	stop        chan struct{}
	done        chan struct{}
	closeOnce   sync.Once
}

type storedScan struct {
	resp      *model.ScanResponse
	createdAt time.Time
}

func NewScanService(ipIntel *IPIntelService, consistency *ConsistencyService, reputation *ReputationService, rttLat, rttLon float64, journal *Journal) *ScanService {
	s := &ScanService{
		ipIntel:     ipIntel,
		consistency: consistency,
		reputation:  reputation,
		rttCoords:   rttServerCoords{Lat: rttLat, Lon: rttLon},
		ttl:         30 * time.Minute,
		journal:     journal, store: make(map[string]storedScan), latest: make(map[string]string),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

// Analyze 执行一次完整扫描分析。
// tcpRTT/tcpRTTVar 是 nginx 透传的 X-TCP-RTT / X-TCP-RTTVAR 头（µs 整数字符串，
// 描述「出口 IP↔服务器」这一腿；本地开发无 nginx 时为空，优雅降级）。
func (s *ScanService) Analyze(ctx context.Context, req *model.ScanRequest, clientIP, httpVersion, tcpRTT, tcpRTTVar, ua string) (*model.ScanResponse, error) {
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

	if _, err := s.persistScan(resp, ua); err != nil {
		return nil, err
	}
	return resp, nil
}

// persistScan 在事件和汇总共同提交后，才将结果放入内存。
func (s *ScanService) persistScan(resp *model.ScanResponse, ua string) (string, error) {
	instanceID := uuid.NewString()
	if err := s.journal.ScanEvent(instanceID, resp, ua); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[instanceID] = storedScan{resp: resp, createdAt: time.Now()}
	if resp.ScanID != "" {
		s.latest[resp.ScanID] = instanceID
	}
	return instanceID, nil
}

// Get 仅查询短期内存缓存；SQLite 用于保存内部遥测记录。
func (s *ScanService) Get(scanID string) (*model.ScanResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.store[s.latest[scanID]]
	if !ok || time.Since(stored.createdAt) >= s.ttl {
		return nil, false
	}
	return stored.resp, true
}

// InstanceID 仅关联数据库中的唯一实例，避免内存清理后误判重复标识。
func (s *ScanService) InstanceID(scanID string) (string, error) {
	if scanID == "" {
		return "", nil
	}
	if s.journal == nil || s.journal.store == nil {
		return "", ErrStorageUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, binding, err := s.journal.store.ResolveInstance(ctx, scanID)
	if err != nil {
		return "", fmt.Errorf("%w：查询扫描实例失败：%v", ErrStorageUnavailable, err)
	}
	if binding == "ambiguous" {
		return "", ErrAmbiguousScan
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.store[id]
	if !ok || time.Since(stored.createdAt) >= s.ttl {
		return "", nil
	}
	return id, nil
}

// UpdateDNSLeak 将客户端提交的 DNS 更新持久化后，再更新内存。
func (s *ScanService) UpdateDNSLeak(scanID string, leaked bool) (*model.ScanResponse, bool, error) {
	id, err := s.InstanceID(scanID)
	if err != nil {
		return nil, false, err
	}
	if id == "" {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.store[id]
	if !ok || time.Since(stored.createdAt) >= s.ttl {
		return nil, false, nil
	}
	expected := "safe"
	if leaked {
		expected = "danger"
	}
	if stored.resp.Analysis.Privacy.DNS.Status == expected {
		return stored.resp, true, nil
	}
	updated := rescoreDNS(stored.resp, leaked)
	if err := s.journal.DNSUpdateEvent(id, updated, leaked); err != nil {
		return nil, true, err
	}
	stored.resp = updated
	s.store[id] = stored
	return updated, true, nil
}

// UpdateDNSObservation 使用创建 DNS 测试时绑定的扫描实例。
func (s *ScanService) UpdateDNSObservation(instanceID, clientID string, result *model.DNSLeakResult) (*model.ScanResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.store[instanceID]
	var updated *model.ScanResponse
	if ok && time.Since(stored.createdAt) < s.ttl {
		updated = rescoreDNS(stored.resp, result.Leaked)
	}
	if err := s.journal.DNSLeakEvent(instanceID, clientID, result, updated); err != nil {
		return nil, err
	}
	if updated != nil {
		stored.resp = updated
		s.store[instanceID] = stored
	}
	return updated, nil
}

// 复制响应后再修改，避免影响并发读取或发布未提交的结果。
func rescoreDNS(previous *model.ScanResponse, leaked bool) *model.ScanResponse {
	resp := *previous
	if leaked {
		resp.Analysis.Privacy.DNS = model.PrivacyDNS{Status: "danger", MatchesExitIP: false}
	} else {
		resp.Analysis.Privacy.DNS = model.PrivacyDNS{Status: "safe", MatchesExitIP: true}
	}
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
	return &resp
}

func (s *ScanService) cleanup() {
	defer close(s.done)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, scan := range s.store {
				if now.Sub(scan.createdAt) >= s.ttl {
					delete(s.store, id)
					if s.latest[scan.resp.ScanID] == id {
						delete(s.latest, scan.resp.ScanID)
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *ScanService) Close() error {
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

// ---- 网络身份 ----

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

// ---- 一致性检测输入 ----

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

// ---- 隐私泄露 ----

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

// ---- 浏览器指纹 ----

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

// ---- 自动化检测 ----

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

// ---- 辅助函数 ----

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
