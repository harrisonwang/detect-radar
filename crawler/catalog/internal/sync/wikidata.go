package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const wikidataSPARQLURL = "https://query.wikidata.org/sparql"

type WikidataLabel struct {
	ASN    uint64
	NameEN string
	NameZH string
}

type sparqlResp struct {
	Results struct {
		Bindings []struct {
			ASN     struct{ Value string } `json:"asn"`
			ENLabel struct{ Value string } `json:"enLabel"`
			ZHLabel struct{ Value string } `json:"zhLabel"`
		} `json:"bindings"`
	} `json:"results"`
}

func ParseWikidataSPARQL(data []byte) ([]WikidataLabel, error) {
	var resp sparqlResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 Wikidata SPARQL JSON 失败: %w", err)
	}
	byASN := make(map[uint64]WikidataLabel)
	for _, b := range resp.Results.Bindings {
		raw := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(b.ASN.Value)), "AS")
		asn, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || asn == 0 {
			continue
		}
		cur := byASN[asn]
		cur.ASN = asn
		if cur.NameEN == "" && strings.TrimSpace(b.ENLabel.Value) != "" {
			cur.NameEN = strings.TrimSpace(b.ENLabel.Value)
		}
		if cur.NameZH == "" && strings.TrimSpace(b.ZHLabel.Value) != "" {
			cur.NameZH = strings.TrimSpace(b.ZHLabel.Value)
		}
		byASN[asn] = cur
	}
	out := make([]WikidataLabel, 0, len(byASN))
	for _, v := range byASN {
		if v.NameZH != "" || v.NameEN != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func buildWikidataASNQuery(asns []uint64) string {
	vals := make([]string, 0, len(asns))
	for _, asn := range asns {
		vals = append(vals, fmt.Sprintf(`"%d"`, asn))
	}
	return fmt.Sprintf(`
SELECT ?asn ?enLabel ?zhLabel WHERE {
  VALUES ?asn { %s }
  ?item wdt:P3797 ?asn .
  OPTIONAL { ?item rdfs:label ?enLabel FILTER(LANG(?enLabel) = "en") }
  OPTIONAL { ?item rdfs:label ?zhLabel FILTER(LANG(?zhLabel) = "zh-hans" || LANG(?zhLabel) = "zh-cn" || LANG(?zhLabel) = "zh") }
}`, strings.Join(vals, " "))
}

func ApplyWikidataLabels(db *sql.DB, labels []WikidataLabel) (int, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		UPDATE asn_catalog
		SET name_zh = CASE WHEN name_zh = '' AND ? != '' THEN ? ELSE name_zh END,
		    name_en = CASE WHEN name_en = '' AND ? != '' THEN ? ELSE name_en END,
		    updated_at = ?
		WHERE asn = ? AND override_name_zh = '' AND ((name_zh = '' AND ? != '') OR (name_en = '' AND ? != ''))
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	changed := 0
	for _, l := range labels {
		res, err := stmt.Exec(l.NameZH, l.NameZH, l.NameEN, l.NameEN, now, l.ASN, l.NameZH, l.NameEN)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed += int(n)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

type httpFetchFunc func(url string, timeout time.Duration, headers map[string]string) (*HTTPResponse, error)

func queryWikidataPendingASNs(db *sql.DB, afterASN uint64, limit int) ([]uint64, error) {
	rows, err := db.Query(`
		SELECT asn FROM asn_catalog
		WHERE role = 'access' AND name_zh = '' AND override_name_zh = '' AND asn > ?
		ORDER BY asn ASC LIMIT ?
	`, afterASN, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var asns []uint64
	for rows.Next() {
		var asn uint64
		if err := rows.Scan(&asn); err == nil && asn > 0 {
			asns = append(asns, asn)
		}
	}
	return asns, rows.Err()
}

func SyncWikidata(db *sql.DB, limit int, userAgent string) (int, error) {
	return syncWikidataWithFetch(db, limit, userAgent, Fetch)
}

func syncWikidataWithFetch(db *sql.DB, limit int, userAgent string, fetchFn httpFetchFunc) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	meta, err := GetSyncMeta(db, "wikidata")
	if err != nil {
		return 0, err
	}
	lastASN, _ := strconv.ParseUint(strings.TrimSpace(meta.Cursor), 10, 64)

	asns, err := queryWikidataPendingASNs(db, lastASN, limit)
	if err != nil {
		return 0, err
	}
	if len(asns) == 0 && lastASN > 0 {
		asns, err = queryWikidataPendingASNs(db, 0, limit)
		if err != nil {
			return 0, err
		}
	}
	if len(asns) == 0 {
		var zhCount int64
		_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE name_zh != ''").Scan(&zhCount)
		_ = SetSyncMeta(db, "wikidata", "0", zhCount)
		return 0, nil
	}

	headers := map[string]string{
		"Accept": "application/sparql-results+json",
	}
	if userAgent != "" {
		headers["User-Agent"] = userAgent
	}

	totalChanged := 0
	var processedMax uint64
	const batchSize = 50
	for i := 0; i < len(asns); i += batchSize {
		end := i + batchSize
		if end > len(asns) {
			end = len(asns)
		}
		batch := asns[i:end]
		q := buildWikidataASNQuery(batch)
		reqURL := wikidataSPARQLURL + "?format=json&query=" + url.QueryEscape(q)
		resp, err := fetchFn(reqURL, 30*time.Second, headers)
		if err != nil {
			if processedMax > 0 {
				var zhCount int64
				_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE name_zh != ''").Scan(&zhCount)
				_ = SetSyncMeta(db, "wikidata", strconv.FormatUint(processedMax, 10), zhCount)
			}
			return totalChanged, err
		}
		labels, err := ParseWikidataSPARQL(resp.Body)
		if err != nil {
			return totalChanged, err
		}
		n, err := ApplyWikidataLabels(db, labels)
		if err != nil {
			return totalChanged, err
		}
		totalChanged += n
		processedMax = batch[len(batch)-1]
	}

	nextCursor := strconv.FormatUint(processedMax, 10)
	if len(asns) < limit {
		nextCursor = "0"
	}
	var zhCount int64
	_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE name_zh != ''").Scan(&zhCount)
	if err := SetSyncMeta(db, "wikidata", nextCursor, zhCount); err != nil {
		return totalChanged, err
	}
	return totalChanged, nil
}
