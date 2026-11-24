package crawler

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	cloudflareIPv4URL = "https://www.cloudflare.com/ips-v4"
	cloudflareIPv6URL = "https://www.cloudflare.com/ips-v6"
)

// CloudflareCrawler Cloudflare IP 范围爬虫
type CloudflareCrawler struct{}

func (c *CloudflareCrawler) Name() string {
	return "cloudflare"
}

func (c *CloudflareCrawler) Crawl() ([]Range, error) {
	var ranges []Range

	// IPv4
	ipv4Ranges, err := c.fetchTextList(cloudflareIPv4URL)
	if err != nil {
		return nil, fmt.Errorf("获取 IPv4 失败: %w", err)
	}
	ranges = append(ranges, ipv4Ranges...)

	// IPv6
	ipv6Ranges, err := c.fetchTextList(cloudflareIPv6URL)
	if err != nil {
		return nil, fmt.Errorf("获取 IPv6 失败: %w", err)
	}
	ranges = append(ranges, ipv6Ranges...)

	return ranges, nil
}

func (c *CloudflareCrawler) fetchTextList(url string) ([]Range, error) {
	body, err := fetchBytes(url, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var ranges []Range
	scanner := bufio.NewScanner(bytes.NewReader(body))

	for scanner.Scan() {
		cidr := strings.TrimSpace(scanner.Text())
		if cidr == "" || strings.HasPrefix(cidr, "#") {
			continue
		}

		r, err := createRange("cloudflare", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
