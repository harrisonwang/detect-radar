package service

import (
	"context"
	"net/netip"
	"strings"
	"time"

	"detect-radar/internal/model"
	"detect-radar/internal/util"
)

const (
	// WebRTCPolicyVersion 标识基于规范化 IPv6、/64 前缀与本地归属比对的 WebRTC 判定口径。
	WebRTCPolicyVersion = 2
	maxWebRTCCandidates = 6
)

// WebRTCAssessmentRequest 为 POST /api/v1/leaks/webrtc/assessment 的请求体。
// 出口 IP 始终由服务端从真实连接 c.IP() 获取，不信任客户端上报的出口地址。
type WebRTCAssessmentRequest struct {
	PublicIPs []string `json:"public_ips"`
}

// WebRTCVerdict 描述 WebRTC 公网候选与当前出口的统一判定结论。
type WebRTCVerdict struct {
	PolicyVersion int      `json:"policy_version"`
	State         string   `json:"state"`  // clear | leaked | unknown
	Reason        string   `json:"reason"` // matches-exit | same-prefix | dual-stack | same-network | public-leak | no-public | missing-exit
	SuspectIPs    []string `json:"suspect_ips"`
}

// WebRTCNetworkInfo 为出口或单个 WebRTC 公网候选的最小归属摘要（仅来源于本地库/缓存）。
type WebRTCNetworkInfo struct {
	IP       string `json:"ip"`
	Country  string `json:"country,omitempty"`
	Network  string `json:"network,omitempty"`
	ASN      string `json:"asn,omitempty"`
	Type     string `json:"type,omitempty"`     // proxy | hosting | access
	Relation string `json:"relation,omitempty"` // matches-exit | same-prefix | dual-stack | same-network | leak
}

// WebRTCAssessmentResult 为 POST /api/v1/leaks/webrtc/assessment 的响应体。
type WebRTCAssessmentResult struct {
	PolicyVersion int                 `json:"policy_version"`
	Verdict       WebRTCVerdict       `json:"verdict"`
	Exit          *WebRTCNetworkInfo  `json:"exit,omitempty"`
	Candidates    []WebRTCNetworkInfo `json:"candidates"`
}

// sameIPOrV6Prefix64 判断两个 IP 是否相等（规范化后）或同属同一 IPv6 /64 前缀（RFC 4941 临时隐私地址）。
func sameIPOrV6Prefix64(a, b string) bool {
	pa, errA := netip.ParseAddr(strings.TrimSpace(a))
	pb, errB := netip.ParseAddr(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) && strings.TrimSpace(a) != ""
	}
	pa, pb = pa.Unmap(), pb.Unmap()
	if pa == pb {
		return true
	}
	return sameIPv6Prefix64Addr(pa, pb)
}

func sameIPv6Prefix64Addr(a, b netip.Addr) bool {
	a, b = a.Unmap(), b.Unmap()
	if !a.Is6() || !b.Is6() {
		return false
	}
	ba, bb := a.As16(), b.As16()
	for i := 0; i < 8; i++ {
		if ba[i] != bb[i] {
			return false
		}
	}
	return true
}

// normalizeASN 将 "AS4134" / "as4134" / "4134" 统一剥离为纯数字 "4134" 以便跨缓存比对。
func normalizeASN(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "AS")
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s
}

func formatDisplayASN(raw string) string {
	num := normalizeASN(raw)
	if num == "" {
		return ""
	}
	return "AS" + num
}

// geoBucket 将 IPIntel 的国家/地区归一为用于防泄露比对的地理桶：
// 显式区分中国大陆(CN)与中国香港(HK)/澳门(MO)/台湾(TW)，避免 ip2region-cn 将港澳台标为 CN 导致漏判。
func geoBucket(intel *model.IPIntel) string {
	if intel == nil {
		return ""
	}
	cc := strings.ToUpper(strings.TrimSpace(intel.Country))
	if cc == "" || cc == "LOCAL" || cc == "XX" || cc == "ZZ" {
		return ""
	}
	if cc == "CN" {
		for _, field := range []string{intel.Region, intel.CountryName, intel.City} {
			lower := strings.ToLower(field)
			switch {
			case strings.Contains(lower, "香港") || strings.Contains(lower, "hong kong"):
				return "HK"
			case strings.Contains(lower, "澳门") || strings.Contains(lower, "澳門") || strings.Contains(lower, "macau") || strings.Contains(lower, "macao"):
				return "MO"
			case strings.Contains(lower, "台湾") || strings.Contains(lower, "臺灣") || strings.Contains(lower, "taiwan"):
				return "TW"
			}
		}
		return "CN"
	}
	return cc
}

// canonicalCarrier 归一常见运营商名称，兼容国内三大运营商 IPv4 省网 ASN 与 IPv6 骨干 ASN 不同的双栈场景。
func canonicalCarrier(intel *model.IPIntel) string {
	if intel == nil {
		return ""
	}
	for _, raw := range []string{intel.ISP, intel.Org} {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		switch {
		case strings.Contains(s, "电信") || strings.Contains(s, "china telecom") || strings.Contains(s, "chinanet"):
			return "china-telecom"
		case strings.Contains(s, "移动") || strings.Contains(s, "china mobile") || strings.Contains(s, "cmnet"):
			return "china-mobile"
		case strings.Contains(s, "联通") || strings.Contains(s, "china unicom") || strings.Contains(s, "cncgroup"):
			return "china-unicom"
		case strings.Contains(s, "广电") || strings.Contains(s, "china broadnet") || strings.Contains(s, "cbn"):
			return "china-broadnet"
		case strings.Contains(s, "comcast") || strings.Contains(s, "xfinity"):
			return "comcast"
		case strings.Contains(s, "t-mobile"):
			return "t-mobile"
		case strings.Contains(s, "verizon"):
			return "verizon"
		case strings.Contains(s, "at&t") || strings.Contains(s, "att "):
			return "att"
		case strings.Contains(s, "softbank"):
			return "softbank"
		case strings.Contains(s, "kddi"):
			return "kddi"
		case strings.Contains(s, "ntt"):
			return "ntt"
		}
	}
	return ""
}

func isHostingOrProxyIntel(intel *model.IPIntel) bool {
	return intel != nil && (intel.HasProxy() || intel.IsHosting || intel.InferUsageType() == "hosting")
}

func webrtcNetworkInfo(ip string, intel *model.IPIntel, relation string) WebRTCNetworkInfo {
	info := WebRTCNetworkInfo{
		IP:       util.CanonicalIP(ip),
		Relation: relation,
	}
	if intel == nil {
		return info
	}
	info.Country = geoBucket(intel)
	network := strings.TrimSpace(intel.ISP)
	if network == "" || strings.EqualFold(network, "unknown") {
		network = strings.TrimSpace(intel.Org)
	}
	info.Network = network
	info.ASN = formatDisplayASN(intel.ASN)
	switch {
	case intel.HasProxy():
		info.Type = "proxy"
	case intel.IsHosting || intel.InferUsageType() == "hosting":
		info.Type = "hosting"
	case intel.IsMobile || intel.UsageTypeRaw == "mobile" || intel.UsageTypeRaw == "residential" || intel.InferUsageType() == "isp":
		info.Type = "access"
	}
	return info
}

// classifyWebRTCCandidate 判定单个公网 WebRTC 候选地址相对于当前出口的关系：
//   - matches-exit: 同 IP（规范化后）
//   - same-prefix:  同 IPv6 /64 前缀（RFC 4941 临时隐私地址）
//   - dual-stack:   同归属网络下的正常 IPv4/IPv6 双栈
//   - same-network: 非机房/非代理下的同国同 ASN 运营商 CGNAT 多出口池
//   - leak:         跨地区、跨网络、或代理/机房出口下旁路暴露的公网 IP
func classifyWebRTCCandidate(exitAddr netip.Addr, exitIntel *model.IPIntel, candAddr netip.Addr, candIntel *model.IPIntel) string {
	exitAddr = exitAddr.Unmap()
	candAddr = candAddr.Unmap()

	if candAddr == exitAddr {
		return "matches-exit"
	}
	if sameIPv6Prefix64Addr(candAddr, exitAddr) {
		return "same-prefix"
	}

	oppositeFamily := candAddr.Is6() != exitAddr.Is6()
	exitGeo := geoBucket(exitIntel)
	candGeo := geoBucket(candIntel)
	sameGeo := exitGeo != "" && candGeo != "" && exitGeo == candGeo
	diffGeo := exitGeo != "" && candGeo != "" && exitGeo != candGeo

	var exitASN, candASN string
	if exitIntel != nil {
		exitASN = normalizeASN(exitIntel.ASN)
	}
	if candIntel != nil {
		candASN = normalizeASN(candIntel.ASN)
	}
	sameASN := exitASN != "" && candASN != "" && exitASN == candASN
	exitCarrier := canonicalCarrier(exitIntel)
	candCarrier := canonicalCarrier(candIntel)
	sameCarrier := exitCarrier != "" && candCarrier != "" && exitCarrier == candCarrier

	exitProxyRisk := isHostingOrProxyIntel(exitIntel)
	candProxyRisk := isHostingOrProxyIntel(candIntel)

	// 跨国家/跨地区（含大陆 vs 港澳台）一律判泄露，无论同族还是双栈。
	if diffGeo {
		return "leak"
	}

	if oppositeFamily {
		// 机房/代理出口下，仅允许同国同 ASN 的真双栈代理；否则判泄露（检出纯 IPv4 代理漏出原生 IPv6）。
		if exitProxyRisk {
			if sameGeo && sameASN {
				return "dual-stack"
			}
			return "leak"
		}
		// 直连普通网络下，对侧协议栈却落在机房/代理网段，属非对称代理分流或泄露。
		if candProxyRisk {
			return "leak"
		}
		if sameGeo && (sameASN || sameCarrier) {
			return "dual-stack"
		}
		// 本地库缺失某侧 ASN 或地理信息时，对非代理直连双栈不武断误报。
		if sameGeo && (exitASN == "" || candASN == "") {
			return "dual-stack"
		}
		if exitGeo == "" || candGeo == "" {
			return "dual-stack"
		}
		return "leak"
	}

	// 同协议族不同公网 IP（IPv4 vs IPv4，或不同 /64 的 IPv6）：
	// 仅在双方均非机房/代理且同国同 ASN 时归为运营商 CGNAT 多出口池，其余一律判泄露。
	if !exitProxyRisk && !candProxyRisk && sameGeo && sameASN {
		return "same-network"
	}
	return "leak"
}

// AssessWebRTC 以纯本地 IPIntel（LocalOnly，不消耗 L3 额度）评估 WebRTC 公网候选相对当前出口的泄露状态。
// 首页 ScanService 与专项页 POST /api/v1/leaks/webrtc/assessment 共用本入口，保持唯一口径。
func AssessWebRTC(ctx context.Context, ipIntel *IPIntelService, exitIP string, exitIntel *model.IPIntel, rawPublicIPs []string) *WebRTCAssessmentResult {
	canonicalExit := util.CanonicalIP(exitIP)
	exitAddr, exitErr := netip.ParseAddr(canonicalExit)

	// 去重、规范化并过滤仅保留公网单播 IP（上限 maxWebRTCCandidates）
	seen := make(map[string]struct{}, len(rawPublicIPs))
	publicIPs := make([]string, 0, len(rawPublicIPs))
	for _, raw := range rawPublicIPs {
		if !util.IsGlobalPublicIP(raw) {
			continue
		}
		canon := util.CanonicalIP(raw)
		if _, dup := seen[canon]; dup {
			continue
		}
		seen[canon] = struct{}{}
		publicIPs = append(publicIPs, canon)
		if len(publicIPs) >= maxWebRTCCandidates {
			break
		}
	}

	res := &WebRTCAssessmentResult{
		PolicyVersion: WebRTCPolicyVersion,
		Verdict: WebRTCVerdict{
			PolicyVersion: WebRTCPolicyVersion,
			SuspectIPs:    []string{},
		},
		Candidates: make([]WebRTCNetworkInfo, 0, len(publicIPs)),
	}

	if len(publicIPs) == 0 {
		if exitErr == nil && util.IsGlobalPublicIP(canonicalExit) {
			if exitIntel == nil && ipIntel != nil {
				lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				exitIntel, _ = ipIntel.Lookup(lookupCtx, canonicalExit, LookupOptions{LocalOnly: true})
				cancel()
			}
			exitInfo := webrtcNetworkInfo(canonicalExit, exitIntel, "")
			res.Exit = &exitInfo
		}
		res.Verdict.State = "clear"
		res.Verdict.Reason = "no-public"
		return res
	}

	if exitErr != nil || !util.IsGlobalPublicIP(canonicalExit) {
		for _, ip := range publicIPs {
			res.Candidates = append(res.Candidates, WebRTCNetworkInfo{IP: ip})
		}
		res.Verdict.State = "unknown"
		res.Verdict.Reason = "missing-exit"
		return res
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if exitIntel == nil && ipIntel != nil {
		exitIntel, _ = ipIntel.Lookup(lookupCtx, canonicalExit, LookupOptions{LocalOnly: true})
	}
	exitInfo := webrtcNetworkInfo(canonicalExit, exitIntel, "")
	res.Exit = &exitInfo

	hasSamePrefix := false
	hasDualStack := false
	hasSameNetwork := false

	for _, candIP := range publicIPs {
		candAddr, err := netip.ParseAddr(candIP)
		if err != nil {
			continue
		}
		candAddr = candAddr.Unmap()
		if candAddr == exitAddr.Unmap() {
			res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, exitIntel, "matches-exit"))
			continue
		}
		if sameIPv6Prefix64Addr(candAddr, exitAddr) {
			hasSamePrefix = true
			res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, exitIntel, "same-prefix"))
			continue
		}

		var candIntel *model.IPIntel
		if ipIntel != nil {
			candIntel, _ = ipIntel.Lookup(lookupCtx, candIP, LookupOptions{LocalOnly: true})
		}
		relation := classifyWebRTCCandidate(exitAddr, exitIntel, candAddr, candIntel)
		res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, candIntel, relation))
		switch relation {
		case "leak":
			res.Verdict.SuspectIPs = append(res.Verdict.SuspectIPs, candIP)
		case "same-network":
			hasSameNetwork = true
		case "dual-stack":
			hasDualStack = true
		case "same-prefix":
			hasSamePrefix = true
		}
	}

	switch {
	case len(res.Verdict.SuspectIPs) > 0:
		res.Verdict.State = "leaked"
		res.Verdict.Reason = "public-leak"
	case hasSameNetwork:
		res.Verdict.State = "clear"
		res.Verdict.Reason = "same-network"
	case hasDualStack:
		res.Verdict.State = "clear"
		res.Verdict.Reason = "dual-stack"
	case hasSamePrefix:
		res.Verdict.State = "clear"
		res.Verdict.Reason = "same-prefix"
	default:
		res.Verdict.State = "clear"
		res.Verdict.Reason = "matches-exit"
	}

	return res
}
