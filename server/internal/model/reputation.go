package model

type ReputationResult struct {
	IP               string   `json:"ip"`
	Level            string   `json:"level"`
	BlacklistHit     bool     `json:"blacklist_hit"`
	BlacklistSources []string `json:"blacklist_sources"`
	PBLListed        bool     `json:"pbl_listed"`
	OpenPorts        []int    `json:"open_ports"`
	OpenProxyPort    bool     `json:"open_proxy_port"`
	WeakProxyPort    bool     `json:"weak_proxy_port"`
	IsTorExit        bool     `json:"is_tor_exit"`
	ExposureTags     []string `json:"exposure_tags"`
	CheckedSources   []string `json:"checked_sources"`
	Recommendation   string   `json:"recommendation"`
}
