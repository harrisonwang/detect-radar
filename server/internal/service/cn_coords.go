package service

import "strings"

type cnCoord struct {
	lat float64
	lon float64
}

var cnProvinceCoords = map[string]cnCoord{
	"北京":  {39.9042, 116.4074},
	"天津":  {39.3434, 117.3616},
	"上海":  {31.2304, 121.4737},
	"重庆":  {29.5630, 106.5516},
	"河北":  {38.0428, 114.5149},
	"山西":  {37.8706, 112.5489},
	"内蒙古": {40.8414, 111.7519},
	"辽宁":  {41.8057, 123.4315},
	"吉林":  {43.8171, 125.3235},
	"黑龙江": {45.8038, 126.5349},
	"江苏":  {32.0603, 118.7969},
	"浙江":  {30.2741, 120.1551},
	"安徽":  {31.8206, 117.2272},
	"福建":  {26.0745, 119.2965},
	"江西":  {28.6820, 115.8579},
	"山东":  {36.6512, 117.1201},
	"河南":  {34.7466, 113.6254},
	"湖北":  {30.5928, 114.3055},
	"湖南":  {28.2282, 112.9388},
	"广东":  {23.1291, 113.2644},
	"广西":  {22.8170, 108.3665},
	"海南":  {20.0444, 110.1999},
	"四川":  {30.5728, 104.0668},
	"贵州":  {26.6470, 106.6302},
	"云南":  {25.0389, 102.7183},
	"西藏":  {29.6520, 91.1721},
	"陕西":  {34.3416, 108.9398},
	"甘肃":  {36.0611, 103.8343},
	"青海":  {36.6171, 101.7782},
	"宁夏":  {38.4872, 106.2309},
	"新疆":  {43.8256, 87.6168},
	"台湾":  {25.0330, 121.5654},
	"香港":  {22.3193, 114.1694},
	"澳门":  {22.1987, 113.5439},
}

var cnRegionSuffixes = []string{"省", "市", "特别行政区", "自治区", "壮族", "回族", "维吾尔"}

func normalizeCNRegion(region string) string {
	s := strings.TrimSpace(region)
	for _, suf := range cnRegionSuffixes {
		s = strings.TrimSuffix(s, suf)
	}
	return s
}

func lookupCNProvinceCoords(region string) (lat, lon float64, ok bool) {
	key := normalizeCNRegion(region)
	if key == "" {
		return 0, 0, false
	}
	if c, found := cnProvinceCoords[key]; found {
		return c.lat, c.lon, true
	}
	lower := strings.ToLower(strings.TrimSpace(region))
	switch {
	case strings.Contains(lower, "hong kong") || strings.Contains(lower, "香港"):
		c := cnProvinceCoords["香港"]
		return c.lat, c.lon, true
	case strings.Contains(lower, "macau") || strings.Contains(lower, "macao") || strings.Contains(lower, "澳门") || strings.Contains(lower, "澳門"):
		c := cnProvinceCoords["澳门"]
		return c.lat, c.lon, true
	case strings.Contains(lower, "taiwan") || strings.Contains(lower, "台湾") || strings.Contains(lower, "臺灣"):
		c := cnProvinceCoords["台湾"]
		return c.lat, c.lon, true
	}
	return 0, 0, false
}
