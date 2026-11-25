package service

import (
	"sync"
	"time"
)

const (
	rttProbeTTL        = 5 * time.Minute
	rttProbeMaxEntries = 4096
	rttProbeMaxSamples = 16
	rttProbeMinIDLen   = 8
	rttProbeMaxIDLen   = 64
)

type rttProbeEntry struct {
	minMS     float64
	count     int
	createdAt time.Time
}

// RTTProbeStore 临时归集同一探测窗口内 GET /api/v1/ping 上的 X-TCP-RTT（µs）。
// 解决旧口径拿 /ping min 减 POST /scans srtt 的错窗偏差；自身严格限制 ID 字符集、
// 单 ID 采样数、总条目数与 TTL，消费即删、不落库，防止 /ping 豁免限流后成为内存放大器。
type RTTProbeStore struct {
	ttl        time.Duration
	maxEntries int
	maxSamples int
	nowFn      func() time.Time

	mu        sync.Mutex
	entries   map[string]rttProbeEntry
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewRTTProbeStore 创建生产环境使用的 RTTProbeStore（5 分钟 TTL、最多 4096 条、每 ID 最多 16 次采样）。
func NewRTTProbeStore() *RTTProbeStore {
	return newRTTProbeStore(rttProbeTTL, rttProbeMaxEntries, rttProbeMaxSamples, 1*time.Minute, time.Now)
}

func newRTTProbeStore(ttl time.Duration, maxEntries, maxSamples int, cleanupInterval time.Duration, nowFn func() time.Time) *RTTProbeStore {
	if nowFn == nil {
		nowFn = time.Now
	}
	s := &RTTProbeStore{
		ttl:        ttl,
		maxEntries: maxEntries,
		maxSamples: maxSamples,
		nowFn:      nowFn,
		entries:    make(map[string]rttProbeEntry),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	if cleanupInterval > 0 {
		go s.cleanup(cleanupInterval)
	} else {
		close(s.done)
	}
	return s
}

func isValidProbeID(id string) bool {
	if len(id) < rttProbeMinIDLen || len(id) > rttProbeMaxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (s *RTTProbeStore) isExpired(e rttProbeEntry, now time.Time) bool {
	return s.ttl > 0 && now.Sub(e.createdAt) >= s.ttl
}

// Record 记录单次 /ping 上的 X-TCP-RTT；probe_id 非法、头缺失/非法或超出采样上限时直接忽略。
func (s *RTTProbeStore) Record(probeID, tcpRTTHeader string) bool {
	if s == nil || !isValidProbeID(probeID) {
		return false
	}
	ms, ok := parseTCPInfoMS(tcpRTTHeader)
	if !ok {
		return false
	}

	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.entries[probeID]; exists {
		if !s.isExpired(entry, now) {
			if s.maxSamples > 0 && entry.count >= s.maxSamples {
				return false
			}
			if ms < entry.minMS {
				entry.minMS = ms
			}
			entry.count++
			s.entries[probeID] = entry
			return true
		}
		delete(s.entries, probeID)
	}

	if s.maxEntries > 0 && len(s.entries) >= s.maxEntries {
		for k, e := range s.entries {
			if s.isExpired(e, now) {
				delete(s.entries, k)
			}
		}
		if len(s.entries) >= s.maxEntries {
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, e := range s.entries {
				if first || e.createdAt.Before(oldestTime) || (e.createdAt.Equal(oldestTime) && k < oldestKey) {
					oldestKey = k
					oldestTime = e.createdAt
					first = false
				}
			}
			if oldestKey != "" {
				delete(s.entries, oldestKey)
			}
		}
	}

	s.entries[probeID] = rttProbeEntry{
		minMS:     ms,
		count:     1,
		createdAt: now,
	}
	return true
}

// Consume 取出探测窗口内记录到的最小服务端 TCP RTT 并立即释放条目。
func (s *RTTProbeStore) Consume(probeID string) (float64, bool) {
	if s == nil || !isValidProbeID(probeID) {
		return 0, false
	}
	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[probeID]
	delete(s.entries, probeID)
	if !ok || s.isExpired(entry, now) || entry.minMS <= 0 {
		return 0, false
	}
	return entry.minMS, true
}

func (s *RTTProbeStore) cleanup(interval time.Duration) {
	defer close(s.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			now := s.nowFn()
			s.mu.Lock()
			for k, e := range s.entries {
				if s.isExpired(e, now) {
					delete(s.entries, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *RTTProbeStore) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
	return nil
}
