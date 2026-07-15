package service

import (
	"encoding/json"

	"detect-radar/internal/model"
)

// 影子回放（shadow replay）——仅供 cmd/replay 工具使用。
//
// 用途：改评分规则（rules.go 的权重/阈值/新增规则）后、部署前，用当前代码把历史
// journal 重跑一遍。journal 每行存的 score/risk_level/diagnosis 是「旧版结论」，当前
// 代码用同一份 analysis+intel 重算的是「新版结论」，两相对比即可精确看到哪些真实扫描
// 的判定会翻转。
//
// 之所以能精确重放：score() 只从 intel 读两样东西——HasProxy() 与 FraudScore（见
// rules.go 的 IP_PROXY / IP_FRAUD）。新格式 journal 行完整落了 analysis（score() 的
// 全部结构化输入）、intel.is_*（HasProxy() 的全部输入）与 identity.fraud_score
// （intel.FraudScore 的逐字镜像），因此一行即足以无损复现判定。旧格式行缺 analysis，
// 无法重放。
//
// 本文件的导出面刻意保持最小：只有 ReplayResult、ReplayLine 与三个 Skip* 原因常量。

// 跳过原因（ReplayResult.SkipReason 的取值）。作为工具契约暴露，供 CLI 归类统计。
const (
	// SkipMalformedJSON 该行不是合法 JSON。
	SkipMalformedJSON = "malformed_json"
	// SkipNotScanEvent 该行不是扫描事件（如 DNS 泄露观测 event=dns_leak）。
	SkipNotScanEvent = "not_scan_event"
	// SkipOldFormat 旧格式扫描行，缺 analysis，无法重放。
	SkipOldFormat = "old_format_no_analysis"
)

// ReplayResult 单行 journal 的回放结果。仅供影子回放工具使用。
//
// Replayable=false 时 SkipReason 说明原因，其余字段无意义。Replayable=true 时给出
// 旧（journal 存的）与新（当前代码重算的）结论及其差异。
type ReplayResult struct {
	Replayable bool   `json:"replayable"`
	SkipReason string `json:"skip_reason,omitempty"`
	ScanID     string `json:"scan_id,omitempty"`

	OldScore int    `json:"old_score"`
	NewScore int    `json:"new_score"`
	OldLevel string `json:"old_level,omitempty"`
	NewLevel string `json:"new_level,omitempty"`

	ScoreChanged bool `json:"score_changed"`
	LevelChanged bool `json:"level_changed"`

	// AddedCodes 新版命中、旧版未命中的规则 Code；RemovedCodes 反之。
	AddedCodes   []string `json:"added_codes,omitempty"`
	RemovedCodes []string `json:"removed_codes,omitempty"`

	// Changed 任一维度（分数/等级/命中规则集）发生翻转。
	Changed bool `json:"changed"`
}

// journalLine 是 journal 行里影子回放实际消费的字段子集（键对齐 journal.go 落库）。
type journalLine struct {
	Event     string `json:"event"`
	ScanID    string `json:"scan_id"`
	Score     int    `json:"score"`
	RiskLevel string `json:"risk_level"`
	Diagnosis []struct {
		Code   string `json:"code"`
		Status string `json:"status"`
	} `json:"diagnosis"`
	// identity.fraud_score 是 intel.FraudScore 的逐字镜像（见 journal.go），
	// intel 蒸馏块不含欺诈分，故 FraudScore 只能从这里取。
	Identity struct {
		FraudScore int `json:"fraud_score"`
	} `json:"identity"`
	// intel.is_* 是 HasProxy() 的全部输入（含 analysis.identity 没有的 relay/anonymous）。
	Intel *struct {
		IsVPN       bool `json:"is_vpn"`
		IsProxy     bool `json:"is_proxy"`
		IsTor       bool `json:"is_tor"`
		IsRelay     bool `json:"is_relay"`
		IsAnonymous bool `json:"is_anonymous"`
	} `json:"intel"`
	// analysis 是 score() 的完整结构化输入；旧格式行没有此键，据此判定不可重放。
	Analysis json.RawMessage `json:"analysis"`
}

// ReplayLine 解析单行 journal，若为可重放的扫描事件则用当前代码重算评分并与存储结论
// 对比，返回差异。仅供影子回放工具使用。
//
// 永不 panic：非法 JSON、非扫描事件、旧格式（缺 analysis）行都返回 Replayable=false
// 并带 SkipReason。
func ReplayLine(line []byte) ReplayResult {
	var jl journalLine
	if err := json.Unmarshal(line, &jl); err != nil {
		return ReplayResult{SkipReason: SkipMalformedJSON}
	}
	// 非扫描事件（DNS 泄露观测等）一律跳过。
	if jl.Event != "scan" {
		return ReplayResult{SkipReason: SkipNotScanEvent}
	}
	// 旧格式行缺 analysis，无法重放（null 也视为缺失）。
	if len(jl.Analysis) == 0 || string(jl.Analysis) == "null" {
		return ReplayResult{SkipReason: SkipOldFormat, ScanID: jl.ScanID}
	}

	var analysis model.ScanAnalysis
	if err := json.Unmarshal(jl.Analysis, &analysis); err != nil {
		return ReplayResult{SkipReason: SkipMalformedJSON, ScanID: jl.ScanID}
	}

	// 重建 score() 需要的最小 intel：HasProxy() 布尔取自 intel 蒸馏块的 is_*，
	// FraudScore 取自 identity 镜像。始终非 nil，避免 rules.go 对 intel 解引用。
	intel := &model.IPIntel{FraudScore: jl.Identity.FraudScore}
	if jl.Intel != nil {
		intel.IsVPN = jl.Intel.IsVPN
		intel.IsProxy = jl.Intel.IsProxy
		intel.IsTor = jl.Intel.IsTor
		intel.IsRelay = jl.Intel.IsRelay
		intel.IsAnonymous = jl.Intel.IsAnonymous
	}

	newScore, newLevel, newDiag, _ := score(analysis, intel)

	oldCodes := make(map[string]bool, len(jl.Diagnosis))
	for _, d := range jl.Diagnosis {
		if d.Code != "" {
			oldCodes[d.Code] = true
		}
	}
	newCodes := make(map[string]bool, len(newDiag))
	for _, d := range newDiag {
		if d.Code != "" {
			newCodes[d.Code] = true
		}
	}

	// 保持规则表顺序输出增删，便于阅读与稳定测试断言。
	var added, removed []string
	for _, r := range scoreRules {
		if newCodes[r.Code] && !oldCodes[r.Code] {
			added = append(added, r.Code)
		}
		if oldCodes[r.Code] && !newCodes[r.Code] {
			removed = append(removed, r.Code)
		}
	}

	res := ReplayResult{
		Replayable:   true,
		ScanID:       jl.ScanID,
		OldScore:     jl.Score,
		NewScore:     newScore,
		OldLevel:     jl.RiskLevel,
		NewLevel:     newLevel,
		ScoreChanged: jl.Score != newScore,
		LevelChanged: jl.RiskLevel != newLevel,
		AddedCodes:   added,
		RemovedCodes: removed,
	}
	res.Changed = res.ScoreChanged || res.LevelChanged || len(added) > 0 || len(removed) > 0
	return res
}
