package crawler

import (
	"encoding/json"
	"fmt"
	"time"
)

const gcpURL = "https://www.gstatic.com/ipranges/cloud.json"

// GCPCrawler Google Cloud IP 范围爬虫
type GCPCrawler struct{}

// GCPResponse GCP API 响应结构
type GCPResponse struct {
	SyncToken    string `json:"syncToken"`
	CreationTime string `json:"creationTime"`
	Prefixes     []struct {
		IPv4Prefix string `json:"ipv4Prefix,omitempty"`
		IPv6Prefix string `json:"ipv6Prefix,omitempty"`
		Service    string `json:"service"`
		Scope      string `json:"scope"`
	} `json:"prefixes"`
}

func (c *GCPCrawler) Name() string {
	return "gcp"
}

func (c *GCPCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(gcpURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var data GCPResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range

	for _, p := range data.Prefixes {
		var cidr string
		if p.IPv4Prefix != "" {
			cidr = p.IPv4Prefix
		} else if p.IPv6Prefix != "" {
			cidr = p.IPv6Prefix
		} else {
			continue
		}

		r, err := createRange("gcp", cidr, p.Scope, p.Service)
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
