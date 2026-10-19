// replay 是影子回放对比工具：改评分规则（server/internal/service/rules.go）后、
// 部署前，用当前代码把数据库中的历史扫描重跑一遍，列出哪些结论会翻转。
//
// 数据库保存的是旧版结论，当前代码重算的是新版结论。只读、无网络。
//
// 用法：
//
//	go run ./cmd/replay -db data/scans.db
//	go run ./cmd/replay -db data/scans.db -json
//	go run ./cmd/replay -db data/scans.db -fail-on-diff
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"detect-radar/internal/service"
	"detect-radar/internal/telemetry"
)

// 跳过原因 → 中文标签（键见 service.Skip* 常量）。
var skipReasonLabel = map[string]string{
	service.SkipMalformedJSON: "非法 JSON",
	service.SkipNotScanEvent:  "非扫描事件(如 DNS 观测)",
	service.SkipOldFormat:     "旧格式(缺 analysis,不可回放)",
}

// report 机器可读的汇总结果（-json 输出）。
type report struct {
	Database     string                 `json:"database"`
	TotalEvents  int                    `json:"total_events"`
	Replayable   int                    `json:"replayable"`
	Skipped      int                    `json:"skipped"`
	SkipReasons  map[string]int         `json:"skip_reasons"`
	ChangedCount int                    `json:"changed_count"`
	OldLevelDist map[string]int         `json:"old_level_dist"`
	NewLevelDist map[string]int         `json:"new_level_dist"`
	Flips        []service.ReplayResult `json:"flips"`
}

func main() {
	dbPath := flag.String("db", "data/scans.db", "SQLite 文件路径（只读）")
	failOnDiff := flag.Bool("fail-on-diff", false, "存在结论翻转时以非零退出（部署前闸门用）")
	asJSON := flag.Bool("json", false, "输出机器可读 JSON 而非中文报告")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "用法：replay [-db 数据库] [-json] [-fail-on-diff]")
		flag.PrintDefaults()
	}
	flag.Parse()

	rep, err := run(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "回放失败：%v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "回放结果编码为 JSON 失败：%v\n", err)
			os.Exit(1)
		}
	} else {
		printReport(os.Stdout, rep)
	}

	if *failOnDiff && rep.ChangedCount > 0 {
		os.Exit(2)
	}
}

func run(dbPath string) (*report, error) {
	rep := &report{
		Database:     dbPath,
		SkipReasons:  map[string]int{},
		OldLevelDist: map[string]int{},
		NewLevelDist: map[string]int{},
	}

	visit := func(event []byte) error {
		rep.TotalEvents++
		res := service.ReplayLine(event)
		if !res.Replayable {
			rep.Skipped++
			rep.SkipReasons[res.SkipReason]++
			return nil
		}
		rep.Replayable++
		rep.OldLevelDist[res.OldLevel]++
		rep.NewLevelDist[res.NewLevel]++
		if res.Changed {
			rep.ChangedCount++
			rep.Flips = append(rep.Flips, res)
		}
		return nil
	}
	store, err := telemetry.OpenReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Walk(context.Background(), visit); err != nil {
		return nil, err
	}
	return rep, nil
}

// levelOrder 固定风险等级展示顺序（由安全到危险）。
var levelOrder = []string{"safe", "low", "medium", "high", "critical"}

func printReport(w *os.File, rep *report) {
	levelLabels := map[string]string{
		"safe": "安全", "low": "低风险", "medium": "中风险", "high": "高风险", "critical": "严重风险",
	}
	displayLevel := func(level string) string {
		if label, ok := levelLabels[level]; ok {
			return label
		}
		return "未知（" + level + "）"
	}
	fmt.Fprintln(w, "影子回放对比报告")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "数据库:    %s\n", rep.Database)
	fmt.Fprintf(w, "总事件数:  %d\n", rep.TotalEvents)
	fmt.Fprintf(w, "可回放数:  %d\n", rep.Replayable)
	fmt.Fprintf(w, "跳过数:    %d\n", rep.Skipped)

	if rep.Skipped > 0 {
		fmt.Fprintln(w, "\n跳过原因分布:")
		for _, r := range sortedKeys(rep.SkipReasons) {
			label := skipReasonLabel[r]
			if label == "" {
				label = "未知原因（" + r + "）"
			}
			fmt.Fprintf(w, "  %-28s %d\n", label, rep.SkipReasons[r])
		}
	}

	fmt.Fprintf(w, "\n结论翻转: %d / %d 可回放事件\n", rep.ChangedCount, rep.Replayable)
	if rep.ChangedCount > 0 {
		fmt.Fprintln(w, "结论翻转清单:")
		for _, r := range rep.Flips {
			fmt.Fprintf(w, "  %s  分数 %d→%d  风险等级 %s→%s\n",
				scanPrefix(r.ScanID), r.OldScore, r.NewScore, displayLevel(r.OldLevel), displayLevel(r.NewLevel))
			if len(r.AddedCodes) > 0 {
				fmt.Fprintf(w, "      + 新增命中: %v\n", r.AddedCodes)
			}
			if len(r.RemovedCodes) > 0 {
				fmt.Fprintf(w, "      - 消失命中: %v\n", r.RemovedCodes)
			}
		}
	}

	if rep.Replayable > 0 {
		fmt.Fprintln(w, "\n风险等级分布对比 (旧 → 新):")
		fmt.Fprintf(w, "  %-10s %6s %6s\n", "风险等级", "旧", "新")
		for _, lvl := range levelDisplayOrder(rep) {
			fmt.Fprintf(w, "  %-10s %6d %6d\n", displayLevel(lvl), rep.OldLevelDist[lvl], rep.NewLevelDist[lvl])
		}
	}
}

// levelDisplayOrder 先按固定顺序列出已知等级，再补上任何未预期的等级。
func levelDisplayOrder(rep *report) []string {
	seen := map[string]bool{}
	var out []string
	for _, lvl := range levelOrder {
		if rep.OldLevelDist[lvl] > 0 || rep.NewLevelDist[lvl] > 0 {
			out = append(out, lvl)
			seen[lvl] = true
		}
	}
	extra := map[string]bool{}
	for lvl := range rep.OldLevelDist {
		extra[lvl] = true
	}
	for lvl := range rep.NewLevelDist {
		extra[lvl] = true
	}
	var rest []string
	for lvl := range extra {
		if !seen[lvl] {
			rest = append(rest, lvl)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// scanPrefix 只展示 scan_id 前缀（对齐分享卡上的短标识），避免刷屏。
func scanPrefix(id string) string {
	const n = 12
	if len(id) <= n {
		if id == "" {
			return "(无 scan_id)"
		}
		return id
	}
	return id[:n] + "…"
}
