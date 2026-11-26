package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CatalogASN struct {
	ASN          uint64 `json:"asn"`
	Role         string `json:"role"`
	NetworkKind  string `json:"network_kind"`
	Country      string `json:"country"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
	CarrierGroup string `json:"carrier_group"`
	Source       string `json:"source"`
}

type CatalogAlias struct {
	AliasKey     string `json:"alias_key"`
	MatchMode    string `json:"match_mode"`
	Priority     int    `json:"priority"`
	ASN          uint64 `json:"asn"`
	Role         string `json:"role"`
	CanonicalKey string `json:"canonical_key"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
}

type PublicDNSIP struct {
	IP           string `json:"ip"`
	CanonicalKey string `json:"canonical_key"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
}

type GeoName struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	NameEN string `json:"name_en"`
	NameZH string `json:"name_zh"`
}

type CatalogSeed struct {
	ASNCatalog     []CatalogASN   `json:"asn_catalog"`
	NetworkAliases []CatalogAlias `json:"network_aliases"`
	PublicDNSIPs   []PublicDNSIP  `json:"public_dns_ips"`
	GeoNames       []GeoName      `json:"geo_names"`
}

type CatalogSnapshot struct {
	Version      string                  `json:"version"`
	LoadedAt     int64                   `json:"loaded_at"`
	ASNs         map[uint64]CatalogASN   `json:"asns"`
	ExactAliases map[string]CatalogAlias `json:"exact_aliases"`
	SubAliases   []CatalogAlias          `json:"sub_aliases"`
	PublicDNSIPs map[string]PublicDNSIP  `json:"public_dns_ips"`
	ProvincesEN  map[string]string       `json:"provinces_en"`
	CitiesEN     map[string]string       `json:"cities_en"`
}

type CatalogRepository struct {
	db *sql.DB
}

func NewCatalogRepository(db *sql.DB) *CatalogRepository {
	if db == nil {
		return nil
	}
	return &CatalogRepository{db: db}
}

func ResolveSeedPath(configured string) string {
	candidates := []string{}
	if s := strings.TrimSpace(configured); s != "" {
		candidates = append(candidates, s)
	}
	candidates = append(candidates,
		"./data/catalog_seed.json",
		"/app/seed/catalog_seed.json",
		"/app/data/catalog_seed.json",
		"../data/catalog_seed.json",
		"../../data/catalog_seed.json",
		"../../../data/catalog_seed.json",
		"../../../../data/catalog_seed.json",
	)
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}
	}
	return ""
}

func LoadSeedFile(path string) (*CatalogSeed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var seed CatalogSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		return nil, fmt.Errorf("parse catalog seed %s: %w", path, err)
	}
	return &seed, nil
}

func BuildSnapshotFromSeed(seed *CatalogSeed) *CatalogSnapshot {
	snap := newEmptySnapshot()
	if seed == nil {
		return snap
	}
	snap.Version = "seed"
	for _, row := range seed.ASNCatalog {
		if row.ASN == 0 {
			continue
		}
		snap.ASNs[row.ASN] = row
	}
	for _, row := range seed.NetworkAliases {
		k := strings.ToLower(strings.TrimSpace(row.AliasKey))
		if k == "" {
			continue
		}
		row.AliasKey = k
		if row.MatchMode == "" {
			row.MatchMode = "exact"
		}
		if row.Priority == 0 {
			row.Priority = 100
		}
		snap.ExactAliases[k] = row
		if row.MatchMode == "contains" {
			snap.SubAliases = append(snap.SubAliases, row)
		}
	}
	sortSubAliases(snap.SubAliases)
	for _, row := range seed.PublicDNSIPs {
		ip := strings.TrimSpace(row.IP)
		if ip == "" {
			continue
		}
		row.IP = ip
		snap.PublicDNSIPs[ip] = row
	}
	for _, row := range seed.GeoNames {
		if row.Key == "" || row.NameEN == "" {
			continue
		}
		switch row.Kind {
		case "province":
			snap.ProvincesEN[row.Key] = row.NameEN
		case "city":
			snap.CitiesEN[row.Key] = row.NameEN
		}
	}
	return snap
}

func LoadSnapshotFromSeedFile(configuredPath string) (*CatalogSnapshot, error) {
	path := ResolveSeedPath(configuredPath)
	if path == "" {
		return nil, fmt.Errorf("catalog seed file not found")
	}
	seed, err := LoadSeedFile(path)
	if err != nil {
		return nil, err
	}
	return BuildSnapshotFromSeed(seed), nil
}

func newEmptySnapshot() *CatalogSnapshot {
	return &CatalogSnapshot{
		LoadedAt:     time.Now().Unix(),
		ASNs:         make(map[uint64]CatalogASN),
		ExactAliases: make(map[string]CatalogAlias),
		SubAliases:   make([]CatalogAlias, 0),
		PublicDNSIPs: make(map[string]PublicDNSIP),
		ProvincesEN:  make(map[string]string),
		CitiesEN:     make(map[string]string),
	}
}

func sortSubAliases(list []CatalogAlias) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority < list[j].Priority
		}
		if len(list[i].AliasKey) != len(list[j].AliasKey) {
			return len(list[i].AliasKey) > len(list[j].AliasKey)
		}
		return list[i].AliasKey < list[j].AliasKey
	})
}

func (r *CatalogRepository) tableCount(table string) (int64, error) {
	var n int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n, err
}

func (r *CatalogRepository) SeedIfEmpty(configuredPath string) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	asnCount, err := r.tableCount("asn_catalog")
	if err != nil {
		return false, err
	}
	aliasCount, err := r.tableCount("network_aliases")
	if err != nil {
		return false, err
	}
	dnsCount, err := r.tableCount("public_dns_ips")
	if err != nil {
		return false, err
	}
	geoCount, err := r.tableCount("geo_names")
	if err != nil {
		return false, err
	}
	if asnCount > 0 && aliasCount > 0 && dnsCount > 0 && geoCount > 0 {
		return false, nil
	}

	seedPath := ResolveSeedPath(configuredPath)
	if seedPath == "" {
		return false, fmt.Errorf("catalog tables empty and seed file not found")
	}
	seed, err := LoadSeedFile(seedPath)
	if err != nil {
		return false, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	if asnCount == 0 && len(seed.ASNCatalog) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO asn_catalog
				(asn, role, network_kind, country, name_en, name_zh, carrier_group, source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return false, err
		}
		for _, row := range seed.ASNCatalog {
			if row.ASN == 0 {
				continue
			}
			src := row.Source
			if src == "" {
				src = "seed"
			}
			if _, err := stmt.Exec(row.ASN, row.Role, row.NetworkKind, row.Country, row.NameEN, row.NameZH, row.CarrierGroup, src, now); err != nil {
				stmt.Close()
				return false, err
			}
		}
		stmt.Close()
	}

	if aliasCount == 0 && len(seed.NetworkAliases) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO network_aliases
				(alias_key, match_mode, priority, asn, role, canonical_key, name_en, name_zh, source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'seed', ?)
		`)
		if err != nil {
			return false, err
		}
		for _, row := range seed.NetworkAliases {
			k := strings.ToLower(strings.TrimSpace(row.AliasKey))
			if k == "" {
				continue
			}
			mode := row.MatchMode
			if mode == "" {
				mode = "exact"
			}
			prio := row.Priority
			if prio == 0 {
				prio = 100
			}
			if _, err := stmt.Exec(k, mode, prio, row.ASN, row.Role, row.CanonicalKey, row.NameEN, row.NameZH, now); err != nil {
				stmt.Close()
				return false, err
			}
		}
		stmt.Close()
	}

	if dnsCount == 0 && len(seed.PublicDNSIPs) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO public_dns_ips (ip, canonical_key, name_en, name_zh, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return false, err
		}
		for _, row := range seed.PublicDNSIPs {
			ip := strings.TrimSpace(row.IP)
			if ip == "" {
				continue
			}
			if _, err := stmt.Exec(ip, row.CanonicalKey, row.NameEN, row.NameZH, now); err != nil {
				stmt.Close()
				return false, err
			}
		}
		stmt.Close()
	}

	if geoCount == 0 && len(seed.GeoNames) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO geo_names (kind, key, name_en, name_zh)
			VALUES (?, ?, ?, ?)
		`)
		if err != nil {
			return false, err
		}
		for _, row := range seed.GeoNames {
			if row.Kind == "" || row.Key == "" || row.NameEN == "" {
				continue
			}
			if _, err := stmt.Exec(row.Kind, row.Key, row.NameEN, row.NameZH); err != nil {
				stmt.Close()
				return false, err
			}
		}
		stmt.Close()
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO sync_meta (source, cursor, row_count, last_synced_at)
		VALUES ('catalog_version', '1', ?, ?)
	`, len(seed.ASNCatalog), now); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *CatalogRepository) CatalogVersion() (string, error) {
	if r == nil || r.db == nil {
		return "", nil
	}
	var cursor string
	err := r.db.QueryRow("SELECT cursor FROM sync_meta WHERE source = 'catalog_version'").Scan(&cursor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return strings.TrimSpace(cursor), err
}

func (r *CatalogRepository) LoadSnapshot() (*CatalogSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("catalog repository unavailable")
	}

	tx, err := r.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin catalog snapshot tx: %w", err)
	}
	defer tx.Rollback()

	snap := newEmptySnapshot()
	var ver string
	if err := tx.QueryRow("SELECT cursor FROM sync_meta WHERE source = 'catalog_version'").Scan(&ver); err == nil {
		snap.Version = strings.TrimSpace(ver)
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("query catalog_version: %w", err)
	}

	asnRows, err := tx.Query(`
		SELECT
			asn,
			CASE WHEN override_role != '' THEN override_role ELSE role END,
			network_kind,
			country,
			CASE WHEN override_name_en != '' THEN override_name_en ELSE name_en END,
			CASE WHEN override_name_zh != '' THEN override_name_zh ELSE name_zh END,
			carrier_group,
			source
		FROM asn_catalog
	`)
	if err != nil {
		return nil, fmt.Errorf("query asn_catalog: %w", err)
	}
	defer asnRows.Close()
	for asnRows.Next() {
		var row CatalogASN
		if err := asnRows.Scan(
			&row.ASN, &row.Role, &row.NetworkKind, &row.Country,
			&row.NameEN, &row.NameZH, &row.CarrierGroup, &row.Source,
		); err != nil {
			return nil, err
		}
		if row.ASN > 0 {
			snap.ASNs[row.ASN] = row
		}
	}
	if err := asnRows.Err(); err != nil {
		return nil, err
	}

	aliasRows, err := tx.Query(`
		SELECT alias_key, match_mode, priority, asn, role, canonical_key, name_en, name_zh
		FROM network_aliases
		ORDER BY priority ASC, LENGTH(alias_key) DESC, alias_key ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query network_aliases: %w", err)
	}
	defer aliasRows.Close()
	for aliasRows.Next() {
		var row CatalogAlias
		if err := aliasRows.Scan(
			&row.AliasKey, &row.MatchMode, &row.Priority, &row.ASN,
			&row.Role, &row.CanonicalKey, &row.NameEN, &row.NameZH,
		); err != nil {
			return nil, err
		}
		k := strings.ToLower(strings.TrimSpace(row.AliasKey))
		if k == "" {
			continue
		}
		row.AliasKey = k
		snap.ExactAliases[k] = row
		if row.MatchMode == "contains" {
			snap.SubAliases = append(snap.SubAliases, row)
		}
	}
	if err := aliasRows.Err(); err != nil {
		return nil, err
	}
	sortSubAliases(snap.SubAliases)

	dnsRows, err := tx.Query(`SELECT ip, canonical_key, name_en, name_zh FROM public_dns_ips`)
	if err != nil {
		return nil, fmt.Errorf("query public_dns_ips: %w", err)
	}
	defer dnsRows.Close()
	for dnsRows.Next() {
		var row PublicDNSIP
		if err := dnsRows.Scan(&row.IP, &row.CanonicalKey, &row.NameEN, &row.NameZH); err != nil {
			return nil, err
		}
		ip := strings.TrimSpace(row.IP)
		if ip != "" {
			row.IP = ip
			snap.PublicDNSIPs[ip] = row
		}
	}
	if err := dnsRows.Err(); err != nil {
		return nil, err
	}

	geoRows, err := tx.Query(`SELECT kind, key, name_en FROM geo_names`)
	if err != nil {
		return nil, fmt.Errorf("query geo_names: %w", err)
	}
	defer geoRows.Close()
	for geoRows.Next() {
		var kind, key, nameEN string
		if err := geoRows.Scan(&kind, &key, &nameEN); err != nil {
			return nil, err
		}
		if key == "" || nameEN == "" {
			continue
		}
		switch kind {
		case "province":
			snap.ProvincesEN[key] = nameEN
		case "city":
			snap.CitiesEN[key] = nameEN
		}
	}
	if err := geoRows.Err(); err != nil {
		return nil, err
	}

	return snap, nil
}
