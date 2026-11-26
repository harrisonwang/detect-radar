package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"detect-radar/internal/util"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	CORS       CORSConfig
	RateLimit  RateLimitConfig `mapstructure:"rate_limit"`
	DNS        DNSConfig
	STUN       STUNConfig
	IPIntel    IPIntelConfig
	Redis      RedisConfig
	SQLite     SQLiteConfig
	Reputation ReputationConfig
	RTT        RTTConfig
	Telemetry  TelemetryConfig
	Mail       MailConfig `mapstructure:"mail"`
}

type MailConfig struct {
	AccountID string `mapstructure:"account_id"`
	APIToken  string `mapstructure:"api_token"`
	From      string `mapstructure:"from"`
	To        string `mapstructure:"to"`
}

func (c MailConfig) Enabled() bool { return c.AccountID != "" && c.APIToken != "" }

func (c MailConfig) validate() error {
	if c.AccountID == "" && c.APIToken == "" {
		return nil
	}
	if !c.Enabled() {
		return fmt.Errorf("邮件通知需同时设置 mail.account_id 和 mail.api_token")
	}
	if !regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(c.AccountID) {
		return fmt.Errorf("mail.account_id 必须为 32 位十六进制账户 ID")
	}
	if strings.ContainsAny(c.APIToken, "\r\n") || !util.IsValidEmailAddress(c.From) || !util.IsValidEmailAddress(c.To) {
		return fmt.Errorf("mail 的凭证或收发邮箱格式无效")
	}
	return nil
}

type RTTConfig struct {
	ServerLat float64 `mapstructure:"server_lat"`
	ServerLon float64 `mapstructure:"server_lon"`
}

type ReputationConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	ShodanBaseURL string `mapstructure:"shodan_base_url"`
	TorListURL    string `mapstructure:"tor_list_url"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type SQLiteConfig struct {
	Path     string `mapstructure:"path"`
	SeedPath string `mapstructure:"seed_path"`
}

type TelemetryConfig struct {
	Path                  string        `mapstructure:"path"`
	RetentionEnabled      bool          `mapstructure:"retention_enabled"`
	ScanRetentionDays     int           `mapstructure:"scan_retention_days"`
	FeedbackRetentionDays int           `mapstructure:"feedback_retention_days"`
	CleanupInterval       time.Duration `mapstructure:"cleanup_interval"`
}

type IPIntelConfig struct {
	CityDBPath string `mapstructure:"city_db_path"`
	ASNDBPath  string `mapstructure:"asn_db_path"`

	CNV4DBPath string `mapstructure:"cn_v4_db_path"`
	CNV6DBPath string `mapstructure:"cn_v6_db_path"`

	IPAPIISKey      string `mapstructure:"ipapiis_key"`
	BigDataCloudKey string `mapstructure:"bigdatacloud_key"`
	IPRegistryKey   string `mapstructure:"ipregistry_key"`
	IP2LocationKey  string `mapstructure:"ip2location_key"`

	CloudflareRadarToken string `mapstructure:"cloudflare_radar_token"`

	CacheTTL time.Duration `mapstructure:"cache_ttl"`

	L3DailyLimit int `mapstructure:"l3_daily_limit"`
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	TrustedProxies []string `mapstructure:"trusted_proxies"`
	ProxyHeader    string   `mapstructure:"proxy_header"`
}

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

type STUNConfig struct {
	Addr string `mapstructure:"addr"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.Mail.validate(); err != nil {
		return nil, err
	}

	if cfg.Telemetry.Path == "" || cfg.Telemetry.ScanRetentionDays <= 0 || cfg.Telemetry.FeedbackRetentionDays <= 0 || cfg.Telemetry.CleanupInterval <= 0 {
		return nil, fmt.Errorf("遥测数据库路径不能为空，保留天数和清理间隔必须大于零")
	}
	if cfg.Telemetry.ScanRetentionDays > 106751 || cfg.Telemetry.FeedbackRetentionDays > 106751 {
		return nil, fmt.Errorf("遥测数据保留天数超出支持范围")
	}
	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.port", ":8080")
	viper.SetDefault("server.read_timeout", 10*time.Second)
	viper.SetDefault("server.write_timeout", 10*time.Second)
	viper.SetDefault("server.trusted_proxies", []string{})
	viper.SetDefault("server.proxy_header", "X-Real-IP")

	viper.SetDefault("cors.allow_origins", []string{"https://detectradar.com"})
	viper.SetDefault("cors.allow_methods", []string{"GET", "POST", "OPTIONS"})
	viper.SetDefault("cors.allow_headers", []string{"Content-Type"})
	viper.SetDefault("cors.allow_credentials", false)

	viper.SetDefault("rate_limit.max", 100)
	viper.SetDefault("rate_limit.expiration", 60*time.Second)

	viper.SetDefault("dns.domain", "leak.detectradar.com")
	viper.SetDefault("dns.ttl", 1*time.Hour)
	viper.SetDefault("dns.dnstap_addr", ":6000")
	viper.SetDefault("stun.addr", ":3478")

	viper.SetDefault("ipintel.city_db_path", "./data/dbip-city-lite.mmdb")
	viper.SetDefault("ipintel.asn_db_path", "./data/dbip-asn-lite.mmdb")
	viper.SetDefault("ipintel.cn_v4_db_path", "./data/ip2region-cn/ip2region_v4.xdb")
	viper.SetDefault("ipintel.cn_v6_db_path", "./data/ip2region-cn/ip2region_v6.xdb")
	viper.SetDefault("ipintel.cache_ttl", 24*time.Hour)
	viper.SetDefault("ipintel.l3_daily_limit", 800)

	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("sqlite.path", "./data/hosting.db")
	viper.SetDefault("telemetry.path", "./data/scans.db")
	viper.SetDefault("telemetry.retention_enabled", true)
	viper.SetDefault("telemetry.scan_retention_days", 90)
	viper.SetDefault("telemetry.feedback_retention_days", 90)
	viper.SetDefault("telemetry.cleanup_interval", time.Hour)

	viper.SetDefault("mail.from", "noreply@detectradar.com")
	viper.SetDefault("mail.to", "feedback@detectradar.com")

	viper.SetDefault("reputation.enabled", true)
	viper.SetDefault("reputation.shodan_base_url", "https://internetdb.shodan.io")
	viper.SetDefault("reputation.tor_list_url", "https://check.torproject.org/torbulkexitlist")

	viper.SetDefault("rtt.server_lat", 34.0522)
	viper.SetDefault("rtt.server_lon", -118.2437)
}
