package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"detect-radar/internal/adapter"
	"detect-radar/internal/model"
)

type fakeRadarStore struct {
	mu   sync.Mutex
	data map[string]string
	ttls map[string]time.Duration
}

func newFakeRadarStore() *fakeRadarStore {
	return &fakeRadarStore{
		data: make(map[string]string),
		ttls: make(map[string]time.Duration),
	}
}

func (f *fakeRadarStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (f *fakeRadarStore) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	f.ttls[key] = ttl
	return nil
}

func (f *fakeRadarStore) getValue(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	return v, ok
}

func TestRadarService_FetchAndCacheRedis(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		if got := r.URL.Query().Get("asn"); got != "7922" {
			t.Errorf("asn query = %q, want 7922", got)
		}
		if got := r.URL.Query().Get("dateRange"); got != "30d" {
			t.Errorf("dateRange = %q, want 30d", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"summary_0":{"human":"81.530843","bot":"18.469157"}}}`))
	}))
	defer srv.Close()

	store := newFakeRadarStore()
	svc := newRadarServiceWithStore(RadarConfig{
		Token:    "test-token",
		BaseURL:  srv.URL,
		CacheTTL: 24 * time.Hour,
	}, store)

	ratio, err := svc.GetHumanRatio(context.Background(), "AS7922")
	if err != nil {
		t.Fatalf("GetHumanRatio error: %v", err)
	}
	if ratio == nil || *ratio < 81.5 || *ratio > 81.6 {
		t.Fatalf("ratio = %v, want ~81.53", ratio)
	}
	if v, ok := store.getValue("radar:asn:7922"); !ok || v != "81.53" {
		t.Fatalf("cached value = %q (ok=%v), want 81.53", v, ok)
	}

	ratio2, err := svc.GetHumanRatio(context.Background(), "7922")
	if err != nil {
		t.Fatalf("second GetHumanRatio error: %v", err)
	}
	if ratio2 == nil || *ratio2 != 81.53 {
		t.Fatalf("ratio2 = %v, want 81.53", ratio2)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits.Load())
	}
}

func TestRadarService_NegativeCacheEmptySummary(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"summary_0":{}}}`))
	}))
	defer srv.Close()

	store := newFakeRadarStore()
	svc := newRadarServiceWithStore(RadarConfig{
		Token:            "test-token",
		BaseURL:          srv.URL,
		NegativeCacheTTL: 6 * time.Hour,
	}, store)

	ratio, err := svc.GetHumanRatio(context.Background(), "AS13335")
	if err != nil || ratio != nil {
		t.Fatalf("first call = (%v, %v), want (nil, nil)", ratio, err)
	}
	if v, ok := store.getValue("radar:asn:13335"); !ok || v != radarNegativeSentinel {
		t.Fatalf("negative cache = %q (ok=%v), want %q", v, ok, radarNegativeSentinel)
	}

	ratio2, err := svc.GetHumanRatio(context.Background(), "AS13335")
	if err != nil || ratio2 != nil {
		t.Fatalf("second call = (%v, %v), want (nil, nil)", ratio2, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1 (negative cached)", hits.Load())
	}
}

func TestRadarService_SlowUpstreamWarmsCacheWithoutBlocking(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(180 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"summary_0":{"human":"5.48","bot":"94.52"}}}`))
	}))
	defer srv.Close()

	store := newFakeRadarStore()
	svc := newRadarServiceWithStore(RadarConfig{
		Token:          "test-token",
		BaseURL:        srv.URL,
		RequestTimeout: 2 * time.Second,
		WaitTimeout:    40 * time.Millisecond,
	}, store)

	start := time.Now()
	ratio, err := svc.GetHumanRatio(context.Background(), "AS16509")
	elapsed := time.Since(start)
	if err != nil || ratio != nil {
		t.Fatalf("early return = (%v, %v), want (nil, nil)", ratio, err)
	}
	if elapsed > 120*time.Millisecond {
		t.Fatalf("wait took %v, expected prompt early return around 40ms", elapsed)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := store.getValue("radar:asn:16509"); ok {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	ratio2, err := svc.GetHumanRatio(context.Background(), "AS16509")
	if err != nil || ratio2 == nil || *ratio2 != 5.48 {
		t.Fatalf("warmed hit = (%v, %v), want 5.48", ratio2, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits.Load())
	}
}

func TestRadarService_ConcurrentSingleflight(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"summary_0":{"human":"39.25","bot":"60.75"}}}`))
	}))
	defer srv.Close()

	store := newFakeRadarStore()
	svc := newRadarServiceWithStore(RadarConfig{
		Token:   "test-token",
		BaseURL: srv.URL,
	}, store)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := svc.GetHumanRatio(context.Background(), "AS4134")
			if err != nil || r == nil || *r < 39.2 || *r > 39.3 {
				t.Errorf("unexpected result: (%v, %v)", r, err)
			}
		}()
	}
	wg.Wait()

	if hits.Load() != 1 {
		t.Fatalf("concurrent hits = %d, want 1", hits.Load())
	}
}

func TestIPIntelService_CacheHitBackfillsHumanRatio(t *testing.T) {
	store := newFakeRadarStore()
	_ = store.Set(context.Background(), "radar:asn:7922", "81.53", time.Hour)
	radarSvc := newRadarServiceWithStore(RadarConfig{Token: "test-token"}, store)

	intelCache := newFakeIntelCache()
	_ = intelCache.SetDeep(context.Background(), "73.1.2.3", &model.IPIntel{
		IP:        "73.1.2.3",
		Country:   "US",
		ASN:       "AS7922",
		Org:       "Comcast Cable",
		UsageType: "isp",
		Source:    "ipapiis",
		Tier:      "L1+L3",
	})

	svc := NewIPIntelService(adapter.NewRegistry(), nil, nil, radarSvc, nil)
	svc.cache = intelCache

	got, err := svc.Lookup(context.Background(), "73.1.2.3", LookupOptions{DeepScan: true})
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if got.HumanRatio == nil || *got.HumanRatio != 81.53 {
		t.Fatalf("HumanRatio = %v, want 81.53", got.HumanRatio)
	}
	reloaded, _ := intelCache.GetDeep(context.Background(), "73.1.2.3")
	if reloaded == nil || reloaded.HumanRatio == nil || *reloaded.HumanRatio != 81.53 {
		t.Fatalf("deep cache not backfilled: %+v", reloaded)
	}
}
