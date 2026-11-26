package crawler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

var zscalerClouds = []string{
	"zscaler.net",
	"zscalerone.net",
	"zscalertwo.net",
	"zscalerthree.net",
	"zscloud.net",
}

const zscalerURLFmt = "https://config.zscaler.com/api/%s/cenr/json"

type ZscalerCrawler struct{}

func (c *ZscalerCrawler) Name() string {
	return "zscaler"
}

func (c *ZscalerCrawler) Crawl() ([]Range, error) {
	var ranges []Range
	ok := 0
	for _, cloud := range zscalerClouds {
		rs, err := c.fetchCloud(cloud)
		if err != nil {
			log.Printf("zscaler %s 抓取失败: %v", cloud, err)
			continue
		}
		ranges = append(ranges, rs...)
		ok++
	}
	if ok == 0 {
		return nil, fmt.Errorf("全部 %d 个 Zscaler cloud 均抓取失败", len(zscalerClouds))
	}

	return ranges, nil
}

func (c *ZscalerCrawler) fetchCloud(cloud string) ([]Range, error) {
	body, err := fetchBytes(fmt.Sprintf(zscalerURLFmt, cloud), 30*time.Second)
	if err != nil {
		return nil, err
	}

	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range
	collectZscalerRanges(doc, cloud, &ranges)
	return ranges, nil
}

func collectZscalerRanges(node any, cloud string, out *[]Range) {
	switch v := node.(type) {
	case map[string]any:
		if cidr, ok := v["range"].(string); ok && cidr != "" {
			if r, err := createRange("zscaler", cidr, "", cloud); err == nil {
				*out = append(*out, *r)
			}
			return
		}
		for _, child := range v {
			collectZscalerRanges(child, cloud, out)
		}
	case []any:
		for _, child := range v {
			collectZscalerRanges(child, cloud, out)
		}
	}
}
