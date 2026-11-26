package crawler

import (
	"net/netip"
	"strings"
	"testing"
)

func TestRangeToCIDRs(t *testing.T) {
	cases := []struct {
		start, end string
		want       []string
	}{
		{"10.0.0.0", "10.0.0.255", []string{"10.0.0.0/24"}},
		{"10.0.0.1", "10.0.0.1", []string{"10.0.0.1/32"}},
		{"10.0.0.1", "10.0.0.2", []string{"10.0.0.1/32", "10.0.0.2/32"}},
		{"1.0.0.0", "1.0.1.255", []string{"1.0.0.0/23"}},
		{"1.0.0.0", "1.0.2.255", []string{"1.0.0.0/23", "1.0.2.0/24"}},
		{"0.0.0.0", "255.255.255.255", []string{"0.0.0.0/0"}},
		{"10.0.0.2", "10.0.0.1", nil},
		{"2a01:4f8::", "2a01:4f8:ffff:ffff:ffff:ffff:ffff:ffff", []string{"2a01:4f8::/32"}},
		{"2606:4700::1", "2606:4700::1", []string{"2606:4700::1/128"}},
		{"2606:4700::1", "2606:4700::2", []string{"2606:4700::1/128", "2606:4700::2/128"}},
		{"::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", []string{"::/0"}},
		{"10.0.0.1", "2606:4700::1", nil},
	}
	for _, c := range cases {
		got := rangeToCIDRs(netip.MustParseAddr(c.start), netip.MustParseAddr(c.end))
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("rangeToCIDRs(%s, %s) = %v, want %v", c.start, c.end, got, c.want)
		}
	}
}

func TestIsHostingDesc(t *testing.T) {
	positive := []string{
		"ABC-HOSTING Ltd",
		"SUPERVPS NETWORK",
		"MYCLOUD-AS Something",
		"CLOUDFLARENET",
		"XYZ DEDICATED SERVERS",
		"FOO COLOCATION GMBH",
		"CHINANET-IDC-BJ",
		"PULSEHEBERG-AS PulseHeberg",
		"ACME DATA CENTER SOLUTIONS",
	}
	for _, d := range positive {
		if !isHostingDesc(d) {
			t.Errorf("应判为托管商: %q", d)
		}
	}

	negative := []string{
		"COMCAST-7922 - Comcast Cable",
		"COLOMBIA-TELECOM",
		"TELECOM SERVICES SA",
		"CHINANET-BACKBONE",
		"VODAFONE-BROADBAND",
		"MIDCONTINENT-COMMUNICATIONS",
		"Caixa de Assistencia dos Servidores do Estado",
	}
	for _, d := range negative {
		if isHostingDesc(d) {
			t.Errorf("不该判为托管商: %q", d)
		}
	}
}

func TestParseIPToASN(t *testing.T) {
	tsv := strings.Join([]string{
		"5.9.0.0\t5.9.255.255\t24940\tDE\tHETZNER-AS Hetzner Online GmbH",
		"2a01:4f8::\t2a01:4f8:ffff:ffff:ffff:ffff:ffff:ffff\t24940\tDE\tHETZNER-AS Hetzner Online GmbH",
		"185.199.108.0\t185.199.108.255\t65001\tUS\tEXAMPLE-HOSTING Ltd",
		"1.1.1.0\t1.1.1.255\t13335\tUS\tCLOUDFLARENET",
		"9.9.9.0\t9.9.9.255\t65002\tUS\tSOME-TELECOM",
		"23.128.0.0\t23.128.0.255\t0\tNone\tNot routed",
		"192.0.2.0\t192.0.2.255\t65003\tUS\tBOGON-HOSTING Ltd",
	}, "\n")

	ds, err := parseIPToASN(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	hz := ds.byProvider["hetzner"]
	if len(hz) != 2 {
		t.Fatalf("hetzner 应有 v4+v6 各一条，实际: %+v", hz)
	}
	if hz[0].CIDR != "5.9.0.0/16" || hz[0].Service != "AS24940" || hz[0].Region != "DE" || hz[0].IPVersion != 4 {
		t.Errorf("hetzner v4 解析不对: %+v", hz[0])
	}
	if hz[1].CIDR != "2a01:4f8::/32" || hz[1].IPVersion != 6 {
		t.Errorf("hetzner v6 解析不对: %+v", hz[1])
	}

	if len(ds.hosting) != 2 {
		t.Fatalf("hosting 应有 2 条（关键词 1 + 显式清单 1），实际 %d: %+v", len(ds.hosting), ds.hosting)
	}
	for _, r := range ds.hosting {
		if r.Provider != "hosting" || !strings.HasPrefix(r.Service, "AS") {
			t.Errorf("hosting 字段不对: %+v", r)
		}
	}
}
