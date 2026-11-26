package sync

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const bgpToolsASNsURL = "https://bgp.tools/asns.csv"

func ParseBGPToolsCSV(data []byte) ([]ASNCatalogRow, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取 bgp.tools CSV 表头失败: %w", err)
	}
	colIdx := map[string]int{}
	for i, h := range header {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	asnCol, okASN := colIdx["asn"]
	nameCol, okName := colIdx["name"]
	classCol, okClass := colIdx["class"]
	ccCol := -1
	if idx, ok := colIdx["cc"]; ok {
		ccCol = idx
	}
	if !okASN || !okName || !okClass {
		return nil, fmt.Errorf("bgp.tools CSV 缺少必要列 (header=%v)", header)
	}

	now := time.Now().Unix()
	var rows []ASNCatalogRow
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(rec) <= asnCol || len(rec) <= nameCol || len(rec) <= classCol {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rec[classCol]), "eyeball") {
			continue
		}
		rawASN := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(rec[asnCol])), "AS")
		asn, err := strconv.ParseUint(rawASN, 10, 64)
		if err != nil || asn == 0 {
			continue
		}
		name := strings.TrimSpace(rec[nameCol])
		if name == "" {
			continue
		}
		cc := ""
		if ccCol >= 0 && len(rec) > ccCol {
			cc = strings.ToUpper(strings.TrimSpace(rec[ccCol]))
		}
		kind := "residential"
		lower := strings.ToLower(name)
		if strings.Contains(lower, "mobile") || strings.Contains(lower, "wireless") || strings.Contains(lower, "cellular") {
			kind = "mobile"
		}
		rows = append(rows, ASNCatalogRow{
			ASN:         asn,
			Role:        "access",
			NetworkKind: kind,
			Country:     cc,
			NameEN:      name,
			Source:      "bgptools",
			UpdatedAt:   now,
		})
	}
	return rows, nil
}

func ApplyBGPToolsDiff(db *sql.DB, items []ASNCatalogRow) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	existing := make(map[uint64]ASNCatalogRow)
	rows, err := db.Query("SELECT asn, role, network_kind, country, name_en, source FROM asn_catalog")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var r ASNCatalogRow
		if err := rows.Scan(&r.ASN, &r.Role, &r.NetworkKind, &r.Country, &r.NameEN, &r.Source); err == nil {
			existing[r.ASN] = r
		}
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, carrier_group, source, updated_at)
		VALUES (?, ?, ?, ?, ?, '', '', 'bgptools', ?)
		ON CONFLICT(asn) DO UPDATE SET
			role = excluded.role,
			network_kind = excluded.network_kind,
			country = CASE WHEN excluded.country != '' THEN excluded.country ELSE asn_catalog.country END,
			name_en = excluded.name_en,
			updated_at = excluded.updated_at
		WHERE asn_catalog.source = 'bgptools'
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	fillCCStmt, err := tx.Prepare("UPDATE asn_catalog SET country = ?, updated_at = ? WHERE asn = ? AND country = ''")
	if err != nil {
		return 0, err
	}
	defer fillCCStmt.Close()

	changed := 0
	for _, item := range items {
		cur, found := existing[item.ASN]
		if found {
			if cur.Source != "bgptools" {
				if cur.Country == "" && item.Country != "" {
					if _, err := fillCCStmt.Exec(item.Country, item.UpdatedAt, item.ASN); err != nil {
						return 0, err
					}
					changed++
				}
				continue
			}
			if cur.Role == item.Role && cur.NetworkKind == item.NetworkKind && cur.Country == item.Country && cur.NameEN == item.NameEN {
				continue
			}
		}
		if _, err := stmt.Exec(item.ASN, item.Role, item.NetworkKind, item.Country, item.NameEN, item.UpdatedAt); err != nil {
			return 0, err
		}
		changed++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

func SyncBGPTools(db *sql.DB, full bool, userAgent string) (int, bool, error) {
	meta, err := GetSyncMeta(db, "bgptools")
	if err != nil {
		return 0, false, err
	}

	headers := map[string]string{}
	if !full && meta.Cursor != "" {
		headers["If-None-Match"] = meta.Cursor
	}
	if userAgent != "" {
		headers["User-Agent"] = userAgent
	}

	resp, err := Fetch(bgpToolsASNsURL, 45*time.Second, headers)
	if err != nil {
		return 0, false, err
	}
	if resp.StatusCode == http.StatusNotModified {
		return 0, true, nil
	}

	items, err := ParseBGPToolsCSV(resp.Body)
	if err != nil {
		return 0, false, err
	}
	changed, err := ApplyBGPToolsDiff(db, items)
	if err != nil {
		return 0, false, err
	}
	etag := resp.Headers.Get("ETag")
	if etag == "" {
		etag = meta.Cursor
	}
	var count int64
	_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE source = 'bgptools'").Scan(&count)
	if err := SetSyncMeta(db, "bgptools", etag, count); err != nil {
		return changed, false, err
	}
	return changed, false, nil
}
