package crawler

import (
	"encoding/json"
	"fmt"
	"time"
)

const awsURL = "https://ip-ranges.amazonaws.com/ip-ranges.json"

// AWSCrawler AWS IP 范围爬虫
type AWSCrawler struct{}

// AWSResponse AWS API 响应结构
type AWSResponse struct {
	SyncToken  string `json:"syncToken"`
	CreateDate string `json:"createDate"`
	Prefixes   []struct {
		IPPrefix           string `json:"ip_prefix"`
		Region             string `json:"region"`
		Service            string `json:"service"`
		NetworkBorderGroup string `json:"network_border_group"`
	} `json:"prefixes"`
	IPv6Prefixes []struct {
		IPv6Prefix         string `json:"ipv6_prefix"`
		Region             string `json:"region"`
		Service            string `json:"service"`
		NetworkBorderGroup string `json:"network_border_group"`
	} `json:"ipv6_prefixes"`
}

func (c *AWSCrawler) Name() string {
	return "aws"
}

func (c *AWSCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(awsURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var data AWSResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range

	// IPv4
	for _, p := range data.Prefixes {
		r, err := createRange("aws", p.IPPrefix, p.Region, p.Service)
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	// IPv6
	for _, p := range data.IPv6Prefixes {
		r, err := createRange("aws", p.IPv6Prefix, p.Region, p.Service)
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
