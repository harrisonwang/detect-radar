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

type AzureCrawler struct{}

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
	transport := &http.Transport{}
	if os.Getenv("AZURE_SKIP_TLS_VERIFY") == "1" {
		log.Printf("[azure] 警告: AZURE_SKIP_TLS_VERIFY=1，本次爬取不校验 TLS 证书")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	downloadURL := os.Getenv("AZURE_IP_RANGES_URL")
	var err error

	if downloadURL == "" {
		downloadURL, err = c.extractDownloadURL(client)
		if err != nil {
			return nil, fmt.Errorf("提取下载链接失败: %w (可设置 AZURE_IP_RANGES_URL 环境变量指定下载链接)", err)
		}
	}

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

func (c *AzureCrawler) extractDownloadURL(client *http.Client) (string, error) {
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

	baseURL := "https://download.microsoft.com/download/7/1/D/71D86715-5596-4529-9B13-DA13A5DE5B63"

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
