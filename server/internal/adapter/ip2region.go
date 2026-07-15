package adapter

import (
	"log"
	"net"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

// CN 权威地理层（ip2region xdb）
//
// 西方 geo 源（本地 lite mmdb 与 L3 付费 API）对中国 IP 普遍错判：中国移动网段被
// 钉在北京（注册地址）、IPv6 自相矛盾。实测（61 个真实 CN IP，pconline+baidu 共识为准）：
// 省级 87%→98%、市级 ~62%→~95%、IPv6 1/3→3/3。ip2region 是纯 Go、离线、Apache-2.0，
// 数据由国内众包，故作为中国 IP 的权威地理来源覆盖 western 源。
//
// 数据串格式：国家|省份|城市|ISP|iso-alpha2，例如「中国|陕西省|西安市|电信|CN」。
// 空段可能是 "0" 或 ""。非中国 IP 国家段非「中国」，一律视为未命中。

// cnCloudISPKeywords CN 云厂商运营商关键词（命中即视为机房证据）。
// 注意：ip2region 运营商段用简称，实测为「阿里」「腾讯」「华为」「金山云」，
// 而非「阿里云」「腾讯云」，故按下列判别子串做包含匹配。填补 cloud_ip_ranges
// 爬虫零国内云厂商的已知缺口。
var cnCloudISPKeywords = []string{
	"阿里", "腾讯", "华为", "百度", "金山云", "天翼云", "UCLOUD", "优刻得", "青云",
}

// CNGeoResult ip2region 单次查询解析结果（5 段）
type CNGeoResult struct {
	Country  string // 国家中文，如「中国」
	Province string // 省份中文，如「陕西省」
	City     string // 城市中文，如「西安市」
	ISP      string // 运营商中文，如「电信」「移动」「阿里」
	ISO      string // ISO 3166-1 alpha-2，如「CN」
	Raw      string // 原始 region 串（归因/调试）
}

// IsChina 是否为中国归属（含港澳台，均为 UTC+8）
func (r CNGeoResult) IsChina() bool {
	return strings.Contains(r.Country, "中国")
}

// CloudProvider 若运营商命中国内云厂商关键词，返回命中的关键词，否则返回 ""
func (r CNGeoResult) CloudProvider() string {
	for _, kw := range cnCloudISPKeywords {
		if strings.Contains(r.ISP, kw) {
			return kw
		}
	}
	return ""
}

// ConsumerUsageRaw 若运营商是消费级 ISP，返回对应的 UsageTypeRaw
// （mobile / education / residential），否则返回 ""。
//
// 覆盖运营商列表 {电信, 联通, 移动, 广电, 铁通, 教育网, 科技网}，按包含匹配以兼容
// 「中国电信」「中华电信」等变体。用于放行 CN 家宽/移动为住宅、抑制灰区深扫。
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

// IsConsumerISP 运营商是否为消费级 ISP（家宽/移动/广电/教育网等）
func (r CNGeoResult) IsConsumerISP() bool {
	return r.ConsumerUsageRaw() != ""
}

// parseCNRegion 解析 ip2region 的 region 串为结构化结果。
// 纯函数、不依赖 xdb 文件，便于单测；空段（"0" 或 ""）归一化为空字符串。
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

// IP2RegionCN ip2region CN 地理层：v4/v6 两个 xdb 缓冲全量加载进内存
// （buffer 模式并发只读安全，v4+v6 约 47MB）。换文件后必须重启进程（持内存缓冲）。
type IP2RegionCN struct {
	v4 *xdb.Searcher
	v6 *xdb.Searcher
}

// NewIP2RegionCN 加载 v4/v6 xdb。
//   - 两个路径都为空 → 返回 (nil, nil)，特性关闭，行为不变。
//   - 某路径非空但文件缺失/损坏 → 记 warning 并关闭该版本，绝不 crash。
//   - 两个版本都加载失败 → 返回 (nil, nil)，整体关闭。
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

// loadCNSearcher 整文件读入内存并建立 buffer 模式 searcher
func loadCNSearcher(version *xdb.Version, path string) (*xdb.Searcher, error) {
	buf, err := xdb.LoadContentFromFile(path)
	if err != nil {
		return nil, err
	}
	return xdb.NewWithBuffer(version, buf)
}

// Lookup 查询 IP 的 CN 地理归属；仅当命中且国家为中国时返回 ok=true。
// 非中国、解析失败、对应版本库未加载均返回 ok=false（视为未命中，退回西方源）。
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

// Close 释放两个 searcher（buffer 模式下为 no-op，保留以对齐其他适配器）
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
