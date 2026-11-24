package crawler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Zscaler 把 CENR（Cloud Enforcement Node Ranges）按 cloud 分成多份发布，
// 每个 cloud 一个 JSON，这里覆盖主要生产 cloud。
var zscalerClouds = []string{
	"zscaler.net",
	"zscalerone.net",
	"zscalertwo.net",
	"zscalerthree.net",
	"zscloud.net",
}

const zscalerURLFmt = "https://config.zscaler.com/api/%s/cenr/json"

// ZscalerCrawler Zscaler 安全云节点 IP 爬虫
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
			// 单个 cloud 文件下架/异常不该拖垮整个数据源，记一笔继续抓下一个；
			// 只有全部 cloud 都失败（下面 ok==0）才判定 Zscaler 抓取失败。
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

	// 层级不固定（cloud → "continent : X" → "city : Y" → [ {range} ]，另有 svpnIPs 是扁平列表），
	// 递归收集所有带 range 字段的叶子对象，比硬编码嵌套结构更耐上游改版。
	var ranges []Range
	collectZscalerRanges(doc, cloud, &ranges)
	return ranges, nil
}

// collectZscalerRanges 深度遍历任意嵌套，凡是 {"range": "..."} 的对象就取出其 CIDR
func collectZscalerRanges(node any, cloud string, out *[]Range) {
	switch v := node.(type) {
	case map[string]any:
		if cidr, ok := v["range"].(string); ok && cidr != "" {
			if r, err := createRange("zscaler", cidr, "", cloud); err == nil {
				*out = append(*out, *r)
			}
			return // 叶子对象，不必再往下钻
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
