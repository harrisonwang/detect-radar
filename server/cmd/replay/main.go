// Command replay 是影子回放对比工具：改评分规则（server/internal/service/rules.go）后、
// 部署前，用当前代码把历史 journal 重跑一遍，精确列出哪些真实扫描的结论会翻转。
//
// journal 每行存的 score/risk_level/diagnosis 是「旧版结论」，当前代码重算的是「新版
// 结论」。只读、无网络，绝不写 journal。
//
// 用法：
//
//	go run ./cmd/replay -journal data/scans.jsonl
//	go run ./cmd/replay -journal data/scans.jsonl -json
//	go run ./cmd/replay -journal data/scans.jsonl -fail-on-diff   # 部署前闸门，有翻转即非零退出
//
// 从部署机（ssh la1）拉取 journal：
//
//	scp la1:~/detect-radar/server/data/scans.jsonl /tmp/prod-scans.jsonl
//	go run ./cmd/replay -journal /tmp/prod-scans.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"detect-radar/internal/service"
)

// 跳过原因 → 中文标签（键见 service.Skip* 常量）。
var skipReasonLabel = map[string]string{
	service.SkipMalformedJSON: "非法 JSON",
	service.SkipNotScanEvent:  "非扫描事件(如 DNS 观测)",
	service.SkipOldFormat:     "旧格式(缺 analysis,不可回放)",
}

// report 机器可读的汇总结果（-json 输出）。
type report struct {
	Journal      string                 `json:"journal"`
	TotalLines   int                    `json:"total_lines"`
	Replayable   int                    `json:"replayable"`
	Skipped      int                    `json:"skipped"`
	SkipReasons  map[string]int         `json:"skip_reasons"`
	ChangedCount int                    `json:"changed_count"`
	OldLevelDist map[string]int         `json:"old_level_dist"`
	NewLevelDist map[string]int         `json:"new_level_dist"`
	Flips        []service.ReplayResult `json:"flips"`
}

func main() {
	journalPath := flag.String("journal", "data/scans.jsonl", "journal 文件路径（只读）")
	failOnDiff := flag.Bool("fail-on-diff", false, "存在结论翻转时以非零退出（部署前闸门用）")
	asJSON := flag.Bool("json", false, "输出机器可读 JSON 而非中文报告")
	flag.Parse()

	rep, err := run(*journalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "replay: 编码 JSON 失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		printReport(os.Stdout, rep)
	}

	if *failOnDiff && rep.ChangedCount > 0 {
		os.Exit(2)
	}
}

// run 逐行读取 journal 并聚合结果。只读，绝不写入。
func run(path string) (*report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rep := &report{
		Journal:      path,
		SkipReasons:  map[string]int{},
		OldLevelDist: map[string]int{},
		NewLevelDist: map[string]int{},
	}

	sc := bufio.NewScanner(f)
	// analysis 整块落在一行里，可能超过默认 64KB 上限，放宽到 16MB。
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(trimSpace(line)) == 0 {
			continue // 空行不计入总数
		}
		rep.TotalLines++
		res := service.ReplayLine(line)
		if !res.Replayable {
			rep.Skipped++
			rep.SkipReasons[res.SkipReason]++
			continue
		}
		rep.Replayable++
		rep.OldLevelDist[res.OldLevel]++
		rep.NewLevelDist[res.NewLevel]++
		if res.Changed {
			rep.ChangedCount++
			rep.Flips = append(rep.Flips, res)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return rep, nil
}

// trimSpace 去掉行首尾空白（避免为一个 helper 引入 strings/bytes 依赖歧义）。
func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpace(b[start]) {
		start++
	}
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// levelOrder 固定风险等级展示顺序（由安全到危险）。
var levelOrder = []string{"safe", "low", "medium", "high", "critical"}

func printReport(w *os.File, rep *report) {
	fmt.Fprintln(w, "影子回放对比报告")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "journal:   %s\n", rep.Journal)
	fmt.Fprintf(w, "总行数:    %d\n", rep.TotalLines)
	fmt.Fprintf(w, "可回放数:  %d\n", rep.Replayable)
	fmt.Fprintf(w, "跳过数:    %d\n", rep.Skipped)

	if rep.Skipped > 0 {
		fmt.Fprintln(w, "\n跳过原因分布:")
		for _, r := range sortedKeys(rep.SkipReasons) {
			label := skipReasonLabel[r]
			if label == "" {
				label = r
			}
			fmt.Fprintf(w, "  %-28s %d\n", label, rep.SkipReasons[r])
		}
	}

	fmt.Fprintf(w, "\n结论翻转: %d / %d 可回放行\n", rep.ChangedCount, rep.Replayable)
	if rep.ChangedCount > 0 {
		fmt.Fprintln(w, "结论翻转清单:")
		for _, r := range rep.Flips {
			fmt.Fprintf(w, "  %s  score %d→%d  level %s→%s\n",
				scanPrefix(r.ScanID), r.OldScore, r.NewScore, r.OldLevel, r.NewLevel)
			if len(r.AddedCodes) > 0 {
				fmt.Fprintf(w, "      + 新增命中: %v\n", r.AddedCodes)
			}
			if len(r.RemovedCodes) > 0 {
				fmt.Fprintf(w, "      - 消失命中: %v\n", r.RemovedCodes)
			}
		}
	}

	if rep.Replayable > 0 {
		fmt.Fprintln(w, "\nrisk_level 分布对比 (旧 → 新):")
		fmt.Fprintf(w, "  %-10s %6s %6s\n", "level", "旧", "新")
		for _, lvl := range levelDisplayOrder(rep) {
			fmt.Fprintf(w, "  %-10s %6d %6d\n", lvl, rep.OldLevelDist[lvl], rep.NewLevelDist[lvl])
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
