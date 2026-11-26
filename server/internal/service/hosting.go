package service

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"detect-radar/internal/repository"
)

const (
	hostingDNSTimeout  = 2 * time.Second
	cymruCacheTTL      = 6 * time.Hour
	cymruErrCacheTTL   = 5 * time.Minute
	rdnsCacheTTL       = 6 * time.Hour
	rdnsEmptyCacheTTL  = 1 * time.Hour
	hostingDNSCacheMax = 10000
)

type cymruCacheEntry struct {
	asns      []CymruASNInfo
	err       error
	expiresAt time.Time
}

type rdnsCacheEntry struct {
	rdns          string
	isResidential bool
	expiresAt     time.Time
}

var residentialRDNSKeywords = []string{
	"dynamic", "dyn", "dhcp", "ppp", "pppoe",
	"dsl", "adsl", "vdsl", "fiber", "fios", "ftth", "fttb",
	"cable", "catv", "docsis",
	"pool", "dial", "dialup",
	"cpe", "modem", "router",
	"residential", "home", "customer", "hsd",
	"mobile", "3g", "4g", "5g", "lte", "gprs",
	"broadband",
}

var ispNameKeywords = []string{
	"telecom", "telecommunication", "telekom",
	"mobile", "wireless",
	"broadband", "cable", "fiber",
	"isp", "internet service",
	"communications",
}

type HostingResult struct {
	IP            string `json:"ip"`
	ASN           uint64 `json:"asn"`
	ASNStr        string `json:"asn_str"`
	Org           string `json:"org"`
	IsHosting     bool   `json:"is_hosting"`
	IsResidential bool   `json:"is_residential"`
	ASNStale      bool   `json:"asn_stale,omitempty"`
	Confidence    int    `json:"confidence"`
	Method        string `json:"method,omitempty"`
	RDNS          string `json:"rdns,omitempty"`
	NeedsDeepScan bool   `json:"needs_deep_scan"`
}

type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

type HostingDetector struct {
	asnDB  *maxminddb.Reader
	mu     sync.RWMutex
	dbPath string

	cloudIPRepo  *repository.CloudIPRangeRepository
	providerRepo *repository.HostingProviderRepository
	keywordRepo  *repository.HostingKeywordRepository

	lookupASNRecord func(ip net.IP) (ASNRecord, error)
	lookupCymru     func(ipStr string) ([]CymruASNInfo, error)
	lookupRDNS      func(ipStr string) (string, bool)

	dnsCacheMu sync.RWMutex
	cymruCache map[string]cymruCacheEntry
	rdnsCache  map[string]rdnsCacheEntry
	nowFunc    func() time.Time
}

func NewHostingDetector(asnDBPath string, cloudIPRepo *repository.CloudIPRangeRepository, providerRepo *repository.HostingProviderRepository, keywordRepo *repository.HostingKeywordRepository) (*HostingDetector, error) {
	db, err := maxminddb.Open(asnDBPath)
	if err != nil {
		return nil, fmt.Errorf("无法打开 ASN 数据库: %w", err)
	}

	if keywordRepo != nil {
		if err := keywordRepo.LoadAll(); err != nil {
			db.Close()
			return nil, fmt.Errorf("无法加载关键词: %w", err)
		}
	}

	d := &HostingDetector{
		asnDB:        db,
		dbPath:       asnDBPath,
		cloudIPRepo:  cloudIPRepo,
		providerRepo: providerRepo,
		keywordRepo:  keywordRepo,
		cymruCache:   make(map[string]cymruCacheEntry),
		rdnsCache:    make(map[string]rdnsCacheEntry),
		nowFunc:      time.Now,
	}
	d.lookupASNRecord = d.lookupASNFromMMDB
	d.lookupCymru = d.lookupCymruASNs
	d.lookupRDNS = d.checkResidentialByRDNS

	return d, nil
}

func (d *HostingDetector) now() time.Time {
	if d.nowFunc != nil {
		return d.nowFunc()
	}
	return time.Now()
}

func (d *HostingDetector) cachedLookupCymru(ipStr string) ([]CymruASNInfo, error) {
	if d.cymruCache == nil {
		return d.lookupCymru(ipStr)
	}
	now := d.now()
	d.dnsCacheMu.RLock()
	entry, ok := d.cymruCache[ipStr]
	d.dnsCacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		if len(entry.asns) == 0 {
			return nil, entry.err
		}
		cp := make([]CymruASNInfo, len(entry.asns))
		copy(cp, entry.asns)
		return cp, entry.err
	}

	asns, err := d.lookupCymru(ipStr)
	ttl := cymruCacheTTL
	if err != nil {
		ttl = cymruErrCacheTTL
	}
	var stored []CymruASNInfo
	if len(asns) > 0 {
		stored = make([]CymruASNInfo, len(asns))
		copy(stored, asns)
	}

	d.dnsCacheMu.Lock()
	if len(d.cymruCache) >= hostingDNSCacheMax {
		d.cymruCache = make(map[string]cymruCacheEntry)
	}
	d.cymruCache[ipStr] = cymruCacheEntry{
		asns:      stored,
		err:       err,
		expiresAt: now.Add(ttl),
	}
	d.dnsCacheMu.Unlock()

	return asns, err
}

func (d *HostingDetector) cachedLookupRDNS(ipStr string) (string, bool) {
	if d.rdnsCache == nil {
		return d.lookupRDNS(ipStr)
	}
	now := d.now()
	d.dnsCacheMu.RLock()
	entry, ok := d.rdnsCache[ipStr]
	d.dnsCacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.rdns, entry.isResidential
	}

	rdns, isResidential := d.lookupRDNS(ipStr)
	ttl := rdnsCacheTTL
	if rdns == "" {
		ttl = rdnsEmptyCacheTTL
	}

	d.dnsCacheMu.Lock()
	if len(d.rdnsCache) >= hostingDNSCacheMax {
		d.rdnsCache = make(map[string]rdnsCacheEntry)
	}
	d.rdnsCache[ipStr] = rdnsCacheEntry{
		rdns:          rdns,
		isResidential: isResidential,
		expiresAt:     now.Add(ttl),
	}
	d.dnsCacheMu.Unlock()

	return rdns, isResidential
}

func (d *HostingDetector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.asnDB != nil {
		return d.asnDB.Close()
	}
	return nil
}

func (d *HostingDetector) Detect(ipStr string) (*HostingResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	result := &HostingResult{
		IP: ipStr,
	}

	if d.cloudIPRepo != nil {
		cloudRange, err := d.cloudIPRepo.FindByIP(ip)
		if err == nil && cloudRange != nil {
			result.IsHosting = true
			result.Confidence = 100
			result.Method = "cloud_ip_range"
			return result, nil
		}
	}

	record, err := d.lookupASNRecord(ip)
	if err != nil {
		return nil, fmt.Errorf("ASN 查询失败: %w", err)
	}

	result.ASN = record.ASNumber
	result.ASNStr = fmt.Sprintf("AS%d", record.ASNumber)
	result.Org = record.ASOrg

	cymruASNs, cymruErr := d.cachedLookupCymru(ipStr)
	if cymruErr == nil && len(cymruASNs) > 0 {
		result.ASNStale = !containsASN(cymruASNs, record.ASNumber)

		if cymruASNs[0].ASN != record.ASNumber {
			result.ASN = cymruASNs[0].ASN
			result.ASNStr = fmt.Sprintf("AS%d", cymruASNs[0].ASN)
			if result.ASNStale {
				result.Org = ""
			}
		}

		if provider, matchedASN := d.checkASNsInBlacklist(cymruASNs); provider != nil {
			result.IsHosting = true
			result.Confidence = provider.Confidence
			if result.Org == "" {
				result.Org = provider.Name
			}
			switch matchedASN.ASN {
			case record.ASNumber:
				result.Method = "asn_match"
			case result.ASN:
				result.Method = "asn_match_cymru"
			default:
				result.Method = fmt.Sprintf("asn_match_parent(AS%d)", matchedASN.ASN)
			}
			return result, nil
		}
	} else if d.providerRepo != nil {
		provider, err := d.providerRepo.FindByASN(record.ASNumber)
		if err == nil && provider != nil {
			result.IsHosting = true
			result.Confidence = provider.Confidence
			result.Method = "asn_match"
			return result, nil
		}
	}

	if d.providerRepo != nil && !result.ASNStale {
		provider, err := d.providerRepo.FindByNormalizedName(record.ASOrg)
		if err == nil && provider != nil {
			result.IsHosting = true
			result.Confidence = 90
			result.Method = "name_match"
			return result, nil
		}
	}

	rdns, isResidential := d.cachedLookupRDNS(ipStr)
	result.RDNS = rdns

	if isResidential {
		result.IsHosting = false
		result.IsResidential = true
		result.Confidence = 90
		result.Method = "rdns_residential"
		return result, nil
	}

	if result.ASNStale {
		result.IsHosting = false
		result.Confidence = 50
		result.Method = "asn_stale"
		result.NeedsDeepScan = true
		return result, nil
	}

	if d.keywordRepo != nil {
		match := d.keywordRepo.GetBestMatch(record.ASOrg)
		if match != nil && match.Weight >= 70 {
			result.IsHosting = true
			result.Confidence = match.Weight
			result.Method = "keyword_match"
			return result, nil
		}
	}

	isLikelyISP := d.isLikelyISPByName(record.ASOrg)

	if isLikelyISP {
		result.IsHosting = false
		result.Confidence = 60
		result.Method = "isp_static"
		result.NeedsDeepScan = true
		return result, nil
	}

	result.IsHosting = false
	result.Confidence = 50
	result.Method = "unknown"
	result.NeedsDeepScan = true
	return result, nil
}

func (d *HostingDetector) lookupASNFromMMDB(ip net.IP) (ASNRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var record ASNRecord
	err := d.asnDB.Lookup(ip, &record)
	return record, err
}

func (d *HostingDetector) checkResidentialByRDNS(ip string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), hostingDNSTimeout)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return "", false
	}

	rdns := strings.ToLower(names[0])

	for _, kw := range residentialRDNSKeywords {
		if strings.Contains(rdns, kw) {
			return rdns, true
		}
	}

	return rdns, false
}

func (d *HostingDetector) isLikelyISPByName(org string) bool {
	orgLower := strings.ToLower(org)

	for _, kw := range ispNameKeywords {
		if strings.Contains(orgLower, kw) {
			return true
		}
	}

	return false
}

func (d *HostingDetector) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"db_path": d.dbPath,
	}

	if d.providerRepo != nil {
		count, _ := d.providerRepo.Count()
		stats["provider_count"] = count
	}

	if d.keywordRepo != nil {
		count, _ := d.keywordRepo.Count()
		stats["keyword_count"] = count
	}

	if d.cloudIPRepo != nil {
		count, _ := d.cloudIPRepo.Count()
		stats["cloud_ip_range_count"] = count
	}

	return stats
}

type CymruASNInfo struct {
	ASN  uint64
	CIDR string
	CC   string
	RIR  string
	Date string
}

func (d *HostingDetector) lookupCymruASNs(ipStr string) ([]CymruASNInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("暂不支持 IPv6 Cymru 查询")
	}

	reversedIP := fmt.Sprintf("%d.%d.%d.%d", ip4[3], ip4[2], ip4[1], ip4[0])
	query := fmt.Sprintf("%s.origin.asn.cymru.com", reversedIP)

	ctx, cancel := context.WithTimeout(context.Background(), hostingDNSTimeout)
	defer cancel()
	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("Cymru DNS 查询失败: %w", err)
	}

	var results []CymruASNInfo

	for _, txt := range txtRecords {
		parts := strings.Split(txt, "|")
		if len(parts) < 5 {
			continue
		}

		asnStr := strings.TrimSpace(parts[0])
		asn, err := strconv.ParseUint(asnStr, 10, 64)
		if err != nil {
			continue
		}

		info := CymruASNInfo{
			ASN:  asn,
			CIDR: strings.TrimSpace(parts[1]),
			CC:   strings.TrimSpace(parts[2]),
			RIR:  strings.TrimSpace(parts[3]),
			Date: strings.TrimSpace(parts[4]),
		}

		results = append(results, info)
	}

	return results, nil
}

func containsASN(asns []CymruASNInfo, asn uint64) bool {
	for i := range asns {
		if asns[i].ASN == asn {
			return true
		}
	}
	return false
}

func (d *HostingDetector) checkASNsInBlacklist(asns []CymruASNInfo) (*repository.HostingProvider, *CymruASNInfo) {
	if d.providerRepo == nil {
		return nil, nil
	}

	for i := range asns {
		provider, err := d.providerRepo.FindByASN(asns[i].ASN)
		if err == nil && provider != nil {
			return provider, &asns[i]
		}
	}

	return nil, nil
}
