package service

import (
	"fmt"
	"strings"

	"detect-radar/internal/model"
)

const (
	dimIdentity    = "identity"
	dimConsistency = "consistency"
	dimLeak        = "leak"
	dimEnvironment = "environment"

	ScenarioStrict = "strict"
	ScenarioDaily  = "daily"
)

func NormalizeScenario(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), ScenarioDaily) {
		return ScenarioDaily
	}
	return ScenarioStrict
}

var dimCap = map[string]int{
	dimIdentity:    35,
	dimLeak:        35,
	dimConsistency: 30,
	dimEnvironment: 30,
}

const (
	penTimezone = 15
	penLanguage = 10
)

type scoreRule struct {
	Code   string
	Dim    string
	Status string
	eval   func(a model.ScanAnalysis, intel *model.IPIntel) (points int, text string)
}

var scoreRules = []scoreRule{
	{"IP_DATACENTER", dimIdentity, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Identity.Verdict == "datacenter_fail" {
			return 20, "出口 IP 判定为机房/托管 IP，易被风控标记"
		}
		return 0, ""
	}},
	{"IP_GRAY", dimIdentity, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Identity.Verdict == "gray_unknown" {
			return 5, "出口 IP 类型灰区，无法高置信判定为住宅"
		}
		return 0, ""
	}},
	{"IP_PROXY", dimIdentity, "error", func(_ model.ScanAnalysis, intel *model.IPIntel) (int, string) {
		if intel.HasProxy() {
			return 20, "IP 信息标记为代理/VPN/Tor"
		}
		return 0, ""
	}},
	{"IP_FRAUD", dimIdentity, "warning", func(_ model.ScanAnalysis, intel *model.IPIntel) (int, string) {
		if intel.FraudScore > 50 {
			return min(intel.FraudScore/5, 20), fmt.Sprintf("IP 欺诈评分较高 (%d/100)", intel.FraudScore)
		}
		return 0, ""
	}},
	{"IP_BLACKLIST", dimIdentity, "error", func(a model.ScanAnalysis, intel *model.IPIntel) (int, string) {
		if len(a.Identity.BlacklistHits) > 0 && !intel.IsMobile {
			return 30, "出口 IP 命中黑名单: " + strings.Join(a.Identity.BlacklistHits, ",")
		}
		return 0, ""
	}},
	{"IP_BLACKLIST_CGNAT", dimIdentity, "warning", func(a model.ScanAnalysis, intel *model.IPIntel) (int, string) {
		if len(a.Identity.BlacklistHits) > 0 && intel.IsMobile {
			return 10, "移动/CGNAT 出口命中黑名单（多为共享池邻居行为）: " + strings.Join(a.Identity.BlacklistHits, ",")
		}
		return 0, ""
	}},
	{"IP_TOR_EXIT", dimIdentity, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Identity.IsTorExit {
			return 20, "出口 IP 是 Tor 出口节点"
		}
		return 0, ""
	}},
	{"IP_OPEN_PROXY_PORT", dimIdentity, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Identity.OpenProxyPort {
			return 15, "出口 IP 开放了已知代理端口，极易被识别"
		}
		return 0, ""
	}},
	{"IP_PROXY_PORT_WEAK", dimIdentity, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Identity.WeakProxyPort && a.Identity.Verdict == "datacenter_fail" {
			return 8, "机房出口开放了常见 HTTP 代理端口，疑似对外提供代理"
		}
		return 0, ""
	}},

	{"WEBRTC_LEAK", dimLeak, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Privacy.WebRTC.Status == "danger" {
			return 20, "WebRTC 泄露真实公网 IP，与出口 IP 不一致"
		}
		return 0, ""
	}},
	{"DNS_LEAK", dimLeak, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Privacy.DNS.Status == "danger" {
			return 10, "检测到 DNS 泄露"
		}
		return 0, ""
	}},
	{"IPV6_LEAK", dimLeak, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Privacy.IPv6.Status == "danger" {
			return 10, "检测到真实 IPv6 地址泄露"
		}
		return 0, ""
	}},

	{"TZ_MISMATCH", dimConsistency, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if !a.Consistency.Timezone.Passed {
			return penTimezone, fmt.Sprintf("时区不一致: 浏览器 %s / IP %s", a.Consistency.Timezone.Browser, a.Consistency.Timezone.Expected)
		}
		return 0, ""
	}},
	{"LANG_MISMATCH", dimConsistency, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if !a.Consistency.Language.Passed {
			return penLanguage, fmt.Sprintf("语言不一致: 浏览器 %s / IP 国家 %s", a.Consistency.Language.Browser, a.Consistency.Language.IPCountry)
		}
		return 0, ""
	}},

	{"WEBGL_MISMATCH", dimEnvironment, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Fingerprint.WebGL.Status == "mismatch" {
			return 15, "WebGL 渲染器与声称的系统不符: " + a.Fingerprint.WebGL.MismatchReason
		}
		return 0, ""
	}},
	{"FP_HOOKED", dimEnvironment, "warning", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Fingerprint.Canvas.IsHooked || a.Fingerprint.Audio.IsHooked {
			return 10, "检测到指纹 API 被 Hook（反指纹工具痕迹）"
		}
		return 0, ""
	}},
	{"AUTOMATION_DETECTED", dimEnvironment, "error", func(a model.ScanAnalysis, _ *model.IPIntel) (int, string) {
		if a.Automation.WebdriverDetected {
			return 25, "检测到自动化浏览器标记 (webdriver)"
		}
		return 0, ""
	}},
}

func score(a model.ScanAnalysis, intel *model.IPIntel) (int, string, []model.DiagnosisItem, []model.DimensionScore) {
	return scoreForScenario(a, intel, ScenarioStrict)
}

func scoreForScenario(a model.ScanAnalysis, intel *model.IPIntel, rawScenario string) (int, string, []model.DiagnosisItem, []model.DimensionScore) {
	scenario := NormalizeScenario(rawScenario)
	diag := make([]model.DiagnosisItem, 0, len(scoreRules)+1)

	if a.Identity.Verdict == "residential_pass" {
		diag = append(diag, model.DiagnosisItem{
			Status: "success",
			Text:   fmt.Sprintf("出口 IP 判定为住宅/移动网络 (置信度 %d)", a.Identity.IPTypeConfidence),
			Code:   "RESIDENTIAL_PASS",
		})
	}

	rawPenalty := map[string]int{}
	anyError := false
	for _, r := range scoreRules {
		if scenario == ScenarioDaily && isRelaxedInDaily(r.Code) {
			continue
		}
		points, text := r.eval(a, intel)
		if points <= 0 {
			continue
		}
		rawPenalty[r.Dim] += points
		if r.Status == "error" {
			anyError = true
		}
		diag = append(diag, model.DiagnosisItem{Status: r.Status, Text: text, Code: r.Code})
	}

	total := 100
	for dim, raw := range rawPenalty {
		total -= min(raw, dimCap[dim])
	}
	if total < 0 {
		total = 0
	}

	level := riskLevel(total)
	if anyError && level == "safe" {
		level = "low"
	}

	return total, level, diag, buildDimensions(a, intel, rawPenalty, scenario)
}

func isRelaxedInDaily(code string) bool {
	switch code {
	case "TZ_MISMATCH", "LANG_MISMATCH", "IP_DATACENTER", "IP_GRAY", "FP_HOOKED":
		return true
	default:
		return false
	}
}

func riskLevel(s int) string {
	switch {
	case s >= 90:
		return "safe"
	case s >= 70:
		return "low"
	case s >= 50:
		return "medium"
	case s >= 30:
		return "high"
	default:
		return "critical"
	}
}

func buildDimensions(a model.ScanAnalysis, intel *model.IPIntel, rawPenalty map[string]int, scenario string) []model.DimensionScore {
	dimScore := func(dim string) int {
		return max(100-rawPenalty[dim], 0)
	}

	idState, idPassed := "coherent", 1
	switch {
	case intel.HasProxy() || len(a.Identity.BlacklistHits) > 0 || a.Identity.IsTorExit || a.Identity.OpenProxyPort:
		idState, idPassed = "leak", 0
	case (a.Identity.WeakProxyPort && a.Identity.Verdict == "datacenter_fail") || intel.FraudScore > 50:
		idState, idPassed = "contradiction", 0
	case scenario != ScenarioDaily && a.Identity.Verdict == "datacenter_fail":
		idState, idPassed = "contradiction", 0
	case scenario != ScenarioDaily && a.Identity.Verdict == "gray_unknown":
		idState, idPassed = "unknown", 0
	}

	cons := a.Consistency
	consTotal, consPassed := 0, 0
	count := func(passed bool) {
		consTotal++
		if passed {
			consPassed++
		}
	}
	count(cons.Timezone.Passed)
	count(cons.Language.Passed)
	consState := "coherent"
	if scenario == ScenarioDaily {
		consPassed = consTotal
	} else if consPassed < consTotal {
		consState = "contradiction"
	} else if tzUnknown(cons.Timezone.Expected) {
		consState = "unknown"
	}

	pv := a.Privacy
	dnsUncertain := pv.DNS.Status == "blocked" || pv.DNS.Status == "unknown"
	leakChecks := []bool{
		pv.WebRTC.Status != "danger",
		pv.DNS.Status != "danger" && !dnsUncertain,
		pv.IPv6.Status != "danger",
	}
	leakPassed := 0
	for _, passed := range leakChecks {
		if passed {
			leakPassed++
		}
	}
	leakState := "coherent"
	switch {
	case pv.WebRTC.Status == "danger" || pv.DNS.Status == "danger" || pv.IPv6.Status == "danger":
		leakState = "leak"
	case pv.WebRTC.Status == "warning" || dnsUncertain:
		leakState = "unknown"
	}

	fp := a.Fingerprint
	envChecks := []bool{
		fp.WebGL.Status != "mismatch",
		!a.Automation.WebdriverDetected,
	}
	if scenario != ScenarioDaily {
		envChecks = append(envChecks, !fp.Canvas.IsHooked && !fp.Audio.IsHooked)
	}
	envPassed := 0
	for _, ok := range envChecks {
		if ok {
			envPassed++
		}
	}
	envState := "coherent"
	if envPassed < len(envChecks) {
		envState = "contradiction"
	}

	return []model.DimensionScore{
		{Key: dimIdentity, Label: "网络身份", Score: dimScore(dimIdentity), State: idState, Passed: idPassed, Total: 1},
		{Key: dimConsistency, Label: "环境一致性", Score: dimScore(dimConsistency), State: consState, Passed: consPassed, Total: consTotal},
		{Key: dimLeak, Label: "DNS/IP 泄露", Score: dimScore(dimLeak), State: leakState, Passed: leakPassed, Total: len(leakChecks)},
		{Key: dimEnvironment, Label: "环境指纹", Score: dimScore(dimEnvironment), State: envState, Passed: envPassed, Total: len(envChecks)},
	}
}

func tzUnknown(expected string) bool {
	return expected == "" || expected == "unknown" || expected == "-"
}
