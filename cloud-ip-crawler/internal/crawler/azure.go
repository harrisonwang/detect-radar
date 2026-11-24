package crawler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

// AzureCrawler Azure IP 范围爬虫
type AzureCrawler struct{}

// AzureResponse Azure JSON 响应结构
type AzureResponse struct {
	ChangeNumber int    `json:"changeNumber"`
	Cloud        string `json:"cloud"`
	Values       []struct {
		Name       string `json:"name"`
		ID         string `json:"id"`
		Properties struct {
			ChangeNumber    int      `json:"changeNumber"`
			Region          string   `json:"region"`
			RegionID        int      `json:"regionId"`
			Platform        string   `json:"platform"`
			SystemService   string   `json:"systemService"`
			AddressPrefixes []string `json:"addressPrefixes"`
		} `json:"properties"`
	} `json:"values"`
}

func (c *AzureCrawler) Name() string {
	return "azure"
}

func (c *AzureCrawler) Crawl() ([]Range, error) {
	// 默认走标准 TLS 校验（实测 microsoft.com / download.microsoft.com 证书链均正常）。
	// 仅在个别网络对这两个域名做了 TLS 拦截、握手失败时，
	// 才设 AZURE_SKIP_TLS_VERIFY=1 显式关闭校验（会打印告警，不做静默降级）。
	transport := &http.Transport{}
	if os.Getenv("AZURE_SKIP_TLS_VERIFY") == "1" {
		log.Printf("[azure] 警告: AZURE_SKIP_TLS_VERIFY=1，本次爬取不校验 TLS 证书")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	// 支持通过环境变量指定下载链接
	downloadURL := os.Getenv("AZURE_IP_RANGES_URL")
	var err error

	if downloadURL == "" {
		// 尝试多种方式获取下载链接
		downloadURL, err = c.extractDownloadURL(client)
		if err != nil {
			return nil, fmt.Errorf("提取下载链接失败: %w (可设置 AZURE_IP_RANGES_URL 环境变量指定下载链接)", err)
		}
	}

	// 下载 JSON 文件
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载 JSON 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	var data AzureResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range

	for _, v := range data.Values {
		region := v.Properties.Region
		if region == "" {
			region = v.Name
		}
		service := v.Properties.SystemService

		for _, prefix := range v.Properties.AddressPrefixes {
			r, err := createRange("azure", prefix, region, service)
			if err != nil {
				continue
			}
			ranges = append(ranges, *r)
		}
	}

	return ranges, nil
}

// extractDownloadURL 从 Azure 下载页面提取 JSON 文件链接
func (c *AzureCrawler) extractDownloadURL(client *http.Client) (string, error) {
	// 方法 1: 从确认页面提取下载链接
	confirmURL := "https://www.microsoft.com/en-us/download/confirmation.aspx?id=56519"

	req, err := http.NewRequest("GET", confirmURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			// 尝试多种正则模式
			patterns := []string{
				`https://download\.microsoft\.com/download/[^"'\s<>]+ServiceTags_Public[^"'\s<>]+\.json`,
				`href="(https://download\.microsoft\.com[^"]+\.json)"`,
			}

			for _, pattern := range patterns {
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(string(body))
				if len(matches) > 0 {
					url := matches[0]
					if len(matches) > 1 {
						url = matches[1]
					}
					return url, nil
				}
			}
		}
	}

	// 方法 2: 尝试猜测最新的下载链接（基于日期模式）
	// Azure 的文件名格式: ServiceTags_Public_YYYYMMDD.json
	// 基础下载路径是固定的
	baseURL := "https://download.microsoft.com/download/7/1/D/71D86715-5596-4529-9B13-DA13A5DE5B63"

	// 尝试最近几天的日期
	for i := 0; i <= 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		filename := fmt.Sprintf("ServiceTags_Public_%s.json", date.Format("20060102"))
		testURL := fmt.Sprintf("%s/%s", baseURL, filename)

		testReq, _ := http.NewRequest("HEAD", testURL, nil)
		testReq.Header.Set("User-Agent", userAgent())

		testResp, err := client.Do(testReq)
		if err == nil {
			testResp.Body.Close()
			if testResp.StatusCode == http.StatusOK {
				return testURL, nil
			}
		}
	}

	return "", fmt.Errorf("无法获取 Azure IP 范围下载链接，请手动指定")
}
