package crawler

import (
	"fmt"
	"net"
	"net/netip"
)

// Version 由 -ldflags "-X .../internal/crawler.Version=..." 注入
var Version = "dev"

func userAgent() string {
	return "cloud-ip-crawler/" + Version + " (+https://github.com/harrisonwang/detect-radar)"
}

// Range 云厂商 IP 范围
type Range struct {
	Provider  string
	CIDR      string
	IPStart   net.IP
	IPEnd     net.IP
	Region    string
	Service   string
	IPVersion int
}

// Crawler 爬虫接口
type Crawler interface {
	Name() string
	Crawl() ([]Range, error)
}

// errSkipReserved 是预期的跳过，不是抓取错误。调用方一律 continue。
var errSkipReserved = fmt.Errorf("skip reserved network")

// mustPrefixes 把 CIDR 字符串列表解析成 netip.Prefix 列表
func mustPrefixes(cidrs []string) []netip.Prefix {
	ps := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		ps[i] = netip.MustParsePrefix(c)
	}
	return ps
}

// reservedV4 / reservedV6 特殊用途网段（RFC 6890、IANA 特殊地址注册表）：
// 私网、TEST-NET、组播、ULA、链路本地……上游数据偶尔混进这类垃圾——
// Vultr 的 geofeed 就把三个 TEST-NET 当数据发出来。公共数据集不该声称
// 它们属于任何厂商，入库前统一丢弃；mmdbwriter 也会直接拒绝其中一部分
// （保留段和 6to4 / Teredo 这类在 v6 树里被别名的段），导出前必须滤干净。
var reservedV4 = mustPrefixes([]string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
})

var reservedV6 = mustPrefixes([]string{
	"::/96",         // 未指定 + 旧式 IPv4 兼容（含 ::、::1 邻域之外的遗留段）
	"::1/128",       // 环回
	"::ffff:0:0/96", // IPv4 映射：v4 数据已单独收录，这里出现只会是重复或垃圾
	"64:ff9b::/96",  // NAT64 公共前缀
	"100::/64",      // 黑洞（discard-only）
	"2001::/23",     // IETF 协议专用块：Teredo、基准测试(2001:2::)、ORCHID、AS112……
	"2001:db8::/32", // 文档示例
	"2002::/16",     // 6to4（同样被别名）
	"3fff::/20",     // 文档示例（RFC 9637）
	"fc00::/7",      // ULA
	"fe80::/10",     // 链路本地
	"ff00::/8",      // 组播
})

// isReservedCIDR 判断一个 CIDR 是否整段落在特殊用途网段里
func isReservedCIDR(cidr string) bool {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false
	}
	list := reservedV6
	if p.Addr().Is4() {
		list = reservedV4
	}
	for _, res := range list {
		if res.Overlaps(p) && res.Bits() <= p.Bits() {
			return true
		}
	}
	return false
}

// createRange 是全部爬虫写入前的唯一收口：v4/v6 都收，特殊用途网段在这里统一丢弃
func createRange(provider, cidr, region, service string) (*Range, error) {
	start, end, version, err := parseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	if isReservedCIDR(cidr) {
		return nil, errSkipReserved
	}
	return &Range{
		Provider:  provider,
		CIDR:      cidr,
		IPStart:   start,
		IPEnd:     end,
		Region:    region,
		Service:   service,
		IPVersion: version,
	}, nil
}

// parseCIDR 解析 CIDR 并返回该网段的首尾地址
func parseCIDR(cidr string) (start, end net.IP, version int, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, 0, err
	}

	start = ipNet.IP
	end = make(net.IP, len(start))
	copy(end, start)
	for i := range end {
		end[i] |= ^ipNet.Mask[i]
	}

	version = 4
	if start.To4() == nil {
		version = 6
	}
	return start, end, version, nil
}

// ipToBytes 定长存储，使 ip_start <= x <= ip_end 的 BLOB 比较成立
func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}
