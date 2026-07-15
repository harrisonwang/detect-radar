package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RadarService Cloudflare Radar API 服务
// 用于获取 ASN 级别的真实用户占比数据
type RadarService struct {
	token      string
	httpClient *http.Client
	redis      *redis.Client
	cacheTTL   time.Duration
}

// RadarConfig Radar 服务配置
type RadarConfig struct {
	Token    string
	CacheTTL time.Duration
}

// NewRadarService 创建 Radar 服务
func NewRadarService(cfg RadarConfig, redisClient *redis.Client) *RadarService {
	if cfg.Token == "" {
		return nil
	}

	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = 24 * time.Hour // 默认 24 小时缓存
	}

	return &RadarService{
		token: cfg.Token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		redis:    redisClient,
		cacheTTL: ttl,
	}
}

// cloudflareRadarResponse Cloudflare Radar API 响应结构
type cloudflareRadarResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		Summary0 struct {
			Human string `json:"human"`
			Bot   string `json:"bot"`
		} `json:"summary_0"`
	} `json:"result"`
}

// GetHumanRatio 获取 ASN 的真实用户占比
// 返回 0-100 的百分比，nil 表示无法获取
func (s *RadarService) GetHumanRatio(ctx context.Context, asn string) (*float64, error) {
	if s == nil {
		return nil, nil
	}

	// 规范化 ASN 格式（去掉 "AS" 前缀）
	asnNum := strings.TrimPrefix(strings.ToUpper(asn), "AS")
	if asnNum == "" {
		return nil, nil
	}

	// 1. 查缓存
	cacheKey := s.cacheKey(asnNum)
	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			ratio, err := strconv.ParseFloat(cached, 64)
			if err == nil {
				return &ratio, nil
			}
		}
	}

	// 2. 调用 Cloudflare Radar API
	ratio, err := s.fetchFromAPI(ctx, asnNum)
	if err != nil {
		log.Printf("[Radar] API error for ASN %s: %v", asnNum, err)
		return nil, err
	}

	// 3. 写缓存
	if s.redis != nil && ratio != nil {
		s.redis.Set(ctx, cacheKey, fmt.Sprintf("%.2f", *ratio), s.cacheTTL)
	}

	return ratio, nil
}

// fetchFromAPI 从 Cloudflare Radar API 获取数据
func (s *RadarService) fetchFromAPI(ctx context.Context, asnNum string) (*float64, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/radar/http/summary/BOT_CLASS?asn=%s&dateRange=30d", asnNum)
	log.Printf("[Radar] Fetching from API: %s", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取完整响应体用于调试
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("[Radar] API response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result cloudflareRadarResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf("API error: %s", result.Errors[0].Message)
		}
		return nil, fmt.Errorf("API returned success=false")
	}

	// 解析 human 占比
	humanStr := result.Result.Summary0.Human
	log.Printf("[Radar] Parsed human ratio for ASN %s: %s", asnNum, humanStr)
	if humanStr == "" {
		return nil, nil
	}

	ratio, err := strconv.ParseFloat(humanStr, 64)
	if err != nil {
		return nil, err
	}

	return &ratio, nil
}

// cacheKey 生成缓存键
func (s *RadarService) cacheKey(asnNum string) string {
	return "radar:asn:" + asnNum
}

// IsEnabled 是否启用了 Radar 服务
func (s *RadarService) IsEnabled() bool {
	return s != nil && s.token != ""
}
