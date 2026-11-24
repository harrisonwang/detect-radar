package crawler

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchBytes 是所有爬虫取数的统一入口：带 User-Agent、超时和轻量重试。
// 每日构建里任一数据源失败当天就不发布，所以对偶发抖动（网络错误、5xx、429）
// 重试三次是划算的；4xx 这类确定性错误则立刻返回，不浪费时间。
func fetchBytes(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}

		body, retryable, err := fetchOnce(client, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func fetchOnce(client *http.Client, url string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("读取响应失败: %w", err)
	}
	return body, false, nil
}
