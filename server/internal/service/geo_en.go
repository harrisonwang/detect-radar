package service

import (
	"fmt"
	"strings"
	"unicode"

	"detect-radar/internal/model"
)

func trimCNAdminSuffix(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{"特别行政区", "壮族自治区", "回族自治区", "维吾尔自治区", "自治区", "地区", "省", "市", "盟", "县"} {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}

func translateCNProvinceEN(province string) string {
	if cat := DefaultCatalog(); cat != nil {
		return cat.TranslateProvinceEN(province)
	}
	if !containsCJK(province) {
		return strings.TrimSpace(province)
	}
	return ""
}

func translateCNCityEN(city string) string {
	if cat := DefaultCatalog(); cat != nil {
		return cat.TranslateCityEN(city)
	}
	if !containsCJK(city) {
		return strings.TrimSpace(city)
	}
	return ""
}

func translateCNISPEN(isp string) string {
	if cat := DefaultCatalog(); cat != nil {
		return cat.TranslateISPEN(isp)
	}
	if !containsCJK(isp) {
		return strings.TrimSpace(isp)
	}
	return ""
}

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func ProjectIPIntel(intel *model.IPIntel, lang string) *model.IPIntel {
	if intel == nil {
		return nil
	}
	cp := cloneIntel(intel)

	if lang == "en" {
		if cp.GeoEN != nil {
			if cp.GeoEN.CountryName != "" {
				cp.CountryName = cp.GeoEN.CountryName
			}
			if cp.GeoEN.Region != "" {
				cp.Region = cp.GeoEN.Region
			}
			if cp.GeoEN.City != "" {
				cp.City = cp.GeoEN.City
			}
			if cp.GeoEN.ISP != "" {
				cp.ISP = cp.GeoEN.ISP
			}
		}
		if containsCJK(cp.CountryName) {
			switch strings.ToUpper(cp.Country) {
			case "CN":
				cp.CountryName = "China"
			case "HK":
				cp.CountryName = "Hong Kong"
			case "MO":
				cp.CountryName = "Macao"
			case "TW":
				cp.CountryName = "Taiwan"
			default:
				cp.CountryName = cp.Country
			}
		}
		if containsCJK(cp.Region) {
			cp.Region = translateCNProvinceEN(cp.Region)
		}
		if containsCJK(cp.City) {
			cp.City = translateCNCityEN(cp.City)
		}
		if cat := DefaultCatalog(); cat != nil {
			cp.ISP = cat.NormalizeIntelISP(cp.ISP, cp.Org, cp.ASN, "en")
		} else if containsCJK(cp.ISP) {
			if !containsCJK(cp.Org) && cp.Org != "" {
				cp.ISP = cp.Org
			} else {
				cp.ISP = ""
			}
		}
	} else {
		if cp.GeoZH != nil {
			if cp.GeoZH.CountryName != "" {
				cp.CountryName = cp.GeoZH.CountryName
			}
			if cp.GeoZH.Region != "" {
				cp.Region = cp.GeoZH.Region
			}
			if cp.GeoZH.City != "" {
				cp.City = cp.GeoZH.City
			}
			if cp.GeoZH.ISP != "" {
				cp.ISP = cp.GeoZH.ISP
			}
		}
		if cat := DefaultCatalog(); cat != nil {
			cp.ISP = cat.NormalizeIntelISP(cp.ISP, cp.Org, cp.ASN, "zh")
		}
	}

	cp.GeoZH = nil
	cp.GeoEN = nil
	return cp
}

func ProjectISP(isp, lang string) string {
	s := strings.TrimSpace(isp)
	if s == "" || lang != "en" {
		return s
	}
	if mapped := translateCNISPEN(s); mapped != "" {
		return mapped
	}
	if containsCJK(s) {
		return ""
	}
	return s
}

func ProjectScanResponse(resp *model.ScanResponse, lang string) *model.ScanResponse {
	if resp == nil {
		return nil
	}
	cp := *resp
	cp.ServerData.IPIntel = ProjectIPIntel(resp.ServerData.IPIntel, lang)
	if lang != "en" {
		if cat := DefaultCatalog(); cat != nil {
			if cp.Analysis.Privacy.DNS.ISP != "" {
				if localized := cat.LocalizeNetwork(cp.Analysis.Privacy.DNS.ISP, "", "zh"); localized != "" {
					cp.Analysis.Privacy.DNS.ISP = localized
				}
			}
			if cp.Analysis.Privacy.DNS.Assessment != nil && len(cp.Analysis.Privacy.DNS.Assessment.Networks) > 0 {
				assessmentCopy := *cp.Analysis.Privacy.DNS.Assessment
				nets := make([]string, len(assessmentCopy.Networks))
				for i, name := range assessmentCopy.Networks {
					if localized := cat.LocalizeNetwork(name, "", "zh"); localized != "" {
						nets[i] = localized
					} else {
						nets[i] = name
					}
				}
				assessmentCopy.Networks = nets
				cp.Analysis.Privacy.DNS.Assessment = &assessmentCopy
			}
		}
		return &cp
	}

	if len(resp.Dimensions) > 0 {
		dims := make([]model.DimensionScore, len(resp.Dimensions))
		copy(dims, resp.Dimensions)
		for i := range dims {
			switch dims[i].Key {
			case dimIdentity:
				dims[i].Label = "Identity"
			case dimConsistency:
				dims[i].Label = "Consistency"
			case dimLeak:
				dims[i].Label = "DNS/IP Leaks"
			case dimEnvironment:
				dims[i].Label = "Fingerprint"
			}
		}
		cp.Dimensions = dims
	}

	if cp.Analysis.Fingerprint.WebGL.MismatchCode != "" {
		switch cp.Analysis.Fingerprint.WebGL.MismatchCode {
		case "apple_d3d_nvidia":
			cp.Analysis.Fingerprint.WebGL.MismatchReason = "Apple platforms should not report a Direct3D or NVIDIA renderer"
		case "ios_angle":
			cp.Analysis.Fingerprint.WebGL.MismatchReason = "iOS should not report a desktop ANGLE renderer"
		}
	}

	if len(resp.Analysis.Automation.AutomationTraces) > 0 {
		traces := make([]string, 0, len(resp.Analysis.Automation.AutomationTraces))
		for _, t := range resp.Analysis.Automation.AutomationTraces {
			switch {
			case strings.HasPrefix(t, "plugins_length=0"):
				traces = append(traces, "plugins_length=0 (desktop browser reports zero plugins)")
			case strings.HasPrefix(t, "检测到原生方法被 Hook: "):
				traces = append(traces, "Native methods appear overridden: "+strings.TrimPrefix(t, "检测到原生方法被 Hook: "))
			default:
				traces = append(traces, t)
			}
		}
		cp.Analysis.Automation.AutomationTraces = traces
	}

	if cp.Analysis.Privacy.DNS.ISP != "" {
		if mapped := ProjectISP(cp.Analysis.Privacy.DNS.ISP, "en"); mapped != "" {
			cp.Analysis.Privacy.DNS.ISP = mapped
		} else if containsCJK(cp.Analysis.Privacy.DNS.ISP) {
			cp.Analysis.Privacy.DNS.ISP = ""
		}
	}
	if cp.Analysis.Privacy.DNS.Assessment != nil {
		assessmentCopy := *cp.Analysis.Privacy.DNS.Assessment
		if len(assessmentCopy.Networks) > 0 {
			nets := make([]string, 0, len(assessmentCopy.Networks))
			for _, name := range assessmentCopy.Networks {
				if mapped := ProjectISP(name, "en"); mapped != "" {
					nets = append(nets, mapped)
				} else if !containsCJK(name) {
					nets = append(nets, name)
				}
			}
			assessmentCopy.Networks = nets
		}
		cp.Analysis.Privacy.DNS.Assessment = &assessmentCopy
	}

	if len(resp.Diagnosis) > 0 {
		diag := make([]model.DiagnosisItem, len(resp.Diagnosis))
		copy(diag, resp.Diagnosis)
		a := cp.Analysis
		intel := cp.ServerData.IPIntel
		for i := range diag {
			diag[i].Text = localizeDiagnosisEN(diag[i], a, intel)
		}
		cp.Diagnosis = diag
	}

	return &cp
}

func localizeDiagnosisEN(item model.DiagnosisItem, a model.ScanAnalysis, intel *model.IPIntel) string {
	switch item.Code {
	case "RESIDENTIAL_PASS":
		return fmt.Sprintf("Exit IP classified as residential/mobile network (confidence %d)", a.Identity.IPTypeConfidence)
	case "IP_DATACENTER":
		return "Exit IP belongs to a datacenter or hosting provider"
	case "IP_GRAY":
		return "Exit IP type is unclassified; residential status cannot be confirmed with high confidence"
	case "IP_PROXY":
		return "Exit IP is flagged as a proxy, VPN, or Tor node"
	case "IP_FRAUD":
		if intel != nil {
			return fmt.Sprintf("Elevated IP fraud score (%d/100)", intel.FraudScore)
		}
		return "Elevated IP fraud score"
	case "IP_BLACKLIST":
		return "Exit IP listed on public blocklists: " + strings.Join(a.Identity.BlacklistHits, ", ")
	case "IP_BLACKLIST_CGNAT":
		return "Mobile/CGNAT exit IP listed on public blocklists (likely shared pool): " + strings.Join(a.Identity.BlacklistHits, ", ")
	case "IP_TOR_EXIT":
		return "Exit IP is a known Tor exit node"
	case "IP_OPEN_PROXY_PORT":
		return "Exit IP exposes a known open proxy port"
	case "IP_PROXY_PORT_WEAK":
		return "Datacenter exit IP exposes a common HTTP proxy port"
	case "WEBRTC_LEAK":
		if a.Privacy.WebRTC.Reason == "reverse-split" {
			return "WebRTC exposed a proxy or datacenter public IP while this page used a direct ISP exit (reverse split-tunnel)"
		}
		return "WebRTC exposed a public IP that differs from your exit IP"
	case "DNS_LEAK":
		return "DNS leak detected"
	case "IPV6_LEAK":
		return "IPv6 leak detected"
	case "TZ_MISMATCH":
		return fmt.Sprintf("Timezone mismatch: browser %s vs. IP %s", a.Consistency.Timezone.Browser, a.Consistency.Timezone.Expected)
	case "LANG_MISMATCH":
		return fmt.Sprintf("Language mismatch: browser %s vs. IP country %s", a.Consistency.Language.Browser, a.Consistency.Language.IPCountry)
	case "WEBGL_MISMATCH":
		if a.Fingerprint.WebGL.MismatchReason != "" {
			return "WebGL renderer does not match declared OS: " + a.Fingerprint.WebGL.MismatchReason
		}
		return "WebGL renderer does not match declared OS"
	case "FP_HOOKED":
		return "Native fingerprint APIs appear overridden"
	case "AUTOMATION_DETECTED":
		return "Browser automation flag detected (navigator.webdriver)"
	default:
		return item.Text
	}
}
