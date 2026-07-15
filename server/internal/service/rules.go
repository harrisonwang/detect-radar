package service

import (
	"fmt"
	"strings"

	"detect-radar/internal/model"
)

// 评分维度——对齐前端四轴雷达。fingerprint + automation 合并进 environment。
const (
	dimIdentity    = "identity"
	dimConsistency = "consistency"
	dimLeak        = "leak"
	dimEnvironment = "environment"
)

// dimCap 各维度扣分上限：同一维度内多条高相关规则（如机房+代理+黑名单常指同一个
// "脏"出口）不再线性叠加，扣到封顶为止，避免重复计分同一件事。
var dimCap = map[string]int{
	dimIdentity:    35,
	dimLeak:        35,
	dimConsistency: 30,
	dimEnvironment: 30,
}

// 一致性扣分权重——集中定义，作为唯一口径。
const (
	penTimezone = 15
	penLanguage = 10
)

// scoreRule 单条评分规则。eval 命中时返回 (扣分, 文案)，未命中返回 (0, "")。
//
// 所有扣分规则集中在此表——这是评分的唯一事实来源（前端不再本地计算任何分数）。
// 新增信号只需在表里加一行。规则的 Code 同时是 DiagnosisItem.Code，供前端渲染文案/i18n。
type scoreRule struct {
	Code   string
	Dim    string // identity / consistency / leak / environment
	Status string // error | warning
	eval   func(a model.ScanAnalysis, intel *model.IPIntel) (points int, text string)
}

var scoreRules = []scoreRule{
	// —— 网络身份 ——
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
		// 移动/CGNAT 出口的黑名单命中降级到 IP_BLACKLIST_CGNAT（见下条），此处不再按 error 重罚
		if len(a.Identity.BlacklistHits) > 0 && !intel.IsMobile {
			return 30, "出口 IP 命中黑名单: " + strings.Join(a.Identity.BlacklistHits, ",")
		}
		return 0, ""
	}},
	{"IP_BLACKLIST_CGNAT", dimIdentity, "warning", func(a model.ScanAnalysis, intel *model.IPIntel) (int, string) {
		// 移动/CGNAT 出口：成千上万用户共享同一公网出口，黑名单命中多半是共享池里邻居的行为，
		// 不能按整段 IP 把这笔账算到本用户头上。故不走 -30 的 IP_BLACKLIST，降级为 10 分警告，
		// 仅提示存在共享池污染风险（对齐产品「移动出口可信、不因邻居受罚」的口径）。
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
		// 弱代理端口（3128/8080/8888 http-proxy）：家用路由器管理页/Web UI 也常占用，单看不足为凭。
		// 仅当出口本就判为机房/托管（datacenter_fail）时才计分——服务器开 8080 更像代理服务，
		// 家宽路由器开 8080 不是。住宅/移动/灰区一律不计分，避免家宽与 CGNAT 误报。
		if a.Identity.WeakProxyPort && a.Identity.Verdict == "datacenter_fail" {
			return 8, "机房出口开放了常见 HTTP 代理端口，疑似对外提供代理"
		}
		return 0, ""
	}},

	// —— DNS/IP 泄露 ——
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

	// —— 环境一致性 ——
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
	// 刻意不做地理位置检查——需要权限弹窗，与产品「零打扰」采集哲学冲突；
	// 如未来回归，将以用户主动授权的可选检查形式实现。

	// —— 环境指纹（含自动化）——
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

// score 遍历规则表，分维度封顶后得出干净度评分（0-100，越高越安全）、风险等级、
// 诊断清单与四维雷达分。这是唯一评分入口，前端直接消费，不再本地计算。
func score(a model.ScanAnalysis, intel *model.IPIntel) (int, string, []model.DiagnosisItem, []model.DimensionScore) {
	diag := make([]model.DiagnosisItem, 0, len(scoreRules)+1)

	// 正向诊断：身份判定为住宅/移动时给一条 success（不计分）
	if a.Identity.Verdict == "residential_pass" {
		diag = append(diag, model.DiagnosisItem{
			Status: "success",
			Text:   fmt.Sprintf("出口 IP 判定为住宅/移动网络 (置信度 %d)", a.Identity.IPTypeConfidence),
		})
	}

	// 分维度累计原始扣分
	rawPenalty := map[string]int{}
	anyError := false
	for _, r := range scoreRules {
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

	// 每维度封顶后汇总为总分
	total := 100
	for dim, raw := range rawPenalty {
		total -= min(raw, dimCap[dim])
	}
	if total < 0 {
		total = 0
	}

	level := riskLevel(total)
	// 强信号压等级：命中任一 error 级规则时不得判 safe
	if anyError && level == "safe" {
		level = "low"
	}

	return total, level, diag, buildDimensions(a, intel, rawPenalty)
}

// riskLevel 把干净度评分映射为风险等级。
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

// buildDimensions 产出四轴雷达所需的维度分（0-100）、语义状态与通过数。
// 维度轴分独立于总分：某维度重创时轴分接近 0，而总分受该维度封顶保护。
func buildDimensions(a model.ScanAnalysis, intel *model.IPIntel, rawPenalty map[string]int) []model.DimensionScore {
	dimScore := func(dim string) int {
		return max(100-rawPenalty[dim], 0)
	}

	// —— 身份 ——
	idState, idPassed := "coherent", 1
	switch {
	case intel.HasProxy() || len(a.Identity.BlacklistHits) > 0 || a.Identity.IsTorExit:
		idState, idPassed = "leak", 0
	case a.Identity.Verdict == "datacenter_fail":
		idState, idPassed = "contradiction", 0
	case a.Identity.Verdict == "gray_unknown":
		idState, idPassed = "unknown", 0
	}

	// —— 一致性 ——（地理位置检查已下线，见 rules.go 规则表处说明；仅时区/语言两轴）
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
	if consPassed < consTotal {
		consState = "contradiction"
	} else if tzUnknown(cons.Timezone.Expected) {
		consState = "unknown"
	}

	// —— 泄露 ——
	pv := a.Privacy
	dangers := []bool{
		pv.WebRTC.Status == "danger",
		pv.DNS.Status == "danger",
		pv.IPv6.Status == "danger",
	}
	leakPassed, anyLeak := 0, false
	for _, d := range dangers {
		if d {
			anyLeak = true
		} else {
			leakPassed++
		}
	}
	leakState := "coherent"
	if anyLeak {
		leakState = "leak"
	} else if pv.WebRTC.Status == "warning" || pv.DNS.Status == "blocked" {
		leakState = "unknown"
	}

	// —— 环境指纹（含自动化）——
	fp := a.Fingerprint
	envChecks := []bool{
		fp.WebGL.Status != "mismatch",
		!fp.Canvas.IsHooked && !fp.Audio.IsHooked,
		!a.Automation.WebdriverDetected,
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
		{Key: dimLeak, Label: "DNS/IP 泄露", Score: dimScore(dimLeak), State: leakState, Passed: leakPassed, Total: len(dangers)},
		{Key: dimEnvironment, Label: "环境指纹", Score: dimScore(dimEnvironment), State: envState, Passed: envPassed, Total: len(envChecks)},
	}
}

func tzUnknown(expected string) bool {
	return expected == "" || expected == "unknown" || expected == "-"
}
