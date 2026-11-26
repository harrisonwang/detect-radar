package sync

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "catalog.db")+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	var mode string
	var timeout int
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || strings.ToLower(mode) != "wal" {
		t.Fatalf("PRAGMA journal_mode = %q err=%v, want wal", mode, err)
	}
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout < 5000 {
		t.Fatalf("PRAGMA busy_timeout = %d err=%v, want >= 5000", timeout, err)
	}
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func TestApplySeed_PreservesManualAndOverrides(t *testing.T) {
	db := openTestDB(t)
	seed := &CatalogSeed{
		ASNCatalog: []ASNCatalogRow{
			{ASN: 4134, Role: "access", NetworkKind: "residential", Country: "CN", NameEN: "China Telecom", NameZH: "中国电信", CarrierGroup: "china-telecom", Source: "seed"},
		},
		NetworkAliases: []NetworkAliasRow{
			{AliasKey: "电信", MatchMode: "contains", Priority: 40, ASN: 4134, Role: "access", CanonicalKey: "china-telecom", NameEN: "China Telecom", NameZH: "中国电信", Source: "seed"},
		},
		PublicDNSIPs: []PublicDNSIPRow{
			{IP: "1.1.1.1", CanonicalKey: "cloudflare", NameEN: "Cloudflare", NameZH: "Cloudflare"},
		},
		GeoNames: []GeoNameRow{
			{Kind: "city", Key: "绵阳", NameEN: "Mianyang", NameZH: "绵阳"},
		},
	}

	if _, err := ApplySeed(db, seed); err != nil {
		t.Fatalf("ApplySeed 1: %v", err)
	}

	if _, err := db.Exec("UPDATE asn_catalog SET override_name_zh = '自定义电信', override_role = 'hosting' WHERE asn = 4134"); err != nil {
		t.Fatalf("set override: %v", err)
	}

	if _, err := ApplySeed(db, seed); err != nil {
		t.Fatalf("ApplySeed 2: %v", err)
	}

	var ovRole, ovZH string
	if err := db.QueryRow("SELECT override_role, override_name_zh FROM asn_catalog WHERE asn = 4134").Scan(&ovRole, &ovZH); err != nil {
		t.Fatalf("scan override: %v", err)
	}
	if ovRole != "hosting" || ovZH != "自定义电信" {
		t.Fatalf("override clobbered: role=%q zh=%q", ovRole, ovZH)
	}
}

func TestPeeringDBDelta_IncrementalUpsertAndSoftDelete(t *testing.T) {
	db := openTestDB(t)
	rawFull := []byte(`{"data":[
		{"asn":7922,"name":"Comcast Cable","info_type":"Cable/DSL/ISP","status":"ok","updated":"2026-11-20T00:00:00Z"},
		{"asn":99999,"name":"Example Mobile Wireless","info_type":"Cable/DSL/ISP","status":"ok","updated":"2026-11-21T00:00:00Z"},
		{"asn":13335,"name":"Cloudflare","info_type":"Content","status":"ok","updated":"2026-11-21T00:00:00Z"}
	]}`)
	delta1, err := ParsePeeringDBDelta(rawFull, false)
	if err != nil {
		t.Fatalf("ParsePeeringDBDelta full: %v", err)
	}
	if len(delta1.Upserts) != 2 {
		t.Fatalf("full upserts = %d, want 2", len(delta1.Upserts))
	}
	if _, err := ApplyPeeringDBDelta(db, delta1); err != nil {
		t.Fatalf("ApplyPeeringDBDelta full: %v", err)
	}

	var kind string
	if err := db.QueryRow("SELECT network_kind FROM asn_catalog WHERE asn = 99999").Scan(&kind); err != nil || kind != "mobile" {
		t.Fatalf("AS99999 kind = %q err=%v, want mobile", kind, err)
	}

	// Incremental soft delete of AS99999
	rawInc := []byte(`{"data":[
		{"asn":99999,"name":"Example Mobile Wireless","info_type":"Cable/DSL/ISP","status":"deleted","updated":"2026-11-22T00:00:00Z"}
	]}`)
	delta2, err := ParsePeeringDBDelta(rawInc, true)
	if err != nil {
		t.Fatalf("ParsePeeringDBDelta inc: %v", err)
	}
	if _, err := ApplyPeeringDBDelta(db, delta2); err != nil {
		t.Fatalf("ApplyPeeringDBDelta inc: %v", err)
	}

	var cnt int
	_ = db.QueryRow("SELECT COUNT(*) FROM asn_catalog WHERE asn = 99999").Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("soft-deleted peeringdb ASN should be removed, count=%d", cnt)
	}
}

func TestBGPToolsDiff_PrecedenceAndIdempotency(t *testing.T) {
	db := openTestDB(t)
	if _, err := ApplySeed(db, &CatalogSeed{
		ASNCatalog: []ASNCatalogRow{
			{ASN: 4134, Role: "access", NetworkKind: "residential", Country: "", NameEN: "China Telecom", NameZH: "中国电信", Source: "seed"},
		},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	csvData := []byte("asn,name,class,cc\nAS4134,Chinanet,Eyeball,CN\nAS10000,\"Nagasaki Cable Media Inc.\",Eyeball,JP\nAS13335,Cloudflare,Content,US\n")
	items, err := ParseBGPToolsCSV(csvData)
	if err != nil {
		t.Fatalf("ParseBGPToolsCSV: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("parsed eyeball items = %d, want 2", len(items))
	}

	changed1, err := ApplyBGPToolsDiff(db, items)
	if err != nil {
		t.Fatalf("ApplyBGPToolsDiff 1: %v", err)
	}
	if changed1 != 2 { // 1 country backfill for AS4134 + 1 insert AS10000
		t.Fatalf("changed1 = %d, want 2", changed1)
	}

	// Seed row's name_en/name_zh must NOT be overwritten by bgptools, but empty country should be backfilled.
	var nameEN, nameZH, cc, src string
	if err := db.QueryRow("SELECT name_en, name_zh, country, source FROM asn_catalog WHERE asn = 4134").Scan(&nameEN, &nameZH, &cc, &src); err != nil {
		t.Fatalf("query AS4134: %v", err)
	}
	if nameEN != "China Telecom" || nameZH != "中国电信" || cc != "CN" || src != "seed" {
		t.Fatalf("AS4134 = (%q, %q, %q, %q)", nameEN, nameZH, cc, src)
	}

	// Second identical diff should be 0 changes.
	changed2, err := ApplyBGPToolsDiff(db, items)
	if err != nil {
		t.Fatalf("ApplyBGPToolsDiff 2: %v", err)
	}
	if changed2 != 0 {
		t.Fatalf("changed2 = %d, want 0", changed2)
	}
}

func TestWikidataParseAndApply(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec("INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, source, updated_at) VALUES (10000, 'access', 'residential', 'JP', 'Nagasaki Cable', '', 'bgptools', 1)")

	raw := []byte(`{"results":{"bindings":[
		{"asn":{"value":"10000"},"enLabel":{"value":"Nagasaki Cable Media"},"zhLabel":{"value":"长崎有线电视"}}
	]}}`)
	labels, err := ParseWikidataSPARQL(raw)
	if err != nil {
		t.Fatalf("ParseWikidataSPARQL: %v", err)
	}
	n, err := ApplyWikidataLabels(db, labels)
	if err != nil || n != 1 {
		t.Fatalf("ApplyWikidataLabels n=%d err=%v", n, err)
	}
	var zh string
	_ = db.QueryRow("SELECT name_zh FROM asn_catalog WHERE asn = 10000").Scan(&zh)
	if zh != "长崎有线电视" {
		t.Fatalf("name_zh = %q, want 长崎有线电视", zh)
	}

	ver, err := BumpCatalogVersion(db)
	if err != nil || ver != 1 {
		t.Fatalf("BumpCatalogVersion = %d, %v", ver, err)
	}
}

func TestPeeringDBDelta_PreservesSeedRowsAndApplySeedPromotes(t *testing.T) {
	db := openTestDB(t)
	if _, err := ApplySeed(db, &CatalogSeed{
		ASNCatalog: []ASNCatalogRow{
			{ASN: 9808, Role: "access", NetworkKind: "mobile", Country: "CN", NameEN: "China Mobile", NameZH: "中国移动", CarrierGroup: "china-mobile", Source: "seed"},
			{ASN: 13335, Role: "public_dns", NetworkKind: "", Country: "", NameEN: "Cloudflare", NameZH: "Cloudflare", CarrierGroup: "cloudflare", Source: "seed"},
		},
	}); err != nil {
		t.Fatalf("ApplySeed: %v", err)
	}

	raw := []byte(`{"data":[
		{"asn":9808,"name":"CMNET Backbone","info_type":"Cable/DSL/ISP","status":"ok","updated":"2026-11-25T00:00:00Z"},
		{"asn":13335,"name":"CloudflareNet","info_type":"Cable/DSL/ISP","status":"ok","updated":"2026-11-25T00:00:00Z"},
		{"asn":4134,"name":"Chinanet Raw","info_type":"Cable/DSL/ISP","status":"ok","updated":"2026-11-25T00:00:00Z"}
	]}`)
	delta, err := ParsePeeringDBDelta(raw, false)
	if err != nil {
		t.Fatalf("ParsePeeringDBDelta: %v", err)
	}
	if _, err := ApplyPeeringDBDelta(db, delta); err != nil {
		t.Fatalf("ApplyPeeringDBDelta: %v", err)
	}

	var role, kind, nameEN, src string
	if err := db.QueryRow("SELECT role, network_kind, name_en, source FROM asn_catalog WHERE asn = 9808").Scan(&role, &kind, &nameEN, &src); err != nil {
		t.Fatalf("query AS9808: %v", err)
	}
	if role != "access" || kind != "mobile" || nameEN != "China Mobile" || src != "seed" {
		t.Fatalf("PeeringDB clobbered seed AS9808: (%q, %q, %q, %q)", role, kind, nameEN, src)
	}
	if err := db.QueryRow("SELECT role, name_en, source FROM asn_catalog WHERE asn = 13335").Scan(&role, &nameEN, &src); err != nil {
		t.Fatalf("query AS13335: %v", err)
	}
	if role != "public_dns" || nameEN != "Cloudflare" || src != "seed" {
		t.Fatalf("PeeringDB clobbered seed public_dns AS13335: (%q, %q, %q)", role, nameEN, src)
	}

	// ApplySeed after PeeringDB must promote peeringdb row (AS4134) to seed with curated names while preserving manual rows.
	if _, err := db.Exec(`INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, source, updated_at) VALUES (4837, 'access', 'residential', 'CN', 'Manual Unicom', '手工联通', 'manual', 1)`); err != nil {
		t.Fatalf("insert manual row: %v", err)
	}
	if _, err := ApplySeed(db, &CatalogSeed{
		ASNCatalog: []ASNCatalogRow{
			{ASN: 4134, Role: "access", NetworkKind: "residential", Country: "CN", NameEN: "China Telecom", NameZH: "中国电信", CarrierGroup: "china-telecom", Source: "seed"},
			{ASN: 4837, Role: "access", NetworkKind: "residential", Country: "CN", NameEN: "China Unicom", NameZH: "中国联通", CarrierGroup: "china-unicom", Source: "seed"},
		},
	}); err != nil {
		t.Fatalf("ApplySeed promote: %v", err)
	}
	var nameZH string
	if err := db.QueryRow("SELECT name_en, name_zh, source FROM asn_catalog WHERE asn = 4134").Scan(&nameEN, &nameZH, &src); err != nil {
		t.Fatalf("query AS4134: %v", err)
	}
	if nameEN != "China Telecom" || nameZH != "中国电信" || src != "seed" {
		t.Fatalf("ApplySeed failed to promote peeringdb row AS4134: (%q, %q, %q)", nameEN, nameZH, src)
	}
	if err := db.QueryRow("SELECT name_en, name_zh, source FROM asn_catalog WHERE asn = 4837").Scan(&nameEN, &nameZH, &src); err != nil {
		t.Fatalf("query AS4837: %v", err)
	}
	if nameEN != "Manual Unicom" || nameZH != "手工联通" || src != "manual" {
		t.Fatalf("ApplySeed overwrote manual row AS4837: (%q, %q, %q)", nameEN, nameZH, src)
	}
}

func TestSyncWikidata_CursorProgressionAndWrapAround(t *testing.T) {
	db := openTestDB(t)
	for _, asn := range []uint64{100, 200, 300, 400, 500} {
		if _, err := db.Exec(`INSERT INTO asn_catalog (asn, role, network_kind, country, name_en, name_zh, source, updated_at) VALUES (?, 'access', 'residential', 'JP', 'ISP', '', 'bgptools', 1)`, asn); err != nil {
			t.Fatalf("insert asn %d: %v", asn, err)
		}
	}

	var seenQueries []string
	stubFetch := func(rawURL string, _ time.Duration, _ map[string]string) (*HTTPResponse, error) {
		seenQueries = append(seenQueries, rawURL)
		body := `{"results":{"bindings":[]}}`
		if strings.Contains(rawURL, "%22400%22") {
			body = `{"results":{"bindings":[{"asn":{"value":"400"},"enLabel":{"value":"ISP 400"},"zhLabel":{"value":"运营商400"}}]}}`
		}
		return &HTTPResponse{StatusCode: http.StatusOK, Body: []byte(body)}, nil
	}

	// Run 1: scans 100, 200 (0 matches) -> cursor advances to 200
	n1, err := syncWikidataWithFetch(db, 2, "test", stubFetch)
	if err != nil || n1 != 0 {
		t.Fatalf("run 1 = (%d, %v), want (0, nil)", n1, err)
	}
	meta1, _ := GetSyncMeta(db, "wikidata")
	if meta1.Cursor != "200" {
		t.Fatalf("run 1 cursor = %q, want 200", meta1.Cursor)
	}

	// Run 2: scans 300, 400 (translates 400) -> cursor advances to 400
	n2, err := syncWikidataWithFetch(db, 2, "test", stubFetch)
	if err != nil || n2 != 1 {
		t.Fatalf("run 2 = (%d, %v), want (1, nil)", n2, err)
	}
	meta2, _ := GetSyncMeta(db, "wikidata")
	if meta2.Cursor != "400" {
		t.Fatalf("run 2 cursor = %q, want 400", meta2.Cursor)
	}
	var zh400 string
	_ = db.QueryRow("SELECT name_zh FROM asn_catalog WHERE asn = 400").Scan(&zh400)
	if zh400 != "运营商400" {
		t.Fatalf("AS400 name_zh = %q, want 运营商400", zh400)
	}

	// Run 3: scans 500 (< limit 2) -> wraps cursor back to 0
	n3, err := syncWikidataWithFetch(db, 2, "test", stubFetch)
	if err != nil || n3 != 0 {
		t.Fatalf("run 3 = (%d, %v), want (0, nil)", n3, err)
	}
	meta3, _ := GetSyncMeta(db, "wikidata")
	if meta3.Cursor != "0" {
		t.Fatalf("run 3 cursor = %q, want 0 (wrap-around)", meta3.Cursor)
	}
	if len(seenQueries) != 3 {
		t.Fatalf("expected 3 wikidata requests, got %d", len(seenQueries))
	}
}
