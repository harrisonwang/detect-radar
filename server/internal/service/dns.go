package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"detect-radar/dnsassessment"
	"detect-radar/internal/config"
	"detect-radar/internal/model"
)

const geoCacheMax = 20000

var ErrDNSTestNotFound = errors.New("DNS 测试不存在或已过期")

type DNSService struct {
	dnsCfg    config.DNSConfig
	ipIntel   *IPIntelService
	journal   *Journal
	scan      *ScanService
	testCache sync.Map
	geoCache  sync.Map
	geoCount  atomic.Int64
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type dnsTestState struct {
	mu      sync.Mutex
	test    model.DNSLeakTest
	expired bool
}

type geoInfo struct {
	Country     string
	ISP         string
	ASN         string
	NetworkType string
}

func NewDNSService(dnsCfg config.DNSConfig, ipIntel *IPIntelService, journal *Journal) *DNSService {
	s := &DNSService{
		dnsCfg: dnsCfg, ipIntel: ipIntel, journal: journal,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go s.cleanupExpiredTests()
	return s
}

func (s *DNSService) Close() error {
	if s == nil || s.stop == nil {
		return nil
	}
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

func (s *DNSService) SetScanService(scan *ScanService) {
	s.scan = scan
}

func (s *DNSService) CreateLeakTest(ctx context.Context, scanID string) (*model.DNSLeakTestResponse, error) {
	fullID := s.generateTestID()
	shortID := fullID[:8]
	testDomain := fmt.Sprintf("%s.%s", fullID, s.dnsCfg.Domain)
	now := time.Now()
	expiresAt := now.Add(s.dnsCfg.TTL)

	instanceID := ""
	expectedCountry := ""
	var exit *dnsassessment.Exit
	if s.scan != nil && scanID != "" {
		var err error
		instanceID, err = s.scan.InstanceID(scanID)
		if err != nil {
			return nil, err
		}
		s.scan.mu.RLock()
		if stored, ok := s.scan.store[instanceID]; ok && stored.resp.ServerData.IPIntel != nil {
			expectedCountry = stored.resp.ServerData.IPIntel.Country
			exit = dnsExit(stored.resp.ServerData.IPIntel)
		}
		s.scan.mu.RUnlock()
	}
	test := model.DNSLeakTest{
		TestID:          fullID,
		TestDomain:      testDomain,
		ScanID:          scanID,
		ScanInstanceID:  instanceID,
		ExpectedCountry: expectedCountry,
		Exit:            exit,
		ExpiresAt:       expiresAt.Unix(),
		CreatedAt:       now,
		DNSQueries:      []model.DNSQuery{},
	}
	if err := s.journal.DNSLifecycle("dns_started", instanceID, scanID, fmt.Sprintf("leak_dns_%s", shortID)); err != nil {
		return nil, fmt.Errorf("保存 DNS 测试开始事件失败：%w", err)
	}
	state := &dnsTestState{test: test}

	s.testCache.Store(fullID, state)
	s.testCache.Store(shortID, state)

	return &model.DNSLeakTestResponse{
		ID:         fmt.Sprintf("leak_dns_%s", shortID),
		TestDomain: testDomain,
		CreatedAt:  now.UTC().Format(time.RFC3339),
		ExpiresAt:  expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func dnsExit(intel *model.IPIntel) *dnsassessment.Exit {
	network := intel.ISP
	if strings.TrimSpace(network) == "" || strings.EqualFold(strings.TrimSpace(network), "unknown") {
		network = intel.Org
	}
	country := geoBucket(intel)
	if country == "" {
		country = intel.Country
	}
	exit := &dnsassessment.Exit{IP: intel.IP, Country: country, Network: network, ASN: formatDisplayASN(intel.ASN)}
	switch {
	case intel.IsVPN || intel.IsProxy || intel.IsTor:
		exit.Type = "proxy"
	case intel.IsHosting || intel.InferUsageType() == "hosting":
		exit.Type = "hosting"
	case intel.IsMobile || intel.UsageTypeRaw == "mobile" || intel.UsageTypeRaw == "residential" || intel.InferUsageType() == "isp":
		exit.Type = "access"
	}
	return exit
}

func (s *DNSService) RecordDNSQuery(testID, resolverIP string) error {
	value, ok := s.testCache.Load(testID)
	if !ok {
		return ErrDNSTestNotFound
	}
	state := value.(*dnsTestState)

	geo := s.getGeoInfo(resolverIP)
	state.mu.Lock()
	defer state.mu.Unlock()
	test := &state.test
	if state.expired || time.Now().After(test.CreatedAt.Add(s.dnsCfg.TTL)) {
		return ErrDNSTestNotFound
	}
	test.DNSQueries = append(test.DNSQueries, model.DNSQuery{
		IP:          resolverIP,
		Country:     geo.Country,
		ISP:         geo.ISP,
		ASN:         geo.ASN,
		NetworkType: geo.NetworkType,
		QueriedAt:   time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func (s *DNSService) GetLeakResult(ctx context.Context, testID string) (*model.DNSLeakResult, error) {
	value, ok := s.testCache.Load(testID)
	if !ok {
		return nil, ErrDNSTestNotFound
	}
	state := value.(*dnsTestState)
	state.mu.Lock()
	defer state.mu.Unlock()
	test := &state.test
	if state.expired || time.Now().After(test.CreatedAt.Add(s.dnsCfg.TTL)) {
		return nil, ErrDNSTestNotFound
	}
	result := s.analyzeLeakage(test)

	if len(result.DNSServers) > 0 && !test.Journaled {
		var err error
		if s.scan != nil {
			_, err = s.scan.UpdateDNSObservation(test.ScanInstanceID, test.ScanID, result)
		} else {
			err = s.journal.DNSLeakEvent(test.ScanInstanceID, test.ScanID, result, nil)
		}
		if err != nil {
			return nil, fmt.Errorf("保存 DNS 观测结果失败：%w", err)
		}
		test.Journaled = true
	}
	return result, nil
}

func (s *DNSService) analyzeLeakage(test *model.DNSLeakTest) *model.DNSLeakResult {
	result := &model.DNSLeakResult{
		ID:              fmt.Sprintf("leak_dns_%s", test.TestID[:8]),
		DNSServers:      []model.DNSQuery{},
		ActualCountries: []string{},
		ExpectedCountry: normalizeDNSCountry(test.ExpectedCountry),
	}

	if len(test.DNSQueries) == 0 {
		result.Level = "unknown"
		result.Assessment = &dnsassessment.Verdict{PolicyVersion: dnsassessment.PolicyVersion, State: "unobserved", Reason: "no-samples"}
		result.Recommendation = "无 DNS 结果"
		return result
	}

	unique := make(map[string]model.DNSQuery)
	for _, q := range test.DNSQueries {
		if existing, ok := unique[q.IP]; ok {
			if q.Country != "Unknown" && q.Country != "" && (existing.Country == "Unknown" || existing.Country == "") {
				unique[q.IP] = q
			}
			continue
		}
		unique[q.IP] = q
	}

	countries := make(map[string]int)
	for _, q := range unique {
		result.DNSServers = append(result.DNSServers, q)
		if country := normalizeDNSCountry(q.Country); country != "" {
			countries[country]++
		}
	}
	sort.Slice(result.DNSServers, func(i, j int) bool {
		return result.DNSServers[i].IP < result.DNSServers[j].IP
	})
	for c := range countries {
		result.ActualCountries = append(result.ActualCountries, c)
	}
	sort.Strings(result.ActualCountries)

	resolvers := make([]dnsassessment.Resolver, 0, len(result.DNSServers))
	for _, q := range result.DNSServers {
		resolvers = append(resolvers, dnsassessment.Resolver{
			IP: q.IP, Country: q.Country, ISP: q.ISP, ASN: q.ASN, NetworkType: q.NetworkType,
		})
	}
	assessment := dnsassessment.Evaluate(dnsassessment.Input{
		Exit: test.Exit, Resolvers: resolvers, ObservedRounds: 1, TotalRounds: 1,
	}).Verdict
	result.Assessment = &assessment
	switch assessment.State {
	case "suspected":
		result.Level = "danger"
		result.Leaked = true
		result.Recommendation = "泄露"
	case "clear":
		result.Level = "safe"
		result.Recommendation = "未泄露"
	default:
		result.Level = "unknown"
		result.Recommendation = "无法判断"
	}

	return result
}

func normalizeDNSCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 || country == "ZZ" || country == "XX" ||
		country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		return ""
	}
	return country
}

func (s *DNSService) getGeoInfo(ip string) geoInfo {
	if cached, ok := s.geoCache.Load(ip); ok {
		return cached.(geoInfo)
	}

	geo := geoInfo{Country: "Unknown", ISP: "Unknown"}
	if s.ipIntel != nil {
		intel, err := s.ipIntel.Lookup(context.Background(), ip, LookupOptions{LocalOnly: true})
		if err == nil && intel != nil {
			if bucket := geoBucket(intel); bucket != "" {
				geo.Country = bucket
			} else if intel.Country != "" {
				geo.Country = intel.Country
			}
			if intel.ISP != "" {
				geo.ISP = intel.ISP
			} else if intel.Org != "" {
				geo.ISP = intel.Org
			}
			geo.ASN = formatDisplayASN(intel.ASN)
			switch {
			case intel.IsHosting || intel.InferUsageType() == "hosting":
				geo.NetworkType = "hosting"
			case intel.IsMobile || intel.UsageTypeRaw == "residential" || intel.UsageTypeRaw == "mobile" || intel.InferUsageType() == "isp":
				geo.NetworkType = "access"
			}
		}
	}

	if s.geoCount.Add(1) > geoCacheMax {
		s.geoCache.Range(func(k, _ any) bool { s.geoCache.Delete(k); return true })
		s.geoCount.Store(0)
	}
	s.geoCache.Store(ip, geo)
	return geo
}

func (s *DNSService) generateTestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *DNSService) cleanupExpiredTests() {
	defer close(s.done)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			s.expireTests(now)
		case <-s.stop:
			return
		}
	}
}

func (s *DNSService) expireTests(now time.Time) {
	s.testCache.Range(func(key, value any) bool {
		state := value.(*dnsTestState)
		state.mu.Lock()
		defer state.mu.Unlock()
		test := &state.test
		if key != test.TestID || state.expired || !now.After(test.CreatedAt.Add(s.dnsCfg.TTL)) {
			return true
		}
		if !test.Journaled {
			if err := s.journal.DNSLifecycle("dns_expired", test.ScanInstanceID, test.ScanID, fmt.Sprintf("leak_dns_%s", test.TestID[:8])); err != nil {
				log.Printf("[DNS] 保存测试 %s 的过期事件失败，将重试：%v", test.TestID[:8], err)
				return true
			}
		}
		state.expired = true
		s.testCache.CompareAndDelete(test.TestID, state)
		s.testCache.CompareAndDelete(test.TestID[:8], state)
		return true
	})
}
