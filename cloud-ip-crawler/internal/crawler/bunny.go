package crawler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	bunnyURL     = "https://bunnycdn.com/api/system/edgeserverlist"
	bunnyIPv6URL = "https://bunnycdn.com/api/system/edgeserverlist/ipv6"
)

type BunnyCrawler struct{}

func (c *BunnyCrawler) Name() string {
	return "bunny"
}

func (c *BunnyCrawler) Crawl() ([]Range, error) {
	var ips []string
	for _, url := range []string{bunnyURL, bunnyIPv6URL} {
		body, err := fetchBytes(url, 30*time.Second)
		if err != nil {
			return nil, err
		}
		var list []string
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("解析 JSON 失败: %w", err)
		}
		ips = append(ips, list...)
	}

	var ranges []Range
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		cidr := ip
		if !strings.Contains(cidr, "/") {
			if strings.Contains(ip, ":") {
				cidr = ip + "/128"
			} else {
				cidr = ip + "/32"
			}
		}

		r, err := createRange("bunny", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
