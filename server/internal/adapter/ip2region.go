package adapter

import (
	"log"
	"net"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

var cnCloudISPKeywords = []string{
	"阿里", "腾讯", "华为", "百度", "金山云", "天翼云", "UCLOUD", "优刻得", "青云",
}

type CNGeoResult struct {
	Country  string
	Province string
	City     string
	ISP      string
	ISO      string
	Raw      string
}

func (r CNGeoResult) IsChina() bool {
	return strings.Contains(r.Country, "中国")
}

func (r CNGeoResult) CloudProvider() string {
	for _, kw := range cnCloudISPKeywords {
		if strings.Contains(r.ISP, kw) {
			return kw
		}
	}
	return ""
}

func (r CNGeoResult) ConsumerUsageRaw() string {
	isp := r.ISP
	switch {
	case strings.Contains(isp, "移动"):
		return "mobile"
	case strings.Contains(isp, "教育网"), strings.Contains(isp, "科技网"):
		return "education"
	case strings.Contains(isp, "电信"), strings.Contains(isp, "联通"),
		strings.Contains(isp, "广电"), strings.Contains(isp, "铁通"):
		return "residential"
	}
	return ""
}

func (r CNGeoResult) IsConsumerISP() bool {
	return r.ConsumerUsageRaw() != ""
}

func parseCNRegion(raw string) CNGeoResult {
	res := CNGeoResult{Raw: raw}
	parts := strings.Split(raw, "|")
	seg := func(i int) string {
		if i >= len(parts) {
			return ""
		}
		s := strings.TrimSpace(parts[i])
		if s == "0" {
			return ""
		}
		return s
	}
	res.Country = seg(0)
	res.Province = seg(1)
	res.City = seg(2)
	res.ISP = seg(3)
	res.ISO = seg(4)
	return res
}

type IP2RegionCN struct {
	v4 *xdb.Searcher
	v6 *xdb.Searcher
}

func NewIP2RegionCN(v4Path, v6Path string) (*IP2RegionCN, error) {
	if v4Path == "" && v6Path == "" {
		return nil, nil
	}

	cn := &IP2RegionCN{}
	if v4Path != "" {
		if s, err := loadCNSearcher(xdb.IPv4, v4Path); err != nil {
			log.Printf("[IP2RegionCN] v4 库加载失败，IPv4 CN 层关闭: %v", err)
		} else {
			cn.v4 = s
			log.Printf("[IP2RegionCN] 已加载 CN v4 库: %s", v4Path)
		}
	}
	if v6Path != "" {
		if s, err := loadCNSearcher(xdb.IPv6, v6Path); err != nil {
			log.Printf("[IP2RegionCN] v6 库加载失败，IPv6 CN 层关闭: %v", err)
		} else {
			cn.v6 = s
			log.Printf("[IP2RegionCN] 已加载 CN v6 库: %s", v6Path)
		}
	}
	if cn.v4 == nil && cn.v6 == nil {
		return nil, nil
	}
	return cn, nil
}

func loadCNSearcher(version *xdb.Version, path string) (*xdb.Searcher, error) {
	buf, err := xdb.LoadContentFromFile(path)
	if err != nil {
		return nil, err
	}
	return xdb.NewWithBuffer(version, buf)
}

func (c *IP2RegionCN) Lookup(ip string) (*CNGeoResult, bool) {
	if c == nil {
		return nil, false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, false
	}
	searcher := c.v4
	if parsed.To4() == nil {
		searcher = c.v6
	}
	if searcher == nil {
		return nil, false
	}
	raw, err := searcher.Search(ip)
	if err != nil || raw == "" {
		return nil, false
	}
	res := parseCNRegion(raw)
	if !res.IsChina() {
		return nil, false
	}
	return &res, true
}

func (c *IP2RegionCN) Close() error {
	if c == nil {
		return nil
	}
	if c.v4 != nil {
		c.v4.Close()
	}
	if c.v6 != nil {
		c.v6.Close()
	}
	return nil
}
