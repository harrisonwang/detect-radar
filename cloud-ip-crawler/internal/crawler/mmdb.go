package crawler

import (
	"database/sql"
	"fmt"
	"io"
	"net"
	"sort"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// MMDB 每个 IP 只存一条记录，而库里同一个 IP 可能有多条归属（官方清单一条 +
// hosting 兜底一条）。按置信度从低到高插入，高层覆盖低层，最终查到的就是最可信
// 的那条；tier 字段告诉使用方这条结论是哪一层给的。
const (
	tierHosting  = "hosting"  // 启发式 ASN 分类
	tierASN      = "asn"      // 命名厂商的 ASN 宣告前缀
	tierOfficial = "official" // 厂商自己发布的清单
)

// providerTier provider → 置信层。官方文件厂商在 main 里注册的名字是固定的，
// 命名 ASN 厂商以 asnProviderList 为准，hosting 单独一层。
func providerTier(provider string) string {
	switch provider {
	case "aws", "gcp", "azure", "cloudflare", "oracle", "digitalocean",
		"linode", "vultr", "fastly", "bunny", "zscaler":
		return tierOfficial
	case "hosting":
		return tierHosting
	default:
		return tierASN
	}
}

func tierRank(tier string) int {
	switch tier {
	case tierHosting:
		return 0
	case tierASN:
		return 1
	default:
		return 2
	}
}

// ExportMMDB 把 SQLite 里的数据（v4 + v6）导出成 MaxMind DB 格式，
// nginx（geoip2 模块）、HAProxy 等可以直接加载。返回写入的网段数。
// 树是 IPv6 的（v4 走 ::ffff:0:0/96 映射），这是 GeoLite2 的标准做法。
func ExportMMDB(db *sql.DB, w io.Writer) (int, error) {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "cloud-ip-crawler",
		Description: map[string]string{
			"en": "Datacenter / cloud provider IP ranges (github.com/harrisonwang/detect-radar)",
			"zh": "云厂商 / 机房 IP 段",
		},
		RecordSize: 24,
	})
	if err != nil {
		return 0, fmt.Errorf("初始化 MMDB 树失败: %w", err)
	}

	rows, err := db.Query("SELECT provider, cidr, region, service FROM cloud_ip_ranges")
	if err != nil {
		return 0, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	type record struct {
		provider, cidr, region, service, tier string
	}
	var records []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.provider, &r.cidr, &r.region, &r.service); err != nil {
			return 0, fmt.Errorf("读取行失败: %w", err)
		}
		r.tier = providerTier(r.provider)
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("查询失败: %w", err)
	}
	if len(records) == 0 {
		return 0, fmt.Errorf("库里没有 IPv4 数据，先运行抓取")
	}

	// 置信度低的先插，高的后插覆盖之；同层内顺序无所谓（(provider,cidr) 已唯一）
	sort.SliceStable(records, func(i, j int) bool {
		return tierRank(records[i].tier) < tierRank(records[j].tier)
	})

	inserted := 0
	for _, r := range records {
		// 新抓的数据在入库时就滤掉了特殊用途网段，这里再拦一道是给老库兜底：
		// mmdbwriter 会直接拒绝保留网段（Vultr geofeed 里的 TEST-NET 踩过）
		if isReservedCIDR(r.cidr) {
			continue
		}
		_, network, err := net.ParseCIDR(r.cidr)
		if err != nil {
			continue // 入库时都经过 ParseCIDR，这里防御性跳过即可
		}
		data := mmdbtype.Map{
			"provider": mmdbtype.String(r.provider),
			"tier":     mmdbtype.String(r.tier),
		}
		if r.region != "" {
			data["region"] = mmdbtype.String(r.region)
		}
		if r.service != "" {
			data["service"] = mmdbtype.String(r.service)
		}
		if err := tree.Insert(network, data); err != nil {
			return 0, fmt.Errorf("插入 %s 失败: %w", r.cidr, err)
		}
		inserted++
	}

	if _, err := tree.WriteTo(w); err != nil {
		return 0, fmt.Errorf("写出 MMDB 失败: %w", err)
	}
	return inserted, nil
}
