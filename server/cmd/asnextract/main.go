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

type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

var highConfidenceKeywords = []string{
	"hosting",
	"datacenter", "data center", "data-center",
	"colocation", "colo ",
	"vps ",
	"dedicated server",
	"cloud service",
	"web host",
	"hoster",

	"amazon web services", "amazon.com", "amazon technologies",
	"google cloud", "google llc",
	"microsoft azure", "microsoft corporation",
	"alibaba cloud", "alibaba.com", "aliyun", "alibaba (",
	"tencent cloud", "tencent holdings",
	"huawei cloud", "huawei technologies",
	"oracle cloud", "oracle corporation",
	"ibm cloud",
	"baidu", "jd.com", "jdcloud",
	"bytedance", "volcengine",
	"ucloud", "kingsoft cloud",

	"digitalocean",
	"linode",
	"vultr", "choopa", "constant company",
	"hetzner",
	"ovh sas", "ovhcloud",
	"scaleway",
	"upcloud",
	"contabo",
	"hostinger",
	"kamatera",

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

var excludeKeywords = []string{
	"telecom", "telefonica", "vodafone", "t-mobile",
	"verizon", "at&t", "sprint", "china telecom",
	"china unicom", "china mobile", "chinanet",
	"mobile", "cellular", "wireless",
	"broadband", "fiber", "dsl",
	"communications inc", "communications llc",

	"university", "college", "school", "academy",
	"institute of technology", "research",
	"education", "academic",

	"government", "ministry", "department of",
	"federal", "state of", "city of",
	"national", "municipal",

	"bank", "financial", "insurance", "credit union",
	"investment", "securities", "capital",

	"hospital", "medical", "health", "clinic",

	"association", "society", "foundation",
	"corporation", "company", "enterprise",
	"trading", "manufacturing", "industrial",
	"construction", "engineering",

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

	asnMap := make(map[uint64]string)

	networks := db.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		var record ASNRecord
		_, err := networks.Network(&record)
		if err != nil {
			continue
		}
		if record.ASNumber > 0 && record.ASOrg != "" {
			if _, exists := asnMap[record.ASNumber]; !exists {
				asnMap[record.ASNumber] = record.ASOrg
			}
		}
	}

	fmt.Fprintf(os.Stderr, "从 MMDB 中提取了 %d 个唯一 ASN\n", len(asnMap))

	hostingASNs := make(map[uint64]string)
	for asn, org := range asnMap {
		if isHosting(org) {
			hostingASNs[asn] = org
		}
	}

	fmt.Fprintf(os.Stderr, "筛选出 %d 个 Hosting/Cloud ASN\n", len(hostingASNs))

	sortedASNs := make([]uint64, 0, len(hostingASNs))
	for asn := range hostingASNs {
		sortedASNs = append(sortedASNs, asn)
	}
	sort.Slice(sortedASNs, func(i, j int) bool {
		return sortedASNs[i] < sortedASNs[j]
	})

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

	for _, kw := range highConfidenceKeywords {
		if strings.Contains(orgLower, kw) {
			return true
		}
	}

	for _, kw := range excludeKeywords {
		if strings.Contains(orgLower, kw) {
			return false
		}
	}

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
		if strings.Contains(org, ",") || strings.Contains(org, "\"") {
			org = `"` + strings.ReplaceAll(org, `"`, `""`) + `"`
		}
		sb.WriteString(fmt.Sprintf("%d,%s\n", asn, org))
	}
	return sb.String()
}

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
