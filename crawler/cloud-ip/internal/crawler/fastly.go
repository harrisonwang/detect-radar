package crawler

import (
	"encoding/json"
	"fmt"
	"time"
)

const fastlyURL = "https://api.fastly.com/public-ip-list"

type FastlyCrawler struct{}

type FastlyResponse struct {
	Addresses     []string `json:"addresses"`
	IPv6Addresses []string `json:"ipv6_addresses"`
}

func (c *FastlyCrawler) Name() string {
	return "fastly"
}

func (c *FastlyCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(fastlyURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var data FastlyResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range

	for _, cidr := range data.Addresses {
		r, err := createRange("fastly", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	for _, cidr := range data.IPv6Addresses {
		r, err := createRange("fastly", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
