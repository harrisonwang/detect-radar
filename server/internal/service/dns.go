package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"detect-radar/internal/config"
	"detect-radar/internal/model"
)

// geoCacheMax resolver 地理信息缓存条目上限，超过则整体清空，防止长时间运行内存无限增长
const geoCacheMax = 20000

// DNSService 管理 DNS 泄露测试：分配唯一子域，接收 resolver 查询记录，分析泄露
//
// 数据流：client 触发 r{rand}-{testID}.{domain} → 递归 resolver 向权威 NS 查询
//
//	→ DNSTap/权威 NS 记录 resolver 出口 IP → RecordDNSQuery → 分析
type DNSService struct {
	dnsCfg    config.DNSConfig
	ipIntel   *IPIntelService
	journal   *Journal
	scan      *ScanService // DNS 定案后重算已存扫描（可空；未注入时只记 resolver 观测）
	testCache sync.Map     // testID -> *model.DNSLeakTest
	geoCache  sync.Map     // resolverIP -> geoInfo
	geoCount  atomic.Int64 // geoCache 近似条目数（用于有界淘汰）
}

type geoInfo struct {
	Country string
	ISP     string
}

func NewDNSService(dnsCfg config.DNSConfig, ipIntel *IPIntelService, journal *Journal) *DNSService {
	s := &DNSService{dnsCfg: dnsCfg, ipIntel: ipIntel, journal: journal}
	go s.cleanupExpiredTests()
	return s
}

// SetScanService 注入扫描服务：DNS 观测到位后据此重算已存扫描，拿到 post-DNS 判定写入遥测。
// 用 setter 而非构造参数，避开 ScanService 与 DNSService 的创建先后依赖。
func (s *DNSService) SetScanService(scan *ScanService) {
	s.scan = scan
}

// CreateLeakTest 生成一次测试，返回测试域名（客户端据此触发 {0-9}.{domain} 查询）
func (s *DNSService) CreateLeakTest(ctx context.Context, scanID string) (*model.DNSLeakTestResponse, error) {
	fullID := s.generateTestID()
	shortID := fullID[:8]
	testDomain := fmt.Sprintf("%s.%s", fullID, s.dnsCfg.Domain)
	now := time.Now()
	expiresAt := now.Add(s.dnsCfg.TTL)

	test := &model.DNSLeakTest{
		TestID:     fullID,
		TestDomain: testDomain,
		ScanID:     scanID,
		ExpiresAt:  expiresAt.Unix(),
		CreatedAt:  now,
		DNSQueries: []model.DNSQuery{},
	}

	// 完整 ID 与短 ID 都作为 key，方便 DNSTap 与客户端两侧查询
	s.testCache.Store(fullID, test)
	s.testCache.Store(shortID, test)

	return &model.DNSLeakTestResponse{
		ID:         fmt.Sprintf("leak_dns_%s", shortID),
		TestDomain: testDomain,
		CreatedAt:  now.UTC().Format(time.RFC3339),
		ExpiresAt:  expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

// RecordDNSQuery 由 DNSTap（或权威 NS）在观测到某 testID 的查询时调用
func (s *DNSService) RecordDNSQuery(testID, resolverIP string) error {
	value, ok := s.testCache.Load(testID)
	if !ok {
		return fmt.Errorf("test not found or expired")
	}
	test := value.(*model.DNSLeakTest)

	country, isp := s.getGeoInfo(resolverIP)
	test.DNSQueries = append(test.DNSQueries, model.DNSQuery{
		IP:        resolverIP,
		Country:   country,
		ISP:       isp,
		QueriedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

// GetLeakResult 查询并分析某测试的泄露情况
func (s *DNSService) GetLeakResult(ctx context.Context, testID string) (*model.DNSLeakResult, error) {
	value, ok := s.testCache.Load(testID)
	if !ok {
		return nil, fmt.Errorf("test not found or expired")
	}
	test := value.(*model.DNSLeakTest)
	result := s.analyzeLeakage(test)

	// 观测到 resolver 后写一次遥测流水（客户端轮询多次，只记首次有结果的）
	if len(result.DNSServers) > 0 && !test.Journaled {
		test.Journaled = true
		// DNS 定案：以本次 leaked 结果重算已存扫描，拿到 post-DNS 判定一并落库（评分始终纯后端产出）
		var resp *model.ScanResponse
		if s.scan != nil && test.ScanID != "" {
			if r, ok := s.scan.UpdateDNSLeak(test.ScanID, result.Leaked); ok {
				resp = r
			}
		}
		s.journal.DNSLeakEvent(test.ScanID, result, resp)
	}
	return result, nil
}

func (s *DNSService) analyzeLeakage(test *model.DNSLeakTest) *model.DNSLeakResult {
	result := &model.DNSLeakResult{
		ID:              fmt.Sprintf("leak_dns_%s", test.TestID[:8]),
		DNSServers:      []model.DNSQuery{},
		ActualCountries: []string{},
	}

	if len(test.DNSQueries) == 0 {
		result.Leaked = false
		result.Level = "safe"
		result.Recommendation = "未检测到 DNS 查询，可能查询尚未到达或已被缓存"
		return result
	}

	// 按 resolver IP 去重（保留信息更全的一条）
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
	isps := make(map[string]int)
	hasPublicDNS, hasLocalISP := false, false
	for _, q := range unique {
		result.DNSServers = append(result.DNSServers, q)
		if q.Country != "" && q.Country != "Unknown" {
			countries[q.Country]++
		}
		if q.ISP != "" && q.ISP != "Unknown" {
			isps[q.ISP]++
		}
		if isPublicDNSProvider(q.ISP) {
			hasPublicDNS = true
		} else if q.ISP != "" && q.ISP != "Unknown" {
			hasLocalISP = true
		}
	}
	for c := range countries {
		result.ActualCountries = append(result.ActualCountries, c)
	}

	numCountries := len(countries)
	numISPs := len(isps)

	switch {
	case hasPublicDNS && hasLocalISP:
		result.Leaked = true
		result.Level = "danger"
		result.Recommendation = "检测到公共 DNS 与本地 ISP DNS 混用，存在严重 DNS 泄露"
	case numCountries > 2:
		result.Leaked = true
		result.Level = "danger"
		result.Recommendation = fmt.Sprintf("DNS 解析器分布在 %d 个国家，存在严重 DNS 泄露", numCountries)
	case numCountries > 1 || numISPs > 2:
		result.Leaked = true
		result.Level = "warning"
		result.Recommendation = "DNS 解析器来自多个国家或 ISP，建议检查 DNS 配置"
	default:
		result.Leaked = false
		result.Level = "safe"
		result.Recommendation = "DNS 配置正常，未检测到泄露"
	}

	return result
}

// getGeoInfo 复用本项目的 IP 信息服务解析 resolver 出口 IP 的国家/ISP
func (s *DNSService) getGeoInfo(ip string) (country, isp string) {
	if cached, ok := s.geoCache.Load(ip); ok {
		g := cached.(geoInfo)
		return g.Country, g.ISP
	}

	country, isp = "Unknown", "Unknown"
	if s.ipIntel != nil {
		intel, err := s.ipIntel.Lookup(context.Background(), ip, LookupOptions{DeepScan: false})
		if err == nil && intel != nil {
			if intel.Country != "" {
				country = intel.Country
			}
			if intel.ISP != "" {
				isp = intel.ISP
			} else if intel.Org != "" {
				isp = intel.Org
			}
		}
	}

	// 有界缓存：超过上限直接清空重建，避免长期运行内存无限增长
	if s.geoCount.Add(1) > geoCacheMax {
		s.geoCache.Range(func(k, _ any) bool { s.geoCache.Delete(k); return true })
		s.geoCount.Store(0)
	}
	s.geoCache.Store(ip, geoInfo{Country: country, ISP: isp})
	return country, isp
}

func (s *DNSService) generateTestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *DNSService) cleanupExpiredTests() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.testCache.Range(func(key, value any) bool {
			if now.Sub(value.(*model.DNSLeakTest).CreatedAt) > s.dnsCfg.TTL {
				s.testCache.Delete(key)
			}
			return true
		})
	}
}

// isPublicDNSProvider 判断 ISP 名称是否属于常见公共 DNS 提供商
func isPublicDNSProvider(isp string) bool {
	known := map[string]bool{
		"Cloudflare": true, "CloudFlare": true, "CLOUDFLARENET": true,
		"Cloudflare, Inc.": true, "Google": true, "GOOGLE": true,
		"Google LLC": true, "Quad9": true, "OpenDNS": true,
	}
	return known[isp]
}
