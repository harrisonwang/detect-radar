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

type Journal struct {
	store  *telemetry.Store
	mailer *Mailer
}

func NewJournal(store *telemetry.Store, mailer *Mailer) *Journal {
	return &Journal{store: store, mailer: mailer}
}

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

func (j *Journal) ScanEvent(instanceID string, resp *model.ScanResponse, ua string) error {
	if resp == nil {
		return fmt.Errorf("%w：缺少扫描结果", ErrStorageUnavailable)
	}
	a := resp.Analysis
	ev := map[string]any{
		"event":      "scan",
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id":    resp.ScanID,
		"scenario":   NormalizeScenario(resp.Scenario),
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
			"country":        intel.Country,
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
			"method":         intel.DetectMethod,
			"is_vpn":         intel.IsVPN,
			"is_proxy":       intel.IsProxy,
			"is_tor":         intel.IsTor,
			"is_relay":       intel.IsRelay,
			"is_anonymous":   intel.IsAnonymous,
		}
	}
	ev["diagnosis"] = firedRules(resp.Diagnosis)
	ev["analysis"] = a
	if rtt := distillRTT(a.RTT); rtt != nil {
		ev["rtt"] = rtt
	}
	return j.append(instanceID, "", ev)
}

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
	if r.ServerTCPMinMS > 0 {
		m["server_tcp_min_ms"] = r.ServerTCPMinMS
	}
	if r.DeltaMinMS != nil {
		m["delta_min_ms"] = *r.DeltaMinMS
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

func firedRules(diag []model.DiagnosisItem) []map[string]any {
	rules := make([]map[string]any, 0, len(diag))
	for _, d := range diag {
		if d.Code == "" {
			continue
		}
		rules = append(rules, map[string]any{
			"code":   d.Code,
			"status": d.Status,
		})
	}
	return rules
}

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
	if resp != nil {
		ev["score"] = resp.Score
		ev["risk_level"] = resp.RiskLevel
		ev["diagnosis"] = firedRules(resp.Diagnosis)
		ev["analysis"] = resp.Analysis
	}
	return j.append(instanceID, "dns_leak:"+instanceID+":"+result.ID, ev)
}

func (j *Journal) DNSLifecycle(eventType, instanceID, scanID, testID string) error {
	return j.append(instanceID, eventType+":"+instanceID+":"+testID, map[string]any{
		"event": eventType, "ts": time.Now().UTC().Format(time.RFC3339Nano),
		"scan_id": scanID, "test_id": testID,
	})
}
