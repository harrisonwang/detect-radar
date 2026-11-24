package crawler

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

const linodeURL = "https://geoip.linode.com/"

// LinodeCrawler Linode IP 范围爬虫
type LinodeCrawler struct{}

func (c *LinodeCrawler) Name() string {
	return "linode"
}

func (c *LinodeCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(linodeURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var ranges []Range
	scanner := bufio.NewScanner(bytes.NewReader(body))

	// Geofeed CSV 格式: ip_prefix,country_code,region_code,city,postal_code
	// 例如: 45.33.32.0/24,US,US-CA,Fremont,
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 1 {
			continue
		}

		cidr := strings.TrimSpace(parts[0])
		if cidr == "" || !strings.Contains(cidr, "/") {
			continue
		}

		region := ""
		if len(parts) >= 4 {
			country := strings.TrimSpace(parts[1])
			regionCode := strings.TrimSpace(parts[2])
			city := strings.TrimSpace(parts[3])
			if city != "" {
				region = fmt.Sprintf("%s-%s", regionCode, city)
			} else if regionCode != "" {
				region = regionCode
			} else {
				region = country
			}
		}

		r, err := createRange("linode", cidr, region, "")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
