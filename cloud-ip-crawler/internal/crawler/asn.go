package crawler

import "fmt"

var asnProviderList = []struct {
	name string
	asns []int
}{
	{"hetzner", []int{24940, 213230, 212317}},
	{"ovh", []int{16276}},
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
	{"akamai", []int{20940, 16625, 63949}},
	{"rackspace", []int{27357, 33070, 12200, 19994}},
	{"godaddy", []int{26496}},
	{"namecheap", []int{22612}},
	{"m247", []int{9009}},
	{"selectel", []int{49505}},
	{"sakura", []int{9370, 7684}},
	{"flyio", []int{40509}},
	{"datacamp", []int{60068}},
	{"kamatera", []int{36007}},
	{"aeza", []int{210644}},
	{"timeweb", []int{9123}},
}

func ASNCrawlers() []Crawler {
	cs := make([]Crawler, 0, len(asnProviderList))
	for _, p := range asnProviderList {
		cs = append(cs, &asnCrawler{name: p.name})
	}
	return cs
}

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
	if len(ranges) == 0 {
		return nil, fmt.Errorf("iptoasn 表里没有 %s 名下任何 ASN 的前缀", c.name)
	}
	return ranges, nil
}
