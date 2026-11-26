package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"detect-radar/internal/model"
	"detect-radar/internal/util"
	"github.com/google/uuid"
)

var ErrAmbiguousScan = errors.New("扫描标识对应多个扫描实例")

type ScanService struct {
	ipIntel     *IPIntelService
	consistency *ConsistencyService
	reputation  *ReputationService
	rttCoords   rttServerCoords
	rttProbes   *RTTProbeStore
	mu          sync.RWMutex
	store       map[string]storedScan
	latest      map[string]string
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

func (s *ScanService) SetRTTProbeStore(probes *RTTProbeStore) {
	s.rttProbes = probes
}

func (s *ScanService) Analyze(ctx context.Context, req *model.ScanRequest, clientIP, httpVersion, tcpRTT, tcpRTTVar, ua string) (*model.ScanResponse, error) {
	start := time.Now()
	clientIP = util.CanonicalIP(clientIP)

	intel, err := s.ipIntel.Lookup(ctx, clientIP, LookupOptions{DeepScan: true})
	if err != nil || intel == nil {
		intel = &model.IPIntel{IP: clientIP}
	}

	scenario := NormalizeScenario(req.Scenario)
	resp := &model.ScanResponse{
		ScanID:   req.ScanID,
		Status:   "success",
		Scenario: scenario,
		ServerData: model.ScanServerData{
			ClientIP:    clientIP,
			IPIntel:     intel,
			HTTPVersion: httpVersion,
		},
	}

	resp.Analysis.Identity = buildIdentity(intel)

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
	s.refineWebRTCPrivacy(ctx, &resp.Analysis.Privacy, req.Leaks.WebRTC, clientIP, intel)
	s.refineIPv6Privacy(ctx, &resp.Analysis.Privacy, req.Leaks.IPv6, intel)
	resp.Analysis.Fingerprint = buildFingerprint(req)
	resp.Analysis.Automation = buildAutomation(req)

	clientRTT := clientRTTData(req)
	var serverTCPMinMS float64
	if s.rttProbes != nil && clientRTT != nil && clientRTT.ProbeID != "" {
		if minMS, ok := s.rttProbes.Consume(clientRTT.ProbeID); ok {
			serverTCPMinMS = minMS
		}
	}
	resp.Analysis.RTT = deriveRTTWithServerMin(clientRTT, tcpRTT, tcpRTTVar, serverTCPMinMS, intel, s.rttCoords)

	resp.Score, resp.RiskLevel, resp.Diagnosis, resp.Dimensions = scoreForScenario(resp.Analysis, intel, scenario)

	resp.ProcessedAt = time.Now().UTC().Format(time.RFC3339)
	resp.ProcessingTimeMs = time.Since(start).Milliseconds()

	if _, err := s.persistScan(resp, ua); err != nil {
		return nil, err
	}
	return resp, nil
}

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

func (s *ScanService) Get(scanID string) (*model.ScanResponse, bool) {
	return s.GetWithScenario(scanID, "")
}

func (s *ScanService) GetWithScenario(scanID, rawScenario string) (*model.ScanResponse, bool) {
	s.mu.RLock()
	stored, ok := s.store[s.latest[scanID]]
	if !ok || time.Since(stored.createdAt) >= s.ttl {
		s.mu.RUnlock()
		return nil, false
	}
	cp := *stored.resp
	s.mu.RUnlock()

	if strings.TrimSpace(rawScenario) == "" {
		return &cp, true
	}
	target := NormalizeScenario(rawScenario)
	if NormalizeScenario(cp.Scenario) == target && cp.Scenario == target {
		return &cp, true
	}
	cp.Scenario = target
	cp.Score, cp.RiskLevel, cp.Diagnosis, cp.Dimensions = scoreForScenario(cp.Analysis, cp.ServerData.IPIntel, target)
	return &cp, true
}

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

func (s *ScanService) UpdateDNSObservation(instanceID, clientID string, result *model.DNSLeakResult) (*model.ScanResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.store[instanceID]
	var updated *model.ScanResponse
	if ok && time.Since(stored.createdAt) < s.ttl && result.Assessment != nil {
		status := "unknown"
		switch result.Assessment.State {
		case "suspected":
			status = "danger"
		case "clear":
			status = "safe"
		case "unobserved":
			status = "blocked"
		}
		dns := model.PrivacyDNS{Status: status, Assessment: result.Assessment}
		if len(result.Assessment.SuspectIPs) > 0 {
			for _, resolver := range result.DNSServers {
				if resolver.IP == result.Assessment.SuspectIPs[0] {
					dns.Country, dns.ISP = resolver.Country, resolver.ISP
					break
				}
			}
		}
		updated = rescoreDNSStatus(stored.resp, dns)
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

func rescoreDNSStatus(previous *model.ScanResponse, dns model.PrivacyDNS) *model.ScanResponse {
	resp := *previous
	resp.Scenario = NormalizeScenario(previous.Scenario)
	resp.Analysis.Privacy.DNS = dns
	resp.Analysis.Privacy.OverallStatus = privacyOverallStatus(resp.Analysis.Privacy)
	resp.Score, resp.RiskLevel, resp.Diagnosis, resp.Dimensions = scoreForScenario(resp.Analysis, resp.ServerData.IPIntel, resp.Scenario)
	return &resp
}

func privacyOverallStatus(pv model.PrivacyAnalysis) string {
	switch {
	case pv.WebRTC.Status == "danger" || pv.DNS.Status == "danger" || pv.IPv6.Status == "danger":
		return "danger"
	case pv.WebRTC.Status == "warning":
		return "warning"
	default:
		return "safe"
	}
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

func buildPrivacy(req *model.ScanRequest, exitIP string, intel *model.IPIntel) model.PrivacyAnalysis {
	pa := model.PrivacyAnalysis{
		WebRTC: model.PrivacyWebRTC{Status: "blocked", LeakedIPs: []string{}, MatchesExitIP: true},
		DNS:    model.PrivacyDNS{Status: "blocked", MatchesExitIP: true},
		IPv6:   model.PrivacyIPv6{Status: "disabled"},
	}

	exitHasProxyRisk := intel != nil && (intel.HasProxy() || intel.IsHosting || intel.InferUsageType() == "hosting")

	if w := req.Leaks.WebRTC; w != nil && w.Data != nil {
		canonicalExit := util.CanonicalIP(exitIP)
		var extra []string
		for _, rawIP := range w.Data.PublicIPs {
			ip := util.CanonicalIP(rawIP)
			if ip == "" || sameIPOrV6Prefix64(ip, canonicalExit) {
				continue
			}
			extra = append(extra, ip)
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

	if d := req.Leaks.DNS; d != nil && d.Data != nil {
		if d.Data.Leaked {
			pa.DNS = model.PrivacyDNS{Status: "danger", MatchesExitIP: false}
		} else if len(d.Data.Resolvers) > 0 {
			pa.DNS = model.PrivacyDNS{Status: "safe", MatchesExitIP: true}
		}
	}

	if v := req.Leaks.IPv6; v != nil && v.Data != nil {
		if v.Data.Leaked && v.Data.IPv6Address != "" {
			if exitHasProxyRisk {
				pa.IPv6 = model.PrivacyIPv6{Status: "danger", Address: v.Data.IPv6Address}
			} else {
				pa.IPv6 = model.PrivacyIPv6{Status: "safe", Address: v.Data.IPv6Address}
			}
		} else if v.Data.IPv6Address != "" {
			pa.IPv6 = model.PrivacyIPv6{Status: "safe", Address: v.Data.IPv6Address}
		} else {
			pa.IPv6 = model.PrivacyIPv6{Status: "safe"}
		}
	}

	pa.OverallStatus = privacyOverallStatus(pa)
	return pa
}

func isMainlandCN(intel *model.IPIntel) bool {
	if intel == nil || !strings.EqualFold(strings.TrimSpace(intel.Country), "CN") {
		return false
	}
	for _, field := range []string{intel.Region, intel.CountryName, intel.City} {
		lower := strings.ToLower(field)
		for _, token := range []string{"香港", "澳门", "澳門", "台湾", "臺灣", "hong kong", "macau", "macao", "taiwan"} {
			if strings.Contains(lower, token) {
				return false
			}
		}
	}
	return true
}

func (s *ScanService) refineWebRTCPrivacy(
	ctx context.Context,
	pa *model.PrivacyAnalysis,
	wReq *model.CollectorResult[model.WebRTCLeakData],
	exitIP string,
	exitIntel *model.IPIntel,
) {
	if s == nil || s.ipIntel == nil || wReq == nil || wReq.Data == nil || len(wReq.Data.PublicIPs) == 0 {
		return
	}
	res := AssessWebRTC(ctx, s.ipIntel, exitIP, exitIntel, wReq.Data.PublicIPs)
	switch res.Verdict.State {
	case "clear":
		pa.WebRTC = model.PrivacyWebRTC{Status: "safe", LeakedIPs: []string{}, MatchesExitIP: true}
	case "leaked":
		pa.WebRTC = model.PrivacyWebRTC{Status: "danger", LeakedIPs: res.Verdict.SuspectIPs, MatchesExitIP: false}
	default:
		return
	}
	pa.OverallStatus = privacyOverallStatus(*pa)
}

func (s *ScanService) refineIPv6Privacy(
	ctx context.Context,
	pa *model.PrivacyAnalysis,
	v6Req *model.CollectorResult[model.IPv6LeakData],
	exitIntel *model.IPIntel,
) {
	if s == nil || s.ipIntel == nil || v6Req == nil || v6Req.Data == nil {
		return
	}
	addrStr := strings.TrimSpace(v6Req.Data.IPv6Address)
	if addrStr == "" {
		return
	}
	addr, err := netip.ParseAddr(addrStr)
	if err != nil || !addr.Is6() || addr.Is4In6() || !addr.IsGlobalUnicast() {
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	v6Intel, err := s.ipIntel.Lookup(lookupCtx, addr.String(), LookupOptions{LocalOnly: true})
	if err != nil || v6Intel == nil || v6Intel.Country == "" || v6Intel.Country == "LOCAL" {
		return
	}

	exitOverseas := exitIntel != nil && exitIntel.Country != "" && exitIntel.Country != "LOCAL" && !isMainlandCN(exitIntel)
	v6MainlandAccess := isMainlandCN(v6Intel) && !v6Intel.IsHosting && v6Intel.InferUsageType() != "hosting"
	if exitOverseas && v6MainlandAccess {
		pa.IPv6 = model.PrivacyIPv6{Status: "danger", Address: addr.String()}
	} else {
		pa.IPv6 = model.PrivacyIPv6{Status: "safe", Address: addr.String()}
	}
	pa.OverallStatus = privacyOverallStatus(*pa)
}

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

func isNativeCode(code string) bool {
	return strings.Contains(code, "[native code]")
}

func webglMatchesPlatform(renderer, platform string) (bool, string) {
	r := strings.ToLower(renderer)
	p := strings.ToLower(platform)
	if r == "" || p == "" {
		return true, ""
	}

	isMac := strings.Contains(p, "mac")
	isIOS := strings.Contains(p, "iphone") || strings.Contains(p, "ipad")

	if isMac || isIOS {
		if strings.Contains(r, "direct3d") || strings.Contains(r, "d3d") || strings.Contains(r, "nvidia") {
			return false, "Apple 平台不应出现 Direct3D/NVIDIA 渲染器"
		}
	}

	if isIOS && strings.Contains(r, "angle") {
		return false, "iOS 不应出现桌面 ANGLE 渲染器"
	}

	return true, ""
}
