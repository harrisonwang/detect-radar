package crawler

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

const digitaloceanURL = "https://digitalocean.com/geo/google.csv"

type DigitalOceanCrawler struct{}

func (c *DigitalOceanCrawler) Name() string {
	return "digitalocean"
}

func (c *DigitalOceanCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(digitaloceanURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var ranges []Range
	scanner := bufio.NewScanner(bytes.NewReader(body))

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
		if cidr == "" {
			continue
		}

		region := ""
		if len(parts) >= 4 {
			country := strings.TrimSpace(parts[1])
			state := strings.TrimSpace(parts[2])
			city := strings.TrimSpace(parts[3])
			if city != "" {
				region = fmt.Sprintf("%s-%s-%s", country, state, city)
			} else if state != "" {
				region = fmt.Sprintf("%s-%s", country, state)
			} else {
				region = country
			}
		}

		r, err := createRange("digitalocean", cidr, region, "")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
