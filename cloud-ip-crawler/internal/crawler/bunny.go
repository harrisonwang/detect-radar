package crawler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// v4 和 v6 是两个端点，都返回一串单个边缘节点 IP
const (
	bunnyURL     = "https://bunnycdn.com/api/system/edgeserverlist"
	bunnyIPv6URL = "https://bunnycdn.com/api/system/edgeserverlist/ipv6"
)

// BunnyCrawler Bunny CDN 边缘节点 IP 爬虫
type BunnyCrawler struct{}

func (c *BunnyCrawler) Name() string {
	return "bunny"
}

func (c *BunnyCrawler) Crawl() ([]Range, error) {
	// Bunny 给的是一串单个边缘节点 IP（不是 CIDR），逐个当主机路由处理
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

		// 单个地址补成主机路由：v4 → /32，v6 → /128
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
