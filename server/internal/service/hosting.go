package service

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"

	"detect-radar/internal/repository"
)

// 家宽 rDNS 特征关键词（用于洗白）
// 注意：只保留明确的家宽特征词，避免与 VPS rDNS 重叠
// 例如 "host" 不安全，因为 VPS 也可能有 "host-by.dmit.com" 这样的 rDNS
var residentialRDNSKeywords = []string{
	// 动态分配标识（最可靠）
	"dynamic", "dyn", "dhcp", "ppp", "pppoe",
	// 接入类型（可靠）
	"dsl", "adsl", "vdsl", "fiber", "fios", "ftth", "fttb",
	"cable", "catv", "docsis",
	// 地址池标识（可靠）
	"pool", "dial", "dialup",
	// 用户端设备（可靠）
	"cpe", "modem", "router",
	// 明确的家宽/用户标识（可靠）
	"residential", "home", "customer", "hsd",
	// 移动网络（可靠）
	"mobile", "3g", "4g", "5g", "lte", "gprs",
	// ISP 宽带标识（可靠）
	"broadband",
}

// ISP 常见名称关键词（辅助判断）
var ispNameKeywords = []string{
	"telecom", "telecommunication", "telekom",
	"mobile", "wireless",
	"broadband", "cable", "fiber",
	"isp", "internet service",
	"communications",
}

// HostingResult Hosting 检测结果
type HostingResult struct {
	IP            string   `json:"ip"`
	ASN           uint64   `json:"asn"`
	ASNStr        string   `json:"asn_str"` // 如 "AS15169"
	Org           string   `json:"org"`
	AllASNs       []string `json:"all_asns,omitempty"` // 所有关联的 ASN（从 Cymru 获取）
	IsHosting     bool     `json:"is_hosting"`
	IsResidential bool     `json:"is_residential"`      // 是否为家宽（通过 rDNS 确认）- 内部使用，传递给 ipintel 后转为 UsageTypeRaw
	IsUnknown     bool     `json:"is_unknown"`          // 无法确定（灰区）
	ASNStale      bool     `json:"asn_stale,omitempty"` // 本地 ASN 库对该网段已过期（MMDB ASN 不在当前 BGP 宣告中，网段易主/租赁后重新宣告）
	Provider      string   `json:"provider,omitempty"`  // 如果是已知厂商，返回厂商名
	Confidence    int      `json:"confidence"`          // 置信度 0-100
	Method        string   `json:"method,omitempty"`    // 检测方法
	RDNS          string   `json:"rdns,omitempty"`      // rDNS 记录（调试用）
	NeedsDeepScan bool     `json:"needs_deep_scan"`     // 是否建议深度扫描
}

// ASNRecord MMDB 中的 ASN 记录结构
type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

// HostingDetector Hosting/Datacenter IP 检测器
// 检测流程：
// 1. SQLite 云厂商 IP 范围查询
// 2. MMDB 查询 ASN 信息（离线快照，仅作基线）
// 3. Team Cymru 实时 BGP 校准 + ASN 黑名单匹配（Cymru 不可用时退回 MMDB ASN 匹配）
// 4. rDNS 洗白检测（识别动态家宽）
// 5. SQLite 关键词 Token 切分匹配（本地库过期时跳过）
type HostingDetector struct {
	asnDB  *maxminddb.Reader
	mu     sync.RWMutex
	dbPath string

	// SQLite 仓库
	cloudIPRepo  *repository.CloudIPRangeRepository
	providerRepo *repository.HostingProviderRepository
	keywordRepo  *repository.HostingKeywordRepository

	// 可注入的查询函数（单元测试替换用），构造时指向真实实现
	lookupASNRecord func(ip net.IP) (ASNRecord, error)
	lookupCymru     func(ipStr string) ([]CymruASNInfo, error)
	lookupRDNS      func(ipStr string) (string, bool)
}

// NewHostingDetector 创建 Hosting 检测器
func NewHostingDetector(asnDBPath string, cloudIPRepo *repository.CloudIPRangeRepository, providerRepo *repository.HostingProviderRepository, keywordRepo *repository.HostingKeywordRepository) (*HostingDetector, error) {
	db, err := maxminddb.Open(asnDBPath)
	if err != nil {
		return nil, fmt.Errorf("无法打开 ASN 数据库: %w", err)
	}

	// 加载关键词到内存
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
	}
	d.lookupASNRecord = d.lookupASNFromMMDB
	d.lookupCymru = d.lookupCymruASNs
	d.lookupRDNS = d.checkResidentialByRDNS

	return d, nil
}

// Close 关闭数据库连接
func (d *HostingDetector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.asnDB != nil {
		return d.asnDB.Close()
	}
	return nil
}

// Detect 检测 IP 是否为 Hosting/Datacenter IP
// 优化后的检测流程（低误判、高置信）：
//  1. 云厂商 CIDR 查询（实锤）
//  2. MMDB 查询 ASN（离线快照，网段易主/租赁后重新宣告时会过期）
//  3. Team Cymru 实时 BGP 查询：校准 ASN、标记本地库过期（ASNStale），
//     任一当前宣告 ASN 命中黑名单即实锤；Cymru 不可用时退回 MMDB ASN 精确匹配
//  4. rDNS 洗白检测（识别家宽）
//  5. 关键词匹配（收紧阈值；ASNStale 时跳过，MMDB org 已不可信）
//  6. 灰区处理（标记 Unknown）
func (d *HostingDetector) Detect(ipStr string) (*HostingResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	result := &HostingResult{
		IP: ipStr,
	}

	// === Step 1: 云厂商 CIDR 查询（最准，100% 实锤） ===
	if d.cloudIPRepo != nil {
		cloudRange, err := d.cloudIPRepo.FindByIP(ip)
		if err == nil && cloudRange != nil {
			result.IsHosting = true
			result.Provider = cloudRange.Provider
			result.Confidence = 100
			result.Method = "cloud_ip_range"
			return result, nil
		}
	}

	// === Step 2: MMDB 查询 ASN 信息（离线快照，仅作基线） ===
	record, err := d.lookupASNRecord(ip)
	if err != nil {
		return nil, fmt.Errorf("ASN 查询失败: %w", err)
	}

	result.ASN = record.ASNumber
	result.ASNStr = fmt.Sprintf("AS%d", record.ASNumber)
	result.Org = record.ASOrg

	// === Step 3: Team Cymru 实时 BGP 查询（以当前宣告为准） ===
	// MMDB 是离线快照，网段易主/租赁后重新宣告时会过期（例如家宽段租给机房商），
	// 因此 ASN 黑名单以实时宣告为准，MMDB 仅在 Cymru 不可用时兜底
	cymruASNs, cymruErr := d.lookupCymru(ipStr)
	if cymruErr == nil && len(cymruASNs) > 0 {
		// 保存所有 ASN 到结果（调试用）
		for _, ca := range cymruASNs {
			result.AllASNs = append(result.AllASNs, fmt.Sprintf("AS%d(%s)", ca.ASN, ca.CIDR))
		}

		// MMDB 的 ASN 不在任何当前宣告中 → 本地库对该网段已过期
		result.ASNStale = !containsASN(cymruASNs, record.ASNumber)

		// 更新主 ASN 为最具体的路由（第一个通常是最具体的）
		if cymruASNs[0].ASN != record.ASNumber {
			result.ASN = cymruASNs[0].ASN
			result.ASNStr = fmt.Sprintf("AS%d", cymruASNs[0].ASN)
			if result.ASNStale {
				// MMDB 的 org 属于已过期的 ASN，不可信
				result.Org = ""
			}
		}

		// 检查所有当前宣告 ASN 是否有任一在黑名单中
		if provider, matchedASN := d.checkASNsInBlacklist(cymruASNs); provider != nil {
			result.IsHosting = true
			result.Provider = provider.Name
			result.Confidence = provider.Confidence
			if result.Org == "" {
				result.Org = provider.Name
			}
			switch matchedASN.ASN {
			case record.ASNumber:
				// 命中的就是 MMDB 记录的 ASN，与本地精确匹配等价
				result.Method = "asn_match"
			case result.ASN:
				// 命中 Cymru 校准后的主 ASN（MMDB 已过期或路由更具体）
				result.Method = "asn_match_cymru"
			default:
				// 通过父级 ASN 匹配
				result.Method = fmt.Sprintf("asn_match_parent(AS%d)", matchedASN.ASN)
			}
			return result, nil
		}
	} else if d.providerRepo != nil {
		// Cymru 不可用：退回 MMDB ASN 黑名单精确匹配（无法校验本地库是否过期）
		provider, err := d.providerRepo.FindByASN(record.ASNumber)
		if err == nil && provider != nil {
			result.IsHosting = true
			result.Provider = provider.Name
			result.Confidence = provider.Confidence
			result.Method = "asn_match"
			return result, nil
		}
	}

	// === Step 3.1: 按规范化名称匹配（MMDB org 未过期时才可信） ===
	if d.providerRepo != nil && !result.ASNStale {
		provider, err := d.providerRepo.FindByNormalizedName(record.ASOrg)
		if err == nil && provider != nil {
			result.IsHosting = true
			result.Provider = provider.Name
			result.Confidence = 90
			result.Method = "name_match"
			return result, nil
		}
	}

	// === Step 4: rDNS 洗白检测（识别动态家宽，实时数据不受本地库过期影响） ===
	rdns, isResidential := d.lookupRDNS(ipStr)
	result.RDNS = rdns

	if isResidential {
		// rDNS 明确显示是动态家宽，直接洗白
		result.IsHosting = false
		result.IsResidential = true
		result.Confidence = 90
		result.Method = "rdns_residential"
		return result, nil
	}

	// === Step 4.5: 本地库过期且无任何实锤 → 灰区，交给 L3 修正 ===
	// record.ASOrg 属于已过期的 ASN，后续基于 org 的关键词/ISP 名称启发不再可用
	if result.ASNStale {
		result.IsHosting = false
		result.IsUnknown = true
		result.Confidence = 50
		result.Method = "asn_stale"
		result.NeedsDeepScan = true
		return result, nil
	}

	// === Step 5: 关键词匹配（提高阈值到 70，避免误伤） ===
	if d.keywordRepo != nil {
		match := d.keywordRepo.GetBestMatch(record.ASOrg)
		// 阈值从 50 提高到 70，只有高置信关键词才判定
		if match != nil && match.Weight >= 70 {
			result.IsHosting = true
			result.Provider = match.Provider
			result.Confidence = match.Weight
			result.Method = "keyword_match"
			return result, nil
		}
	}

	// === Step 6: 灰区处理 ===
	// 到这里说明：
	// - 不在云厂商 CIDR
	// - 不在 ASN 黑名单
	// - rDNS 没有家宽特征
	// - 关键词匹配不够置信
	// 可能是：静态家宽、小型 ISP、企业专线、或者伪装的机房

	// 检查 ASN 名称是否像 ISP
	isLikelyISP := d.isLikelyISPByName(record.ASOrg)

	if isLikelyISP {
		// ASN 名称像 ISP，但 rDNS 没有动态特征
		// 可能是静态住宅或企业专线，标记为低风险但建议深度扫描
		result.IsHosting = false
		result.IsUnknown = true
		result.Confidence = 60
		result.Method = "isp_static"
		result.NeedsDeepScan = true
		return result, nil
	}

	// 完全无法判断
	result.IsHosting = false
	result.IsUnknown = true
	result.Confidence = 50
	result.Method = "unknown"
	result.NeedsDeepScan = true
	return result, nil
}

// lookupASNFromMMDB 从本地 MMDB 查询 ASN 记录（lookupASNRecord 的默认实现）
func (d *HostingDetector) lookupASNFromMMDB(ip net.IP) (ASNRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var record ASNRecord
	err := d.asnDB.Lookup(ip, &record)
	return record, err
}

// checkResidentialByRDNS 通过 rDNS 检测是否为动态家宽
// 返回 rDNS 记录和是否为家宽
// 注意：只依赖明确的家宽关键词，不使用 IP 编码检测（因为 VPS 也可能有类似 rDNS）
func (d *HostingDetector) checkResidentialByRDNS(ip string) (string, bool) {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return "", false
	}

	rdns := strings.ToLower(names[0])

	// 检查是否包含家宽特征关键词
	for _, kw := range residentialRDNSKeywords {
		if strings.Contains(rdns, kw) {
			return rdns, true
		}
	}

	return rdns, false
}

// isLikelyISPByName 根据 ASN 名称判断是否像 ISP
func (d *HostingDetector) isLikelyISPByName(org string) bool {
	orgLower := strings.ToLower(org)

	for _, kw := range ispNameKeywords {
		if strings.Contains(orgLower, kw) {
			return true
		}
	}

	return false
}

// DetectBatch 批量检测 IP
func (d *HostingDetector) DetectBatch(ips []string) ([]*HostingResult, error) {
	results := make([]*HostingResult, 0, len(ips))
	for _, ip := range ips {
		result, err := d.Detect(ip)
		if err != nil {
			// 单个 IP 失败不影响其他
			results = append(results, &HostingResult{
				IP:         ip,
				IsHosting:  false,
				Confidence: 0,
				Method:     "error",
			})
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// LookupASN 仅查询 ASN 信息（不做 Hosting 判断）
func (d *HostingDetector) LookupASN(ipStr string) (*ASNRecord, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var record ASNRecord
	err := d.asnDB.Lookup(ip, &record)
	if err != nil {
		return nil, fmt.Errorf("ASN 查询失败: %w", err)
	}

	return &record, nil
}

// GetStats 获取检测器统计信息
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

// CymruASNInfo Team Cymru 返回的 ASN 信息
type CymruASNInfo struct {
	ASN    uint64
	CIDR   string
	CC     string // Country Code
	RIR    string // 区域注册机构
	Date   string // 分配日期
	OrgStr string // ASN 描述（从第二次查询获取）
}

// lookupCymruASNs 使用 Team Cymru DNS 查询 IP 对应的所有 ASN
// 返回所有匹配的 ASN（从最具体到最宽泛的路由）
// 参考: https://www.team-cymru.com/ip-asn-mapping
func (d *HostingDetector) lookupCymruASNs(ipStr string) ([]CymruASNInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	// 只支持 IPv4（IPv6 需要不同的格式）
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("暂不支持 IPv6 Cymru 查询")
	}

	// 构造反向 DNS 查询: 4.3.2.1.origin.asn.cymru.com
	reversedIP := fmt.Sprintf("%d.%d.%d.%d", ip4[3], ip4[2], ip4[1], ip4[0])
	query := fmt.Sprintf("%s.origin.asn.cymru.com", reversedIP)

	// 执行 DNS TXT 查询
	txtRecords, err := net.LookupTXT(query)
	if err != nil {
		return nil, fmt.Errorf("Cymru DNS 查询失败: %w", err)
	}

	var results []CymruASNInfo

	for _, txt := range txtRecords {
		// 解析格式: "3258 | 172.93.220.0/23 | US | arin | 2015-06-01"
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

// containsASN 判断 ASN 是否出现在 Cymru 返回的当前宣告列表中
func containsASN(asns []CymruASNInfo, asn uint64) bool {
	for i := range asns {
		if asns[i].ASN == asn {
			return true
		}
	}
	return false
}

// checkASNsInBlacklist 检查多个 ASN 中是否有任一在黑名单中
// 返回第一个匹配的 provider 和对应的 ASN
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
