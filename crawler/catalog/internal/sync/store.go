package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const catalogSchemaSQL = `
CREATE TABLE IF NOT EXISTS asn_catalog (
    asn              INTEGER PRIMARY KEY,
    role             TEXT NOT NULL DEFAULT '',
    network_kind     TEXT NOT NULL DEFAULT '',
    country          TEXT NOT NULL DEFAULT '',
    name_en          TEXT NOT NULL DEFAULT '',
    name_zh          TEXT NOT NULL DEFAULT '',
    carrier_group    TEXT NOT NULL DEFAULT '',
    source           TEXT NOT NULL DEFAULT 'seed',
    override_role    TEXT NOT NULL DEFAULT '',
    override_name_en TEXT NOT NULL DEFAULT '',
    override_name_zh TEXT NOT NULL DEFAULT '',
    updated_at       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_asn_catalog_role ON asn_catalog(role);
CREATE TABLE IF NOT EXISTS network_aliases (
    alias_key     TEXT PRIMARY KEY,
    match_mode    TEXT NOT NULL DEFAULT 'exact',
    priority      INTEGER NOT NULL DEFAULT 100,
    asn           INTEGER NOT NULL DEFAULT 0,
    role          TEXT NOT NULL DEFAULT '',
    canonical_key TEXT NOT NULL DEFAULT '',
    name_en       TEXT NOT NULL DEFAULT '',
    name_zh       TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT 'seed',
    updated_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_network_aliases_mode ON network_aliases(match_mode, priority);
CREATE TABLE IF NOT EXISTS public_dns_ips (
    ip            TEXT PRIMARY KEY,
    canonical_key TEXT NOT NULL,
    name_en       TEXT NOT NULL,
    name_zh       TEXT NOT NULL,
    updated_at    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS geo_names (
    kind    TEXT NOT NULL,
    key     TEXT NOT NULL,
    name_en TEXT NOT NULL,
    name_zh TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (kind, key)
);
CREATE TABLE IF NOT EXISTS sync_meta (
    source         TEXT PRIMARY KEY,
    cursor         TEXT NOT NULL DEFAULT '',
    row_count      INTEGER NOT NULL DEFAULT 0,
    last_synced_at INTEGER NOT NULL DEFAULT 0
);
`

type ASNCatalogRow struct {
	ASN          uint64 `json:"asn"`
	Role         string `json:"role"`
	NetworkKind  string `json:"network_kind"`
	Country      string `json:"country"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
	CarrierGroup string `json:"carrier_group"`
	Source       string `json:"source"`
	UpdatedAt    int64  `json:"updated_at,omitempty"`
}

type NetworkAliasRow struct {
	AliasKey     string `json:"alias_key"`
	MatchMode    string `json:"match_mode"`
	Priority     int    `json:"priority"`
	ASN          uint64 `json:"asn"`
	Role         string `json:"role"`
	CanonicalKey string `json:"canonical_key"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
	Source       string `json:"source"`
}

type PublicDNSIPRow struct {
	IP           string `json:"ip"`
	CanonicalKey string `json:"canonical_key"`
	NameEN       string `json:"name_en"`
	NameZH       string `json:"name_zh"`
}

type GeoNameRow struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	NameEN string `json:"name_en"`
	NameZH string `json:"name_zh"`
}

type CatalogSeed struct {
	ASNCatalog     []ASNCatalogRow   `json:"asn_catalog"`
	NetworkAliases []NetworkAliasRow `json:"network_aliases"`
	PublicDNSIPs   []PublicDNSIPRow  `json:"public_dns_ips"`
	GeoNames       []GeoNameRow      `json:"geo_names"`
}

type SyncMeta struct {
	Source       string
	Cursor       string
	RowCount     int64
	LastSyncedAt int64
}

func EnsureSchema(db *sql.DB) error {
	_, err := db.Exec(catalogSchemaSQL)
	return err
}

func GetSyncMeta(db *sql.DB, source string) (SyncMeta, error) {
	var m SyncMeta
	m.Source = source
	err := db.QueryRow(
		"SELECT cursor, row_count, last_synced_at FROM sync_meta WHERE source = ?",
		source,
	).Scan(&m.Cursor, &m.RowCount, &m.LastSyncedAt)
	if err == sql.ErrNoRows {
		return m, nil
	}
	return m, err
}

func SetSyncMeta(db *sql.DB, source, cursor string, rowCount int64) error {
	_, err := db.Exec(`
		INSERT INTO sync_meta (source, cursor, row_count, last_synced_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(source) DO UPDATE SET
			cursor = excluded.cursor,
			row_count = excluded.row_count,
			last_synced_at = excluded.last_synced_at
	`, source, cursor, rowCount, time.Now().Unix())
	return err
}

func BumpCatalogVersion(db *sql.DB) (int64, error) {
	meta, err := GetSyncMeta(db, "catalog_version")
	if err != nil {
		return 0, err
	}
	cur, _ := strconv.ParseInt(strings.TrimSpace(meta.Cursor), 10, 64)
	next := cur + 1
	var total int64
	_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog").Scan(&total)
	if err := SetSyncMeta(db, "catalog_version", strconv.FormatInt(next, 10), total); err != nil {
		return 0, err
	}
	return next, nil
}

func LoadSeedFile(path string) (*CatalogSeed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var seed CatalogSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		return nil, fmt.Errorf("解析种子文件失败: %w", err)
	}
	return &seed, nil
}

func ApplySeed(db *sql.DB, seed *CatalogSeed) (int, error) {
	if seed == nil {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().Unix()
	changed := 0

	if len(seed.ASNCatalog) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, carrier_group, source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(asn) DO UPDATE SET
				role = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.role ELSE excluded.role END,
				network_kind = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.network_kind ELSE excluded.network_kind END,
				country = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.country WHEN excluded.country != '' THEN excluded.country ELSE asn_catalog.country END,
				name_en = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.name_en WHEN excluded.name_en != '' THEN excluded.name_en ELSE asn_catalog.name_en END,
				name_zh = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.name_zh WHEN excluded.name_zh != '' THEN excluded.name_zh ELSE asn_catalog.name_zh END,
				carrier_group = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.carrier_group WHEN excluded.carrier_group != '' THEN excluded.carrier_group ELSE asn_catalog.carrier_group END,
				source = CASE WHEN asn_catalog.source = 'manual' THEN asn_catalog.source ELSE excluded.source END,
				updated_at = excluded.updated_at
		`)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
		for _, r := range seed.ASNCatalog {
			if r.ASN == 0 {
				continue
			}
			src := r.Source
			if src == "" {
				src = "seed"
			}
			if _, err := stmt.Exec(r.ASN, r.Role, r.NetworkKind, r.Country, r.NameEN, r.NameZH, r.CarrierGroup, src, now); err != nil {
				return 0, err
			}
			changed++
		}
	}

	if len(seed.NetworkAliases) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO network_aliases (alias_key, match_mode, priority, asn, role, canonical_key, name_en, name_zh, source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(alias_key) DO UPDATE SET
				match_mode = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.match_mode ELSE excluded.match_mode END,
				priority = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.priority ELSE excluded.priority END,
				asn = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.asn ELSE excluded.asn END,
				role = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.role ELSE excluded.role END,
				canonical_key = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.canonical_key ELSE excluded.canonical_key END,
				name_en = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.name_en ELSE excluded.name_en END,
				name_zh = CASE WHEN network_aliases.source = 'manual' THEN network_aliases.name_zh ELSE excluded.name_zh END,
				updated_at = excluded.updated_at
		`)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
		for _, a := range seed.NetworkAliases {
			k := strings.ToLower(strings.TrimSpace(a.AliasKey))
			if k == "" {
				continue
			}
			mode := a.MatchMode
			if mode == "" {
				mode = "exact"
			}
			prio := a.Priority
			if prio == 0 {
				prio = 100
			}
			src := a.Source
			if src == "" {
				src = "seed"
			}
			if _, err := stmt.Exec(k, mode, prio, a.ASN, a.Role, a.CanonicalKey, a.NameEN, a.NameZH, src, now); err != nil {
				return 0, err
			}
			changed++
		}
	}

	if len(seed.PublicDNSIPs) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO public_dns_ips (ip, canonical_key, name_en, name_zh, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(ip) DO UPDATE SET
				canonical_key = excluded.canonical_key,
				name_en = excluded.name_en,
				name_zh = excluded.name_zh,
				updated_at = excluded.updated_at
		`)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
		for _, p := range seed.PublicDNSIPs {
			ip := strings.TrimSpace(p.IP)
			if ip == "" {
				continue
			}
			if _, err := stmt.Exec(ip, p.CanonicalKey, p.NameEN, p.NameZH, now); err != nil {
				return 0, err
			}
			changed++
		}
	}

	if len(seed.GeoNames) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO geo_names (kind, key, name_en, name_zh)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(kind, key) DO UPDATE SET
				name_en = excluded.name_en,
				name_zh = excluded.name_zh
		`)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
		for _, g := range seed.GeoNames {
			if g.Kind == "" || g.Key == "" || g.NameEN == "" {
				continue
			}
			if _, err := stmt.Exec(g.Kind, g.Key, g.NameEN, g.NameZH); err != nil {
				return 0, err
			}
			changed++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}
