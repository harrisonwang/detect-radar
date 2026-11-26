package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const peeringDBNetBaseURL = "https://www.peeringdb.com/api/net"

type peeringDBNetItem struct {
	ASN      uint64 `json:"asn"`
	Name     string `json:"name"`
	InfoType string `json:"info_type"`
	Status   string `json:"status"`
	Updated  string `json:"updated"`
}

type peeringDBNetResp struct {
	Data []peeringDBNetItem `json:"data"`
}

type PeeringDBDelta struct {
	Upserts    []ASNCatalogRow
	Deletes    []uint64
	MaxUpdated int64
}

func classifyPeeringDB(infoType, name string) (role, kind string) {
	switch strings.ToLower(strings.TrimSpace(infoType)) {
	case "cable/dsl/isp":
		lower := strings.ToLower(name)
		if strings.Contains(lower, "mobile") || strings.Contains(lower, "wireless") || strings.Contains(lower, "cellular") {
			return "access", "mobile"
		}
		return "access", "residential"
	case "educational/research":
		return "access", "education"
	case "nsp":
		return "transit", ""
	case "content":
		return "content", ""
	case "enterprise":
		return "enterprise", ""
	default:
		return "other", ""
	}
}

func parsePeeringDBTimestamp(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ts
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func ParsePeeringDBDelta(data []byte, incremental bool) (*PeeringDBDelta, error) {
	var resp peeringDBNetResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 PeeringDB JSON 失败: %w", err)
	}
	delta := &PeeringDBDelta{}
	now := time.Now().Unix()

	for _, item := range resp.Data {
		if item.ASN == 0 {
			continue
		}
		ts := parsePeeringDBTimestamp(item.Updated)
		if ts > delta.MaxUpdated {
			delta.MaxUpdated = ts
		}
		if strings.EqualFold(strings.TrimSpace(item.Status), "deleted") {
			delta.Deletes = append(delta.Deletes, item.ASN)
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		role, kind := classifyPeeringDB(item.InfoType, name)
		if !incremental && role != "access" {
			continue
		}
		updatedAt := ts
		if updatedAt == 0 {
			updatedAt = now
		}
		delta.Upserts = append(delta.Upserts, ASNCatalogRow{
			ASN:         item.ASN,
			Role:        role,
			NetworkKind: kind,
			NameEN:      name,
			Source:      "peeringdb",
			UpdatedAt:   updatedAt,
		})
	}
	return delta, nil
}

func ApplyPeeringDBDelta(db *sql.DB, delta *PeeringDBDelta) (int, error) {
	if delta == nil || (len(delta.Upserts) == 0 && len(delta.Deletes) == 0) {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	changed := 0
	if len(delta.Deletes) > 0 {
		delStmt, err := tx.Prepare("DELETE FROM asn_catalog WHERE asn = ? AND source = 'peeringdb' AND override_role = ''")
		if err != nil {
			return 0, err
		}
		defer delStmt.Close()
		for _, asn := range delta.Deletes {
			res, err := delStmt.Exec(asn)
			if err != nil {
				return 0, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				changed += int(n)
			}
		}
	}

	if len(delta.Upserts) > 0 {
		upsertStmt, err := tx.Prepare(`
			INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, carrier_group, source, updated_at)
			VALUES (?, ?, ?, '', ?, '', '', 'peeringdb', ?)
			ON CONFLICT(asn) DO UPDATE SET
				role = CASE WHEN asn_catalog.source IN ('manual', 'seed') THEN asn_catalog.role ELSE excluded.role END,
				network_kind = CASE WHEN asn_catalog.source IN ('manual', 'seed') THEN asn_catalog.network_kind ELSE excluded.network_kind END,
				name_en = CASE WHEN asn_catalog.source IN ('manual', 'seed') THEN asn_catalog.name_en ELSE excluded.name_en END,
				source = CASE WHEN asn_catalog.source IN ('manual', 'seed') THEN asn_catalog.source ELSE 'peeringdb' END,
				updated_at = CASE WHEN asn_catalog.source IN ('manual', 'seed') THEN asn_catalog.updated_at ELSE excluded.updated_at END
		`)
		if err != nil {
			return 0, err
		}
		defer upsertStmt.Close()

		for _, r := range delta.Upserts {
			if r.Role != "access" {
				// Incremental updates may reclassify an existing peeringdb ASN away from access.
				res, err := tx.Exec(`
					UPDATE asn_catalog
					SET role = ?, network_kind = ?, name_en = ?, updated_at = ?
					WHERE asn = ? AND source = 'peeringdb' AND override_role = ''
				`, r.Role, r.NetworkKind, r.NameEN, r.UpdatedAt, r.ASN)
				if err != nil {
					return 0, err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					changed += int(n)
				}
				continue
			}
			if _, err := upsertStmt.Exec(r.ASN, r.Role, r.NetworkKind, r.NameEN, r.UpdatedAt); err != nil {
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

func SyncPeeringDB(db *sql.DB, apiKey string, full bool, userAgent string) (int, error) {
	meta, err := GetSyncMeta(db, "peeringdb")
	if err != nil {
		return 0, err
	}
	since := strings.TrimSpace(meta.Cursor)
	incremental := !full && since != ""

	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Api-Key " + apiKey
	}
	if userAgent != "" {
		headers["User-Agent"] = userAgent
	}

	var totalChanged int
	var maxUpdated int64
	if curTS, err := strconv.ParseInt(since, 10, 64); err == nil {
		maxUpdated = curTS
	}

	urls := []string{
		peeringDBNetBaseURL + "?info_type=Cable/DSL/ISP",
		peeringDBNetBaseURL + "?info_type=Educational/Research",
	}
	if incremental {
		urls = []string{peeringDBNetBaseURL + "?since=" + since}
	}

	for _, u := range urls {
		resp, err := Fetch(u, 45*time.Second, headers)
		if err != nil {
			return totalChanged, err
		}
		delta, err := ParsePeeringDBDelta(resp.Body, incremental)
		if err != nil {
			return totalChanged, err
		}
		n, err := ApplyPeeringDBDelta(db, delta)
		if err != nil {
			return totalChanged, err
		}
		totalChanged += n
		if delta.MaxUpdated > maxUpdated {
			maxUpdated = delta.MaxUpdated
		}
	}

	if maxUpdated == 0 {
		maxUpdated = time.Now().Unix()
	}
	var count int64
	_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE source = 'peeringdb'").Scan(&count)
	if err := SetSyncMeta(db, "peeringdb", strconv.FormatInt(maxUpdated, 10), count); err != nil {
		return totalChanged, err
	}
	return totalChanged, nil
}
