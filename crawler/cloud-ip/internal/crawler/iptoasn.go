package crawler

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/bits"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const iptoasnURL = "https://iptoasn.com/data/ip2asn-combined.tsv.gz"

type asnDataset struct {
	byProvider map[string][]Range
	hosting    []Range
	err        error
}

var (
	asnDatasetOnce sync.Once
	asnDatasetVal  asnDataset
)

func loadASNDataset() *asnDataset {
	asnDatasetOnce.Do(func() {
		asnDatasetVal = buildASNDataset()
	})
	return &asnDatasetVal
}

func buildASNDataset() asnDataset {
	ds, err := fetchAndParseIPToASN(iptoasnURL)
	if err != nil {
		return asnDataset{err: err}
	}
	return ds
}

func fetchAndParseIPToASN(url string) (asnDataset, error) {
	body, err := fetchBytes(url, 60*time.Second)
	if err != nil {
		return asnDataset{}, fmt.Errorf("下载 iptoasn 全量表失败: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return asnDataset{}, fmt.Errorf("解压失败: %w", err)
	}
	defer gz.Close()

	return parseIPToASN(gz)
}

func parseIPToASN(r io.Reader) (asnDataset, error) {
	asnToProvider := map[int]string{}
	for _, p := range asnProviderList {
		for _, a := range p.asns {
			asnToProvider[a] = p.name
		}
	}

	ds := asnDataset{byProvider: map[string][]Range{}}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		cols := strings.Split(scanner.Text(), "\t")
		if len(cols) < 5 {
			continue
		}
		asn, err := strconv.Atoi(cols[2])
		if err != nil || asn == 0 {
			continue
		}

		provider, named := asnToProvider[asn]
		hosting := !named && (extraHostingASNs[asn] || isHostingDesc(cols[4]))
		if !named && !hosting {
			continue
		}

		start, err1 := netip.ParseAddr(strings.TrimSpace(cols[0]))
		end, err2 := netip.ParseAddr(strings.TrimSpace(cols[1]))
		if err1 != nil || err2 != nil {
			continue
		}

		country := cols[3]
		if country == "None" {
			country = ""
		}

		for _, cidr := range rangeToCIDRs(start, end) {
			if named {
				if rg, err := createRange(provider, cidr, country, fmt.Sprintf("AS%d", asn)); err == nil {
					ds.byProvider[provider] = append(ds.byProvider[provider], *rg)
				}
			} else {
				service := fmt.Sprintf("AS%d %s", asn, cols[4])
				if len(service) > 100 {
					service = service[:100]
				}
				if rg, err := createRange("hosting", cidr, country, service); err == nil {
					ds.hosting = append(ds.hosting, *rg)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return asnDataset{}, fmt.Errorf("扫描 iptoasn 表失败: %w", err)
	}
	return ds, nil
}

func rangeToCIDRs(start, end netip.Addr) []string {
	if start.Is4() != end.Is4() || start.Compare(end) > 0 {
		return nil
	}
	totalBits := 32
	if !start.Is4() {
		totalBits = 128
	}

	var out []string
	cur := start
	for {
		prefixLen := totalBits - addrTrailingZeros(cur)
		for prefixLast(cur, prefixLen).Compare(end) > 0 {
			prefixLen++
		}
		out = append(out, netip.PrefixFrom(cur, prefixLen).String())

		last := prefixLast(cur, prefixLen)
		if last.Compare(end) >= 0 {
			return out
		}
		cur = last.Next()
	}
}

func addrTrailingZeros(a netip.Addr) int {
	b := a.AsSlice()
	tz := 0
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != 0 {
			return tz + bits.TrailingZeros8(b[i])
		}
		tz += 8
	}
	return tz
}

func prefixLast(a netip.Addr, prefixLen int) netip.Addr {
	b := a.AsSlice()
	host := len(b)*8 - prefixLen
	for i := len(b) - 1; i >= 0 && host > 0; i-- {
		if host >= 8 {
			b[i] = 0xff
			host -= 8
		} else {
			b[i] |= byte(1<<host) - 1
			host = 0
		}
	}
	addr, _ := netip.AddrFromSlice(b)
	return addr
}

var hostingKeywordSubstrings = []string{
	"HOST",
	"VPS",
	"VDS",
	"CLOUD",
	"SERVER",
	"DATACENT",
	"DEDIC",
	"COLOC",
	"HEBERG",
}

var hostingKeywordTokens = []string{"IDC"}

var extraHostingASNs = map[int]bool{
	16509:  true,
	14618:  true,
	8075:   true,
	15169:  true,
	396982: true,
	13335:  true,
	31898:  true,
	14061:  true,
	20473:  true,
	54113:  true,
}

func isHostingDesc(desc string) bool {
	u := strings.ToUpper(desc)
	if strings.Contains(u, "DATA CENTER") || strings.Contains(u, "DATA-CENTER") {
		return true
	}
	for _, kw := range hostingKeywordSubstrings {
		if strings.Contains(u, kw) {
			return true
		}
	}
	tokens := strings.FieldsFunc(u, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	for _, tok := range tokens {
		if slices.Contains(hostingKeywordTokens, tok) {
			return true
		}
	}
	return false
}

type HostingCrawler struct{}

func (c *HostingCrawler) Name() string {
	return "hosting"
}

func (c *HostingCrawler) Crawl() ([]Range, error) {
	ds := loadASNDataset()
	if ds.err != nil {
		return nil, ds.err
	}
	if len(ds.hosting) == 0 {
		return nil, fmt.Errorf("分类结果为空，疑似上游表结构变化")
	}
	return ds.hosting, nil
}
