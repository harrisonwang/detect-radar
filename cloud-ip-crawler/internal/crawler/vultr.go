package crawler

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

const vultrURL = "https://geofeed.constant.com/?list-format=csv"

// VultrCrawler Vultr IP 范围爬虫
type VultrCrawler struct{}

func (c *VultrCrawler) Name() string {
	return "vultr"
}

func (c *VultrCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(vultrURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var ranges []Range
	scanner := bufio.NewScanner(bytes.NewReader(body))

	// Geofeed CSV 格式: ip_prefix,country_code,region_code,city,postal_code
	// 例如: 108.61.0.0/18,US,US-NJ,Piscataway,08854
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

		r, err := createRange("vultr", cidr, region, "")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
