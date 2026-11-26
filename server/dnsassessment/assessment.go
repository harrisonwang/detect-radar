package dnsassessment

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
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

type CatalogEntry struct {
	Role   string
	NameEN string
	NameZH string
}

type Catalog interface {
	LookupPublicDNSIP(ip string) (CatalogEntry, bool)
	LookupASN(asn uint64) (CatalogEntry, bool)
	MatchNetworkAlias(raw string) (CatalogEntry, bool)
}

type catalogHolder struct {
	cat Catalog
}

var defaultCatalog atomic.Pointer[catalogHolder]

func SetDefaultCatalog(cat Catalog) {
	if cat == nil {
		defaultCatalog.Store(nil)
		return
	}
	defaultCatalog.Store(&catalogHolder{cat: cat})
}

func DefaultCatalog() Catalog {
	if h := defaultCatalog.Load(); h != nil {
		return h.cat
	}
	return nil
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

func parseASNNumber(asn string) uint64 {
	s := strings.ToUpper(strings.TrimSpace(asn))
	s = strings.TrimPrefix(s, "AS")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func cleanNetwork(name string) string {
	normalized := key(name)
	if normalized == "" || normalized == "unknown" || normalized == "n/a" || normalized == "-" {
		return ""
	}
	return strings.TrimSpace(name)
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func entryDisplayName(e CatalogEntry, lang string) string {
	if lang == "zh" {
		if e.NameZH != "" {
			return e.NameZH
		}
		return e.NameEN
	}
	if e.NameEN != "" {
		return e.NameEN
	}
	if !containsHan(e.NameZH) {
		return e.NameZH
	}
	return ""
}

func normalizeResolver(r Resolver, cat Catalog, lang string) Resolver {
	r.IP = address(r.IP)
	r.Country = country(r.Country)
	r.ASN = normalizeASN(r.ASN)

	if cat != nil {
		if entry, ok := cat.LookupPublicDNSIP(r.IP); ok {
			r.ISP = entryDisplayName(entry, lang)
			r.NetworkType = "public"
			return r
		}

		alias, hasAlias := cat.MatchNetworkAlias(r.ISP)
		if hasAlias && alias.Role == "public_dns" {
			r.ISP = entryDisplayName(alias, lang)
			r.NetworkType = "public"
			return r
		}

		asnNum := parseASNNumber(r.ASN)
		asnEntry, hasASN := CatalogEntry{}, false
		if asnNum > 0 {
			asnEntry, hasASN = cat.LookupASN(asnNum)
		}
		if hasASN && asnEntry.Role == "public_dns" {
			r.ISP = entryDisplayName(asnEntry, lang)
			r.NetworkType = "public"
			return r
		}

		if r.NetworkType == "hosting" {
			if hasAlias {
				r.ISP = entryDisplayName(alias, lang)
			} else {
				cleaned := cleanNetwork(r.ISP)
				if lang == "en" && containsHan(cleaned) {
					if hasASN {
						r.ISP = entryDisplayName(asnEntry, lang)
					} else {
						r.ISP = ""
					}
				} else {
					r.ISP = cleaned
				}
			}
			return r
		}

		if hasAlias && alias.Role == "access" {
			r.NetworkType = "access"
			r.ISP = entryDisplayName(alias, lang)
			return r
		}

		if (r.NetworkType == "" || r.NetworkType == "access") && hasASN && asnEntry.Role == "access" {
			r.NetworkType = "access"
			cleaned := cleanNetwork(r.ISP)
			if cleaned == "" {
				r.ISP = entryDisplayName(asnEntry, lang)
			} else if hasAlias {
				r.ISP = entryDisplayName(alias, lang)
			} else if lang == "en" && containsHan(cleaned) {
				r.ISP = entryDisplayName(asnEntry, lang)
			} else if lang == "zh" && asnEntry.NameZH != "" && containsHan(asnEntry.NameZH) {
				r.ISP = asnEntry.NameZH
			} else {
				r.ISP = cleaned
			}
			return r
		}

		if hasAlias {
			r.ISP = entryDisplayName(alias, lang)
			return r
		}

		if lang == "en" && containsHan(cleanNetwork(r.ISP)) {
			if hasASN {
				r.ISP = entryDisplayName(asnEntry, lang)
			} else {
				r.ISP = ""
			}
			return r
		}
	}

	cleaned := cleanNetwork(r.ISP)
	if lang == "en" && containsHan(cleaned) {
		r.ISP = ""
		return r
	}
	r.ISP = cleaned
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
	return EvaluateWithCatalog(input, DefaultCatalog(), "en")
}

func EvaluateWithCatalog(input Input, cat Catalog, lang string) Result {
	if lang != "zh" {
		lang = "en"
	}
	result := Result{Resolvers: make([]Resolver, 0, len(input.Resolvers))}
	if input.Exit != nil {
		exit := *input.Exit
		exit.IP = address(exit.IP)
		if exit.Country != "LOCAL" {
			exit.Country = country(exit.Country)
		}
		exit.ASN = normalizeASN(exit.ASN)

		cleaned := cleanNetwork(exit.Network)
		var alias CatalogEntry
		var hasAlias bool
		var asnEntry CatalogEntry
		var hasASN bool
		if cat != nil {
			if cleaned != "" {
				alias, hasAlias = cat.MatchNetworkAlias(cleaned)
			}
			if asnNum := parseASNNumber(exit.ASN); asnNum > 0 {
				asnEntry, hasASN = cat.LookupASN(asnNum)
			}
		}

		if exit.Type == "" {
			if (hasAlias && alias.Role == "access") || (hasASN && asnEntry.Role == "access") {
				exit.Type = "access"
			}
		}

		if hasAlias {
			exit.Network = entryDisplayName(alias, lang)
		} else if lang == "en" && containsHan(cleaned) && hasASN {
			exit.Network = entryDisplayName(asnEntry, lang)
		} else if lang == "zh" && hasASN && asnEntry.NameZH != "" && containsHan(asnEntry.NameZH) {
			exit.Network = asnEntry.NameZH
		} else if cleaned != "" && !(lang == "en" && containsHan(cleaned)) {
			exit.Network = cleaned
		} else if hasASN {
			exit.Network = entryDisplayName(asnEntry, lang)
		} else {
			exit.Network = ""
		}
		result.Exit = &exit
	}
	for _, resolver := range input.Resolvers {
		resolver = normalizeResolver(resolver, cat, lang)
		if resolver.IP != "" {
			result.Resolvers = append(result.Resolvers, resolver)
		}
	}
	result.Verdict = assess(result.Exit, result.Resolvers, input.ObservedRounds, input.TotalRounds)
	result.Verdict.PolicyVersion = PolicyVersion
	return result
}
