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
	"detect-radar/internal/util"
)

type ipIntelCache interface {
	GetBasic(ctx context.Context, ip string) (*model.IPIntel, error)
	GetDeep(ctx context.Context, ip string) (*model.IPIntel, error)
	SetBasic(ctx context.Context, ip string, intel *model.IPIntel) error
	SetDeep(ctx context.Context, ip string, intel *model.IPIntel) error
	IncrDailyCounter(ctx context.Context, name, day string, ttl time.Duration) (int64, error)
}

type inflightLookup struct {
	done  chan struct{}
	intel *model.IPIntel
	err   error
}

type IPIntelService struct {
	registry        *adapter.Registry
	cache           ipIntelCache
	hostingDetector *HostingDetector
	radarService    *RadarService
	cnGeo           *adapter.IP2RegionCN

	l3DailyLimit   int
	l3QuotaMu      sync.Mutex
	l3QuotaDay     string
	l3QuotaUsed    int
	l3QuotaWarnDay string

	inflightMu sync.Mutex
	inflight   map[string]*inflightLookup

	nowFunc func() time.Time
}

func NewIPIntelService(registry *adapter.Registry, redisCache *cache.RedisCache, hostingDetector *HostingDetector, radarService *RadarService, cnGeo *adapter.IP2RegionCN) *IPIntelService {
	var intelCache ipIntelCache
	if redisCache != nil {
		intelCache = redisCache
	}
	return &IPIntelService{
		registry:        registry,
		cache:           intelCache,
		hostingDetector: hostingDetector,
		radarService:    radarService,
		cnGeo:           cnGeo,
		inflight:        make(map[string]*inflightLookup),
		nowFunc:         time.Now,
	}
}

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

type LookupOptions struct {
	DeepScan     bool
	LocalOnly    bool
	PreferSource string
}

func (s *IPIntelService) Lookup(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, error) {
	ip = util.CanonicalIP(ip)

	if s.cache != nil {
		if cached, fromDeep := s.getCache(ctx, ip, opts); cached != nil {
			cp := cloneIntel(cached)
			if cp.HumanRatio == nil && s.radarService != nil && s.radarService.IsEnabled() && cp.ASN != "" {
				if humanRatio, err := s.radarService.GetHumanRatio(ctx, cp.ASN); err == nil && humanRatio != nil {
					cp.HumanRatio = humanRatio
					s.setCache(ctx, ip, cp, fromDeep)
				}
			}
			return cp, nil
		}
	}

	if opts.PreferSource == "" {
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
	if src.GeoZH != nil {
		g := *src.GeoZH
		cp.GeoZH = &g
	}
	if src.GeoEN != nil {
		g := *src.GeoEN
		cp.GeoEN = &g
	}
	return &cp
}

func (s *IPIntelService) lookupUncached(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, error) {
	var intel *model.IPIntel
	var err error

	if s.registry.HasLocal() {
		intel, err = s.registry.Local().Lookup(ctx, ip)
		if err != nil {
			log.Printf("[IPIntel] L1 lookup failed for %s: %v", ip, err)
			intel = &model.IPIntel{IP: ip}
		}
	} else {
		intel = &model.IPIntel{IP: ip}
	}

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

	var cnGeo *adapter.CNGeoResult
	cnConsumerISP := false
	if s.cnGeo != nil {
		if res, ok := s.cnGeo.Lookup(ip); ok {
			cnGeo = res
			applyCNGeo(intel, res)
			cnConsumerISP = res.IsConsumerISP() && !intel.IsHosting
		}
	}

	allowL3 := !opts.LocalOnly && s.registry.RemoteCount() > 0
	shouldDeepScan := opts.DeepScan

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
			if cnGeo != nil {
				applyCNGeo(intel, cnGeo)
			}
		}
	}

	intel.FetchedAt = s.now()
	intel.UsageType = intel.InferUsageType()

	if allowL3 && !l3QuotaBlocked && intel.UsageType == "unknown" && s.registry.RemoteCount() > 1 {
		secondIntel, blocked := s.lookupRemoteExcluding(ctx, ip, intel.Source)
		if blocked {
			l3QuotaBlocked = true
		}
		if secondIntel != nil {
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
			if intel.UsageType != "unknown" {
				intel.Confidence = 90
				intel.Tier = appendTier(intel.Tier, "L3")
			}
		}
	}

	if s.radarService != nil && s.radarService.IsEnabled() && intel.ASN != "" {
		humanRatio, err := s.radarService.GetHumanRatio(ctx, intel.ASN)
		if err == nil && humanRatio != nil {
			intel.HumanRatio = humanRatio
		}
	}

	if normalized := util.CanonicalIP(intel.IP); normalized != "" {
		intel.IP = normalized
	} else {
		intel.IP = ip
	}

	if s.cache != nil {
		s.setCache(ctx, ip, intel, opts.DeepScan && !l3QuotaBlocked)
	}

	return intel, nil
}

func (s *IPIntelService) tryConsumeL3Quota(ctx context.Context) bool {
	s.l3QuotaMu.Lock()
	limit := s.l3DailyLimit
	s.l3QuotaMu.Unlock()
	if limit <= 0 {
		return true
	}

	day := s.now().UTC().Format("2006-01-02")

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

func (s *IPIntelService) lookupRemote(ctx context.Context, ip, preferSource string) (*model.IPIntel, bool) {
	remotes := s.registry.Remotes()

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

	for _, r := range remotes {
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
	}

	return nil, false
}

func (s *IPIntelService) lookupRemoteExcluding(ctx context.Context, ip, excludeSource string) (*model.IPIntel, bool) {
	remotes := s.registry.Remotes()

	for _, r := range remotes {
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

func applyHostingResult(intel *model.IPIntel, result *HostingResult) bool {
	intel.Confidence = result.Confidence
	intel.DetectMethod = result.Method
	intel.RDNS = result.RDNS
	intel.NeedsDeepScan = result.NeedsDeepScan

	if strings.EqualFold(strings.TrimSpace(intel.ASN), "AS0") {
		intel.ASN = ""
	}

	if result.ASNStale {
		if result.ASNStr != "" && result.ASNStr != "AS0" {
			intel.ASN = result.ASNStr
			intel.Org = result.Org
		}
		intel.NeedsDeepScan = true
	} else {
		if intel.ASN == "" && result.ASNStr != "" && result.ASNStr != "AS0" {
			intel.ASN = result.ASNStr
		}
		if cleanNetworkLabel(intel.Org) == "" && result.Org != "" {
			intel.Org = result.Org
		}
		if result.Method == "cloud_ip_range" && cleanNetworkLabel(intel.ISP) == "" && result.Org != "" {
			intel.ISP = result.Org
		}
	}

	if result.IsHosting {
		intel.IsHosting = true
	} else if result.IsResidential {
		if result.IsMobile {
			intel.UsageTypeRaw = "mobile"
			intel.IsMobile = true
		} else {
			intel.UsageTypeRaw = "residential"
		}
		intel.IsHosting = false
	}

	return result.ASNStale
}

func detectGreaterChinaGeo(cn *adapter.CNGeoResult) (code, zhCountry, enCountry, tz string) {
	joined := strings.ToLower(cn.Province + " " + cn.City)
	switch {
	case strings.Contains(joined, "香港") || strings.Contains(joined, "hong kong"):
		return "HK", "中国香港", "Hong Kong", "Asia/Hong_Kong"
	case strings.Contains(joined, "澳门") || strings.Contains(joined, "澳門") || strings.Contains(joined, "macau") || strings.Contains(joined, "macao"):
		return "MO", "中国澳门", "Macao", "Asia/Macau"
	case strings.Contains(joined, "台湾") || strings.Contains(joined, "臺灣") || strings.Contains(joined, "taiwan"):
		return "TW", "中国台湾", "Taiwan", "Asia/Taipei"
	default:
		return "CN", "中国", "China", "Asia/Shanghai"
	}
}

func applyCNGeo(intel *model.IPIntel, cn *adapter.CNGeoResult) {
	prevEN := intel.GeoEN
	code, zhCountry, enCountry, defaultTZ := detectGreaterChinaGeo(cn)
	intel.Country = code
	intel.CountryName = zhCountry
	intel.Region = cn.Province
	intel.City = cn.City

	zhGeo := &model.LocalizedGeo{
		CountryName: zhCountry,
		Region:      cn.Province,
		City:        cn.City,
	}
	enGeo := &model.LocalizedGeo{
		CountryName: enCountry,
	}
	if cn.Province != "" {
		enGeo.Region = translateCNProvinceEN(cn.Province)
	}
	if cn.City != "" {
		enGeo.City = translateCNCityEN(cn.City)
	}
	if cn.ISP != "" {
		intel.ISP = cn.ISP
		zhGeo.ISP = cn.ISP
		enGeo.ISP = translateCNISPEN(cn.ISP)
	} else if prevEN != nil && !containsCJK(prevEN.ISP) {
		enGeo.ISP = prevEN.ISP
	}
	if intel.GeoZH != nil && zhGeo.ISP == "" {
		zhGeo.ISP = intel.GeoZH.ISP
	}
	intel.GeoZH = zhGeo
	intel.GeoEN = enGeo
	if intel.Timezone == "" {
		intel.Timezone = defaultTZ
	}
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

	if cn.CloudProvider() != "" {
		if !intel.IsHosting {
			intel.IsHosting = true
			intel.UsageTypeRaw = "hosting"
			intel.DetectMethod = "ip2region_cn_cloud"
		}
		intel.NeedsDeepScan = false
		return
	}

	if intel.IsHosting {
		return
	}

	if usage := cn.ConsumerUsageRaw(); usage != "" {
		intel.UsageTypeRaw = usage
		if usage == "mobile" {
			intel.IsMobile = true
		}
		if intel.DetectMethod == "" || intel.NeedsDeepScan {
			intel.DetectMethod = "ip2region_cn_isp"
		}
		intel.NeedsDeepScan = false
	}
}

func (s *IPIntelService) merge(local, remote *model.IPIntel) *model.IPIntel {
	if normalized := util.CanonicalIP(remote.IP); normalized != "" {
		remote.IP = normalized
	} else if normalized := util.CanonicalIP(local.IP); normalized != "" {
		remote.IP = normalized
	}

	if remote.Country == "" && local.Country != "" {
		remote.Country = local.Country
	}
	sameCountry := strings.EqualFold(strings.TrimSpace(remote.Country), strings.TrimSpace(local.Country))

	enGeo := &model.LocalizedGeo{
		CountryName: remote.CountryName,
		Region:      remote.Region,
		City:        remote.City,
		ISP:         remote.ISP,
	}
	if sameCountry && local.GeoEN != nil {
		if enGeo.CountryName == "" {
			enGeo.CountryName = local.GeoEN.CountryName
		}
		if enGeo.Region == "" {
			enGeo.Region = local.GeoEN.Region
		}
		if enGeo.City == "" {
			enGeo.City = local.GeoEN.City
		}
		if enGeo.ISP == "" {
			enGeo.ISP = local.GeoEN.ISP
		}
	}
	if enGeo.CountryName != "" || enGeo.Region != "" || enGeo.City != "" || enGeo.ISP != "" {
		remote.GeoEN = enGeo
	}
	if sameCountry && local.GeoZH != nil {
		g := model.LocalizedGeo{
			CountryName: local.GeoZH.CountryName,
			ISP:         local.GeoZH.ISP,
		}
		if remote.Region == "" || (local.GeoEN != nil && strings.EqualFold(strings.TrimSpace(remote.Region), strings.TrimSpace(local.GeoEN.Region))) {
			g.Region = local.GeoZH.Region
		} else {
			g.Region = remote.Region
		}
		if remote.City == "" || (local.GeoEN != nil && strings.EqualFold(strings.TrimSpace(remote.City), strings.TrimSpace(local.GeoEN.City))) {
			g.City = local.GeoZH.City
		} else {
			g.City = remote.City
		}
		remote.GeoZH = &g
	}

	if sameCountry {
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
	}
	if remote.ASN == "" && local.ASN != "" {
		remote.ASN = local.ASN
	}
	if remote.Org == "" && local.Org != "" {
		remote.Org = local.Org
	}

	if local.IsHosting && !remote.IsHosting {
		remote.IsHosting = true
	}
	if local.Confidence > remote.Confidence {
		remote.Confidence = local.Confidence
		remote.DetectMethod = local.DetectMethod + "+api"
	}

	remote.Tier = appendTier(local.Tier, "L3")

	return remote
}

func appendTier(existing, layer string) string {
	if existing == "" {
		return layer
	}
	return existing + "+" + layer
}

func (s *IPIntelService) getCache(ctx context.Context, ip string, opts LookupOptions) (*model.IPIntel, bool) {
	if opts.DeepScan {
		intel, err := s.cache.GetDeep(ctx, ip)
		if err != nil {
			log.Printf("[IPIntel] Cache get deep error for %s: %v", ip, err)
			return nil, false
		}
		return intel, intel != nil
	}

	if intel, err := s.cache.GetDeep(ctx, ip); err != nil {
		log.Printf("[IPIntel] Cache get deep error for %s: %v", ip, err)
	} else if intel != nil {
		return intel, true
	}

	intel, err := s.cache.GetBasic(ctx, ip)
	if err != nil {
		log.Printf("[IPIntel] Cache get basic error for %s: %v", ip, err)
		return nil, false
	}
	return intel, false
}

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
