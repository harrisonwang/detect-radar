package service

import (
	"encoding/json"

	"detect-radar/internal/model"
)

const (
	SkipMalformedJSON = "malformed_json"
	SkipNotScanEvent  = "not_scan_event"
	SkipOldFormat     = "old_format_no_analysis"
)

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

	AddedCodes   []string `json:"added_codes,omitempty"`
	RemovedCodes []string `json:"removed_codes,omitempty"`

	Changed bool `json:"changed"`
}

type journalLine struct {
	Event     string `json:"event"`
	ScanID    string `json:"scan_id"`
	Score     int    `json:"score"`
	RiskLevel string `json:"risk_level"`
	Diagnosis []struct {
		Code   string `json:"code"`
		Status string `json:"status"`
	} `json:"diagnosis"`
	Identity struct {
		FraudScore int `json:"fraud_score"`
	} `json:"identity"`
	Intel *struct {
		IsVPN       bool `json:"is_vpn"`
		IsProxy     bool `json:"is_proxy"`
		IsTor       bool `json:"is_tor"`
		IsRelay     bool `json:"is_relay"`
		IsAnonymous bool `json:"is_anonymous"`
	} `json:"intel"`
	Analysis json.RawMessage `json:"analysis"`
}

func ReplayLine(line []byte) ReplayResult {
	var jl journalLine
	if err := json.Unmarshal(line, &jl); err != nil {
		return ReplayResult{SkipReason: SkipMalformedJSON}
	}
	if jl.Event != "scan" {
		return ReplayResult{SkipReason: SkipNotScanEvent}
	}
	if len(jl.Analysis) == 0 || string(jl.Analysis) == "null" {
		return ReplayResult{SkipReason: SkipOldFormat, ScanID: jl.ScanID}
	}

	var analysis model.ScanAnalysis
	if err := json.Unmarshal(jl.Analysis, &analysis); err != nil {
		return ReplayResult{SkipReason: SkipMalformedJSON, ScanID: jl.ScanID}
	}

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
