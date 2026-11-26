package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"detect-radar/internal/config"
	"detect-radar/internal/model"
)

type ReputationService struct {
	cfg    config.ReputationConfig
	client *http.Client

	torExits map[string]struct{}
	torMu    sync.RWMutex

	cache sync.Map
}

type cachedReputation struct {
	result    *model.ReputationResult
	createdAt time.Time
}

var dnsblZones = []string{
	"zen.spamhaus.org",
	"bl.spamcop.net",
	"b.barracudacentral.org",
	"dnsbl.dronebl.org",
}

var strongProxyPorts = map[int]string{
	1080:  "socks",
	4145:  "socks",
	9050:  "tor",
	9051:  "tor",
	1194:  "openvpn",
	51820: "wireguard",
}

var weakProxyPorts = map[int]string{
	3128: "http-proxy",
	8080: "http-proxy",
	8888: "http-proxy",
}

func NewReputationService(cfg config.ReputationConfig) *ReputationService {
	s := &ReputationService{
		cfg:      cfg,
		client:   &http.Client{Timeout: 5 * time.Second},
		torExits: map[string]struct{}{},
	}
	if cfg.Enabled && cfg.TorListURL != "" {
		go s.refreshTorLoop()
	}
	return s
}

func (s *ReputationService) Check(ctx context.Context, ip string) *model.ReputationResult {
	res := &model.ReputationResult{
		IP:               ip,
		Level:            "clean",
		BlacklistSources: []string{},
		OpenPorts:        []int{},
		ExposureTags:     []string{},
		CheckedSources:   []string{},
	}
	if !s.cfg.Enabled || net.ParseIP(ip) == nil {
		res.Recommendation = "信誉检测未启用或 IP 无效"
		return res
	}

	if v, ok := s.cache.Load(ip); ok {
		c := v.(cachedReputation)
		if time.Since(c.createdAt) < time.Hour {
			return c.result
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		ports, tags, ok := s.checkShodan(ctx, ip)
		mu.Lock()
		defer mu.Unlock()
		res.CheckedSources = append(res.CheckedSources, "shodan")
		if !ok {
			return
		}
		res.OpenPorts = ports
		for _, p := range ports {
			if label, ok := strongProxyPorts[p]; ok {
				res.OpenProxyPort = true
				res.ExposureTags = append(res.ExposureTags, fmt.Sprintf("open_proxy_port:%d(%s)", p, label))
				continue
			}
			if _, ok := weakProxyPorts[p]; ok {
				res.WeakProxyPort = true
			}
		}
		for _, t := range tags {
			lt := strings.ToLower(t)
			if lt == "vpn" || lt == "proxy" || lt == "tor" {
				res.ExposureTags = append(res.ExposureTags, "shodan_tag:"+lt)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sources, pbl := s.checkDNSBL(ctx, ip)
		mu.Lock()
		defer mu.Unlock()
		res.CheckedSources = append(res.CheckedSources, "dnsbl")
		if pbl {
			res.PBLListed = true
		}
		if len(sources) > 0 {
			res.BlacklistHit = true
			res.BlacklistSources = sources
			for _, z := range sources {
				res.ExposureTags = append(res.ExposureTags, "blacklist:"+z)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		isTor := s.isTorExit(ip)
		mu.Lock()
		defer mu.Unlock()
		res.CheckedSources = append(res.CheckedSources, "tor")
		if isTor {
			res.IsTorExit = true
			res.ExposureTags = append(res.ExposureTags, "tor_exit")
		}
	}()

	wg.Wait()

	switch {
	case res.BlacklistHit || res.IsTorExit || res.OpenProxyPort:
		res.Level = "exposed"
	case len(res.OpenPorts) > 0 || len(res.ExposureTags) > 0:
		res.Level = "suspicious"
	default:
		res.Level = "clean"
	}
	res.Recommendation = reputationRecommendation(res)

	s.cache.Store(ip, cachedReputation{result: res, createdAt: time.Now()})
	return res
}

func (s *ReputationService) checkShodan(ctx context.Context, ip string) (ports []int, tags []string, ok bool) {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.cfg.ShodanBaseURL, "/"), ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []int{}, nil, true
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}

	var body struct {
		Ports []int    `json:"ports"`
		Tags  []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, nil, false
	}
	return body.Ports, body.Tags, true
}

func (s *ReputationService) checkDNSBL(ctx context.Context, ip string) (sources []string, pbl bool) {
	rev, ok := reverseIPv4(ip)
	if !ok {
		return nil, false
	}
	for _, zone := range dnsblZones {
		if ctx.Err() != nil {
			break
		}
		addrs, err := net.DefaultResolver.LookupHost(ctx, rev+"."+zone)
		if err != nil {
			continue
		}
		if dnsblHit(zone, addrs) {
			sources = append(sources, zone)
		}
		if dnsblPBL(zone, addrs) {
			pbl = true
		}
	}
	return sources, pbl
}

func (s *ReputationService) isTorExit(ip string) bool {
	s.torMu.RLock()
	defer s.torMu.RUnlock()
	_, ok := s.torExits[ip]
	return ok
}

func (s *ReputationService) refreshTorLoop() {
	s.refreshTorList()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.refreshTorList()
	}
}

func (s *ReputationService) refreshTorList() {
	resp, err := s.client.Get(s.cfg.TorListURL)
	if err != nil {
		log.Printf("[Reputation] Tor list fetch failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}

	exits := map[string]struct{}{}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if net.ParseIP(line) != nil {
			exits[line] = struct{}{}
		}
	}
	if len(exits) == 0 {
		return
	}

	s.torMu.Lock()
	s.torExits = exits
	s.torMu.Unlock()
	log.Printf("[Reputation] Loaded %d Tor exit nodes", len(exits))
}

func reverseIPv4(ip string) (string, bool) {
	p := net.ParseIP(ip)
	if p == nil {
		return "", false
	}
	p4 := p.To4()
	if p4 == nil {
		return "", false
	}
	return fmt.Sprintf("%d.%d.%d.%d", p4[3], p4[2], p4[1], p4[0]), true
}

var zenPBLCodes = map[string]struct{}{
	"127.0.0.10": {},
	"127.0.0.11": {},
}

func dnsblHit(zone string, addrs []string) bool {
	for _, a := range addrs {
		if !strings.HasPrefix(a, "127.0.0.") {
			continue
		}
		if zone == "zen.spamhaus.org" {
			if _, isPBL := zenPBLCodes[a]; isPBL {
				continue
			}
		}
		return true
	}
	return false
}

func dnsblPBL(zone string, addrs []string) bool {
	if zone != "zen.spamhaus.org" {
		return false
	}
	for _, a := range addrs {
		if _, isPBL := zenPBLCodes[a]; isPBL {
			return true
		}
	}
	return false
}

func reputationRecommendation(r *model.ReputationResult) string {
	return ReputationRecommendation(r, "zh")
}

func ReputationRecommendation(r *model.ReputationResult, lang string) string {
	if r == nil {
		return ""
	}
	if len(r.CheckedSources) == 0 && r.Level == "clean" {
		if lang == "en" {
			return "Reputation check disabled or invalid IP"
		}
		return "信誉检测未启用或 IP 无效"
	}
	var parts []string
	if lang == "en" {
		if r.OpenProxyPort {
			parts = append(parts, "Open proxy port detected on this exit IP; close the port or switch exit IPs")
		}
		if r.BlacklistHit {
			noun := "blocklist"
			if len(r.BlacklistSources) > 1 {
				noun = "blocklists"
			}
			parts = append(parts, fmt.Sprintf("Exit IP listed on public %s (%s); consider switching exit IPs", noun, strings.Join(r.BlacklistSources, ", ")))
		}
		if r.IsTorExit {
			parts = append(parts, "Exit IP is a known Tor exit node")
		}
		if len(parts) == 0 {
			if len(r.OpenPorts) > 0 {
				return "Open service ports detected (server-like profile); residential IPs rarely expose public ports"
			}
			if len(r.ExposureTags) > 0 {
				return fmt.Sprintf("Public scan metadata flags this exit IP (%s)", strings.Join(r.ExposureTags, ", "))
			}
			return "No blocklist, open proxy port, or Tor exit exposure detected"
		}
		return strings.Join(parts, "; ")
	}
	if r.OpenProxyPort {
		parts = append(parts, "出口 IP 开放了代理端口，极易被识别为代理，建议更换出口或关闭端口")
	}
	if r.BlacklistHit {
		parts = append(parts, fmt.Sprintf("出口 IP 命中黑名单(%s)，建议更换 IP", strings.Join(r.BlacklistSources, ",")))
	}
	if r.IsTorExit {
		parts = append(parts, "出口 IP 是 Tor 出口节点，几乎必被风控拦截")
	}
	if len(parts) == 0 {
		if len(r.OpenPorts) > 0 {
			return "出口 IP 有开放服务端口（更像服务器），住宅 IP 通常无开放端口，注意此差异"
		}
		if len(r.ExposureTags) > 0 {
			return fmt.Sprintf("公开扫描库标记了该出口 IP 的暴露标签（%s）", strings.Join(r.ExposureTags, "、"))
		}
		return "出口 IP 信誉良好，未发现黑名单/代理端口/Tor 暴露"
	}
	return strings.Join(parts, "；")
}
