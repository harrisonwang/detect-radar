package service

import (
	"strings"
	"unicode"

	"detect-radar/internal/model"
)

var cnProvinceEN = map[string]string{
	"北京": "Beijing", "天津": "Tianjin", "上海": "Shanghai", "重庆": "Chongqing",
	"河北": "Hebei", "山西": "Shanxi", "辽宁": "Liaoning", "吉林": "Jilin",
	"黑龙江": "Heilongjiang", "江苏": "Jiangsu", "浙江": "Zhejiang", "安徽": "Anhui",
	"福建": "Fujian", "江西": "Jiangxi", "山东": "Shandong", "河南": "Henan",
	"湖北": "Hubei", "湖南": "Hunan", "广东": "Guangdong", "海南": "Hainan",
	"四川": "Sichuan", "贵州": "Guizhou", "云南": "Yunnan", "陕西": "Shaanxi",
	"甘肃": "Gansu", "青海": "Qinghai", "台湾": "Taiwan", "内蒙古": "Inner Mongolia",
	"广西": "Guangxi", "西藏": "Tibet", "宁夏": "Ningxia", "新疆": "Xinjiang",
	"香港": "Hong Kong", "澳门": "Macao",
}

var cnCityEN = map[string]string{
	"北京": "Beijing", "上海": "Shanghai", "天津": "Tianjin", "重庆": "Chongqing",
	"广州": "Guangzhou", "深圳": "Shenzhen", "杭州": "Hangzhou", "南京": "Nanjing",
	"苏州": "Suzhou", "成都": "Chengdu", "武汉": "Wuhan", "西安": "Xi'an",
	"长沙": "Changsha", "郑州": "Zhengzhou", "青岛": "Qingdao", "大连": "Dalian",
	"厦门": "Xiamen", "宁波": "Ningbo", "无锡": "Wuxi", "合肥": "Hefei",
	"福州": "Fuzhou", "济南": "Jinan", "沈阳": "Shenyang", "哈尔滨": "Harbin",
	"长春": "Changchun", "昆明": "Kunming", "南宁": "Nanning", "海口": "Haikou",
	"三亚": "Sanya", "贵阳": "Guiyang", "兰州": "Lanzhou", "太原": "Taiyuan",
	"石家庄": "Shijiazhuang", "南昌": "Nanchang", "呼和浩特": "Hohhot", "乌鲁木齐": "Urumqi",
	"银川": "Yinchuan", "西宁": "Xining", "拉萨": "Lhasa", "东莞": "Dongguan",
	"佛山": "Foshan", "珠海": "Zhuhai", "中山": "Zhongshan", "惠州": "Huizhou",
	"泉州": "Quanzhou", "温州": "Wenzhou", "嘉兴": "Jiaxing", "常州": "Changzhou",
	"南通": "Nantong", "徐州": "Xuzhou", "扬州": "Yangzhou", "烟台": "Yantai",
	"潍坊": "Weifang", "临沂": "Linyi", "洛阳": "Luoyang", "唐山": "Tangshan",
	"保定": "Baoding", "芜湖": "Wuhu", "香港": "Hong Kong", "澳门": "Macao",
	"台北": "Taipei", "高雄": "Kaohsiung", "台中": "Taichung",
}

var cnISPEN = []struct {
	keyword string
	en      string
}{
	{"电信", "China Telecom"},
	{"移动", "China Mobile"},
	{"联通", "China Unicom"},
	{"广电", "China Broadnet"},
	{"铁通", "China Tietong"},
	{"教育网", "CERNET"},
	{"科技网", "CSTNET"},
	{"阿里", "Alibaba Cloud"},
	{"腾讯", "Tencent Cloud"},
	{"华为", "Huawei Cloud"},
	{"百度", "Baidu Cloud"},
	{"金山云", "Kingsoft Cloud"},
	{"天翼云", "CTYun"},
	{"优刻得", "UCloud"},
	{"ucloud", "UCloud"},
	{"青云", "QingCloud"},
}

func trimCNAdminSuffix(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{"特别行政区", "壮族自治区", "回族自治区", "维吾尔自治区", "自治区", "省", "市"} {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}

func translateCNProvinceEN(province string) string {
	key := trimCNAdminSuffix(province)
	if en, ok := cnProvinceEN[key]; ok {
		return en
	}
	if !containsCJK(province) {
		return strings.TrimSpace(province)
	}
	return ""
}

func translateCNCityEN(city string) string {
	key := trimCNAdminSuffix(city)
	if en, ok := cnCityEN[key]; ok {
		return en
	}
	if !containsCJK(city) {
		return strings.TrimSpace(city)
	}
	return ""
}

func translateCNISPEN(isp string) string {
	lower := strings.ToLower(strings.TrimSpace(isp))
	for _, entry := range cnISPEN {
		if strings.Contains(lower, entry.keyword) {
			return entry.en
		}
	}
	if !containsCJK(isp) {
		return strings.TrimSpace(isp)
	}
	return ""
}

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func ProjectIPIntel(intel *model.IPIntel, lang string) *model.IPIntel {
	if intel == nil {
		return nil
	}
	cp := cloneIntel(intel)

	if lang == "en" {
		if cp.GeoEN != nil {
			if cp.GeoEN.CountryName != "" {
				cp.CountryName = cp.GeoEN.CountryName
			}
			if cp.GeoEN.Region != "" {
				cp.Region = cp.GeoEN.Region
			}
			if cp.GeoEN.City != "" {
				cp.City = cp.GeoEN.City
			}
			if cp.GeoEN.ISP != "" {
				cp.ISP = cp.GeoEN.ISP
			}
		}
		if containsCJK(cp.CountryName) {
			switch strings.ToUpper(cp.Country) {
			case "CN":
				cp.CountryName = "China"
			case "HK":
				cp.CountryName = "Hong Kong"
			case "MO":
				cp.CountryName = "Macao"
			case "TW":
				cp.CountryName = "Taiwan"
			default:
				cp.CountryName = cp.Country
			}
		}
		if containsCJK(cp.Region) {
			cp.Region = translateCNProvinceEN(cp.Region)
		}
		if containsCJK(cp.City) {
			cp.City = translateCNCityEN(cp.City)
		}
		if containsCJK(cp.ISP) {
			if mapped := translateCNISPEN(cp.ISP); mapped != "" {
				cp.ISP = mapped
			} else if !containsCJK(cp.Org) && cp.Org != "" {
				cp.ISP = cp.Org
			} else {
				cp.ISP = ""
			}
		}
	} else {
		if cp.GeoZH != nil {
			if cp.GeoZH.CountryName != "" {
				cp.CountryName = cp.GeoZH.CountryName
			}
			if cp.GeoZH.Region != "" {
				cp.Region = cp.GeoZH.Region
			}
			if cp.GeoZH.City != "" {
				cp.City = cp.GeoZH.City
			}
			if cp.GeoZH.ISP != "" {
				cp.ISP = cp.GeoZH.ISP
			}
		}
	}

	cp.GeoZH = nil
	cp.GeoEN = nil
	return cp
}
