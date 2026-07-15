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

// ReputationService 用免费数据源检测出口 IP 的信誉/暴露：
//   - Shodan InternetDB（开放端口 + tags，免 key）
//   - DNSBL 反查（Spamhaus / SpamCop / Barracuda / DroneBL）
//   - Tor 出口节点名单
type ReputationService struct {
	cfg    config.ReputationConfig
	client *http.Client

	torExits map[string]struct{}
	torMu    sync.RWMutex

	cache sync.Map // ip -> cachedReputation
}

type cachedReputation struct {
	result    *model.ReputationResult
	createdAt time.Time
}

// 常见 DNSBL zone
var dnsblZones = []string{
	"zen.spamhaus.org",
	"bl.spamcop.net",
	"b.barracudacentral.org",
	"dnsbl.dronebl.org",
}

// 强代理端口 -> 标签：命中即高置信代理/VPN/Tor 暴露，直接置 OpenProxyPort 并进 ExposureTags。
// 这些端口住宅/家宽几乎不会开放，专属 socks/tor/openvpn/wireguard。
var strongProxyPorts = map[int]string{
	1080:  "socks",
	4145:  "socks",
	9050:  "tor",
	9051:  "tor",
	1194:  "openvpn",
	51820: "wireguard",
}

// 弱代理端口 -> 标签：3128/8080/8888 是 http-proxy 常见端口，但家用路由器管理页/Web UI
// 也大量占用，且 Shodan 数据可能过期数周、CGNAT 共享 IP 会继承邻居端口——单看极易误报家宽。
// 因此仅记为弱信号（WeakProxyPort），不置 OpenProxyPort、不进 ExposureTags（否则 len>0 会把
// Level 抬成 suspicious，从后门重新引入误报）；是否计分交由规则层按身份判定（仅机房出口才罚）。
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

// Check 对单个 IP 并发跑所有信誉检查并聚合
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

	// 缓存（1h）
	if v, ok := s.cache.Load(ip); ok {
		c := v.(cachedReputation)
		if time.Since(c.createdAt) < time.Hour {
			return c.result
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// 1. Shodan InternetDB —— 开放端口 + tags
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
			// 弱代理端口：仅记标志，不置 OpenProxyPort、不进 ExposureTags（避免家宽误报）
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

	// 2. DNSBL 反查
	wg.Add(1)
	go func() {
		defer wg.Done()
		sources, pbl := s.checkDNSBL(ctx, ip)
		mu.Lock()
		defer mu.Unlock()
		res.CheckedSources = append(res.CheckedSources, "dnsbl")
		// PBL 仅记录、不惩罚：不进 ExposureTags（否则 len>0 会把 Level 抬成 suspicious，从后门重新引入误报）
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

	// 3. Tor 出口名单（本地 map，快）
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

	// 聚合等级
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

// checkShodan 查 Shodan InternetDB（免 key）。404 表示无数据（视为干净）。
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
		return []int{}, nil, true // 无记录 = 未暴露
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

// checkDNSBL 对 IPv4 反转后逐个 DNSBL zone 查询 A 记录。
// 用 DefaultResolver.LookupHost(ctx, ...) 而非 net.LookupHost，以便调用方的超时/取消能生效。
func (s *ReputationService) checkDNSBL(ctx context.Context, ip string) (sources []string, pbl bool) {
	rev, ok := reverseIPv4(ip)
	if !ok {
		return nil, false // 仅支持 IPv4
	}
	for _, zone := range dnsblZones {
		if ctx.Err() != nil {
			break // 已超时/取消，停止后续 zone 查询
		}
		addrs, err := net.DefaultResolver.LookupHost(ctx, rev+"."+zone)
		if err != nil {
			continue // NXDOMAIN = 未列入
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

// refreshTorLoop 启动时拉取一次，之后每 6h 刷新 Tor 出口名单
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

// ---- helpers ----

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

// zen.spamhaus.org 的 PBL 返回码：住宅/动态 IP 的政策名单（不该直发 SMTP），
// 不是信誉污点——全球大量干净家宽都在里面，不能算黑名单命中。
var zenPBLCodes = map[string]struct{}{
	"127.0.0.10": {}, // PBL（ISP 维护）
	"127.0.0.11": {}, // PBL（Spamhaus 维护）
}

// dnsblHit 纯函数：判断某 DNSBL zone 的返回码是否算「信誉命中」，可脱网单测。
// 命中码在 127.0.0.x（Spamhaus 的 127.255.255.x 是查询被拒/限速，非命中）。
// zen.spamhaus.org 单独处理：PBL(127.0.0.10/11) 是住宅/动态 IP 政策名单，不算命中；
// 其余码（SBL/CSS/XBL/DROP）以及其它 zone 沿用「任意 127.0.0.x = 命中」。
func dnsblHit(zone string, addrs []string) bool {
	for _, a := range addrs {
		if !strings.HasPrefix(a, "127.0.0.") {
			continue
		}
		if zone == "zen.spamhaus.org" {
			if _, isPBL := zenPBLCodes[a]; isPBL {
				continue // PBL 不算信誉命中
			}
		}
		return true
	}
	return false
}

// dnsblPBL 纯函数：判断 zen 是否返回 PBL 码。
// PBL 命中其实是「住宅/动态 IP」的弱正向证据，留给后续使用；仅 zen.spamhaus.org 有此语义。
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
	var parts []string
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
		return "出口 IP 信誉良好，未发现黑名单/代理端口/Tor 暴露"
	}
	return strings.Join(parts, "；")
}
