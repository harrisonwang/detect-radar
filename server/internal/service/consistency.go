package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // 内嵌 IANA 时区库，保证时区归一化不依赖宿主环境

	"detect-radar/internal/model"
)

var tzOffsetRe = regexp.MustCompile(`^([+-])(\d{2}):?(\d{2})$`)

// ConsistencyService 检测浏览器时区、语言与 IP 归属地是否一致。
type ConsistencyService struct {
	ipIntel *IPIntelService
}

func NewConsistencyService(ipIntel *IPIntelService) *ConsistencyService {
	return &ConsistencyService{ipIntel: ipIntel}
}

// Evaluate 纯评估：由 /scans 复用（避免重复查询 IP 信息）。
func (s *ConsistencyService) Evaluate(browser model.BrowserInfo, intel *model.IPIntel) model.ConsistencyChecks {
	return model.ConsistencyChecks{
		Timezone: s.checkTimezone(browser.Timezone, intel.Timezone),
		Language: s.checkLanguage(browser.Language, browser.Languages, intel.Country),
	}
}

func (s *ConsistencyService) checkTimezone(browserTZ, ipTZ string) model.TimezoneCheck {
	if ipTZ == "" {
		// IP 时区未知，无法判定，给通过（不制造假阳性）
		return model.TimezoneCheck{Passed: true, Browser: browserTZ, Expected: "unknown"}
	}
	if browserTZ == ipTZ {
		return model.TimezoneCheck{Passed: true, Browser: browserTZ, Expected: ipTZ}
	}

	// 归一化为 UTC 偏移后比较：浏览器多为 IANA 名(Asia/Shanghai)，
	// IP 数据中的时区可能是 ±HH:MM 偏移（+08:00），直接比较字符串会误判。
	bOff, bOK := tzOffsetMinutes(browserTZ)
	iOff, iOK := tzOffsetMinutes(ipTZ)
	if bOK && iOK {
		return model.TimezoneCheck{
			Passed:      bOff == iOff,
			Browser:     browserTZ,
			Expected:    ipTZ,
			OffsetHours: (bOff - iOff) / 60,
		}
	}

	// 无法归一化，回退到启发式偏移表
	return model.TimezoneCheck{
		Passed:      false,
		Browser:     browserTZ,
		Expected:    ipTZ,
		OffsetHours: s.timezoneOffset(browserTZ, ipTZ),
	}
}

// tzOffsetMinutes 把时区（IANA 名或 ±HH:MM 偏移）归一化为相对 UTC 的分钟数
func tzOffsetMinutes(tz string) (int, bool) {
	if tz == "" {
		return 0, false
	}
	if tz == "UTC" || tz == "GMT" || tz == "Z" {
		return 0, true
	}
	if m := tzOffsetRe.FindStringSubmatch(tz); m != nil {
		h, _ := strconv.Atoi(m[2])
		mm, _ := strconv.Atoi(m[3])
		v := h*60 + mm
		if m[1] == "-" {
			v = -v
		}
		return v, true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, false
	}
	_, off := time.Now().In(loc).Zone()
	return off / 60, true
}

func (s *ConsistencyService) checkLanguage(browserLang string, browserLangs []string, ipCountry string) model.LanguageCheck {
	expected := countryLanguages(ipCountry)
	check := model.LanguageCheck{Browser: browserLang, Expected: expected, IPCountry: ipCountry}

	if len(expected) == 0 {
		// IP 国家未收录/未知，无法判定，给通过（不制造假阳性）
		check.Passed = true
		return check
	}

	// 主语言或语言列表任一命中即通过
	candidates := append([]string{browserLang}, browserLangs...)
	for _, lang := range candidates {
		ll := strings.ToLower(lang)
		for _, exp := range expected {
			if strings.HasPrefix(ll, strings.ToLower(exp)) {
				check.Passed = true
				return check
			}
		}
	}
	check.Passed = false
	return check
}

// timezoneOffset 粗略估算两个时区的小时差（仅覆盖常见时区，供展示参考）
func (s *ConsistencyService) timezoneOffset(tz1, tz2 string) int {
	offsets := map[string]int{
		"America/Los_Angeles": -8, "America/New_York": -5, "America/Chicago": -6,
		"Europe/London": 0, "Europe/Paris": 1, "Europe/Berlin": 1, "Europe/Moscow": 3,
		"Asia/Shanghai": 8, "Asia/Hong_Kong": 8, "Asia/Tokyo": 9, "Asia/Singapore": 8,
		"Asia/Kolkata": 5, "Australia/Sydney": 10,
	}
	o1, ok1 := offsets[tz1]
	o2, ok2 := offsets[tz2]
	if ok1 && ok2 {
		return o1 - o2
	}
	return 0
}

// countryLanguages 返回某国家常用语言前缀；未收录/未知返回 nil，
// 由调用方判通过（不制造假阳性），而非硬套 en 造成结构性误报
func countryLanguages(country string) []string {
	m := map[string][]string{
		"US": {"en-US", "en", "es"}, "CN": {"zh-CN", "zh"}, "JP": {"ja"},
		"GB": {"en-GB", "en"}, "DE": {"de"}, "FR": {"fr"}, "ES": {"es"},
		"IT": {"it"}, "RU": {"ru"}, "KR": {"ko"}, "BR": {"pt-BR", "pt"},
		"IN": {"en", "hi"}, "CA": {"en", "fr"}, "AU": {"en-AU", "en"},
		"SG": {"en", "zh", "ms"}, "HK": {"zh-HK", "zh", "en"}, "TW": {"zh-TW", "zh"},
		"NL": {"nl", "en"}, "VN": {"vi"}, "TH": {"th"}, "ID": {"id"},
		// 扩充无歧义的常见国家，减少「未收录即自动通过」的覆盖盲区
		"TR": {"tr"}, "PL": {"pl"}, "MX": {"es"}, "AR": {"es"}, "CL": {"es"},
		"CO": {"es"}, "PE": {"es"}, "PT": {"pt"}, "GR": {"el"}, "CZ": {"cs"},
		"SK": {"sk"}, "HU": {"hu"}, "RO": {"ro"}, "BG": {"bg"},
		"UA": {"uk", "ru"}, "SE": {"sv", "en"}, "NO": {"no", "nb", "en"},
		"DK": {"da", "en"}, "FI": {"fi", "sv", "en"}, "AT": {"de"},
		"CH": {"de", "fr", "it", "en"}, "BE": {"nl", "fr", "en"}, "IE": {"en"},
		"NZ": {"en"}, "ZA": {"en", "af"}, "MY": {"ms", "en", "zh"},
		"PH": {"en", "tl", "fil"}, "PK": {"en", "ur"}, "BD": {"bn", "en"},
		"SA": {"ar"}, "AE": {"ar", "en"}, "IL": {"he", "en"}, "EG": {"ar"},
		"MA": {"ar", "fr"}, "NG": {"en"}, "KE": {"en", "sw"},
	}
	if langs, ok := m[strings.ToUpper(country)]; ok {
		return langs
	}
	return nil
}
