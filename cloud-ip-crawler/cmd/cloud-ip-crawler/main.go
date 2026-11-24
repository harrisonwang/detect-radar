// cloud-ip-crawler 抓取云厂商 / 机房 / 托管商的 IP 段（IPv4 + IPv6），
// 汇总成一个 SQLite 数据库，用来回答「这个 IP 是不是机房 IP」。
// 各家 v6 覆盖不齐（Oracle 的官方文件就没有 v6），按厂商粒度记录在 README。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/harrisonwang/detect-radar/cloud-ip-crawler/internal/crawler"

	_ "modernc.org/sqlite" // 纯 Go 实现，无需 CGO，可交叉编译到所有平台
)

func main() {
	// 子命令：lookup 查归属，export 导出 MMDB，都不走抓取流程
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "lookup":
			os.Exit(runLookup(os.Args[2:]))
		case "export":
			os.Exit(runExport(os.Args[2:]))
		}
	}

	// ASN 系厂商清单在 asn.go 里维护，帮助文本动态生成，加厂商不用改这里
	asnCrawlers := crawler.ASNCrawlers()
	asnNames := make([]string, 0, len(asnCrawlers))
	for _, c := range asnCrawlers {
		asnNames = append(asnNames, c.Name())
	}

	dbPath := flag.String("db", "cloud-ip.db", "SQLite 数据库路径（不存在则自动创建）")
	providers := flag.String("providers", "all", "要爬取的云厂商，逗号分隔，或 all。发官方文件的：aws,gcp,azure,cloudflare,oracle,digitalocean,linode,vultr,fastly,bunny,zscaler；走 ASN 的："+strings.Join(asnNames, ",")+"；启发式兜底层：hosting")
	dryRun := flag.Bool("dry-run", false, "仅抓取并打印样例，不写数据库")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("cloud-ip-crawler", crawler.Version)
		return
	}

	crawlers := map[string]crawler.Crawler{
		"aws":          &crawler.AWSCrawler{},
		"gcp":          &crawler.GCPCrawler{},
		"azure":        &crawler.AzureCrawler{},
		"cloudflare":   &crawler.CloudflareCrawler{},
		"oracle":       &crawler.OracleCrawler{},
		"digitalocean": &crawler.DigitalOceanCrawler{},
		"linode":       &crawler.LinodeCrawler{},
		"vultr":        &crawler.VultrCrawler{},
		"fastly":       &crawler.FastlyCrawler{},
		"bunny":        &crawler.BunnyCrawler{},
		"zscaler":      &crawler.ZscalerCrawler{},
	}

	// map 迭代序随机，固定顺序让日志可比对
	order := []string{"aws", "gcp", "azure", "cloudflare", "oracle", "digitalocean", "linode", "vultr", "fastly", "bunny", "zscaler"}

	// 大机房 / VPS 商家大多不发官方文件，改从 ASN 取宣告前缀（iptoasn 全量表），每家一个爬虫
	for _, c := range asnCrawlers {
		crawlers[c.Name()] = c
		order = append(order, c.Name())
	}

	// 兜底层：按 AS 名称关键词把「叫不出名字」的托管商也收进来，放最后（最泛、置信度最低）
	crawlers["hosting"] = &crawler.HostingCrawler{}
	order = append(order, "hosting")

	var selected []crawler.Crawler
	if *providers == "all" {
		for _, name := range order {
			selected = append(selected, crawlers[name])
		}
	} else {
		for _, name := range strings.Split(*providers, ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			c, ok := crawlers[name]
			if !ok {
				log.Fatalf("未知的云厂商 %q，可选：%s", name, strings.Join(order, ", "))
			}
			selected = append(selected, c)
		}
	}

	var db *sql.DB
	if !*dryRun {
		var err error
		db, err = sql.Open("sqlite", *dbPath)
		if err != nil {
			log.Fatalf("无法打开数据库 %s: %v", *dbPath, err)
		}
		defer db.Close()
		if err := crawler.EnsureSchema(db); err != nil {
			log.Fatalf("建表失败: %v", err)
		}
	}

	total, failed := 0, 0
	for _, c := range selected {
		log.Printf("开始爬取 %s...", c.Name())
		start := time.Now()

		ranges, err := c.Crawl()
		if err != nil {
			log.Printf("爬取 %s 失败: %v", c.Name(), err)
			failed++
			continue
		}
		// 0 条视为失败：多半是上游改版而不是真没数据，直接入库会静默清空该厂商
		if len(ranges) == 0 {
			log.Printf("爬取 %s 拿到 0 条数据，视为失败", c.Name())
			failed++
			continue
		}
		log.Printf("%s 获取到 %d 条 IP 范围, 耗时 %v", c.Name(), len(ranges), time.Since(start))

		if *dryRun {
			for i, r := range ranges {
				if i >= 5 {
					fmt.Printf("  ... 还有 %d 条\n", len(ranges)-5)
					break
				}
				fmt.Printf("  %s: %s (region=%s, service=%s)\n", r.Provider, r.CIDR, r.Region, r.Service)
			}
		} else {
			inserted, err := crawler.InsertRanges(db, c.Name(), ranges)
			if err != nil {
				log.Printf("入库 %s 失败: %v", c.Name(), err)
				failed++
				continue
			}
			log.Printf("%s 入库 %d 条", c.Name(), inserted)
		}
		total += len(ranges)
	}

	log.Printf("完成! 共处理 %d 条 IP 范围，%d 个数据源失败", total, failed)

	// 每日自动构建要能据此判断产物是否可信：任一数据源失败就非零退出
	if failed > 0 {
		os.Exit(1)
	}
}

// runLookup 实现 `cloud-ip-crawler lookup <ip>`：命中打印所有归属并返回 0，
// 未命中返回 1（脚本可以直接拿退出码当判断结果），用法错误返回 2。
// 同一个 IP 可能有多条归属（比如既在 Cloudflare 官方清单、又在 hosting 层的
// AS13335 里），全部列出，置信度排序交给调用方。
func runLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	dbPath := fs.String("db", "cloud-ip.db", "SQLite 数据库路径")
	fs.Parse(args) //nolint:errcheck // ExitOnError 模式下错误直接退出

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: cloud-ip-crawler lookup [--db cloud-ip.db] <ip>")
		return 2
	}

	addr, err := netip.ParseAddr(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效的 IP: %s\n", fs.Arg(0))
		return 2
	}

	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "数据库 %s 不存在：先运行抓取，或从 Release 下载现成数据集\n", *dbPath)
		return 2
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法打开数据库: %v\n", err)
		return 2
	}
	defer db.Close()

	var key []byte
	version := 6
	if addr.Is4() {
		b := addr.As4()
		key = b[:]
		version = 4
	} else {
		b := addr.As16()
		key = b[:]
	}

	rows, err := db.Query(
		"SELECT provider, cidr, region, service FROM cloud_ip_ranges "+
			"WHERE ip_version = ? AND ip_start <= ? AND ip_end >= ? ORDER BY provider",
		version, key, key,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		return 2
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		var provider, cidr, region, service string
		if err := rows.Scan(&provider, &cidr, &region, &service); err != nil {
			fmt.Fprintf(os.Stderr, "读取结果失败: %v\n", err)
			return 2
		}
		found++
		line := fmt.Sprintf("%s  %s", provider, cidr)
		if region != "" {
			line += "  region=" + region
		}
		if service != "" {
			line += "  service=" + service
		}
		fmt.Println(line)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		return 2
	}

	if found == 0 {
		fmt.Println("未命中：不在已收录的清单里（注意：不等于它不是机房 IP）")
		return 1
	}
	return 0
}

// runExport 实现 `cloud-ip-crawler export`：把 SQLite 导出成 MMDB，
// nginx（geoip2 模块）、HAProxy 等可以直接加载。
func runExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbPath := fs.String("db", "cloud-ip.db", "SQLite 数据库路径")
	outPath := fs.String("out", "cloud-ip.mmdb", "输出的 MMDB 文件路径")
	fs.Parse(args) //nolint:errcheck // ExitOnError 模式下错误直接退出

	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "数据库 %s 不存在：先运行抓取，或从 Release 下载现成数据集\n", *dbPath)
		return 2
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法打开数据库: %v\n", err)
		return 2
	}
	defer db.Close()

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法创建输出文件: %v\n", err)
		return 2
	}
	defer f.Close()

	n, err := crawler.ExportMMDB(db, f)
	if err != nil {
		os.Remove(*outPath) //nolint:errcheck // 尽力清掉半成品
		fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
		return 2
	}
	log.Printf("已导出 %d 个网段到 %s", n, *outPath)
	return 0
}
