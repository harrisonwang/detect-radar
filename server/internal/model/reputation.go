package model

// ReputationResult 出口 IP 信誉/暴露检测结果（全部来自免费数据源）
type ReputationResult struct {
	IP               string   `json:"ip"`
	Level            string   `json:"level"` // clean / suspicious / exposed
	BlacklistHit     bool     `json:"blacklist_hit"`
	BlacklistSources []string `json:"blacklist_sources"` // 命中的 DNSBL zone
	PBLListed        bool     `json:"pbl_listed"`        // zen PBL(127.0.0.10/11) 命中：其实是「住宅/动态 IP」的弱正向证据，不惩罚、不改 Level，留给后续使用
	OpenPorts        []int    `json:"open_ports"`        // Shodan 观测到的开放端口
	OpenProxyPort    bool     `json:"open_proxy_port"`   // 是否开放强代理端口（socks/tor/openvpn/wireguard），高置信
	WeakProxyPort    bool     `json:"weak_proxy_port"`   // 是否开放弱代理端口（3128/8080/8888 http-proxy）：家宽路由器管理页也常占用，仅在机房出口下才计分，不置 OpenProxyPort、不进 ExposureTags
	IsTorExit        bool     `json:"is_tor_exit"`
	ExposureTags     []string `json:"exposure_tags"`   // 人类可读暴露点，如 open_proxy_port:1080 / shodan_tag:vpn
	CheckedSources   []string `json:"checked_sources"` // 实际跑了哪些检查
	Recommendation   string   `json:"recommendation"`
}
