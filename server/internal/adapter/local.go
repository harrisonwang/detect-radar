package adapter

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"detect-radar/internal/model"
)

// CityRecord 通用城市记录结构
// 兼容 MaxMind GeoLite2、DB-IP、IP2Location 三种 MMDB 数据库
type CityRecord struct {
	City struct {
		GeonameID uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`

	Continent struct {
		Code      string            `maxminddb:"code"`
		GeonameID uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"continent"`

	Country struct {
		GeonameID         uint              `maxminddb:"geoname_id"`
		IsoCode           string            `maxminddb:"iso_code"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		Names             map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`

	Location struct {
		AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
		Latitude       float64 `maxminddb:"latitude"`
		Longitude      float64 `maxminddb:"longitude"`
		TimeZone       string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`

	Postal struct {
		Code string `maxminddb:"code"`
	} `maxminddb:"postal"`

	RegisteredCountry struct {
		GeonameID uint              `maxminddb:"geoname_id"`
		IsoCode   string            `maxminddb:"iso_code"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"registered_country"`

	Subdivisions []struct {
		GeonameID uint              `maxminddb:"geoname_id"`
		IsoCode   string            `maxminddb:"iso_code"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

// ASNRecord ASN 记录结构
// 三种数据库（MaxMind、DB-IP、IP2Location）结构完全一致
type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

// LocalAdapter 本地 MMDB 适配器 (L1)
// 支持 MaxMind GeoLite2、DB-IP Lite、IP2Location 等 MMDB 格式数据库
type LocalAdapter struct {
	cityDB *maxminddb.Reader
	asnDB  *maxminddb.Reader
}

// NewLocalAdapter 创建本地适配器
// cityPath: 城市数据库路径 (支持 MaxMind GeoLite2-City、DB-IP City、IP2Location DB11)
// asnPath: ASN 数据库路径 (支持 MaxMind GeoLite2-ASN、DB-IP ASN、IP2Location ASN)
func NewLocalAdapter(cityPath, asnPath string) (*LocalAdapter, error) {
	var cityDB, asnDB *maxminddb.Reader
	var err error

	// 加载城市数据库（必需）
	if cityPath != "" {
		cityDB, err = maxminddb.Open(cityPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open city database: %w", err)
		}
		log.Printf("[LocalAdapter] 已加载城市数据库: %s", cityPath)
	}

	// 加载 ASN 数据库（可选）
	if asnPath != "" {
		asnDB, err = maxminddb.Open(asnPath)
		if err != nil {
			if cityDB != nil {
				cityDB.Close()
			}
			return nil, fmt.Errorf("failed to open ASN database: %w", err)
		}
		log.Printf("[LocalAdapter] 已加载 ASN 数据库: %s", asnPath)
	}

	if cityDB == nil && asnDB == nil {
		return nil, fmt.Errorf("at least one database (city or ASN) is required")
	}

	return &LocalAdapter{
		cityDB: cityDB,
		asnDB:  asnDB,
	}, nil
}

// Name 返回适配器名称
func (a *LocalAdapter) Name() string { return "local" }

// Tier 返回层级
func (a *LocalAdapter) Tier() string { return "L1" }

// Capabilities 返回适配器能力
func (a *LocalAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       a.cityDB != nil,
		HasASN:       a.asnDB != nil,
		HasTimezone:  a.cityDB != nil,
		HasPrivacy:   false, // 本地库无法检测 VPN/Proxy
		HasUsageType: false, // 需要结合 ASN 启发式判断
		HasRiskScore: false,
	}
}

// Lookup 查询 IP 信息
func (a *LocalAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	intel := &model.IPIntel{
		IP:        ip,
		Source:    "local",
		Tier:      "L1",
		FetchedAt: time.Now(),
	}

	// 查询城市数据库
	if a.cityDB != nil {
		if err := a.lookupCity(parsedIP, intel); err != nil {
			log.Printf("[LocalAdapter] 城市数据库查询失败 IP=%s: %v", ip, err)
		}
	}

	// 查询 ASN 数据库
	if a.asnDB != nil {
		if err := a.lookupASN(parsedIP, intel); err != nil {
			log.Printf("[LocalAdapter] ASN 数据库查询失败 IP=%s: %v", ip, err)
		}
	}

	// 启发式推断
	a.inferProperties(intel)

	return intel, nil
}

// lookupCity 查询城市信息
// 使用通用结构体，兼容 MaxMind、DB-IP、IP2Location 三种数据库
func (a *LocalAdapter) lookupCity(ip net.IP, intel *model.IPIntel) error {
	var record CityRecord
	err := a.cityDB.Lookup(ip, &record)
	if err != nil {
		return err
	}

	// 国家（所有数据库都支持）
	intel.Country = record.Country.IsoCode
	intel.CountryName = getLocalizedName(record.Country.Names, "zh-CN", "en")

	// 城市
	intel.City = getLocalizedName(record.City.Names, "zh-CN", "en")

	// 省/州
	if len(record.Subdivisions) > 0 {
		intel.Region = getLocalizedName(record.Subdivisions[0].Names, "zh-CN", "en")
	}

	// 位置信息
	intel.Latitude = record.Location.Latitude
	intel.Longitude = record.Location.Longitude
	intel.Timezone = record.Location.TimeZone // DB-IP 可能为空
	intel.Postal = record.Postal.Code         // DB-IP 可能为空

	return nil
}

// getLocalizedName 获取本地化名称，按优先级尝试多种语言
func getLocalizedName(names map[string]string, preferredLangs ...string) string {
	if names == nil {
		return ""
	}
	for _, lang := range preferredLangs {
		if name, ok := names[lang]; ok && name != "" {
			return name
		}
	}
	return ""
}

// lookupASN 查询 ASN 信息
// 三种数据库的 ASN 结构完全一致
func (a *LocalAdapter) lookupASN(ip net.IP, intel *model.IPIntel) error {
	var record ASNRecord
	err := a.asnDB.Lookup(ip, &record)
	if err != nil {
		return err
	}

	intel.ASN = fmt.Sprintf("AS%d", record.ASNumber)
	intel.Org = record.ASOrg

	return nil
}

// inferProperties 本地适配器不做任何风险推断
// 原因：无法仅通过 ASN 组织名称准确判断 ISP/hosting/mobile
// 这些字段应该由：
//   - HostingDetector（SQLite 数据库）判断 is_hosting
//   - 远程 API（ipapi.is/BigDataCloud/IP2Location）判断 is_mobile, usage_type
//
// 宁可返回 unknown 也不能错判
func (a *LocalAdapter) inferProperties(intel *model.IPIntel) {
	// 本地不做任何推断，所有风险字段保持零值
	// is_hosting, is_mobile, usage_type 等由后续流程填充
	_ = intel // 保留函数签名，便于将来扩展
}

// Close 关闭数据库连接
func (a *LocalAdapter) Close() error {
	var errs []error

	if a.cityDB != nil {
		if err := a.cityDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.asnDB != nil {
		if err := a.asnDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}

	return nil
}
