// Package dnsassessment evaluates observed DNS queries for the home report and the DNS page.
package dnsassessment

import (
	"net/netip"
	"sort"
	"strings"
)

type Exit struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	Network string `json:"network"`
	ASN     string `json:"asn,omitempty"`
	Type    string `json:"type,omitempty"` // proxy / hosting / access
}

type Resolver struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	ISP         string `json:"isp"`
	ASN         string `json:"asn,omitempty"`
	NetworkType string `json:"network_type,omitempty"` // public / access / hosting
	Rounds      int    `json:"rounds,omitempty"`
	RoundIDs    []int  `json:"round_ids,omitempty"`
}

type Input struct {
	Exit           *Exit      `json:"exit"`
	Resolvers      []Resolver `json:"resolvers"`
	ObservedRounds int        `json:"observed_rounds"`
	TotalRounds    int        `json:"total_rounds"`
}

const PolicyVersion = 2

type Verdict struct {
	PolicyVersion int      `json:"policy_version"`
	State         string   `json:"state"` // "suspected" is the original API key for a positive finding.
	Reason        string   `json:"reason"`
	Networks      []string `json:"networks,omitempty"`
	SuspectIPs    []string `json:"suspect_ips,omitempty"`
}

type Result struct {
	Verdict   Verdict    `json:"verdict"`
	Exit      *Exit      `json:"exit"`
	Resolvers []Resolver `json:"resolvers"`
}

var accessNetworks = map[string]string{
	"移动": "中国移动", "中国移动": "中国移动", "中国移动通信": "中国移动",
	"china mobile": "中国移动", "china mobile communications group co ltd": "中国移动", "cmnet": "中国移动",
	"电信": "中国电信", "中国电信": "中国电信", "china telecom": "中国电信",
	"chinanet": "中国电信", "chinanet-backbone": "中国电信", "chinanet backbone": "中国电信",
	"联通": "中国联通", "中国联通": "中国联通", "china unicom": "中国联通", "china unicom backbone": "中国联通",
	"china unicom china169 backbone": "中国联通", "china169-backbone": "中国联通",
	"comcast": "Comcast", "comcast cable communications llc": "Comcast", "xfinity": "Comcast",
}

// APNIC backbone ASNs provide a fallback when the IP database only names a regional branch.
var mainlandBackbones = map[string]string{
	"AS4134": "中国电信", "AS4837": "中国联通", "AS9808": "中国移动",
}

var publicNetworks = map[string]string{
	"cloudflare": "Cloudflare", "cloudflarenet": "Cloudflare", "cloudflare inc": "Cloudflare",
	"google": "Google", "google llc": "Google", "google public dns": "Google",
	"quad9": "Quad9", "quad9 foundation": "Quad9", "opendns": "OpenDNS",
	"nextdns": "NextDNS", "nextdns inc": "NextDNS",
	"alidns": "AliDNS", "阿里公共dns": "AliDNS", "alibaba public dns": "AliDNS",
	"dnspod": "DNSPod", "dnspod public dns": "DNSPod", "腾讯云公共dns": "DNSPod",
	"adguard dns": "AdGuard DNS",
}

// Documented service addresses. A generic Alibaba, Tencent, or hosting ASN does
// not identify its tenant; these matches apply only to the address observed.
var publicAddresses = map[string]string{
	"223.5.5.5": "AliDNS", "223.6.6.6": "AliDNS",
	"119.29.29.29": "DNSPod",
	"94.140.14.14": "AdGuard DNS", "94.140.15.15": "AdGuard DNS",
}

var labelPunctuation = strings.NewReplacer(",", "", ".", "")

func key(name string) string {
	name = strings.ToLower(labelPunctuation.Replace(name))
	return strings.Join(strings.Fields(name), " ")
}

func country(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 || code == "XX" || code == "ZZ" ||
		code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return ""
	}
	return code
}

func address(ip string) string {
	if parsed, err := netip.ParseAddr(ip); err == nil {
		return parsed.Unmap().String()
	}
	return ""
}

func network(name string) string {
	normalized := key(name)
	if normalized == "unknown" || normalized == "n/a" || normalized == "-" {
		return ""
	}
	if label := accessNetworks[normalized]; label != "" {
		return label
	}
	if label := publicNetworks[normalized]; label != "" {
		return label
	}
	return strings.TrimSpace(name)
}

func normalizeResolver(r Resolver) Resolver {
	r.IP = address(r.IP)
	r.Country = country(r.Country)
	r.ASN = strings.ToUpper(strings.TrimSpace(r.ASN))
	if label := publicAddresses[r.IP]; label != "" {
		r.ISP, r.NetworkType = label, "public"
		return r
	}
	if label := publicNetworks[key(r.ISP)]; label != "" {
		r.ISP, r.NetworkType = label, "public"
		return r
	}
	// Prefer a known access operator over a generic infrastructure classification.
	if accessNetworks[key(r.ISP)] != "" {
		r.NetworkType = "access"
	} else if r.Country == "CN" && (r.NetworkType == "" || r.NetworkType == "access") {
		if carrier := mainlandBackbones[r.ASN]; carrier != "" {
			r.ISP, r.NetworkType = carrier, "access"
		}
	}
	r.ISP = network(r.ISP)
	return r
}

func uniqueNames(names []string) []string {
	unique := make(map[string]bool)
	var result []string
	for _, name := range names {
		if name != "" && !unique[name] {
			unique[name] = true
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func assess(exit *Exit, resolvers []Resolver, observed, total int) Verdict {
	if len(resolvers) == 0 || observed == 0 {
		return Verdict{State: "unobserved", Reason: "no-samples"}
	}
	if exit == nil || exit.IP == "" || exit.Country == "" || exit.Country == "LOCAL" {
		return Verdict{State: "unknown", Reason: "missing-exit"}
	}

	var carrierIPs, carrierNames []string
	unknown := false
	for _, r := range resolvers {
		switch {
		case r.NetworkType == "public" && r.ISP != "":
			continue
		case r.Country == "" || r.ISP == "":
			unknown = true
		case r.NetworkType == "access":
			if exit.Country != "CN" && r.Country == "CN" {
				carrierIPs = append(carrierIPs, r.IP)
				carrierNames = append(carrierNames, r.ISP)
			}
		case r.NetworkType != "hosting":
			unknown = true
		}
	}

	if len(carrierIPs) > 0 {
		return Verdict{State: "suspected", Reason: "mainland-carrier", Networks: uniqueNames(carrierNames), SuspectIPs: carrierIPs}
	}
	if observed < total {
		return Verdict{State: "unknown", Reason: "partial-samples"}
	}
	if unknown {
		return Verdict{State: "unknown", Reason: "unclassified-resolver"}
	}
	if exit.Country == "CN" {
		return Verdict{State: "clear", Reason: "domestic-exit"}
	}
	return Verdict{State: "clear", Reason: "no-mainland-carrier"}
}

func (input Input) Valid() bool {
	return input.TotalRounds >= 1 && input.TotalRounds <= 3 &&
		input.ObservedRounds >= 0 && input.ObservedRounds <= input.TotalRounds && len(input.Resolvers) <= 128
}

// Evaluate only uses the available observations; it cannot confirm a tunnel
// policy violation or look up any IP from caller-provided data.
func Evaluate(input Input) Result {
	result := Result{Resolvers: make([]Resolver, 0, len(input.Resolvers))}
	if input.Exit != nil {
		exit := *input.Exit
		exit.IP = address(exit.IP)
		if exit.Country != "LOCAL" {
			exit.Country = country(exit.Country)
		}
		exit.ASN = strings.ToUpper(strings.TrimSpace(exit.ASN))
		if exit.Type == "" && accessNetworks[key(exit.Network)] != "" {
			exit.Type = "access"
		}
		exit.Network = network(exit.Network)
		result.Exit = &exit
	}
	for _, resolver := range input.Resolvers {
		resolver = normalizeResolver(resolver)
		if resolver.IP != "" {
			result.Resolvers = append(result.Resolvers, resolver)
		}
	}
	result.Verdict = assess(result.Exit, result.Resolvers, input.ObservedRounds, input.TotalRounds)
	result.Verdict.PolicyVersion = PolicyVersion
	return result
}
