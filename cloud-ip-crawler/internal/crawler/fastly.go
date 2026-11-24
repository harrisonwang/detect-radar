package crawler

import (
	"encoding/json"
	"fmt"
	"time"
)

const fastlyURL = "https://api.fastly.com/public-ip-list"

// FastlyCrawler Fastly（CDN）IP 范围爬虫
type FastlyCrawler struct{}

// FastlyResponse Fastly 公开 IP 列表：一份扁平的 CIDR 清单，v4/v6 分列两个键
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

	// IPv4
	for _, cidr := range data.Addresses {
		r, err := createRange("fastly", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	// IPv6
	for _, cidr := range data.IPv6Addresses {
		r, err := createRange("fastly", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
