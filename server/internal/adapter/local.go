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

type ASNRecord struct {
	ASNumber uint64 `maxminddb:"autonomous_system_number"`
	ASOrg    string `maxminddb:"autonomous_system_organization"`
}

type LocalAdapter struct {
	cityDB *maxminddb.Reader
	asnDB  *maxminddb.Reader
}

func NewLocalAdapter(cityPath, asnPath string) (*LocalAdapter, error) {
	var cityDB, asnDB *maxminddb.Reader
	var err error

	if cityPath != "" {
		cityDB, err = maxminddb.Open(cityPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open city database: %w", err)
		}
		log.Printf("[LocalAdapter] 已加载城市数据库: %s", cityPath)
	}

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

func (a *LocalAdapter) Name() string { return "local" }

func (a *LocalAdapter) Tier() string { return "L1" }

func (a *LocalAdapter) Capabilities() Capabilities {
	return Capabilities{
		HasGeo:       a.cityDB != nil,
		HasASN:       a.asnDB != nil,
		HasTimezone:  a.cityDB != nil,
		HasPrivacy:   false,
		HasUsageType: false,
		HasRiskScore: false,
	}
}

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

	if a.cityDB != nil {
		if err := a.lookupCity(parsedIP, intel); err != nil {
			log.Printf("[LocalAdapter] 城市数据库查询失败 IP=%s: %v", ip, err)
		}
	}

	if a.asnDB != nil {
		if err := a.lookupASN(parsedIP, intel); err != nil {
			log.Printf("[LocalAdapter] ASN 数据库查询失败 IP=%s: %v", ip, err)
		}
	}

	a.inferProperties(intel)

	return intel, nil
}

func (a *LocalAdapter) lookupCity(ip net.IP, intel *model.IPIntel) error {
	var record CityRecord
	err := a.cityDB.Lookup(ip, &record)
	if err != nil {
		return err
	}

	intel.Country = record.Country.IsoCode
	intel.CountryName = getLocalizedName(record.Country.Names, "zh-CN", "en")

	intel.City = getLocalizedName(record.City.Names, "zh-CN", "en")

	if len(record.Subdivisions) > 0 {
		intel.Region = getLocalizedName(record.Subdivisions[0].Names, "zh-CN", "en")
	}

	intel.Latitude = record.Location.Latitude
	intel.Longitude = record.Location.Longitude
	intel.Timezone = record.Location.TimeZone
	intel.Postal = record.Postal.Code

	return nil
}

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

func (a *LocalAdapter) inferProperties(intel *model.IPIntel) {
	_ = intel
}

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
