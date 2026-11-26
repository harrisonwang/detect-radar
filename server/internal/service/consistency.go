package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"detect-radar/internal/model"
)

var tzOffsetRe = regexp.MustCompile(`^([+-])(\d{2}):?(\d{2})$`)

type ConsistencyService struct{}

func NewConsistencyService() *ConsistencyService {
	return &ConsistencyService{}
}

func (s *ConsistencyService) Evaluate(browser model.BrowserInfo, intel *model.IPIntel) model.ConsistencyChecks {
	return model.ConsistencyChecks{
		Timezone: s.checkTimezone(browser.Timezone, intel.Timezone),
		Language: s.checkLanguage(browser.Language, browser.Languages, geoBucket(intel)),
	}
}

func (s *ConsistencyService) checkTimezone(browserTZ, ipTZ string) model.TimezoneCheck {
	if ipTZ == "" {
		return model.TimezoneCheck{Passed: true, Browser: browserTZ, Expected: "unknown"}
	}
	if browserTZ == ipTZ {
		return model.TimezoneCheck{Passed: true, Browser: browserTZ, Expected: ipTZ}
	}

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

	return model.TimezoneCheck{
		Passed:   false,
		Browser:  browserTZ,
		Expected: ipTZ,
	}
}

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
		check.Passed = true
		return check
	}

	candidates := append([]string{browserLang}, browserLangs...)
	for _, lang := range candidates {
		ll := strings.ToLower(strings.TrimSpace(lang))
		for _, exp := range expected {
			el := strings.ToLower(exp)
			if ll == el || strings.HasPrefix(ll, el+"-") {
				check.Passed = true
				return check
			}
		}
	}
	check.Passed = false
	return check
}

func countryLanguages(country string) []string {
	m := map[string][]string{
		"US": {"en-US", "en", "es"}, "CN": {"zh-CN", "zh"}, "JP": {"ja"},
		"GB": {"en-GB", "en", "cy"}, "DE": {"de"}, "FR": {"fr"}, "ES": {"es", "ca", "eu", "gl"},
		"IT": {"it"}, "RU": {"ru"}, "KR": {"ko"}, "BR": {"pt-BR", "pt"},
		"IN": {"en", "hi"}, "CA": {"en", "fr"}, "AU": {"en-AU", "en"},
		"SG": {"en", "zh", "ms", "ta"}, "HK": {"zh-HK", "zh", "en"}, "MO": {"zh-MO", "zh", "pt"}, "TW": {"zh-TW", "zh"},
		"NL": {"nl", "en"}, "VN": {"vi"}, "TH": {"th"}, "ID": {"id"},
		"TR": {"tr"}, "PL": {"pl"}, "MX": {"es"}, "AR": {"es"}, "CL": {"es"},
		"CO": {"es"}, "PE": {"es"}, "PT": {"pt"}, "GR": {"el"}, "CZ": {"cs"},
		"SK": {"sk"}, "HU": {"hu"}, "RO": {"ro"}, "BG": {"bg"},
		"UA": {"uk", "ru"}, "SE": {"sv", "en"}, "NO": {"no", "nb", "en"},
		"DK": {"da", "en"}, "FI": {"fi", "sv", "en"}, "AT": {"de"},
		"CH": {"de", "fr", "it", "en"}, "BE": {"nl", "fr", "de", "en"}, "IE": {"en", "ga"},
		"NZ": {"en", "mi"}, "ZA": {"en", "af"}, "MY": {"ms", "en", "zh"},
		"PH": {"en", "tl", "fil"}, "PK": {"en", "ur"}, "BD": {"bn", "en"},
		"SA": {"ar"}, "AE": {"ar", "en"}, "IL": {"he", "en"}, "EG": {"ar"},
		"MA": {"ar", "fr"}, "NG": {"en"}, "KE": {"en", "sw"},
	}
	if langs, ok := m[strings.ToUpper(country)]; ok {
		return langs
	}
	return nil
}
