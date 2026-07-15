package main

import (
	"fmt"
	"strings"

	"detect-radar/internal/database"
	"detect-radar/internal/repository"
	"detect-radar/internal/service"
)

func main() {
	// 初始化 SQLite
	sqliteDB, err := database.NewSQLiteDB(database.SQLiteConfig{
		Path: "./data/hosting.db",
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to open SQLite: %v", err))
	}
	defer sqliteDB.Close()

	db := sqliteDB.DB()
	cloudIPRepo := repository.NewCloudIPRangeRepository(db)
	providerRepo := repository.NewHostingProviderRepository(db)
	keywordRepo := repository.NewHostingKeywordRepository(db)

	detector, err := service.NewHostingDetector(
		"./data/dbip-asn-lite.mmdb",
		cloudIPRepo,
		providerRepo,
		keywordRepo,
	)
	if err != nil {
		panic(err)
	}
	defer detector.Close()

	// 测试 IP 列表
	testIPs := []struct {
		IP       string
		Expected bool
		Desc     string
	}{
		// 大型云厂商
		{"8.8.8.8", true, "Google DNS"},
		{"1.1.1.1", true, "Cloudflare DNS"},
		{"20.205.243.166", true, "Microsoft Azure"},
		{"47.74.0.1", true, "阿里云"},
		{"52.94.0.1", true, "Amazon AWS"},

		// VPS 提供商
		{"104.238.128.1", true, "Vultr"},
		{"45.33.32.156", true, "Linode"},
		{"95.216.0.1", true, "Hetzner"},

		// 住宅 IP（应该是 false）
		{"222.244.158.176", false, "中国电信住宅"},
		{"114.114.114.114", false, "114 DNS (ISP)"},
		{"223.5.5.5", false, "阿里公共 DNS"},
	}

	fmt.Printf("%-18s | %-8s | %-6s | %-10s | %-30s | %s\n",
		"IP", "ASN", "结果", "置信度", "组织", "描述")
	fmt.Println(strings.Repeat("-", 110))

	for _, test := range testIPs {
		result, err := detector.Detect(test.IP)
		if err != nil {
			fmt.Printf("%-18s | ERROR: %v\n", test.IP, err)
			continue
		}

		status := "住宅"
		if result.IsHosting {
			status = "机房"
		}

		match := ""
		if result.IsHosting == test.Expected {
			match = "OK"
		} else {
			match = "FAIL"
		}

		fmt.Printf("%-18s | %-8s | %-6s | %-3d%% %s | %-30s | %s\n",
			result.IP,
			result.ASNStr,
			status,
			result.Confidence,
			result.Method,
			truncate(result.Org, 30),
			test.Desc+" "+match,
		)
	}

	fmt.Printf("\n统计信息: %v\n", detector.GetStats())
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
