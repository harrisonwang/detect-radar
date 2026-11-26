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

const (
	tierHosting  = "hosting"
	tierASN      = "asn"
	tierOfficial = "official"
)

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

	sort.SliceStable(records, func(i, j int) bool {
		return tierRank(records[i].tier) < tierRank(records[j].tier)
	})

	inserted := 0
	for _, r := range records {
		if isReservedCIDR(r.cidr) {
			continue
		}
		_, network, err := net.ParseCIDR(r.cidr)
		if err != nil {
			continue
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
