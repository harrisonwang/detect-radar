package crawler

import (
	"database/sql"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	maxminddb "github.com/oschwald/maxminddb-golang/v2"
	_ "modernc.org/sqlite"
)

func TestExportMMDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	defer db.Close()
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	mustRange := func(provider, cidr, region, service string) Range {
		r, err := createRange(provider, cidr, region, service)
		if err != nil {
			t.Fatalf("构造 Range 失败 %s: %v", cidr, err)
		}
		return *r
	}
	seed := map[string][]Range{
		"hosting":    {mustRange("hosting", "52.94.72.0/21", "US", "AS16509 AMAZON-02")},
		"aws":        {mustRange("aws", "52.94.76.0/22", "us-west-2", "AMAZON")},
		"hetzner":    {mustRange("hetzner", "5.9.0.0/16", "DE", "AS24940")},
		"cloudflare": {mustRange("cloudflare", "2606:4700::/32", "", "cdn")},
	}
	for provider, ranges := range seed {
		if _, err := InsertRanges(db, provider, ranges); err != nil {
			t.Fatalf("入库 %s 失败: %v", provider, err)
		}
	}
	for _, legacyCIDR := range []string{"192.0.2.0/24", "fdab::/48"} {
		start, end, version, err := parseCIDR(legacyCIDR)
		if err != nil {
			t.Fatalf("parseCIDR: %v", err)
		}
		legacy := Range{Provider: "vultr", CIDR: legacyCIDR, IPStart: start, IPEnd: end, IPVersion: version}
		if _, err := InsertRanges(db, "vultr", []Range{legacy}); err != nil {
			t.Fatalf("入库 legacy 行失败: %v", err)
		}
	}

	out := filepath.Join(t.TempDir(), "test.mmdb")
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("创建输出文件失败: %v", err)
	}
	n, err := ExportMMDB(db, f)
	f.Close()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if n != 4 {
		t.Errorf("应写入 4 个网段（2 条 legacy 保留段被跳过），实际 %d", n)
	}

	reader, err := maxminddb.Open(out)
	if err != nil {
		t.Fatalf("读回 MMDB 失败: %v", err)
	}
	defer reader.Close()

	type mmdbRecord struct {
		Provider string `maxminddb:"provider"`
		Tier     string `maxminddb:"tier"`
		Region   string `maxminddb:"region"`
		Service  string `maxminddb:"service"`
	}
	lookup := func(ip string) (mmdbRecord, bool) {
		var rec mmdbRecord
		res := reader.Lookup(netip.MustParseAddr(ip))
		if !res.Found() {
			return rec, false
		}
		if err := res.Decode(&rec); err != nil {
			t.Fatalf("解码 %s 失败: %v", ip, err)
		}
		return rec, true
	}

	if rec, ok := lookup("52.94.76.10"); !ok || rec.Provider != "aws" || rec.Tier != "official" || rec.Region != "us-west-2" {
		t.Errorf("52.94.76.10 应归 aws/official，实际: %+v (found=%v)", rec, ok)
	}
	if rec, ok := lookup("52.94.72.1"); !ok || rec.Provider != "hosting" || rec.Tier != "hosting" {
		t.Errorf("52.94.72.1 应归 hosting，实际: %+v (found=%v)", rec, ok)
	}
	if rec, ok := lookup("5.9.0.1"); !ok || rec.Provider != "hetzner" || rec.Tier != "asn" || rec.Service != "AS24940" {
		t.Errorf("5.9.0.1 应归 hetzner/asn，实际: %+v (found=%v)", rec, ok)
	}
	if rec, ok := lookup("2606:4700::1111"); !ok || rec.Provider != "cloudflare" || rec.Tier != "official" {
		t.Errorf("2606:4700::1111 应归 cloudflare/official，实际: %+v (found=%v)", rec, ok)
	}
	if rec, ok := lookup("9.9.9.9"); ok {
		t.Errorf("9.9.9.9 不应命中，实际: %+v", rec)
	}
	if rec, ok := lookup("fdab::1"); ok {
		t.Errorf("fdab::1 不应命中，实际: %+v", rec)
	}
}
