package service

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"detect-radar/dnsassessment"
	"detect-radar/internal/model"
	"detect-radar/internal/repository"
)

const (
	catalogRedisVersionKey  = "dr:catalog:v1:version"
	catalogRedisSnapshotKey = "dr:catalog:v1:snapshot"
	catalogRedisTTL         = 24 * time.Hour
	catalogReloadInterval   = 60 * time.Second
)

var (
	defaultCatalog   atomic.Pointer[NetworkCatalog]
	aliasPunctuation = strings.NewReplacer(",", "", ".", "")
)

func SetDefaultCatalog(cat *NetworkCatalog) {
	defaultCatalog.Store(cat)
	if cat != nil {
		dnsassessment.SetDefaultCatalog(cat)
	} else {
		dnsassessment.SetDefaultCatalog(nil)
	}
}

func DefaultCatalog() *NetworkCatalog {
	return defaultCatalog.Load()
}

type NetworkCatalog struct {
	repo     *repository.CatalogRepository
	redis    *redis.Client
	seedPath string

	snap     atomic.Pointer[repository.CatalogSnapshot]
	reloadMu sync.Mutex

	interval  time.Duration
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func NewNetworkCatalog(repo *repository.CatalogRepository, rdb *redis.Client, seedPath string) (*NetworkCatalog, error) {
	c := &NetworkCatalog{
		repo:     repo,
		redis:    rdb,
		seedPath: seedPath,
		interval: catalogReloadInterval,
	}

	if repo != nil {
		if seeded, err := repo.SeedIfEmpty(seedPath); err != nil {
			log.Printf("[Catalog] 冷启动种子检查提示：%v", err)
		} else if seeded {
			log.Printf("[Catalog] SQLite 目录表为空，已从种子文件导入初始数据")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Reload(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func NewNetworkCatalogFromSeedFile(seedPath string) (*NetworkCatalog, error) {
	snap, err := repository.LoadSnapshotFromSeedFile(seedPath)
	if err != nil {
		return nil, err
	}
	c := &NetworkCatalog{seedPath: seedPath, interval: catalogReloadInterval}
	c.snap.Store(snap)
	return c, nil
}

func (c *NetworkCatalog) Start() {
	if c == nil || (c.repo == nil && c.redis == nil) {
		return
	}
	c.startOnce.Do(func() {
		c.stop = make(chan struct{})
		c.done = make(chan struct{})
		go c.reloadLoop()
	})
}

func (c *NetworkCatalog) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.stop != nil {
			close(c.stop)
			<-c.done
		}
	})
	return nil
}

func (c *NetworkCatalog) reloadLoop() {
	defer close(c.done)
	iv := c.interval
	if iv <= 0 {
		iv = catalogReloadInterval
	}
	ticker := time.NewTicker(iv)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := c.SyncIfChanged(ctx); err != nil {
				log.Printf("[Catalog] 后台热更新检查失败：%v", err)
			}
			cancel()
		case <-c.stop:
			return
		}
	}
}

func (c *NetworkCatalog) Reload(ctx context.Context) error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	if c.repo != nil {
		snap, err := c.repo.LoadSnapshot()
		if err == nil && snap != nil {
			if snap.Version == "" {
				snap.Version = strconv.FormatInt(snap.LoadedAt, 10)
			}
			c.snap.Store(snap)
			c.publishToRedis(ctx, snap)
			return nil
		}
		if err != nil {
			log.Printf("[Catalog] 读取 SQLite 快照失败，尝试 Redis/种子降级：%v", err)
		}
	}

	if snap, err := c.loadFromRedis(ctx); err == nil && snap != nil {
		c.snap.Store(snap)
		return nil
	}

	if snap, err := repository.LoadSnapshotFromSeedFile(c.seedPath); err == nil && snap != nil {
		c.snap.Store(snap)
		return nil
	}

	if c.snap.Load() == nil {
		c.snap.Store(&repository.CatalogSnapshot{
			ASNs:         map[uint64]repository.CatalogASN{},
			ExactAliases: map[string]repository.CatalogAlias{},
			PublicDNSIPs: map[string]repository.PublicDNSIP{},
			ProvincesEN:  map[string]string{},
			CitiesEN:     map[string]string{},
		})
	}
	return nil
}

func (c *NetworkCatalog) SyncIfChanged(ctx context.Context) error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	cur := c.snap.Load()
	curVer := ""
	if cur != nil {
		curVer = cur.Version
	}

	if c.repo != nil {
		dbVer, err := c.repo.CatalogVersion()
		if err == nil {
			if cur == nil || (dbVer != "" && dbVer != curVer) {
				snap, err := c.repo.LoadSnapshot()
				if err != nil {
					return err
				}
				if snap.Version == "" {
					snap.Version = strconv.FormatInt(snap.LoadedAt, 10)
				}
				c.snap.Store(snap)
				c.publishToRedis(ctx, snap)
				log.Printf("[Catalog] 已热重载网络目录快照（version=%s，ASN=%d，别名=%d，地名=%d）",
					snap.Version, len(snap.ASNs), len(snap.ExactAliases), len(snap.ProvincesEN)+len(snap.CitiesEN))
				return nil
			}
			if c.redis != nil && cur != nil {
				redisVer, rErr := c.redis.Get(ctx, catalogRedisVersionKey).Result()
				if rErr == redis.Nil || (rErr == nil && redisVer != curVer) {
					c.publishToRedis(ctx, cur)
				}
			}
			return nil
		}
	}

	if c.redis != nil {
		redisVer, err := c.redis.Get(ctx, catalogRedisVersionKey).Result()
		if err == nil && redisVer != "" && redisVer != curVer {
			if snap, err := c.loadFromRedis(ctx); err == nil && snap != nil {
				if snap.Version == "" {
					snap.Version = redisVer
				}
				c.snap.Store(snap)
			}
		}
	}
	return nil
}

func (c *NetworkCatalog) publishToRedis(ctx context.Context, snap *repository.CatalogSnapshot) {
	if c.redis == nil || snap == nil {
		return
	}
	toPublish := *snap
	if toPublish.Version == "" {
		toPublish.Version = strconv.FormatInt(toPublish.LoadedAt, 10)
	}
	raw, err := json.Marshal(&toPublish)
	if err != nil {
		return
	}
	pipe := c.redis.Pipeline()
	pipe.Set(ctx, catalogRedisSnapshotKey, raw, catalogRedisTTL)
	pipe.Set(ctx, catalogRedisVersionKey, toPublish.Version, catalogRedisTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[Catalog] 写入 Redis 目录快照失败（已降级本地内存）：%v", err)
	}
}

func (c *NetworkCatalog) loadFromRedis(ctx context.Context) (*repository.CatalogSnapshot, error) {
	if c.redis == nil {
		return nil, nil
	}
	raw, err := c.redis.Get(ctx, catalogRedisSnapshotKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var snap repository.CatalogSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (c *NetworkCatalog) Snapshot() *repository.CatalogSnapshot {
	if c == nil {
		return nil
	}
	return c.snap.Load()
}

func (c *NetworkCatalog) Stats() map[string]int {
	snap := c.Snapshot()
	if snap == nil {
		return map[string]int{}
	}
	accessCount := 0
	for _, a := range snap.ASNs {
		if a.Role == "access" {
			accessCount++
		}
	}
	return map[string]int{
		"asn_count":        len(snap.ASNs),
		"access_asn_count": accessCount,
		"alias_count":      len(snap.ExactAliases),
		"public_dns_count": len(snap.PublicDNSIPs),
		"geo_name_count":   len(snap.ProvincesEN) + len(snap.CitiesEN),
	}
}

func (c *NetworkCatalog) LookupASNRow(asn uint64) (repository.CatalogASN, bool) {
	if asn == 0 {
		return repository.CatalogASN{}, false
	}
	snap := c.Snapshot()
	if snap == nil {
		return repository.CatalogASN{}, false
	}
	row, ok := snap.ASNs[asn]
	return row, ok
}

func (c *NetworkCatalog) LookupAccessASN(asn uint64) (repository.CatalogASN, bool) {
	row, ok := c.LookupASNRow(asn)
	if !ok || row.Role != "access" {
		return repository.CatalogASN{}, false
	}
	return row, true
}

func (c *NetworkCatalog) LookupASNStr(raw string) (repository.CatalogASN, bool) {
	num := parseCatalogASN(raw)
	if num == 0 {
		return repository.CatalogASN{}, false
	}
	return c.LookupASNRow(num)
}

func parseCatalogASN(raw string) uint64 {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "AS")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func aliasNormalizeKey(raw string) string {
	s := strings.ToLower(aliasPunctuation.Replace(raw))
	return strings.Join(strings.Fields(s), " ")
}

func (c *NetworkCatalog) MatchAliasRow(raw string) (repository.CatalogAlias, bool) {
	snap := c.Snapshot()
	if snap == nil {
		return repository.CatalogAlias{}, false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return repository.CatalogAlias{}, false
	}
	lower := strings.ToLower(trimmed)
	norm := aliasNormalizeKey(trimmed)

	if row, ok := snap.ExactAliases[norm]; ok {
		return row, true
	}
	if lower != norm {
		if row, ok := snap.ExactAliases[lower]; ok {
			return row, true
		}
	}

	for _, sub := range snap.SubAliases {
		if sub.AliasKey != "" && (strings.Contains(lower, sub.AliasKey) || strings.Contains(norm, sub.AliasKey)) {
			return sub, true
		}
	}
	return repository.CatalogAlias{}, false
}

func (c *NetworkCatalog) LookupPublicDNSIP(ip string) (dnsassessment.CatalogEntry, bool) {
	snap := c.Snapshot()
	if snap == nil || ip == "" {
		return dnsassessment.CatalogEntry{}, false
	}
	row, ok := snap.PublicDNSIPs[strings.TrimSpace(ip)]
	if !ok {
		return dnsassessment.CatalogEntry{}, false
	}
	return dnsassessment.CatalogEntry{
		Role:   "public_dns",
		NameEN: row.NameEN,
		NameZH: row.NameZH,
	}, true
}

func (c *NetworkCatalog) LookupASN(asn uint64) (dnsassessment.CatalogEntry, bool) {
	row, ok := c.LookupASNRow(asn)
	if !ok {
		return dnsassessment.CatalogEntry{}, false
	}
	return dnsassessment.CatalogEntry{
		Role:   row.Role,
		NameEN: row.NameEN,
		NameZH: row.NameZH,
	}, true
}

func (c *NetworkCatalog) MatchNetworkAlias(raw string) (dnsassessment.CatalogEntry, bool) {
	row, ok := c.MatchAliasRow(raw)
	if !ok {
		return dnsassessment.CatalogEntry{}, false
	}
	return dnsassessment.CatalogEntry{
		Role:   row.Role,
		NameEN: row.NameEN,
		NameZH: row.NameZH,
	}, true
}

func (c *NetworkCatalog) TranslateProvinceEN(province string) string {
	key := trimCNAdminSuffix(province)
	if snap := c.Snapshot(); snap != nil {
		if en, ok := snap.ProvincesEN[key]; ok {
			return en
		}
	}
	if !containsCJK(province) {
		return strings.TrimSpace(province)
	}
	return ""
}

func (c *NetworkCatalog) TranslateCityEN(city string) string {
	key := trimCNAdminSuffix(city)
	if snap := c.Snapshot(); snap != nil {
		if en, ok := snap.CitiesEN[key]; ok {
			return en
		}
	}
	if !containsCJK(city) {
		return strings.TrimSpace(city)
	}
	return ""
}

func (c *NetworkCatalog) TranslateISPEN(isp string) string {
	trimmed := strings.TrimSpace(isp)
	if trimmed == "" {
		return ""
	}
	if row, ok := c.MatchAliasRow(trimmed); ok && row.NameEN != "" {
		return row.NameEN
	}
	if !containsCJK(trimmed) {
		return trimmed
	}
	return ""
}

func cleanNetworkLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "unknown") || trimmed == "-" || strings.EqualFold(trimmed, "n/a") {
		return ""
	}
	return trimmed
}

func (c *NetworkCatalog) LocalizeNetwork(raw, asn, lang string) string {
	trimmed := cleanNetworkLabel(raw)
	if trimmed != "" {
		if row, ok := c.MatchAliasRow(trimmed); ok {
			if lang == "zh" && row.NameZH != "" {
				return row.NameZH
			}
			if row.NameEN != "" {
				return row.NameEN
			}
			if lang == "en" && containsCJK(trimmed) {
				return ""
			}
			return trimmed
		}
	}
	if asnRow, ok := c.LookupASNStr(asn); ok {
		if trimmed == "" {
			if lang == "zh" && asnRow.NameZH != "" {
				return asnRow.NameZH
			}
			if asnRow.NameEN != "" {
				return asnRow.NameEN
			}
		}
		if lang == "zh" && asnRow.Source != "bgptools" && asnRow.Role != "public_dns" && asnRow.NameZH != "" && containsCJK(asnRow.NameZH) {
			return asnRow.NameZH
		}
		if lang == "en" && containsCJK(trimmed) && asnRow.NameEN != "" {
			return asnRow.NameEN
		}
	}
	if lang == "en" && containsCJK(trimmed) {
		return ""
	}
	return trimmed
}

func (c *NetworkCatalog) NormalizeIntelISP(isp, org, asn, lang string) string {
	cleaned := cleanNetworkLabel(isp)
	if cleaned != "" {
		if row, ok := c.MatchAliasRow(cleaned); ok && (row.Role != "public_dns" || containsCJK(cleaned)) {
			if lang == "zh" && row.NameZH != "" {
				return row.NameZH
			}
			if row.NameEN != "" {
				return row.NameEN
			}
			if lang == "en" && containsCJK(cleaned) {
				return ""
			}
			return cleaned
		}
		if asnRow, ok := c.LookupASNStr(asn); ok && asnRow.Source != "bgptools" && asnRow.Role != "public_dns" {
			if lang == "zh" && asnRow.NameZH != "" && (containsCJK(asnRow.NameZH) || containsCJK(cleaned)) {
				return asnRow.NameZH
			}
			if lang == "en" && containsCJK(cleaned) && asnRow.NameEN != "" {
				return asnRow.NameEN
			}
		}
		if lang == "en" && containsCJK(cleaned) {
			if cleanedOrg := cleanNetworkLabel(org); cleanedOrg != "" && !containsCJK(cleanedOrg) {
				return cleanedOrg
			}
			return ""
		}
		return cleaned
	}

	cleanedOrg := cleanNetworkLabel(org)
	if cleanedOrg != "" {
		if row, ok := c.MatchAliasRow(cleanedOrg); ok && (row.Role != "public_dns" || containsCJK(cleanedOrg)) {
			if lang == "zh" && row.NameZH != "" {
				return row.NameZH
			}
			if row.NameEN != "" {
				return row.NameEN
			}
			if lang == "en" && containsCJK(cleanedOrg) {
				return ""
			}
			return cleanedOrg
		}
	}
	if asnRow, ok := c.LookupASNStr(asn); ok && asnRow.Source != "bgptools" && asnRow.Role != "public_dns" {
		if lang == "zh" && asnRow.NameZH != "" {
			return asnRow.NameZH
		}
		if asnRow.NameEN != "" {
			return asnRow.NameEN
		}
	}
	return ""
}

func (c *NetworkCatalog) CarrierGroup(intel *model.IPIntel) string {
	if intel == nil {
		return ""
	}
	if row, ok := c.LookupASNStr(intel.ASN); ok && row.Role == "access" && row.CarrierGroup != "" {
		return row.CarrierGroup
	}
	for _, raw := range []string{intel.ISP, intel.Org} {
		if row, ok := c.MatchAliasRow(raw); ok && row.Role == "access" && row.CanonicalKey != "" {
			return row.CanonicalKey
		}
	}
	return ""
}
