package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

// ASNRecord MMDB 中的 ASN 记录结构
type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

// 高置信度关键词（直接匹配）
var highConfidenceKeywords = []string{
	// 明确的托管/数据中心标识
	"hosting",
	"datacenter", "data center", "data-center",
	"colocation", "colo ",
	"vps ",
	"dedicated server",
	"cloud service",
	"web host",
	"hoster",

	// 大型云厂商（完整名称匹配）
	"amazon web services", "amazon.com", "amazon technologies",
	"google cloud", "google llc",
	"microsoft azure", "microsoft corporation",
	"alibaba cloud", "alibaba.com", "aliyun", "alibaba (", // Alibaba (US) Technology
	"tencent cloud", "tencent holdings",
	"huawei cloud", "huawei technologies",
	"oracle cloud", "oracle corporation",
	"ibm cloud",
	"baidu", "jd.com", "jdcloud",
	"bytedance", "volcengine",
	"ucloud", "kingsoft cloud",

	// 知名 VPS/云服务商
	"digitalocean",
	"linode",
	"vultr", "choopa", "constant company", // Vultr 母公司
	"hetzner",
	"ovh sas", "ovhcloud",
	"scaleway",
	"upcloud",
	"contabo",
	"hostinger",
	"kamatera",

	// 知名托管商
	"leaseweb",
	"rackspace",
	"godaddy",
	"bluehost",
	"hostgator",
	"namecheap",
	"dreamhost",
	"ionos",
	"liquidweb", "liquid web",
	"singlehop",
	"colocrossing",
	"quadranet",
	"psychz",
	"servermania",
	"hostwinds",
	"inmotionhosting", "inmotion hosting",
	"greengeeks",
	"fastcomet",
	"cloudways",
	"kinsta",
	"wpengine",
	"siteground",
	"hostpapa",
	"hostdime",
	"interserver",
	"a2 hosting",

	// CDN
	"cloudflare",
	"fastly",
	"akamai",
	"incapsula",
	"imperva",
	"sucuri",
	"stackpath",
	"keycdn",
	"bunnycdn",
	"cdn77",
	"edgecast",

	// 区域性云/托管
	"selectel",
	"yandex cloud",
	"naver cloud",
	"sakura internet",
	"conoha",
	"gmo internet",
	"nifcloud",
	"ucloud",
	"ksyun",
	"jdcloud",
	"baishan cloud",
	"chinac",

	// 其他关键词
	"hurricane electric",
	"zenlayer",
	"packet host",
	"equinix",
	"coresite",
	"cyrusone",
	"digital realty",
	"interxion",
	"ntt communications",
	"cogent",
	"m247",
	"frantech",
	"buyvm",
	"ramnode",
}

// 中置信度关键词（需要更谨慎，使用更长的匹配）
var mediumConfidenceKeywords = []string{
	"server farm",
	"server solutions",
	"cloud computing",
	"cloud platform",
	"web server",
	"game server",
	"dedicated host",
	"virtual server",
	"vserver",
	" idc ",
	" idc,",
}

// 明确排除的关键词（避免误判 ISP/企业/机构）
var excludeKeywords = []string{
	// 电信运营商
	"telecom", "telefonica", "vodafone", "t-mobile",
	"verizon", "at&t", "sprint", "china telecom",
	"china unicom", "china mobile", "chinanet",
	"mobile", "cellular", "wireless",
	"broadband", "fiber", "dsl",
	"communications inc", "communications llc",

	// 教育机构
	"university", "college", "school", "academy",
	"institute of technology", "research",
	"education", "academic",

	// 政府机构
	"government", "ministry", "department of",
	"federal", "state of", "city of",
	"national", "municipal",

	// 金融机构
	"bank", "financial", "insurance", "credit union",
	"investment", "securities", "capital",

	// 医疗机构
	"hospital", "medical", "health", "clinic",

	// 其他企业
	"association", "society", "foundation",
	"corporation", "company", "enterprise",
	"trading", "manufacturing", "industrial",
	"construction", "engineering",

	// 避免误判的特定组织
	"visa international",
	"mastercard",
	"american express",
	"mathematical",
	"geographic",
	"press",
}

func main() {
	mmdbPath := flag.String("mmdb", "/opt/projects/ip2location-convert/IP2LOCATION-LITE-ASN.MMDB", "MMDB 文件路径")
	outputFormat := flag.String("format", "go", "输出格式: go, json, csv")
	outputFile := flag.String("output", "", "输出文件路径（默认输出到 stdout）")
	flag.Parse()

	db, err := maxminddb.Open(*mmdbPath)
	if err != nil {
		log.Fatalf("无法打开 MMDB: %v", err)
	}
	defer db.Close()

	// 使用 map 去重，key 是 ASN
	asnMap := make(map[uint64]string)

	// 遍历所有网络
	networks := db.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		var record ASNRecord
		_, err := networks.Network(&record)
		if err != nil {
			continue
		}
		if record.ASNumber > 0 && record.ASOrg != "" {
			// 只保留第一次遇到的组织名（通常是最准确的）
			if _, exists := asnMap[record.ASNumber]; !exists {
				asnMap[record.ASNumber] = record.ASOrg
			}
		}
	}

	fmt.Fprintf(os.Stderr, "从 MMDB 中提取了 %d 个唯一 ASN\n", len(asnMap))

	// 筛选 Hosting 类型的 ASN
	hostingASNs := make(map[uint64]string)
	for asn, org := range asnMap {
		if isHosting(org) {
			hostingASNs[asn] = org
		}
	}

	fmt.Fprintf(os.Stderr, "筛选出 %d 个 Hosting/Cloud ASN\n", len(hostingASNs))

	// 排序 ASN
	sortedASNs := make([]uint64, 0, len(hostingASNs))
	for asn := range hostingASNs {
		sortedASNs = append(sortedASNs, asn)
	}
	sort.Slice(sortedASNs, func(i, j int) bool {
		return sortedASNs[i] < sortedASNs[j]
	})

	// 输出
	var output string
	switch *outputFormat {
	case "json":
		output = formatJSON(sortedASNs, hostingASNs)
	case "csv":
		output = formatCSV(sortedASNs, hostingASNs)
	default:
		output = formatGo(sortedASNs, hostingASNs)
	}

	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(output), 0644)
		if err != nil {
			log.Fatalf("写入文件失败: %v", err)
		}
		fmt.Fprintf(os.Stderr, "已写入到 %s\n", *outputFile)
	} else {
		fmt.Println(output)
	}
}

func isHosting(org string) bool {
	orgLower := strings.ToLower(org)

	// 1. 先检查高置信度关键词（优先级最高）
	for _, kw := range highConfidenceKeywords {
		if strings.Contains(orgLower, kw) {
			return true
		}
	}

	// 2. 检查排除关键词
	for _, kw := range excludeKeywords {
		if strings.Contains(orgLower, kw) {
			return false
		}
	}

	// 3. 检查中置信度关键词（需要更谨慎，已排除常见误判）
	for _, kw := range mediumConfidenceKeywords {
		if strings.Contains(orgLower, kw) {
			return true
		}
	}

	return false
}

func formatGo(asns []uint64, asnMap map[uint64]string) string {
	var sb strings.Builder
	sb.WriteString(`package service

// HostingASNs 已知的 Hosting/Cloud/Datacenter ASN 列表
// 数据来源: IP2LOCATION-LITE-ASN.MMDB
// 生成命令: go run cmd/asnextract/main.go -format go
var HostingASNs = map[uint64]string{
`)
	for _, asn := range asns {
		org := asnMap[asn]
		// 转义双引号
		org = strings.ReplaceAll(org, `"`, `\"`)
		sb.WriteString(fmt.Sprintf("\t%d: \"%s\",\n", asn, org))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func formatJSON(asns []uint64, asnMap map[uint64]string) string {
	result := make([]map[string]interface{}, 0, len(asns))
	for _, asn := range asns {
		result = append(result, map[string]interface{}{
			"asn": asn,
			"org": asnMap[asn],
		})
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
}

func formatCSV(asns []uint64, asnMap map[uint64]string) string {
	var sb strings.Builder
	sb.WriteString("ASN,Organization\n")
	for _, asn := range asns {
		org := asnMap[asn]
		// CSV 转义
		if strings.Contains(org, ",") || strings.Contains(org, "\"") {
			org = `"` + strings.ReplaceAll(org, `"`, `""`) + `"`
		}
		sb.WriteString(fmt.Sprintf("%d,%s\n", asn, org))
	}
	return sb.String()
}

// 用于测试特定 IP
func testIP(db *maxminddb.Reader, ipStr string) {
	ip := net.ParseIP(ipStr)
	var record ASNRecord
	err := db.Lookup(ip, &record)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("IP: %s, ASN: %d, Org: %s, IsHosting: %v\n",
		ipStr, record.ASNumber, record.ASOrg, isHosting(record.ASOrg))
}
