package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRadarBaseURL          = "https://api.cloudflare.com/client/v4/radar/http/summary/BOT_CLASS"
	defaultRadarCacheTTL         = 24 * time.Hour
	defaultRadarNegativeCacheTTL = 6 * time.Hour
	defaultRadarRequestTimeout   = 12 * time.Second
	defaultRadarWaitTimeout      = 3500 * time.Millisecond
	radarNegativeSentinel        = "none"
)

type radarCacheStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

type redisRadarStore struct {
	client *redis.Client
}

func (r *redisRadarStore) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *redisRadarStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

type radarInflight struct {
	done  chan struct{}
	ratio *float64
	err   error
}

type RadarService struct {
	token            string
	baseURL          string
	httpClient       *http.Client
	store            radarCacheStore
	cacheTTL         time.Duration
	negativeCacheTTL time.Duration
	requestTimeout   time.Duration
	waitTimeout      time.Duration

	inflightMu sync.Mutex
	inflight   map[string]*radarInflight
}

type RadarConfig struct {
	Token            string
	BaseURL          string
	CacheTTL         time.Duration
	NegativeCacheTTL time.Duration
	RequestTimeout   time.Duration
	WaitTimeout      time.Duration
}

func NewRadarService(cfg RadarConfig, redisClient *redis.Client) *RadarService {
	var store radarCacheStore
	if redisClient != nil {
		store = &redisRadarStore{client: redisClient}
	}
	return newRadarServiceWithStore(cfg, store)
}

func newRadarServiceWithStore(cfg RadarConfig, store radarCacheStore) *RadarService {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil
	}

	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = defaultRadarCacheTTL
	}
	negTTL := cfg.NegativeCacheTTL
	if negTTL <= 0 {
		negTTL = defaultRadarNegativeCacheTTL
	}
	reqTimeout := cfg.RequestTimeout
	if reqTimeout <= 0 {
		reqTimeout = defaultRadarRequestTimeout
	}
	waitTimeout := cfg.WaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = defaultRadarWaitTimeout
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultRadarBaseURL
	}

	return &RadarService{
		token:            strings.TrimSpace(cfg.Token),
		baseURL:          baseURL,
		httpClient:       &http.Client{Timeout: reqTimeout},
		store:            store,
		cacheTTL:         ttl,
		negativeCacheTTL: negTTL,
		requestTimeout:   reqTimeout,
		waitTimeout:      waitTimeout,
		inflight:         make(map[string]*radarInflight),
	}
}

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

func normalizeASNNumber(asn string) string {
	s := strings.TrimSpace(asn)
	if len(s) >= 2 && strings.EqualFold(s[:2], "AS") {
		s = strings.TrimSpace(s[2:])
	}
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func (s *RadarService) GetHumanRatio(ctx context.Context, asn string) (*float64, error) {
	if s == nil {
		return nil, nil
	}

	asnNum := normalizeASNNumber(asn)
	if asnNum == "" {
		return nil, nil
	}

	cacheKey := s.cacheKey(asnNum)
	if s.store != nil {
		cached, err := s.store.Get(ctx, cacheKey)
		if err == nil {
			if cached == radarNegativeSentinel {
				return nil, nil
			}
			if ratio, parseErr := strconv.ParseFloat(cached, 64); parseErr == nil {
				v := clampPercent(ratio)
				return &v, nil
			}
		}
	}

	s.inflightMu.Lock()
	call, exists := s.inflight[asnNum]
	if !exists {
		call = &radarInflight{done: make(chan struct{})}
		s.inflight[asnNum] = call
		go s.runFetch(asnNum, cacheKey, call)
	}
	s.inflightMu.Unlock()

	timer := time.NewTimer(s.waitTimeout)
	defer timer.Stop()

	select {
	case <-call.done:
		if call.err != nil || call.ratio == nil {
			return nil, call.err
		}
		v := *call.ratio
		return &v, nil
	case <-timer.C:
		log.Printf("[Radar] AS%s fetch still running after %v; returning early while warming Redis", asnNum, s.waitTimeout)
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *RadarService) runFetch(asnNum, cacheKey string, call *radarInflight) {
	defer func() {
		close(call.done)
		s.inflightMu.Lock()
		delete(s.inflight, asnNum)
		s.inflightMu.Unlock()
	}()

	bgCtx, cancel := context.WithTimeout(context.Background(), s.requestTimeout)
	defer cancel()

	ratio, err := s.fetchFromAPI(bgCtx, asnNum)
	call.ratio = ratio
	call.err = err

	if err != nil || s.store == nil {
		return
	}

	writeCtx, writeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer writeCancel()

	if ratio != nil {
		if setErr := s.store.Set(writeCtx, cacheKey, fmt.Sprintf("%.2f", *ratio), s.cacheTTL); setErr != nil {
			log.Printf("[Radar] Redis set failed for AS%s: %v", asnNum, setErr)
		}
	} else {
		if setErr := s.store.Set(writeCtx, cacheKey, radarNegativeSentinel, s.negativeCacheTTL); setErr != nil {
			log.Printf("[Radar] Redis negative cache set failed for AS%s: %v", asnNum, setErr)
		}
	}
}

func (s *RadarService) fetchFromAPI(ctx context.Context, asnNum string) (*float64, error) {
	sep := "?"
	if strings.Contains(s.baseURL, "?") {
		sep = "&"
	}
	url := fmt.Sprintf("%s%sasn=%s&dateRange=30d", s.baseURL, sep, asnNum)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "DetectRadar/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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

	humanStr := strings.TrimSpace(result.Result.Summary0.Human)
	if humanStr == "" {
		log.Printf("[Radar] AS%s has no 30d BOT_CLASS data", asnNum)
		return nil, nil
	}

	ratio, err := strconv.ParseFloat(humanStr, 64)
	if err != nil {
		return nil, err
	}

	clamped := clampPercent(ratio)
	log.Printf("[Radar] AS%s 30d human ratio: %.2f%%", asnNum, clamped)
	return &clamped, nil
}

func (s *RadarService) cacheKey(asnNum string) string {
	return "radar:asn:" + asnNum
}

func (s *RadarService) IsEnabled() bool {
	return s != nil && s.token != ""
}
