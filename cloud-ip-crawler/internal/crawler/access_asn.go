package crawler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const peeringDBAccessURL = "https://www.peeringdb.com/api/net?info_type=Cable/DSL/ISP"

type AccessASN struct {
	ASN         uint64
	Name        string
	Country     string
	NetworkKind string
	Source      string
}

var builtinAccessASNs = []AccessASN{
	{ASN: 4134, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4809, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4812, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4816, Name: "China Telecom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4837, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 17621, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 17622, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 17623, Name: "China Unicom", Country: "CN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9808, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56040, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56041, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56042, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56044, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56046, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56047, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 56048, Name: "China Mobile", Country: "CN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 7640, Name: "China Broadnet", Country: "CN", NetworkKind: "residential", Source: "builtin"},

	{ASN: 4760, Name: "HKT", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9269, Name: "HKBN", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9304, Name: "HGC Global Communications", Country: "HK", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4609, Name: "CTM", Country: "MO", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3462, Name: "Chunghwa Telecom", Country: "TW", NetworkKind: "residential", Source: "builtin"},
	{ASN: 24158, Name: "Taiwan Mobile", Country: "TW", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 9674, Name: "FarEasTone", Country: "TW", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 9924, Name: "Taiwan Fixed Network", Country: "TW", NetworkKind: "residential", Source: "builtin"},

	{ASN: 7922, Name: "Comcast", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 7018, Name: "AT&T", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 20057, Name: "AT&T Mobility", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 701, Name: "Verizon", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 22394, Name: "Verizon Wireless", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 20115, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 10796, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 11351, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 11427, Name: "Charter Spectrum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 21928, Name: "T-Mobile USA", Country: "US", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 22773, Name: "Cox Communications", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 209, Name: "CenturyLink", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5650, Name: "Frontier Communications", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 6128, Name: "Optimum", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 16591, Name: "Google Fiber", Country: "US", NetworkKind: "residential", Source: "builtin"},
	{ASN: 14593, Name: "Starlink", Country: "US", NetworkKind: "residential", Source: "builtin"},

	{ASN: 577, Name: "Bell Canada", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	{ASN: 812, Name: "Rogers Communications", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	{ASN: 852, Name: "TELUS", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	{ASN: 6327, Name: "Shaw Communications", Country: "CA", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5769, Name: "Videotron", Country: "CA", NetworkKind: "residential", Source: "builtin"},

	{ASN: 2856, Name: "BT", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5089, Name: "Virgin Media", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5607, Name: "Sky UK", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	{ASN: 13285, Name: "TalkTalk", Country: "GB", NetworkKind: "residential", Source: "builtin"},
	{ASN: 12576, Name: "EE", Country: "GB", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 5378, Name: "Vodafone UK", Country: "GB", NetworkKind: "residential", Source: "builtin"},

	{ASN: 3320, Name: "Deutsche Telekom", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3209, Name: "Vodafone Germany", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	{ASN: 6805, Name: "Telefonica Germany", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	{ASN: 8881, Name: "1&1 Versatel", Country: "DE", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3215, Name: "Orange France", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 12322, Name: "Free SAS", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 15557, Name: "SFR", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5410, Name: "Bouygues Telecom", Country: "FR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 1136, Name: "KPN", Country: "NL", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9143, Name: "Ziggo", Country: "NL", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3303, Name: "Swisscom", Country: "CH", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3301, Name: "Telia", Country: "SE", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3269, Name: "TIM", Country: "IT", NetworkKind: "residential", Source: "builtin"},
	{ASN: 12874, Name: "Fastweb", Country: "IT", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3352, Name: "Telefonica Spain", Country: "ES", NetworkKind: "residential", Source: "builtin"},
	{ASN: 12479, Name: "Orange Spain", Country: "ES", NetworkKind: "residential", Source: "builtin"},
	{ASN: 5617, Name: "Orange Poland", Country: "PL", NetworkKind: "residential", Source: "builtin"},

	{ASN: 4713, Name: "NTT OCN", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	{ASN: 2516, Name: "KDDI", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	{ASN: 17676, Name: "SoftBank", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	{ASN: 2527, Name: "Sony Network Communications", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	{ASN: 2497, Name: "IIJ", Country: "JP", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9605, Name: "NTT DOCOMO", Country: "JP", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 4766, Name: "KT", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9318, Name: "SK Broadband", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 3786, Name: "LG Uplus", Country: "KR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9644, Name: "SK Telecom", Country: "KR", NetworkKind: "mobile", Source: "builtin"},

	{ASN: 3758, Name: "Singtel", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4657, Name: "StarHub", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4773, Name: "M1", Country: "SG", NetworkKind: "residential", Source: "builtin"},
	{ASN: 1221, Name: "Telstra", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	{ASN: 7474, Name: "Optus", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	{ASN: 7545, Name: "TPG Telecom", Country: "AU", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4771, Name: "Spark NZ", Country: "NZ", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9500, Name: "One NZ", Country: "NZ", NetworkKind: "residential", Source: "builtin"},

	{ASN: 28573, Name: "Claro Brazil", Country: "BR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 26599, Name: "Vivo", Country: "BR", NetworkKind: "residential", Source: "builtin"},
	{ASN: 55836, Name: "Reliance Jio", Country: "IN", NetworkKind: "mobile", Source: "builtin"},
	{ASN: 24560, Name: "Bharti Airtel", Country: "IN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9829, Name: "BSNL", Country: "IN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 4788, Name: "TM Net", Country: "MY", NetworkKind: "residential", Source: "builtin"},
	{ASN: 7713, Name: "Telkom Indonesia", Country: "ID", NetworkKind: "residential", Source: "builtin"},
	{ASN: 45899, Name: "VNPT", Country: "VN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 7552, Name: "Viettel", Country: "VN", NetworkKind: "residential", Source: "builtin"},
	{ASN: 131445, Name: "AIS Fibre", Country: "TH", NetworkKind: "residential", Source: "builtin"},
	{ASN: 9299, Name: "PLDT", Country: "PH", NetworkKind: "residential", Source: "builtin"},
}

func BuiltinAccessASNs() []AccessASN {
	out := make([]AccessASN, len(builtinAccessASNs))
	copy(out, builtinAccessASNs)
	return out
}

type peeringDBNetResponse struct {
	Data []struct {
		ASN      uint64 `json:"asn"`
		Name     string `json:"name"`
		InfoType string `json:"info_type"`
	} `json:"data"`
}

func ParsePeeringDBAccessASNs(data []byte) ([]AccessASN, error) {
	var resp peeringDBNetResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 PeeringDB JSON 失败: %w", err)
	}

	byASN := make(map[uint64]AccessASN, len(resp.Data)+len(builtinAccessASNs))
	for _, netItem := range resp.Data {
		if netItem.ASN == 0 {
			continue
		}
		if netItem.InfoType != "" && !strings.EqualFold(netItem.InfoType, "Cable/DSL/ISP") {
			continue
		}
		name := strings.TrimSpace(netItem.Name)
		if name == "" {
			continue
		}
		kind := "residential"
		lower := strings.ToLower(name)
		if strings.Contains(lower, "mobile") || strings.Contains(lower, "wireless") || strings.Contains(lower, "cellular") {
			kind = "mobile"
		}
		byASN[netItem.ASN] = AccessASN{
			ASN:         netItem.ASN,
			Name:        name,
			NetworkKind: kind,
			Source:      "peeringdb",
		}
	}

	for _, b := range builtinAccessASNs {
		byASN[b.ASN] = b
	}

	out := make([]AccessASN, 0, len(byASN))
	for _, item := range byASN {
		out = append(out, item)
	}
	return out, nil
}

func CrawlAccessASNs(offline bool) ([]AccessASN, error) {
	if offline {
		return BuiltinAccessASNs(), nil
	}
	body, err := fetchBytes(peeringDBAccessURL, 45*time.Second)
	if err != nil {
		return nil, err
	}
	items, err := ParsePeeringDBAccessASNs(body)
	if err != nil {
		return nil, err
	}
	if len(items) <= len(builtinAccessASNs) {
		return nil, fmt.Errorf("PeeringDB 返回条目异常过少 (%d)", len(items))
	}
	return items, nil
}
