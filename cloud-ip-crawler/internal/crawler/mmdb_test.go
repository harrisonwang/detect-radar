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

// 导出的核心约定：同一 IP 多条归属时，MMDB 里留的必须是置信度最高的那条
// （official > asn > hosting），且各字段完整可读回。
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
	// hosting 的 /21 与 aws 官方的 /22 重叠：/22 内 aws 赢，/21 其余部分归 hosting；
	// cloudflare 的 v6 段验证 v6 也能进 MMDB
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
	// 老库里可能残留保留网段（新代码入库时已滤掉，这里绕过 createRange 直插模拟），
	// v4 / v6 都必须被导出静默跳过而不是整体失败
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

	// 重叠处：官方层必须覆盖 hosting 层
	if rec, ok := lookup("52.94.76.10"); !ok || rec.Provider != "aws" || rec.Tier != "official" || rec.Region != "us-west-2" {
		t.Errorf("52.94.76.10 应归 aws/official，实际: %+v (found=%v)", rec, ok)
	}
	// /21 里 /22 之外的部分：还是 hosting
	if rec, ok := lookup("52.94.72.1"); !ok || rec.Provider != "hosting" || rec.Tier != "hosting" {
		t.Errorf("52.94.72.1 应归 hosting，实际: %+v (found=%v)", rec, ok)
	}
	// 命名 ASN 层
	if rec, ok := lookup("5.9.0.1"); !ok || rec.Provider != "hetzner" || rec.Tier != "asn" || rec.Service != "AS24940" {
		t.Errorf("5.9.0.1 应归 hetzner/asn，实际: %+v (found=%v)", rec, ok)
	}
	// IPv6 原生查询
	if rec, ok := lookup("2606:4700::1111"); !ok || rec.Provider != "cloudflare" || rec.Tier != "official" {
		t.Errorf("2606:4700::1111 应归 cloudflare/official，实际: %+v (found=%v)", rec, ok)
	}
	// 库外地址不该命中（v4、v6 各一个；fdab:: 是被跳过的 legacy 保留段）
	if rec, ok := lookup("9.9.9.9"); ok {
		t.Errorf("9.9.9.9 不应命中，实际: %+v", rec)
	}
	if rec, ok := lookup("fdab::1"); ok {
		t.Errorf("fdab::1 不应命中，实际: %+v", rec)
	}
}
