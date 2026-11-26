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
	WebRTCPolicyVersion = 2
	maxWebRTCCandidates = 6
)

type WebRTCAssessmentRequest struct {
	PublicIPs []string `json:"public_ips"`
}

type WebRTCVerdict struct {
	PolicyVersion int      `json:"policy_version"`
	State         string   `json:"state"`
	Reason        string   `json:"reason"`
	SuspectIPs    []string `json:"suspect_ips"`
}

type WebRTCNetworkInfo struct {
	IP       string `json:"ip"`
	Country  string `json:"country,omitempty"`
	Network  string `json:"network,omitempty"`
	ASN      string `json:"asn,omitempty"`
	Type     string `json:"type,omitempty"`
	Relation string `json:"relation,omitempty"`
}

type WebRTCAssessmentResult struct {
	PolicyVersion int                 `json:"policy_version"`
	Verdict       WebRTCVerdict       `json:"verdict"`
	Exit          *WebRTCNetworkInfo  `json:"exit,omitempty"`
	Candidates    []WebRTCNetworkInfo `json:"candidates"`
}

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

func canonicalCarrier(intel *model.IPIntel) string {
	if intel == nil {
		return ""
	}
	if cat := DefaultCatalog(); cat != nil {
		return cat.CarrierGroup(intel)
	}
	return ""
}

func isHostingOrProxyIntel(intel *model.IPIntel) bool {
	return intel != nil && (intel.HasProxy() || intel.IsHosting || intel.InferUsageType() == "hosting")
}

func webrtcNetworkInfo(ip string, intel *model.IPIntel, relation string, lang string) WebRTCNetworkInfo {
	info := WebRTCNetworkInfo{
		IP:       util.CanonicalIP(ip),
		Relation: relation,
	}
	if intel == nil {
		return info
	}
	projected := ProjectIPIntel(intel, lang)
	info.Country = geoBucket(intel)
	network := strings.TrimSpace(projected.ISP)
	if network == "" || strings.EqualFold(network, "unknown") {
		network = strings.TrimSpace(projected.Org)
	}
	if cat := DefaultCatalog(); cat != nil {
		if localized := cat.LocalizeNetwork(network, intel.ASN, lang); localized != "" {
			network = localized
		} else if lang == "en" && containsCJK(network) {
			network = ""
		}
	} else if lang == "en" && containsCJK(network) {
		network = ProjectISP(network, "en")
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

	if diffGeo {
		return "leak"
	}

	if oppositeFamily {
		if exitProxyRisk {
			if sameGeo && sameASN {
				return "dual-stack"
			}
			return "leak"
		}
		if candProxyRisk {
			return "leak"
		}
		if sameGeo && (sameASN || sameCarrier) {
			return "dual-stack"
		}
		if sameGeo && (exitASN == "" || candASN == "") {
			return "dual-stack"
		}
		if exitGeo == "" || candGeo == "" {
			return "dual-stack"
		}
		return "leak"
	}

	if !exitProxyRisk && !candProxyRisk && sameGeo && sameASN {
		return "same-network"
	}
	return "leak"
}

func AssessWebRTC(ctx context.Context, ipIntel *IPIntelService, exitIP string, exitIntel *model.IPIntel, rawPublicIPs []string) *WebRTCAssessmentResult {
	return AssessWebRTCWithLang(ctx, ipIntel, exitIP, exitIntel, rawPublicIPs, "zh")
}

func AssessWebRTCWithLang(ctx context.Context, ipIntel *IPIntelService, exitIP string, exitIntel *model.IPIntel, rawPublicIPs []string, lang string) *WebRTCAssessmentResult {
	canonicalExit := util.CanonicalIP(exitIP)
	exitAddr, exitErr := netip.ParseAddr(canonicalExit)

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
			exitInfo := webrtcNetworkInfo(canonicalExit, exitIntel, "", lang)
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
	exitInfo := webrtcNetworkInfo(canonicalExit, exitIntel, "", lang)
	res.Exit = &exitInfo

	hasSamePrefix := false
	hasDualStack := false
	hasSameNetwork := false
	allSuspectsProxyOrHosting := true

	for _, candIP := range publicIPs {
		candAddr, err := netip.ParseAddr(candIP)
		if err != nil {
			continue
		}
		candAddr = candAddr.Unmap()
		if candAddr == exitAddr.Unmap() {
			res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, exitIntel, "matches-exit", lang))
			continue
		}
		if sameIPv6Prefix64Addr(candAddr, exitAddr) {
			hasSamePrefix = true
			res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, exitIntel, "same-prefix", lang))
			continue
		}

		var candIntel *model.IPIntel
		if ipIntel != nil {
			candIntel, _ = ipIntel.Lookup(lookupCtx, candIP, LookupOptions{LocalOnly: true})
		}
		relation := classifyWebRTCCandidate(exitAddr, exitIntel, candAddr, candIntel)
		res.Candidates = append(res.Candidates, webrtcNetworkInfo(candIP, candIntel, relation, lang))
		switch relation {
		case "leak":
			res.Verdict.SuspectIPs = append(res.Verdict.SuspectIPs, candIP)
			if !isHostingOrProxyIntel(candIntel) {
				allSuspectsProxyOrHosting = false
			}
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
		if exitIntel != nil && !isHostingOrProxyIntel(exitIntel) && allSuspectsProxyOrHosting {
			res.Verdict.Reason = "reverse-split"
		} else {
			res.Verdict.Reason = "public-leak"
		}
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
