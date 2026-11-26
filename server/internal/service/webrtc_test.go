package service

import (
	"context"
	"testing"

	"detect-radar/internal/adapter"
	"detect-radar/internal/model"
	"detect-radar/internal/util"
)

func TestCanonicalIPAndGlobalPublic(t *testing.T) {
	if got := util.CanonicalIP("240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329"); got != "240e:469:e692:1dbe:b0f0:bafe:4e4c:329" {
		t.Fatalf("CanonicalIP zero-padded IPv6 = %q", got)
	}
	if got := util.CanonicalIP("::ffff:106.16.168.167"); got != "106.16.168.167" {
		t.Fatalf("CanonicalIP v4-mapped = %q", got)
	}
	if !util.IsGlobalPublicIP("240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329") {
		t.Fatal("expected China Telecom IPv6 to be global public")
	}
	for _, nonPublic := range []string{"192.168.1.1", "10.0.0.1", "100.64.0.5", "198.18.0.2", "fe80::1", "fc00::1", "::1", "not-an-ip"} {
		if util.IsGlobalPublicIP(nonPublic) {
			t.Fatalf("expected %q to NOT be global public", nonPublic)
		}
	}
}

func TestIPIntelLookup_NormalizesCachedAndRemoteIPv6(t *testing.T) {
	const (
		canonical = "240e:469:e692:1dbe:b0f0:bafe:4e4c:329"
		padded    = "240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329"
	)

	fakeCache := newFakeIntelCache()
	_ = fakeCache.SetDeep(context.Background(), canonical, &model.IPIntel{
		IP: padded, Country: "CN", ASN: "4134", ISP: "电信", UsageType: "isp",
	})
	svc := NewIPIntelService(adapter.NewRegistry(), nil, nil, nil, nil)
	svc.cache = fakeCache

	got, err := svc.Lookup(context.Background(), padded, LookupOptions{LocalOnly: true})
	if err != nil {
		t.Fatalf("Lookup cached error: %v", err)
	}
	if got.IP != canonical {
		t.Fatalf("cached IP = %q, want canonical %q", got.IP, canonical)
	}

	remote := &fakeAdapter{
		name: "ip2location",
		tier: "L3",
		intel: model.IPIntel{
			IP: padded, Country: "CN", ASN: "4134", Org: "China Telecom", UsageType: "isp",
		},
	}
	reg := adapter.NewRegistry()
	reg.AddRemote(remote)
	svc2 := NewIPIntelService(reg, nil, nil, nil, nil)
	cache2 := newFakeIntelCache()
	svc2.cache = cache2

	got2, err := svc2.Lookup(context.Background(), canonical, LookupOptions{DeepScan: true})
	if err != nil {
		t.Fatalf("Lookup deep error: %v", err)
	}
	if got2.IP != canonical {
		t.Fatalf("remote merged IP = %q, want canonical %q", got2.IP, canonical)
	}
	cachedDeep, _ := cache2.GetDeep(context.Background(), canonical)
	if cachedDeep == nil || cachedDeep.IP != canonical {
		t.Fatalf("stored deep cache IP = %+v, want %q", cachedDeep, canonical)
	}
}

func TestAssessWebRTC(t *testing.T) {
	const (
		ctV6Padded = "240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329"
		ctV6       = "240e:469:e692:1dbe:b0f0:bafe:4e4c:329"
		ctV6Same64 = "240e:469:e692:1dbe:1234:5678:9abc:def0"
		ctV4A      = "106.16.168.167"
		ctV4B      = "106.16.168.210"
		cmV4Prov   = "117.136.12.34"
		cmV6Back   = "2409:8900:1234:5678::1"
		usVPNv4    = "45.9.148.200"
		usVPNv4Alt = "45.9.148.201"
		usVPNv6    = "2a02:6ea0:100::1"
		hkVPNv4    = "103.1.2.3"
		cnCloudV4  = "47.95.10.20"
		usHomeV4   = "73.15.20.30"
		usHomeV4B  = "73.15.20.99"
		usTmoV4A   = "172.58.10.1"
		usTmoV4B   = "172.58.10.88"
	)

	fakeCache := newFakeIntelCache()
	ctx := context.Background()
	seed := func(ip string, intel *model.IPIntel) {
		_ = fakeCache.SetBasic(ctx, util.CanonicalIP(ip), intel)
	}

	_ = fakeCache.SetDeep(ctx, ctV6, &model.IPIntel{
		IP: ctV6Padded, Country: "CN", CountryName: "China", Region: "北京", City: "北京市",
		ASN: "4134", Org: "China Telecom", ISP: "电信", UsageType: "isp", UsageTypeRaw: "residential",
	})
	seed(ctV4A, &model.IPIntel{
		IP: ctV4A, Country: "CN", Region: "湖南省", City: "长沙市",
		ASN: "AS4134", Org: "China Telecom", ISP: "电信", UsageType: "isp", UsageTypeRaw: "residential",
	})
	seed(ctV4B, &model.IPIntel{
		IP: ctV4B, Country: "CN", Region: "湖南省", City: "长沙市",
		ASN: "4134", Org: "China Telecom", ISP: "电信", UsageType: "isp", UsageTypeRaw: "mobile", IsMobile: true,
	})
	seed(cmV4Prov, &model.IPIntel{
		IP: cmV4Prov, Country: "CN", Region: "江苏省",
		ASN: "AS56046", Org: "China Mobile Jiangsu", ISP: "移动", UsageType: "isp", IsMobile: true,
	})
	seed(cmV6Back, &model.IPIntel{
		IP: cmV6Back, Country: "CN", Region: "北京市",
		ASN: "AS9808", Org: "China Mobile", ISP: "移动", UsageType: "isp", IsMobile: true,
	})
	seed(usVPNv4, &model.IPIntel{
		IP: usVPNv4, Country: "US", ASN: "AS9009", Org: "M247 Europe SRL",
		IsHosting: true, IsVPN: true, UsageType: "hosting",
	})
	seed(usVPNv4Alt, &model.IPIntel{
		IP: usVPNv4Alt, Country: "US", ASN: "AS9009", Org: "M247 Europe SRL",
		IsHosting: true, IsVPN: true, UsageType: "hosting",
	})
	seed(usVPNv6, &model.IPIntel{
		IP: usVPNv6, Country: "US", ASN: "AS9009", Org: "M247 Europe SRL",
		IsHosting: true, UsageType: "hosting",
	})
	seed(hkVPNv4, &model.IPIntel{
		IP: hkVPNv4, Country: "CN", CountryName: "中国", Region: "香港特别行政区", City: "九龙",
		ASN: "AS13335", Org: "Cloudflare", IsHosting: true, UsageType: "hosting",
	})
	seed(cnCloudV4, &model.IPIntel{
		IP: cnCloudV4, Country: "CN", Region: "北京市",
		ASN: "AS37963", Org: "Alibaba Cloud", ISP: "阿里云", IsHosting: true, UsageType: "hosting",
	})
	seed(usHomeV4, &model.IPIntel{
		IP: usHomeV4, Country: "US", ASN: "AS7922", Org: "Comcast Cable",
		UsageType: "isp", UsageTypeRaw: "residential",
	})
	seed(usHomeV4B, &model.IPIntel{
		IP: usHomeV4B, Country: "US", ASN: "AS7018", Org: "AT&T Services",
		UsageType: "isp", UsageTypeRaw: "residential",
	})
	seed(usTmoV4A, &model.IPIntel{
		IP: usTmoV4A, Country: "US", ASN: "AS21928", Org: "T-Mobile USA",
		UsageType: "isp", UsageTypeRaw: "mobile", IsMobile: true,
	})
	seed(usTmoV4B, &model.IPIntel{
		IP: usTmoV4B, Country: "US", ASN: "AS21928", Org: "T-Mobile USA",
		UsageType: "isp", UsageTypeRaw: "mobile", IsMobile: true,
	})

	remoteProbe := &fakeAdapter{name: "remote-should-not-run", tier: "L3"}
	reg := adapter.NewRegistry()
	reg.AddRemote(remoteProbe)
	ipSvc := NewIPIntelService(reg, nil, nil, nil, nil)
	ipSvc.cache = fakeCache

	tests := []struct {
		name        string
		exitIP      string
		publicIPs   []string
		wantState   string
		wantReason  string
		wantSuspect int
	}{
		{
			name:       "china_telecom_dual_stack_v6_exit_with_padded_cache",
			exitIP:     ctV6Padded,
			publicIPs:  []string{ctV6, ctV4A},
			wantState:  "clear",
			wantReason: "dual-stack",
		},
		{
			name:       "china_telecom_dual_stack_v4_exit",
			exitIP:     ctV4A,
			publicIPs:  []string{ctV4A, ctV6},
			wantState:  "clear",
			wantReason: "dual-stack",
		},
		{
			name:       "ipv6_rfc4941_same_prefix_64",
			exitIP:     ctV6,
			publicIPs:  []string{ctV6Same64},
			wantState:  "clear",
			wantReason: "same-prefix",
		},
		{
			name:       "china_mobile_dual_stack_provincial_and_backbone_asn",
			exitIP:     cmV4Prov,
			publicIPs:  []string{cmV4Prov, cmV6Back},
			wantState:  "clear",
			wantReason: "dual-stack",
		},
		{
			name:       "china_telecom_cgnat_same_asn_dual_v4",
			exitIP:     ctV4A,
			publicIPs:  []string{ctV4B},
			wantState:  "clear",
			wantReason: "same-network",
		},
		{
			name:       "us_tmobile_5g_cgnat_same_asn_dual_v4",
			exitIP:     usTmoV4A,
			publicIPs:  []string{usTmoV4B},
			wantState:  "clear",
			wantReason: "same-network",
		},
		{
			name:       "dual_stack_vpn_same_hosting_asn",
			exitIP:     usVPNv4,
			publicIPs:  []string{usVPNv4, usVPNv6},
			wantState:  "clear",
			wantReason: "dual-stack",
		},
		{
			name:        "us_ipv4_vpn_leaks_china_telecom_ipv6",
			exitIP:      usVPNv4,
			publicIPs:   []string{ctV6},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
		{
			name:        "hk_vpn_leaks_mainland_ipv6",
			exitIP:      hkVPNv4,
			publicIPs:   []string{ctV6},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
		{
			name:        "return_to_cn_cloud_proxy_leaks_us_home_ip",
			exitIP:      cnCloudV4,
			publicIPs:   []string{usHomeV4},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
		{
			name:        "us_residential_proxy_leaks_china_telecom_ip",
			exitIP:      usHomeV4,
			publicIPs:   []string{ctV4A},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
		{
			name:        "us_residential_proxy_leaks_different_us_isp",
			exitIP:      usHomeV4,
			publicIPs:   []string{usHomeV4B},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
		{
			name:        "hosting_vpn_different_same_family_ip_not_cgnat",
			exitIP:      usVPNv4,
			publicIPs:   []string{usVPNv4Alt},
			wantState:   "leaked",
			wantReason:  "public-leak",
			wantSuspect: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AssessWebRTC(ctx, ipSvc, tc.exitIP, nil, tc.publicIPs)
			if got.Verdict.State != tc.wantState || got.Verdict.Reason != tc.wantReason {
				t.Fatalf("got state=%q reason=%q suspects=%v, want state=%q reason=%q",
					got.Verdict.State, got.Verdict.Reason, got.Verdict.SuspectIPs, tc.wantState, tc.wantReason)
			}
			if len(got.Verdict.SuspectIPs) != tc.wantSuspect {
				t.Fatalf("suspect count = %d (%v), want %d", len(got.Verdict.SuspectIPs), got.Verdict.SuspectIPs, tc.wantSuspect)
			}
		})
	}

	if remoteProbe.calls.Load() != 0 {
		t.Fatalf("AssessWebRTC must be strictly LocalOnly, but remote adapter was called %d times", remoteProbe.calls.Load())
	}

	scanSvc := &ScanService{ipIntel: ipSvc}
	t.Run("homepage_refine_clears_telecom_dual_stack_warning", func(t *testing.T) {
		exitIntel, _ := ipSvc.Lookup(ctx, ctV6, LookupOptions{LocalOnly: true})
		pa := model.PrivacyAnalysis{
			WebRTC: model.PrivacyWebRTC{Status: "warning", LeakedIPs: []string{ctV4A}, MatchesExitIP: true},
			DNS:    model.PrivacyDNS{Status: "safe", MatchesExitIP: true},
			IPv6:   model.PrivacyIPv6{Status: "safe", Address: ctV6},
		}
		wReq := &model.CollectorResult[model.WebRTCLeakData]{
			Status: "success",
			Data:   &model.WebRTCLeakData{PublicIPs: []string{ctV6, ctV4A}},
		}
		scanSvc.refineWebRTCPrivacy(ctx, &pa, wReq, ctV6, exitIntel)
		if pa.WebRTC.Status != "safe" || !pa.WebRTC.MatchesExitIP || len(pa.WebRTC.LeakedIPs) != 0 {
			t.Fatalf("got homepage webrtc = %+v, want safe with empty leaked_ips", pa.WebRTC)
		}
		if pa.OverallStatus != "safe" {
			t.Fatalf("got overall_status = %q, want safe", pa.OverallStatus)
		}
	})

	t.Run("homepage_refine_flags_ipv4_vpn_leaking_native_ipv6", func(t *testing.T) {
		exitIntel, _ := ipSvc.Lookup(ctx, usVPNv4, LookupOptions{LocalOnly: true})
		pa := model.PrivacyAnalysis{
			WebRTC: model.PrivacyWebRTC{Status: "warning", LeakedIPs: []string{ctV6}, MatchesExitIP: true},
			DNS:    model.PrivacyDNS{Status: "safe", MatchesExitIP: true},
			IPv6:   model.PrivacyIPv6{Status: "safe"},
		}
		wReq := &model.CollectorResult[model.WebRTCLeakData]{
			Status: "success",
			Data:   &model.WebRTCLeakData{PublicIPs: []string{ctV6}},
		}
		scanSvc.refineWebRTCPrivacy(ctx, &pa, wReq, usVPNv4, exitIntel)
		if pa.WebRTC.Status != "danger" || pa.WebRTC.MatchesExitIP || len(pa.WebRTC.LeakedIPs) != 1 || pa.WebRTC.LeakedIPs[0] != ctV6 {
			t.Fatalf("got homepage webrtc = %+v, want danger with %q", pa.WebRTC, ctV6)
		}
	})
}
