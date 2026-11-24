package service

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"detect-radar/internal/adapter"
	"detect-radar/internal/cache"
	"detect-radar/internal/model"
)

// ipIntelCache 抽象 IPIntel 缓存与每日配额计数能力（生产为 *cache.RedisCache，单测可注入内存桩）
type ipIntelCache interface {
	GetBasic(ctx context.Context, ip string) (*model.IPIntel, error)
	GetDeep(ctx context.Context, ip string) (*model.IPIntel, error)
	SetBasic(ctx context.Context, ip string, intel *model.IPIntel) error
	SetDeep(ctx context.Context, ip string, intel *model.IPIntel) error
	Delete(ctx context.Context, ip string) error
	Clear(ctx context.Context) error
	IncrDailyCounter(ctx context.Context, name, day string, ttl time.Duration) (int64, error)
}

type inflightLookup struct {
	done  chan struct{}
	intel *model.IPIntel
	err   error
}

// IPIntelService IP 信息服务
//
// 三层数据源，按需组合，intel.Tier 如实记录本次查询实际命中了哪些层：
//
//	L1 本地 mmdb（MaxMind/IP2Location/dbip/ipinfo，离线快照，免费，网段易主时会过期）
//	L2 实时免费信号（HostingDetector，见 hosting.go：云厂商 CIDR / ASN 黑名单 /
//	   Team Cymru 实时 BGP 校准 / rDNS 洗白；带内存 TTL 缓存，不算 L1 也不算 L3）
//	L3 远程付费 API（ipapi.is / BigDataCloud / IPRegistry / IP2Location，按优先级遍历+回退，
//	   仅在显式深扫或允许自动深扫的灰区/过期网段触发，受每日全局熔断上限保护）
//
// 另有 Cloudflare Radar 提供 HumanRatio 作为补充信号，不参与分层、不计入 Tier。
type IPIntelService struct {
	registry        *adapter.Registry
	cache           ipIntelCache // Redis 缓存（可为 nil）
	cacheTTL        time.Duration
	hostingDetector *HostingDetector     // Hosting 检测器
	radarService    *RadarService        // Cloudflare Radar 服务
	cnGeo           *adapter.IP2RegionCN // CN 权威地理层（ip2region xdb，可选；nil=关闭）

	// L3 远程 API 每日全局熔断保护（<= 0 表示不限制）
	l3DailyLimit   int
	l3QuotaMu      sync.Mutex
	l3QuotaDay     string
	l3QuotaUsed    int
	l3QuotaWarnDay string

	// 同 IP 并发查冷缓存时的请求合并（防并发击穿上游）
	inflightMu sync.Mutex
	inflight   map[string]*inflightLookup

	nowFunc func() time.Time
}

// NewIPIntelService 创建 IP 信息服务
func NewIPIntelService(registry *adapter.Registry, redisCache *cache.RedisCache, hostingDetector *HostingDetector, radarService *RadarService, cnGeo *adapter.IP2RegionCN) *IPIntelService {
	var intelCache ipIntelCache
	if redisCache != nil {
		intelCache = redisCache
	}
	return &IPIntelService{
		registry:        registry,
		cache:           intelCache,
		cacheTTL:        24 * time.Hour,
		hostingDetector: hostingDetector,
		radarService:    radarService,
		cnGeo:           cnGeo,
		inflight:        make(map[string]*inflightLookup),
		nowFunc:         time.Now,
	}
}

// SetL3DailyLimit 设置全站每日 L3 远程 API 调用上限（<= 0 表示不限制）。
// 超限后自动降级返回 L1+L2 本地结果，避免打爆免费额度或中断检测。
func (s *IPIntelService) SetL3DailyLimit(limit int) {
	s.l3QuotaMu.Lock()
	s.l3DailyLimit = limit
	s.l3QuotaMu.Unlock()
}

func (s *IPIntelService) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

// LookupOptions 查询选项
type LookupOptions struct {
	DeepScan     bool   // 是否深度扫描（调用 L3 远程 API）
	LocalOnly    bool   // 仅使用本地 L1+L2（禁止自动/二次 L3 远程调用；查缓存时优先复用已有深扫结果）
	PreferSource string // 指定优先数据源
	SkipCache    bool   // 跳过缓存
}

// Lookup 查询 IP 信息
func (s *IPIntelService) Lookup(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, error) {
	// 1. 查缓存（如果启用）
	if !opts.SkipCache && s.cache != nil {
		if cached := s.getCache(ctx, ip, opts); cached != nil {
			return cloneIntel(cached), nil
		}
	}

	// 2. 冷缓存并发合并：同一 IP 同时到达多次请求时合并为一次底层查询
	if !opts.SkipCache && opts.PreferSource == "" {
		return s.lookupCoalesced(ctx, ip, opts)
	}

	return s.lookupUncached(ctx, ip, opts)
}

func (s *IPIntelService) lookupCoalesced(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, error) {
	key := "basic:" + ip
	if opts.DeepScan {
		key = "deep:" + ip
	} else if opts.LocalOnly {
		key = "local:" + ip
	}

	s.inflightMu.Lock()
	if s.inflight == nil {
		s.inflight = make(map[string]*inflightLookup)
	}
	if call, ok := s.inflight[key]; ok {
		s.inflightMu.Unlock()
		select {
		case <-call.done:
			if call.err != nil {
				return nil, call.err
			}
			return cloneIntel(call.intel), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &inflightLookup{done: make(chan struct{})}
	s.inflight[key] = call
	s.inflightMu.Unlock()

	// 领导者使用与单个调用解耦的超时上下文，避免首个请求取消拖累同 IP 并发等待者
	leaderCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	intel, err := s.lookupUncached(leaderCtx, ip, opts)
	call.intel = intel
	call.err = err
	close(call.done)

	s.inflightMu.Lock()
	delete(s.inflight, key)
	s.inflightMu.Unlock()

	if err != nil {
		return nil, err
	}
	return cloneIntel(intel), nil
}

func cloneIntel(src *model.IPIntel) *model.IPIntel {
	if src == nil {
		return nil
	}
	cp := *src
	if src.HumanRatio != nil {
		v := *src.HumanRatio
		cp.HumanRatio = &v
	}
	return &cp
}

func (s *IPIntelService) lookupUncached(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, error) {
	var intel *model.IPIntel
	var err error

	// 2. L1 本地查询
	if s.registry.HasLocal() {
		intel, err = s.registry.Local().Lookup(ctx, ip)
		if err != nil {
			log.Printf("[IPIntel] L1 lookup failed for %s: %v", ip, err)
			// 继续尝试 L3
			intel = &model.IPIntel{IP: ip}
		}
	} else {
		intel = &model.IPIntel{IP: ip}
	}

	// 2.1 使用 HostingDetector 增强检测（L2：CIDR/ASN黑名单/Cymru/rDNS，低误判版本）
	var asnStale bool
	if s.hostingDetector != nil {
		result, err := s.hostingDetector.Detect(ip)
		if err == nil {
			localASN := intel.ASN
			asnStale = applyHostingResult(intel, result)
			intel.Tier = appendTier(intel.Tier, "L2")
			if asnStale {
				log.Printf("[IPIntel] Local ASN db stale for %s: %s -> %s (BGP via Cymru)", ip, localASN, intel.ASN)
			}
		}
	}

	// 2.2 CN 权威地理层（ip2region xdb）：western geo 源对中国 IP 普遍错判
	// （实测省级 87%→98%、市级 ~62%→~95%、IPv6 1/3→3/3），命中中国即以 ip2region
	// 覆盖 geo，并据运营商放行家宽/移动、识别国内云厂商。在 L3 判定之前应用，使
	// CN 家宽/移动仅凭本地数据即可定案、零 L3 调用；灰区的 CN 非消费级 IP 仍可深扫，
	// 但 geo 已锁定为权威值，合并后会重新覆盖，防止 western geo 回灌。
	var cnGeo *adapter.CNGeoResult
	cnConsumerISP := false
	if s.cnGeo != nil {
		if res, ok := s.cnGeo.Lookup(ip); ok {
			cnGeo = res
			applyCNGeo(intel, res)
			// 机房身份优先（cloud CIDR/ASN 黑名单胜出）：仅当未判机房时才按消费级放行
			cnConsumerISP = res.IsConsumerISP() && !intel.IsHosting
		}
	}

	// 3. 判断是否需要 L3 深度扫描
	allowL3 := !opts.LocalOnly && s.registry.RemoteCount() > 0
	shouldDeepScan := opts.DeepScan // 用户显式请求

	// 自动深度扫描（仅非 LocalOnly 时启用）：
	// - 灰区：无法确定是 ISP 还是 Hosting（NeedsDeepScan=true 且无明确结论），调用远程 API 确认类型
	// - 本地库过期（asnStale）：MMDB 对该网段的 geo/org 同样不可信，
	//   即使已确认是机房也要用远程 API 修正展示字段（geo/org）
	//
	// CN 消费级 ISP（家宽/移动）已由 ip2region 权威定案为住宅：既不需 L3 修正 geo
	// （ip2region 更准），也不需 L3 确认身份（家宽即住宅），直接抑制深扫，省 2 次 L3 额度、
	// 且避免错误的 western 答案回灌。western MMDB 判出的 asnStale 对这些段恰是要忽略的假信号。
	if allowL3 && !shouldDeepScan && !cnConsumerISP && (asnStale || (intel.NeedsDeepScan && !intel.IsHosting && intel.UsageTypeRaw == "")) {
		log.Printf("[IPIntel] Auto deep scan triggered for %s (method=%s, asn_stale=%v)", ip, intel.DetectMethod, asnStale)
		shouldDeepScan = true
	}

	l3QuotaBlocked := false
	if allowL3 && shouldDeepScan {
		remoteIntel, blocked := s.lookupRemote(ctx, ip, opts.PreferSource)
		if blocked {
			l3QuotaBlocked = true
		}
		if remoteIntel != nil {
			intel = s.merge(intel, remoteIntel)
			// CN 地理是权威层：merge 以 L3 为主会用 western geo 覆盖正确的中国省市，
			// 故合并后重新覆盖 geo（幂等，不重复追加 Tier）。
			if cnGeo != nil {
				applyCNGeo(intel, cnGeo)
			}
		}
	}

	// 4. 后处理
	intel.FetchedAt = s.now()
	// 强制归一化 UsageType（因为 merge 后 IsHosting 可能变化）
	intel.UsageType = intel.InferUsageType()

	// 4.1 如果仍是 unknown，尝试用第二个 API 确认（仅非 LocalOnly 且未触发每日熔断时）
	if allowL3 && !l3QuotaBlocked && intel.UsageType == "unknown" && s.registry.RemoteCount() > 1 {
		secondIntel, blocked := s.lookupRemoteExcluding(ctx, ip, intel.Source)
		if blocked {
			l3QuotaBlocked = true
		}
		if secondIntel != nil {
			// 合并第二个 API 的结果
			if secondIntel.IsHosting {
				intel.IsHosting = true
				intel.UsageType = "hosting"
				intel.UsageTypeRaw = "hosting"
				intel.DetectMethod = intel.DetectMethod + "+confirmed"
				log.Printf("[IPIntel] Unknown confirmed as hosting by %s for %s", secondIntel.Source, ip)
			} else if secondIntel.UsageTypeRaw != "" || secondIntel.UsageType == "isp" {
				intel.UsageType = "isp"
				if secondIntel.UsageTypeRaw != "" {
					intel.UsageTypeRaw = secondIntel.UsageTypeRaw
				}
				intel.DetectMethod = intel.DetectMethod + "+confirmed"
				log.Printf("[IPIntel] Unknown confirmed as isp by %s for %s", secondIntel.Source, ip)
			}
			// 更新置信度
			if intel.UsageType != "unknown" {
				intel.Confidence = 90 // 多 API 确认，置信度提高
				intel.Tier = appendTier(intel.Tier, "L3")
			}
		}
	}

	// 4.2 获取 Cloudflare Radar 数据（真实用户占比）
	if s.radarService != nil && s.radarService.IsEnabled() && intel.ASN != "" {
		humanRatio, err := s.radarService.GetHumanRatio(ctx, intel.ASN)
		if err == nil && humanRatio != nil {
			intel.HumanRatio = humanRatio
		}
	}

	// 4.3 生成提示文案
	intel.Tip = generateTip(intel)

	// 5. 写缓存（如果启用且未跳过；因每日 L3 熔断降级时写入 basic，避免污染 deep 缓存）
	if !opts.SkipCache && s.cache != nil {
		s.setCache(ctx, ip, intel, opts.DeepScan && !l3QuotaBlocked)
	}

	return intel, nil
}

// LookupBasic 基础查询（仅 L1）
func (s *IPIntelService) LookupBasic(ctx context.Context, ip string) (*model.IPIntel, error) {
	return s.Lookup(ctx, ip, LookupOptions{DeepScan: false})
}

// LookupDeep 深度查询（L1 + L3）
func (s *IPIntelService) LookupDeep(ctx context.Context, ip string) (*model.IPIntel, error) {
	return s.Lookup(ctx, ip, LookupOptions{DeepScan: true})
}

// tryConsumeL3Quota 消耗一次每日全局 L3 额度；超限时返回 false 并每天仅告警一次。
func (s *IPIntelService) tryConsumeL3Quota(ctx context.Context) bool {
	s.l3QuotaMu.Lock()
	limit := s.l3DailyLimit
	s.l3QuotaMu.Unlock()
	if limit <= 0 {
		return true
	}

	day := s.now().UTC().Format("2006-01-02")

	// 优先使用 Redis 原子计数（重启不丢失）；Redis 异常时降级到本地内存计数，防止额度失控
	if s.cache != nil {
		n, err := s.cache.IncrDailyCounter(ctx, "l3:daily", day, 48*time.Hour)
		if err == nil {
			if n > int64(limit) {
				s.warnL3QuotaOnce(day, int(n), limit)
				return false
			}
			return true
		}
		log.Printf("[IPIntel] L3 quota redis error, falling back to local counter: %v", err)
	}

	s.l3QuotaMu.Lock()
	defer s.l3QuotaMu.Unlock()
	if s.l3QuotaDay != day {
		s.l3QuotaDay = day
		s.l3QuotaUsed = 0
	}
	if s.l3QuotaUsed >= limit {
		if s.l3QuotaWarnDay != day {
			s.l3QuotaWarnDay = day
			log.Printf("[IPIntel] L3 daily quota reached (%d/%d) on %s, degrading to L1+L2", s.l3QuotaUsed, limit, day)
		}
		return false
	}
	s.l3QuotaUsed++
	return true
}

func (s *IPIntelService) warnL3QuotaOnce(day string, current, limit int) {
	s.l3QuotaMu.Lock()
	defer s.l3QuotaMu.Unlock()
	if s.l3QuotaWarnDay != day {
		s.l3QuotaWarnDay = day
		log.Printf("[IPIntel] L3 daily quota reached (%d/%d) on %s, degrading to L1+L2", current, limit, day)
	}
}

// lookupRemote 遍历远程适配器（带回退与每日全局熔断）
func (s *IPIntelService) lookupRemote(ctx context.Context, ip, preferSource string) (*model.IPIntel, bool) {
	remotes := s.registry.Remotes()

	// 如果指定了优先数据源，先尝试
	if preferSource != "" {
		for _, r := range remotes {
			if r.Name() == preferSource {
				if !s.tryConsumeL3Quota(ctx) {
					return nil, true
				}
				intel, err := r.Lookup(ctx, ip)
				if err == nil {
					return intel, false
				}
				log.Printf("[IPIntel] Preferred source %s failed for %s: %v", preferSource, ip, err)
				break
			}
		}
	}

	// 按优先级遍历所有远程适配器
	for _, r := range remotes {
		// 跳过已尝试的优先数据源
		if r.Name() == preferSource {
			continue
		}

		if !s.tryConsumeL3Quota(ctx) {
			return nil, true
		}

		intel, err := r.Lookup(ctx, ip)
		if err == nil {
			return intel, false
		}
		log.Printf("[IPIntel] %s lookup failed for %s: %v", r.Name(), ip, err)
		// 继续尝试下一个
	}

	return nil, false
}

// lookupRemoteExcluding 调用除指定源外的其他远程 API（带每日全局熔断）
func (s *IPIntelService) lookupRemoteExcluding(ctx context.Context, ip, excludeSource string) (*model.IPIntel, bool) {
	remotes := s.registry.Remotes()

	for _, r := range remotes {
		// 跳过已使用的数据源
		if r.Name() == excludeSource {
			continue
		}

		if !s.tryConsumeL3Quota(ctx) {
			return nil, true
		}

		intel, err := r.Lookup(ctx, ip)
		if err == nil {
			return intel, false
		}
		log.Printf("[IPIntel] %s lookup failed for %s: %v", r.Name(), ip, err)
	}

	return nil, false
}

// applyHostingResult 将 HostingDetector 的检测结果回填到 intel
// 返回本地 ASN 库对该 IP 是否已过期（过期时调用方需触发 L3 修正 geo/org）
func applyHostingResult(intel *model.IPIntel, result *HostingResult) bool {
	intel.Confidence = result.Confidence
	intel.DetectMethod = result.Method
	intel.RDNS = result.RDNS
	intel.NeedsDeepScan = result.NeedsDeepScan

	// 本地 MMDB 的 ASN 已不在当前 BGP 宣告中（网段易主/租赁后重新宣告）：
	// 以 Cymru 实时校准后的 ASN/Org 为准，并标记深扫让 L3 修正同样过期的地理信息
	if result.ASNStale {
		if result.ASNStr != "" {
			intel.ASN = result.ASNStr
			intel.Org = result.Org
		}
		intel.NeedsDeepScan = true
	}

	if result.IsHosting {
		// 明确是机房 IP
		intel.IsHosting = true
	} else if result.IsResidential {
		// rDNS 确认是动态家宽，标记 UsageTypeRaw
		intel.UsageTypeRaw = "residential"
		intel.IsHosting = false
	}
	// 如果是 Unknown 状态，保持原有标识不变，让 NeedsDeepScan 指导后续

	return result.ASNStale
}

// applyCNGeo 将 ip2region 的 CN 权威地理/运营商结果覆盖到 intel。
// 幂等：L3 合并后可再次调用以重申 CN 地理不被 western geo 覆盖。
//
//   - 地理：Country=CN、Region=省(中文)、City=市(中文)、ISP=运营商(中文)，
//     Timezone 为空时补 Asia/Shanghai；geo 置信度提到 95。
//   - 归因：Source 记为 ip2region-cn，Tier 追加 +CN（去重），使遥测/影子回放可区分 CN 层答案。
//   - 身份：
//     云厂商 → 机房证据（IsHosting + DetectMethod=ip2region_cn_cloud，除非 HostingDetector 已判机房）；
//     消费级 ISP 且未被判机房 → 住宅/移动放行（写 UsageTypeRaw、清 NeedsDeepScan）。
func applyCNGeo(intel *model.IPIntel, cn *adapter.CNGeoResult) {
	intel.Country = "CN"
	if cn.Province != "" {
		intel.Region = cn.Province
	}
	if cn.City != "" {
		intel.City = cn.City
	}
	if cn.ISP != "" {
		intel.ISP = cn.ISP
	}
	if intel.Timezone == "" {
		intel.Timezone = "Asia/Shanghai"
	}
	// western 源的坐标/邮编属于被覆盖掉的错误答案（移动段钉北京、IPv6 深圳配海南坐标），
	// ip2region 只提供行政级归属、不含坐标，故清掉这些自相矛盾的残留字段而非留着误导。
	intel.Postal = ""
	intel.Latitude = 0
	intel.Longitude = 0
	if intel.Confidence < 95 {
		intel.Confidence = 95
	}
	intel.Source = "ip2region-cn"
	if !strings.Contains(intel.Tier, "CN") {
		intel.Tier = appendTier(intel.Tier, "CN")
	}

	// 国内云厂商：机房证据（填补 cloud_ip_ranges 无国内云的缺口）。
	// HostingDetector 已判机房（cloud CIDR/ASN 黑名单）则保留其方法，身份优先。
	if cn.CloudProvider() != "" {
		if !intel.IsHosting {
			intel.IsHosting = true
			intel.UsageTypeRaw = "hosting"
			intel.DetectMethod = "ip2region_cn_cloud"
		}
		// 已确信是国内云机房，清掉 HostingDetector 遗留的灰区深扫标记
		intel.NeedsDeepScan = false
		return
	}

	// 已判机房则不再按消费级放行（机房身份优先）
	if intel.IsHosting {
		return
	}

	// 消费级 ISP：家宽/移动放行，使 InferUsageType 归到 isp、身份判 residential_pass
	if usage := cn.ConsumerUsageRaw(); usage != "" {
		intel.UsageTypeRaw = usage
		if usage == "mobile" {
			intel.IsMobile = true
		}
		// 灰区方法（NeedsDeepScan 的 unknown/isp_static/asn_stale）由 CN 层接管归因；
		// rDNS 已确认家宽（needs_deep_scan=false）等实锤方法保留。
		if intel.DetectMethod == "" || intel.NeedsDeepScan {
			intel.DetectMethod = "ip2region_cn_isp"
		}
		intel.NeedsDeepScan = false
	}
}

// merge 合并 L1 和 L3 数据
func (s *IPIntelService) merge(local, remote *model.IPIntel) *model.IPIntel {
	// 以 L3 (remote) 数据为主，用 L1 (local) 数据补充

	// 基础信息：L3 缺失时用 L1 补充
	if remote.City == "" && local.City != "" {
		remote.City = local.City
	}
	if remote.Region == "" && local.Region != "" {
		remote.Region = local.Region
	}
	if remote.Timezone == "" && local.Timezone != "" {
		remote.Timezone = local.Timezone
	}
	if remote.Latitude == 0 && local.Latitude != 0 {
		remote.Latitude = local.Latitude
		remote.Longitude = local.Longitude
	}
	if remote.ASN == "" && local.ASN != "" {
		remote.ASN = local.ASN
	}
	if remote.Org == "" && local.Org != "" {
		remote.Org = local.Org
	}

	// Hosting 检测：L1 检测到则保留，取较高置信度
	if local.IsHosting && !remote.IsHosting {
		remote.IsHosting = true
	}
	if local.Confidence > remote.Confidence {
		remote.Confidence = local.Confidence
		remote.DetectMethod = local.DetectMethod + "+api"
	}

	// 标记为合并数据：在 local 已累积的层级（L1、可能还有 L2）之上追加 L3，
	// 而非硬编码覆盖——否则 HostingDetector(L2) 已确认的贡献会被悄悄丢弃。
	remote.Tier = appendTier(local.Tier, "L3")

	return remote
}

// appendTier 按命中顺序累加查询层级标记（L1/L2/L3 可能任意组合命中）
func appendTier(existing, layer string) string {
	if existing == "" {
		return layer
	}
	return existing + "+" + layer
}

// getCache 获取缓存；非深扫查询优先复用已存在的 deep 缓存（确保首页深扫与专项页/curl 结果一致）
func (s *IPIntelService) getCache(ctx context.Context, ip string, opts LookupOptions) *model.IPIntel {
	if opts.DeepScan {
		intel, err := s.cache.GetDeep(ctx, ip)
		if err != nil {
			log.Printf("[IPIntel] Cache get deep error for %s: %v", ip, err)
			return nil
		}
		return intel
	}

	// 优先复用深扫缓存，零额外成本且保证同 IP 在首页与专项页/终端结果一致
	if intel, err := s.cache.GetDeep(ctx, ip); err != nil {
		log.Printf("[IPIntel] Cache get deep error for %s: %v", ip, err)
	} else if intel != nil {
		return intel
	}

	intel, err := s.cache.GetBasic(ctx, ip)
	if err != nil {
		log.Printf("[IPIntel] Cache get basic error for %s: %v", ip, err)
		return nil
	}
	return intel
}

// setCache 设置缓存
func (s *IPIntelService) setCache(ctx context.Context, ip string, intel *model.IPIntel, deep bool) {
	var err error

	if deep {
		err = s.cache.SetDeep(ctx, ip, intel)
	} else {
		err = s.cache.SetBasic(ctx, ip, intel)
	}

	if err != nil {
		log.Printf("[IPIntel] Cache set error for %s: %v", ip, err)
	}
}

// ClearCache 清空缓存
func (s *IPIntelService) ClearCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Clear(ctx)
}

// ClearCacheForIP 清除指定 IP 的缓存
func (s *IPIntelService) ClearCacheForIP(ctx context.Context, ip string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, ip)
}

// HasCache 是否启用了缓存
func (s *IPIntelService) HasCache() bool {
	return s.cache != nil
}

// generateTip 根据检测结果生成提示文案
func generateTip(intel *model.IPIntel) string {
	// 1. 代理/匿名检测（优先级最高）
	if intel.IsVPN || intel.IsProxy || intel.IsTor || intel.IsRelay || intel.IsAnonymous {
		return "检测到代理/匿名网络特征"
	}

	// 2. 托管/机房检测
	if intel.IsHosting || intel.UsageType == "hosting" {
		return "当前为托管/机房 IP"
	}

	// 3. 正常用户
	var parts []string
	parts = append(parts, "无代理特征")

	// 补充类型信息
	if intel.UsageTypeRaw != "" {
		switch intel.UsageTypeRaw {
		case "isp", "residential":
			parts = append(parts, "住宅宽带")
		case "mobile":
			parts = append(parts, "移动网络")
		case "satellite":
			parts = append(parts, "卫星网络")
		case "business":
			parts = append(parts, "企业网络")
		case "education":
			parts = append(parts, "教育网络")
		case "government":
			parts = append(parts, "政府网络")
		}
	}

	// 真实用户占比
	if intel.HumanRatio != nil {
		if *intel.HumanRatio >= 80 {
			parts = append(parts, "真实用户占比高")
		} else if *intel.HumanRatio < 50 {
			parts = append(parts, "自动化流量较多")
		}
	}

	return strings.Join(parts, "，")
}
