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
	Type    string `json:"type,omitempty"`
}

type Resolver struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	ISP         string `json:"isp"`
	ASN         string `json:"asn,omitempty"`
	NetworkType string `json:"network_type,omitempty"`
	Rounds      int    `json:"rounds,omitempty"`
	RoundIDs    []int  `json:"round_ids,omitempty"`
}

type Input struct {
	Exit           *Exit      `json:"exit"`
	Resolvers      []Resolver `json:"resolvers"`
	ObservedRounds int        `json:"observed_rounds"`
	TotalRounds    int        `json:"total_rounds"`
}

const PolicyVersion = 3

type Verdict struct {
	PolicyVersion int      `json:"policy_version"`
	State         string   `json:"state"`
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
	"移动": "China Mobile", "中国移动": "China Mobile", "中国移动通信": "China Mobile",
	"china mobile": "China Mobile", "china mobile communications group co ltd": "China Mobile", "cmnet": "China Mobile",
	"电信": "China Telecom", "中国电信": "China Telecom", "china telecom": "China Telecom",
	"chinanet": "China Telecom", "chinanet-backbone": "China Telecom", "chinanet backbone": "China Telecom",
	"联通": "China Unicom", "中国联通": "China Unicom", "china unicom": "China Unicom", "china unicom backbone": "China Unicom",
	"china unicom china169 backbone": "China Unicom", "china169-backbone": "China Unicom",
	"广电": "China Broadnet", "中国广电": "China Broadnet", "china broadnet": "China Broadnet",
	"comcast": "Comcast", "comcast cable communications llc": "Comcast", "xfinity": "Comcast",
	"at&t": "AT&T", "at&t services inc": "AT&T", "att": "AT&T",
	"verizon": "Verizon", "verizon business": "Verizon", "mci communications services": "Verizon",
	"charter communications": "Charter Spectrum", "spectrum": "Charter Spectrum",
	"t-mobile usa": "T-Mobile USA", "t-mobile usa inc": "T-Mobile USA",
	"cox communications": "Cox Communications", "cox communications inc": "Cox Communications",
	"deutsche telekom": "Deutsche Telekom", "deutsche telekom ag": "Deutsche Telekom",
	"vodafone gmbh": "Vodafone Germany", "vodafone germany": "Vodafone Germany",
	"british telecommunications": "BT", "bt": "BT",
	"virgin media": "Virgin Media", "virgin media limited": "Virgin Media",
	"sky uk": "Sky UK", "sky uk limited": "Sky UK",
	"orange sa": "Orange France", "orange france": "Orange France",
	"free sas": "Free SAS",
	"ntt communications": "NTT", "ntt ocn": "NTT OCN",
	"kddi": "KDDI", "kddi corporation": "KDDI",
	"softbank": "SoftBank", "softbank bb": "SoftBank", "softbank corp": "SoftBank",
	"kt": "KT", "korea telecom": "KT",
	"sk broadband": "SK Broadband",
	"lg uplus": "LG Uplus", "lg dacom": "LG Uplus",
	"singtel": "Singtel", "singapore telecommunications": "Singtel",
	"telstra": "Telstra", "telstra limited": "Telstra",
	"bell canada": "Bell Canada", "rogers communications": "Rogers Communications", "telus": "TELUS",
	"hkt": "HKT", "pccw": "HKT", "hkbn": "HKBN",
	"chunghwa telecom": "Chunghwa Telecom", "hinet": "Chunghwa Telecom",
}

var accessASNs = map[string]string{
	"AS4134": "China Telecom", "AS4809": "China Telecom", "AS4812": "China Telecom",
	"AS4837": "China Unicom", "AS17621": "China Unicom", "AS17623": "China Unicom",
	"AS9808": "China Mobile", "AS56040": "China Mobile", "AS56041": "China Mobile", "AS56046": "China Mobile", "AS56048": "China Mobile",
	"AS7640": "China Broadnet",
	"AS4760": "HKT", "AS9269": "HKBN", "AS4609": "CTM", "AS3462": "Chunghwa Telecom",
	"AS7922": "Comcast", "AS7018": "AT&T", "AS20057": "AT&T Mobility", "AS701": "Verizon", "AS22394": "Verizon Wireless",
	"AS20115": "Charter Spectrum", "AS21928": "T-Mobile USA", "AS22773": "Cox Communications",
	"AS577": "Bell Canada", "AS812": "Rogers Communications", "AS852": "TELUS",
	"AS2856": "BT", "AS5089": "Virgin Media", "AS5607": "Sky UK",
	"AS3320": "Deutsche Telekom", "AS3209": "Vodafone Germany", "AS6805": "Telefonica Germany",
	"AS3215": "Orange France", "AS12322": "Free SAS",
	"AS4713": "NTT OCN", "AS2516": "KDDI", "AS17676": "SoftBank",
	"AS4766": "KT", "AS9318": "SK Broadband", "AS3786": "LG Uplus",
	"AS3758": "Singtel", "AS4657": "StarHub", "AS1221": "Telstra", "AS7474": "Optus",
}

var publicNetworks = map[string]string{
	"cloudflare": "Cloudflare", "cloudflarenet": "Cloudflare", "cloudflare inc": "Cloudflare", "cloudflare warp": "Cloudflare",
	"google": "Google", "google llc": "Google", "google public dns": "Google",
	"quad9": "Quad9", "quad9 foundation": "Quad9", "woodynets": "Quad9",
	"opendns": "OpenDNS", "cisco opendns": "OpenDNS", "cisco opendns llc": "OpenDNS", "cisco umbrella": "OpenDNS",
	"nextdns": "NextDNS", "nextdns inc": "NextDNS",
	"control d": "Control D", "controld": "Control D", "windscribe limited": "Control D",
	"mullvad": "Mullvad DNS", "mullvad dns": "Mullvad DNS", "mullvad vpn": "Mullvad DNS",
	"cleanbrowsing": "CleanBrowsing",
	"alidns": "AliDNS", "阿里公共dns": "AliDNS", "alibaba public dns": "AliDNS",
	"dnspod": "DNSPod", "dnspod public dns": "DNSPod", "腾讯云公共dns": "DNSPod",
	"adguard": "AdGuard DNS", "adguard dns": "AdGuard DNS",
	"114dns": "114DNS",
}

var publicAddresses = map[string]string{
	"1.1.1.1": "Cloudflare", "1.0.0.1": "Cloudflare",
	"8.8.8.8": "Google", "8.8.4.4": "Google",
	"9.9.9.9": "Quad9", "149.112.112.112": "Quad9",
	"208.67.222.222": "OpenDNS", "208.67.220.220": "OpenDNS",
	"45.90.28.0": "NextDNS", "45.90.30.0": "NextDNS",
	"76.76.2.0": "Control D", "76.76.10.0": "Control D",
	"194.242.2.2": "Mullvad DNS", "194.242.2.3": "Mullvad DNS",
	"185.228.168.9": "CleanBrowsing", "185.228.169.9": "CleanBrowsing",
	"223.5.5.5": "AliDNS", "223.6.6.6": "AliDNS",
	"119.29.29.29": "DNSPod", "119.28.28.28": "DNSPod",
	"114.114.114.114": "114DNS", "114.114.115.115": "114DNS",
	"94.140.14.14": "AdGuard DNS", "94.140.15.15": "AdGuard DNS",
}

var publicASNs = map[string]string{
	"AS13335":  "Cloudflare",
	"AS15169":  "Google",
	"AS19281":  "Quad9",
	"AS36692":  "OpenDNS",
	"AS34939":  "NextDNS",
	"AS398962": "Control D",
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

func normalizeASN(asn string) string {
	s := strings.ToUpper(strings.TrimSpace(asn))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "AS") {
		return s
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s
		}
	}
	return "AS" + s
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
	r.ASN = normalizeASN(r.ASN)
	if label := publicAddresses[r.IP]; label != "" {
		r.ISP, r.NetworkType = label, "public"
		return r
	}
	if label := publicNetworks[key(r.ISP)]; label != "" {
		r.ISP, r.NetworkType = label, "public"
		return r
	}
	if label := publicASNs[r.ASN]; label != "" {
		r.ISP, r.NetworkType = label, "public"
		return r
	}
	if r.NetworkType == "hosting" {
		r.ISP = network(r.ISP)
		return r
	}
	if accessNetworks[key(r.ISP)] != "" {
		r.NetworkType = "access"
	} else if r.NetworkType == "" || r.NetworkType == "access" {
		if carrier := accessASNs[r.ASN]; carrier != "" {
			if network(r.ISP) == "" {
				r.ISP = carrier
			}
			r.NetworkType = "access"
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

	exitProxyOrHosting := exit.Type == "proxy" || exit.Type == "hosting"
	var crossIPs, crossNames []string
	var bypassIPs, bypassNames []string
	hasSameRegionAccess := false
	allPublicOrHosting := true
	unknown := false

	for _, r := range resolvers {
		switch {
		case r.NetworkType == "public" && r.ISP != "":
			continue
		case r.Country == "" || r.ISP == "":
			allPublicOrHosting = false
			unknown = true
		case r.NetworkType == "access":
			allPublicOrHosting = false
			sameASN := exit.ASN != "" && r.ASN != "" && exit.ASN == r.ASN
			if sameASN {
				hasSameRegionAccess = true
				continue
			}
			if r.Country != exit.Country {
				crossIPs = append(crossIPs, r.IP)
				crossNames = append(crossNames, r.ISP)
			} else if exitProxyOrHosting {
				bypassIPs = append(bypassIPs, r.IP)
				bypassNames = append(bypassNames, r.ISP)
			} else {
				hasSameRegionAccess = true
			}
		case r.NetworkType == "hosting":
			continue
		default:
			allPublicOrHosting = false
			unknown = true
		}
	}

	if len(crossIPs) > 0 || len(bypassIPs) > 0 {
		suspectIPs := append(crossIPs, bypassIPs...)
		suspectNames := uniqueNames(append(crossNames, bypassNames...))
		reason := "proxy-isp-bypass"
		if len(crossIPs) > 0 {
			reason = "cross-region-isp"
		}
		return Verdict{State: "suspected", Reason: reason, Networks: suspectNames, SuspectIPs: suspectIPs}
	}
	if observed < total {
		return Verdict{State: "unknown", Reason: "partial-samples"}
	}
	if unknown {
		return Verdict{State: "unknown", Reason: "unclassified-resolver"}
	}
	if hasSameRegionAccess {
		return Verdict{State: "clear", Reason: "same-isp-exit"}
	}
	if allPublicOrHosting {
		return Verdict{State: "clear", Reason: "public-or-hosting-only"}
	}
	return Verdict{State: "clear", Reason: "no-leaked-isp"}
}

func (input Input) Valid() bool {
	return input.TotalRounds >= 1 && input.TotalRounds <= 3 &&
		input.ObservedRounds >= 0 && input.ObservedRounds <= input.TotalRounds && len(input.Resolvers) <= 128
}

func Evaluate(input Input) Result {
	result := Result{Resolvers: make([]Resolver, 0, len(input.Resolvers))}
	if input.Exit != nil {
		exit := *input.Exit
		exit.IP = address(exit.IP)
		if exit.Country != "LOCAL" {
			exit.Country = country(exit.Country)
		}
		exit.ASN = normalizeASN(exit.ASN)
		if exit.Type == "" {
			if accessNetworks[key(exit.Network)] != "" || accessASNs[exit.ASN] != "" {
				exit.Type = "access"
			}
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
