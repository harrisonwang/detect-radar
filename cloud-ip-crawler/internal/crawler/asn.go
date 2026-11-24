package crawler

import "fmt"

// 大机房和 VPS 商家绝大多数不发官方 IP 文件，只能反过来从它们的 ASN 取「当前对外宣告
// 的前缀」。数据来自 iptoasn.com 的全量 BGP 表（见 iptoasn.go），13 个 ASN 系爬虫
// 共享一次下载。
//
// 注意语义与官方清单不同：这是「该 AS 宣告的所有前缀」，比官方服务清单粗——可能含厂商
// 自用甚至客户自带（BYOIP）网段。对「判断是不是机房 IP」这个目标，宁可全一点反而合适。
//
// 一家厂商常有多个 ASN，下面这些号都用 RIPEstat 的 as-overview 逐个核对过 holder。
var asnProviderList = []struct {
	name string
	asns []int
}{
	{"hetzner", []int{24940, 213230, 212317}},
	{"ovh", []int{16276}}, // Kimsufi / SoYouStart 是 OVH 品牌，同一批 ASN
	{"contabo", []int{51167}},
	{"leaseweb", []int{60781, 30633, 7203, 19148, 396190}},
	{"gcore", []int{199524}},
	{"scaleway", []int{12876}},
	{"netcup", []int{197540}},
	{"softlayer", []int{36351}},
	{"buyvm", []int{53667}},
	{"hosthatch", []int{63473}},
	{"alibaba", []int{45102, 37963}},
	{"tencent", []int{132203, 45090}},
	{"huawei", []int{136907, 55990}},
	{"akamai", []int{20940, 16625, 63949}}, // 63949 = Akamai Connected Cloud（原 Linode 骨干）
	{"rackspace", []int{27357, 33070, 12200, 19994}},
	{"godaddy", []int{26496}},
	{"namecheap", []int{22612}},
	{"m247", []int{9009}},
	{"selectel", []int{49505}},
	{"sakura", []int{9370, 7684}},
	{"flyio", []int{40509}},
	{"datacamp", []int{60068}}, // CDN77
	{"kamatera", []int{36007}},
	{"aeza", []int{210644}},
	{"timeweb", []int{9123}},
}

// ASNCrawlers 返回全部基于 ASN 的命名厂商爬虫，供 main 注册。
// 每家一个爬虫（Name() 即厂商名），这样入库时能按厂商整体替换，和其余爬虫一致。
func ASNCrawlers() []Crawler {
	cs := make([]Crawler, 0, len(asnProviderList))
	for _, p := range asnProviderList {
		cs = append(cs, &asnCrawler{name: p.name})
	}
	return cs
}

// asnCrawler 一个命名厂商，数据从共享的 iptoasn 数据集里取
type asnCrawler struct {
	name string
}

func (c *asnCrawler) Name() string {
	return c.name
}

func (c *asnCrawler) Crawl() ([]Range, error) {
	ds := loadASNDataset()
	if ds.err != nil {
		return nil, ds.err
	}
	ranges := ds.byProvider[c.name]
	// 空结果多半是上游表异常或 ASN 全部停止宣告，宁可当失败也不能静默清空该厂商的库
	if len(ranges) == 0 {
		return nil, fmt.Errorf("iptoasn 表里没有 %s 名下任何 ASN 的前缀", c.name)
	}
	return ranges, nil
}
