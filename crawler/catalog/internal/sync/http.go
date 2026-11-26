package sync

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var Version = "dev"

func DefaultUserAgent() string {
	return "DetectRadar-CatalogCrawler/" + Version + " (+https://github.com/harrisonwang/detect-radar)"
}

type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

func Fetch(url string, timeout time.Duration, headers map[string]string) (*HTTPResponse, error) {
	client := &http.Client{Timeout: timeout}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}
		resp, retryable, err := fetchOnce(client, url, headers)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func fetchOnce(client *http.Client, url string, headers map[string]string) (*HTTPResponse, bool, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent())
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &HTTPResponse{StatusCode: resp.StatusCode, Headers: resp.Header.Clone()}, false, nil
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("读取响应失败: %w", err)
	}
	return &HTTPResponse{StatusCode: resp.StatusCode, Body: body, Headers: resp.Header.Clone()}, false, nil
}
