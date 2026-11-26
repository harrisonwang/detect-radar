package crawler

import (
	"fmt"
	"net"
	"net/netip"
)

var Version = "dev"

func userAgent() string {
	return "cloud-ip-crawler/" + Version + " (+https://github.com/harrisonwang/detect-radar)"
}

type Range struct {
	Provider  string
	CIDR      string
	IPStart   net.IP
	IPEnd     net.IP
	Region    string
	Service   string
	IPVersion int
}

type Crawler interface {
	Name() string
	Crawl() ([]Range, error)
}

var errSkipReserved = fmt.Errorf("skip reserved network")

func mustPrefixes(cidrs []string) []netip.Prefix {
	ps := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		ps[i] = netip.MustParsePrefix(c)
	}
	return ps
}

var reservedV4 = mustPrefixes([]string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
})

var reservedV6 = mustPrefixes([]string{
	"::/96",
	"::1/128",
	"::ffff:0:0/96",
	"64:ff9b::/96",
	"100::/64",
	"2001::/23",
	"2001:db8::/32",
	"2002::/16",
	"3fff::/20",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
})

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

func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}
