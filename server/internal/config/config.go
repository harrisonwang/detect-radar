package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig
	CORS   CORSConfig
	// 无 tag 时 mapstructure 按字段名匹配到键 "ratelimit"，而配置文件写的是 "rate_limit"，
	// 整块配置会被静默忽略、只剩默认值生效。
	RateLimit  RateLimitConfig `mapstructure:"rate_limit"`
	DNS        DNSConfig
	IPIntel    IPIntelConfig
	Redis      RedisConfig
	SQLite     SQLiteConfig
	Reputation ReputationConfig
	RTT        RTTConfig
}

// RTTConfig RTT 物理探测配置（Phase 1 只采集不计分）。
// ServerLat/ServerLon 是服务器自身经纬度，用于地理-延迟下限校验：IP 声称位置到本机的
// 大圆距离换算出物理最小往返时延，若 nginx 实测出口↔服务器 TCP RTT 低于该下限，
// 说明 IP 地理位置与延迟自相矛盾（geo violation）。默认生产机房洛杉矶。
// mapstructure 默认按去下划线的全小写键匹配（serverlat ≠ server_lat），故须显式 tag。
type RTTConfig struct {
	ServerLat float64 `mapstructure:"server_lat"`
	ServerLon float64 `mapstructure:"server_lon"`
}

// ReputationConfig 出口 IP 信誉/暴露检测配置（免费数据源）
type ReputationConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	ShodanBaseURL string `mapstructure:"shodan_base_url"` // Shodan InternetDB（免 key）
	TorListURL    string `mapstructure:"tor_list_url"`    // Tor 出口节点名单
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// SQLiteConfig SQLite 配置
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// IPIntelConfig IP 信息配置
type IPIntelConfig struct {
	// 本地数据库路径
	CityDBPath string `mapstructure:"city_db_path"`
	ASNDBPath  string `mapstructure:"asn_db_path"`

	// CN 权威地理层（ip2region xdb，v4/v6 两个文件；留空即关闭该层）
	// western geo 源对中国 IP 普遍错判，实测省级 87%→98%、市级 ~62%→~95%、IPv6 1/3→3/3。
	CNV4DBPath string `mapstructure:"cn_v4_db_path"`
	CNV6DBPath string `mapstructure:"cn_v6_db_path"`

	// 远程 API Keys（按优先级排序）
	// 优先级: ipapi.is > BigDataCloud > IPRegistry > IP2Location
	IPAPIISKey      string `mapstructure:"ipapiis_key"`
	BigDataCloudKey string `mapstructure:"bigdatacloud_key"`
	IPRegistryKey   string `mapstructure:"ipregistry_key"`
	IP2LocationKey  string `mapstructure:"ip2location_key"`

	// Cloudflare Radar API Token（用于获取 ASN 真实用户占比）
	CloudflareRadarToken string `mapstructure:"cloudflare_radar_token"`

	// 缓存配置
	CacheTTL time.Duration `mapstructure:"cache_ttl"`
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	// TrustedProxies 反代白名单（CIDR 或 IP）。为空时不信任任何代理头，
	// c.IP() 直接返回 socket 对端 IP，杜绝 X-Forwarded-For / X-Real-IP 伪造。
	// 部署在 nginx/CDN 之后时须填入反代地址，否则出口 IP 会解析成反代自身。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// ProxyHeader 取真实客户端 IP 的请求头（仅当对端在 TrustedProxies 内才采信）。
	ProxyHeader string `mapstructure:"proxy_header"`
}

// 字段名含多个大写字母时，mapstructure 匹配的是去下划线的全小写键（alloworigins），
// 与配置文件里的 allow_origins 对不上，整项会被静默忽略。凡是配置文件用下划线的键都要显式 tag。
type CORSConfig struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

type RateLimitConfig struct {
	Max        int           `mapstructure:"max"`
	Expiration time.Duration `mapstructure:"expiration"`
}

type DNSConfig struct {
	Domain     string
	TTL        time.Duration
	DNSTapAddr string `mapstructure:"dnstap_addr"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	// 环境变量覆盖
	viper.AutomaticEnv()

	// 默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.port", ":8080")
	viper.SetDefault("server.read_timeout", 10*time.Second)
	viper.SetDefault("server.write_timeout", 10*time.Second)
	viper.SetDefault("server.trusted_proxies", []string{}) // 默认不信任任何代理头（防伪造）
	viper.SetDefault("server.proxy_header", "X-Forwarded-For")

	// 通配 origin 会让任何站点的 JS 直接调用本 API，把 IP 判定结果当免费数据源；
	// 生产前端与 API 同源（nginx 反代 /api/v1），本就不需要跨域放行。
	viper.SetDefault("cors.allow_origins", []string{"https://detectradar.com"})
	viper.SetDefault("cors.allow_methods", []string{"GET", "POST", "OPTIONS"})
	viper.SetDefault("cors.allow_headers", []string{"Content-Type"})
	viper.SetDefault("cors.allow_credentials", false)

	viper.SetDefault("rate_limit.max", 100)
	viper.SetDefault("rate_limit.expiration", 60*time.Second)

	viper.SetDefault("dns.domain", "leak.detectradar.com")
	viper.SetDefault("dns.ttl", 1*time.Hour)
	viper.SetDefault("dns.dnstap_addr", ":6000")

	// IPIntel 默认值
	viper.SetDefault("ipintel.city_db_path", "./data/dbip-city-lite.mmdb")
	viper.SetDefault("ipintel.asn_db_path", "./data/dbip-asn-lite.mmdb")
	viper.SetDefault("ipintel.cn_v4_db_path", "./data/ip2region-cn/ip2region_v4.xdb")
	viper.SetDefault("ipintel.cn_v6_db_path", "./data/ip2region-cn/ip2region_v6.xdb")
	viper.SetDefault("ipintel.cache_ttl", 24*time.Hour)

	// Redis 默认值
	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	// SQLite 默认值
	viper.SetDefault("sqlite.path", "./data/hosting.db")

	// Reputation 默认值（信誉/暴露检测，免费源）
	viper.SetDefault("reputation.enabled", true)
	viper.SetDefault("reputation.shodan_base_url", "https://internetdb.shodan.io")
	viper.SetDefault("reputation.tor_list_url", "https://check.torproject.org/torbulkexitlist")

	// RTT 默认值：生产机房洛杉矶（34.0522, -118.2437），用于地理-延迟下限校验
	viper.SetDefault("rtt.server_lat", 34.0522)
	viper.SetDefault("rtt.server_lon", -118.2437)
}
