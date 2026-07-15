package service

import "strings"

// CN 省会质心坐标表：为 RTT 的地理-延迟下限校验兜底。
//
// 背景：applyCNGeo（ipintel.go）在 CN 权威地理层（ip2region）命中时会**刻意清空**
// intel.Latitude/Longitude——因为 western geo 源对中国网段给的坐标是错的（移动段钉北京、
// IPv6 深圳配海南坐标），而 ip2region 只给省/市/运营商、不含坐标。这是一个诚实决定：
// 坐标经公共 API 暴露，绝不发布编造的精确坐标（见 model/ipintel.go）。
//
// 后果：deriveRTT 的 geo violation 校验对所有中国 IP 全线失效，恰恰放过了「卖中国住宅 IP、
// 实际托管在别处」的代理商（声称在中国=离 LA 极远，实测 TCP RTT 极小→本应判违背）。
//
// 本表用**省会城市坐标**代表整个省/自治区/直辖市的质心，只喂给 RTT 物理下限计算，
// **绝不回写 intel、绝不经 API 暴露**。省会代替全省是粗略近似（新疆/西藏/内蒙古误差尤大），
// 故 deriveRTT 用一个保守 slack 抵消这份不确定性（见 cnCentroidSlackKM）。

// cnCoord 一个省会质心坐标。
type cnCoord struct {
	lat float64
	lon float64
}

// cnProvinceCoords 归一化省级键 → 省会坐标。34 个省级行政区（含港澳台）。
// 坐标均为 **省会/直辖市/特别行政区 中心** 坐标，代表整个省，故为近似质心。
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

// cnRegionSuffixes 需从 intel.Region 剥离的行政区划后缀，供归一化到省会表的短键。
// 顺序讲究：多字后缀（特别行政区/自治区）必须先于民族后缀（壮族/回族/维吾尔）剥离，
// 这样 广西壮族自治区 → 剥 自治区 → 广西壮族 → 再剥 壮族 → 广西 才能一遍走通。
var cnRegionSuffixes = []string{"省", "市", "特别行政区", "自治区", "壮族", "回族", "维吾尔"}

// normalizeCNRegion 把生产 journal 里各种形态的 intel.Region 归一化到省会表的短键。
// 观测到的真实取值：湖南省 / 上海 / 北京 / 香港特别行政区 / 台湾省（直辖市不带「市」后缀，
// 但带也无妨，一并剥）。例：内蒙古自治区→内蒙古、广西壮族自治区→广西、
// 新疆维吾尔自治区→新疆、宁夏回族自治区→宁夏、香港特别行政区→香港、台湾省→台湾。
func normalizeCNRegion(region string) string {
	s := strings.TrimSpace(region)
	for _, suf := range cnRegionSuffixes {
		s = strings.TrimSuffix(s, suf)
	}
	return s
}

// lookupCNProvinceCoords 按 intel.Region 查省会质心坐标。
// 命中省级行政区返回坐标 + ok=true；空串/无法归一化/不在表内 → ok=false，
// 此时 deriveRTT 让 geo 字段保持 nil（即今天的行为，无回归）。
//
// 注意：key off Region 而非 City——City 可能是非中文（如 region=香港特别行政区 而
// city=Hong Kong）。
func lookupCNProvinceCoords(region string) (lat, lon float64, ok bool) {
	key := normalizeCNRegion(region)
	if key == "" {
		return 0, 0, false
	}
	c, found := cnProvinceCoords[key]
	if !found {
		return 0, 0, false
	}
	return c.lat, c.lon, true
}
