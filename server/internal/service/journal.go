package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"detect-radar/internal/model"
	"detect-radar/internal/telemetry"
)

var ErrStorageUnavailable = errors.New("遥测存储不可用")

// Journal 将完整遥测事件写入 SQLite。
type Journal struct{ store *telemetry.Store }

func NewJournal(store *telemetry.Store) *Journal { return &Journal{store: store} }

func (j *Journal) append(instanceID, key string, event map[string]any) error {
	if j == nil || j.store == nil {
		return ErrStorageUnavailable
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = j.store.Append(ctx, telemetry.Event{InstanceID: instanceID, Key: key, Payload: payload})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return nil
}

// ScanEvent 一次扫描的判定快照（从分析结果蒸馏）
func (j *Journal) ScanEvent(instanceID string, resp *model.ScanResponse, ua string) error {
	if resp == nil {
		return fmt.Errorf("%w：缺少扫描结果", ErrStorageUnavailable)
	}
	a := resp.Analysis
	ev := map[string]any{
		"event":      "scan",
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id":    resp.ScanID,
		"ip":         resp.ServerData.ClientIP,
		"ua":         ua,
		"score":      resp.Score,
		"risk_level": resp.RiskLevel,
		"identity": map[string]any{
			"ip_type":     a.Identity.IPType,
			"verdict":     a.Identity.Verdict,
			"fraud_score": a.Identity.FraudScore,
			"blacklist":   len(a.Identity.BlacklistHits),
			"tor_exit":    a.Identity.IsTorExit,
		},
		"consistency": map[string]any{
			"tz_browser":   a.Consistency.Timezone.Browser,
			"tz_expected":  a.Consistency.Timezone.Expected,
			"tz_passed":    a.Consistency.Timezone.Passed,
			"lang_browser": a.Consistency.Language.Browser,
			"lang_passed":  a.Consistency.Language.Passed,
		},
		"privacy": map[string]any{
			"webrtc": a.Privacy.WebRTC.Status,
			"dns":    a.Privacy.DNS.Status,
			"ipv6":   a.Privacy.IPv6.Status,
		},
		"automation": map[string]any{
			"webdriver": a.Automation.WebdriverDetected,
			"status":    a.Automation.OverallStatus,
		},
		"fingerprint": a.Fingerprint.OverallStatus,
	}
	if intel := resp.ServerData.IPIntel; intel != nil {
		ev["intel"] = map[string]any{
			"country": intel.Country,
			// region/isp/usage_type(_raw)/is_mobile 为 CN 权威地理层（ip2region-cn）落地后新增，
			// 使影子回放与准确率统计能区分 CN 层答案（中文省市 + 运营商归因）。严格增量。
			"region":         intel.Region,
			"city":           intel.City,
			"isp":            intel.ISP,
			"timezone":       intel.Timezone,
			"asn":            intel.ASN,
			"org":            intel.Org,
			"source":         intel.Source,
			"tier":           intel.Tier,
			"confidence":     intel.Confidence,
			"usage_type":     intel.UsageType,
			"usage_type_raw": intel.UsageTypeRaw,
			"is_mobile":      intel.IsMobile,
			// hosting 判定方法（cloud_ip_range / asn_match_cymru / rdns_residential 等，
			// 见 hosting.go），供按命中方法统计机房误判来源
			"method": intel.DetectMethod,
			// HasProxy() 的全部输入（见 model/ipintel.go），使 IP_PROXY 规则可从本行
			// 精确重放：analysis.identity 只镜像了 proxy/vpn/tor 三项，relay/anonymous
			// 仅存在于 intel，缺了就无法复现 relay/anonymous-only 出口的判定
			"is_vpn":       intel.IsVPN,
			"is_proxy":     intel.IsProxy,
			"is_tor":       intel.IsTor,
			"is_relay":     intel.IsRelay,
			"is_anonymous": intel.IsAnonymous,
		}
	}
	// 命中的规则（带 Code 的诊断条目），供按规则统计误报率与影子回放归因
	ev["diagnosis"] = firedRules(resp.Diagnosis)
	// 完整评分输入,用于影子回放（改规则前用历史扫描对比新旧结论）
	ev["analysis"] = a
	// RTT 物理探测（Phase 1 影子采集）：analysis 里已带完整 RTT，这里再蒸馏一个顶层 rtt
	// 便于 jq 直接提分布（无需下钻 analysis.rtt）。status=unavailable（本地无 nginx 且客户端
	// 未上报）时整块省略，保持旧式 jq 查询与行长不受影响。严格增量。
	if rtt := distillRTT(a.RTT); rtt != nil {
		ev["rtt"] = rtt
	}
	return j.append(instanceID, "", ev)
}

// distillRTT 把派生的 RTTAnalysis 蒸馏成便于 jq 的顶层 rtt 对象。
// status=unavailable / 空（老扫描/无 nginx 且客户端未上报）时返回 nil，调用方据此整块省略。
// 只带最常用于标定的少数字段；完整视图仍在 analysis.rtt 里。
func distillRTT(r model.RTTAnalysis) map[string]any {
	if r.Status == "" || r.Status == "unavailable" {
		return nil
	}
	m := map[string]any{"status": r.Status}
	if r.ClientMinMS > 0 {
		m["client_min_ms"] = r.ClientMinMS
	}
	if r.ServerTCPMS > 0 {
		m["server_tcp_ms"] = r.ServerTCPMS
	}
	if r.DeltaMS != nil {
		m["delta_ms"] = *r.DeltaMS
	}
	if r.GeoViolation != nil {
		m["geo_violation"] = *r.GeoViolation
	}
	if r.GeoDistanceKM != nil {
		m["geo_distance_km"] = *r.GeoDistanceKM
	}
	if r.GeoSource != "" {
		m["geo_source"] = r.GeoSource
	}
	return m
}

// firedRules 从诊断清单蒸馏出实际命中的规则（带 Code 的条目）。
// 不落 points：扣分是规则表运行期算出的（部分规则分值随输入变化，见 rules.go），
// 用持久化的 analysis 重放即可精确复现，无需在此冗余存储。
func firedRules(diag []model.DiagnosisItem) []map[string]any {
	rules := make([]map[string]any, 0, len(diag))
	for _, d := range diag {
		if d.Code == "" {
			continue // 正向 success 诊断无 Code，不属于命中规则
		}
		rules = append(rules, map[string]any{
			"code":   d.Code,
			"status": d.Status,
		})
	}
	return rules
}

// DNSLeakEvent 保存异步 DNS 观测结果，按服务端扫描实例关联。
// resp 为 DNS 定案后重算的扫描结论（见 scan.go UpdateDNSLeak）：非空时把新总分/等级/
// 命中规则一并落库，使 post-DNS 判定同样可归因；为空时退回仅记 resolver 观测。
func (j *Journal) DNSLeakEvent(instanceID, scanID string, result *model.DNSLeakResult, resp *model.ScanResponse) error {
	if result == nil {
		return fmt.Errorf("%w：缺少 DNS 检测结果", ErrStorageUnavailable)
	}
	resolvers := make([]map[string]string, 0, len(result.DNSServers))
	for _, q := range result.DNSServers {
		resolvers = append(resolvers, map[string]string{
			"ip": q.IP, "country": q.Country, "isp": q.ISP,
		})
	}
	ev := map[string]any{
		"event":     "dns_leak",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id":   scanID,
		"test_id":   result.ID,
		"leaked":    result.Leaked,
		"level":     result.Level,
		"resolvers": resolvers,
	}
	// post-DNS 重算判定：DNS 定案后重新评分的结论也要可归因（新总分 + 命中规则）
	if resp != nil {
		ev["score"] = resp.Score
		ev["risk_level"] = resp.RiskLevel
		ev["diagnosis"] = firedRules(resp.Diagnosis)
		ev["analysis"] = resp.Analysis
	}
	return j.append(instanceID, "dns_leak:"+instanceID+":"+result.ID, ev)
}

// FeedbackEvent 保留已过期或无法唯一关联扫描的反馈。
func (j *Journal) FeedbackEvent(scanID, category, note string, resp *model.ScanResponse) error {
	if j == nil || j.store == nil {
		return ErrStorageUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instanceID, binding, err := j.store.ResolveInstance(ctx, scanID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	ev := map[string]any{
		"event": "feedback", "ts": time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id": scanID, "category": category, "binding_status": binding,
	}
	if note != "" {
		ev["note"] = note
	}
	if resp != nil && instanceID != "" {
		ev["context"] = map[string]any{
			"score": resp.Score, "risk_level": resp.RiskLevel,
			"fired": firedRules(resp.Diagnosis), "analysis": resp.Analysis,
		}
	}
	return j.append(instanceID, "", ev)
}

func (j *Journal) DNSLifecycle(eventType, instanceID, scanID, testID string) error {
	return j.append(instanceID, eventType+":"+instanceID+":"+testID, map[string]any{
		"event": eventType, "ts": time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id": scanID, "test_id": testID,
	})
}

func (j *Journal) DNSUpdateEvent(instanceID string, resp *model.ScanResponse, leaked bool) error {
	return j.append(instanceID, "", map[string]any{
		"event": "dns_update", "ts": time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id": resp.ScanID, "leaked": leaked, "score": resp.Score,
		"risk_level": resp.RiskLevel, "diagnosis": firedRules(resp.Diagnosis),
		"analysis": resp.Analysis,
	})
}
