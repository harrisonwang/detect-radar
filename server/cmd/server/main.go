package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"detect-radar/internal/adapter"
	"detect-radar/internal/cache"
	"detect-radar/internal/config"
	"detect-radar/internal/database"
	"detect-radar/internal/handler"
	"detect-radar/internal/middleware"
	"detect-radar/internal/repository"
	"detect-radar/internal/service"
)

type Services struct {
	IPIntel     *service.IPIntelService     // 统一 IP 信息服务
	Scan        *service.ScanService        // 综合扫描（P0）
	Consistency *service.ConsistencyService // 环境一致性（P1）
	DNS         *service.DNSService         // DNS 泄露（P2）
	DNSTap      *service.DNSTapService      // DNS 泄露记录端（P2）
	Reputation  *service.ReputationService  // 出口 IP 信誉/暴露（P5）
	Journal     *service.Journal            // 内部遥测流水（不经 HTTP 暴露）
}

type Infrastructure struct {
	RedisCache *cache.RedisCache
	SQLiteDB   *database.SQLiteDB
}

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 初始化基础设施
	infra := initInfrastructure(cfg)
	defer cleanupInfrastructure(infra)

	// 初始化 IPIntel 服务（三层架构）
	ipIntelService := initIPIntelService(cfg, infra)

	// 内部遥测流水（回放用户反馈现场；文件轮转由 logrotate 负责）
	journal := service.NewJournal("data/scans.jsonl")

	// 检测相关服务（P0/P1/P2/P5）
	consistencyService := service.NewConsistencyService(ipIntelService)
	reputationService := service.NewReputationService(cfg.Reputation)
	dnsService := service.NewDNSService(cfg.DNS, ipIntelService, journal)
	dnsTapService := service.NewDNSTapService(cfg.DNS.Domain, dnsService)

	services := &Services{
		IPIntel:     ipIntelService,
		Consistency: consistencyService,
		Reputation:  reputationService,
		DNS:         dnsService,
		DNSTap:      dnsTapService,
		Scan:        service.NewScanService(ipIntelService, consistencyService, reputationService, cfg.RTT.ServerLat, cfg.RTT.ServerLon),
		Journal:     journal,
	}

	// DNS 观测到位后由 DNS 服务重算已存扫描，把 post-DNS 判定写入遥测流水
	dnsService.SetScanService(services.Scan)

	// 启动 DNSTap 记录端（挂真实递归解析器；未配置 dnstap_addr 时跳过）
	if cfg.DNS.DNSTapAddr != "" {
		if err := dnsTapService.Start(cfg.DNS.DNSTapAddr); err != nil {
			log.Printf("Warning: Failed to start DNSTap service: %v", err)
		}
	}

	// 创建Fiber应用
	app := fiber.New(fiber.Config{
		AppName:      "Detect Radar API",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BodyLimit:    256 * 1024, // 扫描载荷都是小 JSON，收紧默认 4MB 上限，抵御内存灌注
		// 仅信任白名单内反代的 X-Forwarded-For/X-Real-IP，其余一律取 socket 对端 IP
		EnableTrustedProxyCheck: true,
		TrustedProxies:          cfg.Server.TrustedProxies,
		ProxyHeader:             cfg.Server.ProxyHeader,
		EnableIPValidation:      true, // 从代理头中提取合法单个 IP，而非原始（可能多 IP）字符串
	})

	// 全局中间件
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(middleware.CORS(cfg.CORS))
	app.Use(middleware.RateLimit(cfg.RateLimit))

	// 路由设置
	setupRoutes(app, services)

	// 优雅关闭
	go func() {
		if err := app.Listen(cfg.Server.Port); err != nil {
			log.Fatal("Failed to start server:", err)
		}
	}()

	log.Printf("Server started on %s", cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if services.DNSTap != nil {
		services.DNSTap.Stop()
	}
	if err := app.Shutdown(); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

func setupRoutes(app *fiber.App, services *Services) {
	api := app.Group("/api/v1")

	// 路由命名约定（本 API 无外部消费者，端点随需整体增删，不留兼容别名）：
	//   - 复数 = 集合资源（可创建、带 /:id 子资源）：/scans、/leaks/dns
	//   - 单数 = 与本次连接绑定的单例：/ip、/ip/reputation
	//   - 动作型探针：/ping

	// 健康检查（nginx 健康检查 location 显式反代到 /api/v1/health）
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "healthy",
			"version":   "1.0.0",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// RTT 探针（Phase 1）：客户端循环打 N 次以测应用层往返时延。
	// 这个端点并不返回 RTT——它只回 204，往返时延由客户端自己掐表测量；叫 ping 更诚实
	//（探针动作，不是资源）。刻意做到最简——不查 IP、不落库、不写遥测，直接 204 且禁缓存
	//（否则浏览器/中间层缓存会让第 2 次起测到 ~0）。全局限流已豁免本路径（见 middleware.RateLimit 注释）。
	api.Get("/ping", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		return c.SendStatus(fiber.StatusNoContent)
	})

	// IP 信息（三层架构）：只暴露「当前连接出口 IP」自查的单例，不接受调用方指定任意 IP。
	// /ip 与 /ip/reputation 是两个独立静态路径（无 :param 冲突），相邻注册。
	ipIntelHandler := handler.NewIPIntelHandler(services.IPIntel)
	reputationHandler := handler.NewReputationHandler(services.Reputation)
	api.Get("/ip", ipIntelHandler.GetMyIPIntel) // 获取客户端 IP 信息（永不缓存）
	api.Get("/ip/reputation", reputationHandler.GetCurrentReputation)

	// 综合扫描（P0）
	scanHandler := handler.NewScanHandler(services.Scan, services.Journal)
	api.Post("/scans", scanHandler.CreateScan)
	api.Get("/scans/:id", scanHandler.GetScan)
	api.Post("/scans/:id/dns", scanHandler.UpdateDNS)           // 异步 DNS 结果回传，后端重算
	api.Post("/scans/:id/feedback", scanHandler.SubmitFeedback) // 结果反馈通道（误报/漏检标注）

	// DNS 泄露（P2）
	dnsHandler := handler.NewDNSHandler(services.DNS)
	api.Post("/leaks/dns", dnsHandler.CreateLeakTest)
	api.Get("/leaks/dns/:id", dnsHandler.GetLeakResult)
}

// initInfrastructure 初始化基础设施（Redis、SQLite）
func initInfrastructure(cfg *config.Config) *Infrastructure {
	infra := &Infrastructure{}

	// 初始化 Redis 缓存
	redisCache, err := cache.NewRedisCache(cache.RedisConfig{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, cfg.IPIntel.CacheTTL)
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v (caching disabled)", err)
	} else {
		infra.RedisCache = redisCache
		log.Printf("Connected to Redis at %s", cfg.Redis.Addr)
	}

	// 初始化 SQLite 数据库
	sqliteDB, err := database.NewSQLiteDB(database.SQLiteConfig{
		Path: cfg.SQLite.Path,
	})
	if err != nil {
		log.Printf("Warning: Failed to open SQLite database: %v", err)
	} else {
		infra.SQLiteDB = sqliteDB
		stats, _ := sqliteDB.Stats()
		log.Printf("Opened SQLite database at %s (providers: %d, keywords: %d)",
			cfg.SQLite.Path, stats["hosting_providers"], stats["hosting_keywords"])
	}

	return infra
}

// cleanupInfrastructure 清理基础设施
func cleanupInfrastructure(infra *Infrastructure) {
	if infra.RedisCache != nil {
		infra.RedisCache.Close()
	}
	if infra.SQLiteDB != nil {
		infra.SQLiteDB.Close()
	}
}

// initIPIntelService 初始化 IP 信息服务（三层架构）
func initIPIntelService(cfg *config.Config, infra *Infrastructure) *service.IPIntelService {
	registry := adapter.NewRegistry()

	// L1: 本地数据库（可选）
	if cfg.IPIntel.CityDBPath != "" || cfg.IPIntel.ASNDBPath != "" {
		local, err := adapter.NewLocalAdapter(cfg.IPIntel.CityDBPath, cfg.IPIntel.ASNDBPath)
		if err != nil {
			log.Printf("Warning: Failed to load local IP database: %v", err)
		} else {
			registry.SetLocal(local)
			log.Printf("Loaded local IP database: city=%s, asn=%s", cfg.IPIntel.CityDBPath, cfg.IPIntel.ASNDBPath)
		}
	}

	// CN 权威地理层（ip2region xdb，可选）：western geo 源对中国 IP 普遍错判，
	// 命中中国即覆盖 geo 并据运营商放行家宽/移动、识别国内云厂商。留空即关闭。
	var cnGeo *adapter.IP2RegionCN
	if cfg.IPIntel.CNV4DBPath != "" || cfg.IPIntel.CNV6DBPath != "" {
		cn, err := adapter.NewIP2RegionCN(cfg.IPIntel.CNV4DBPath, cfg.IPIntel.CNV6DBPath)
		if err != nil {
			log.Printf("Warning: Failed to load ip2region CN layer: %v", err)
		} else if cn != nil {
			cnGeo = cn
			log.Printf("Loaded ip2region CN geo layer: v4=%s, v6=%s", cfg.IPIntel.CNV4DBPath, cfg.IPIntel.CNV6DBPath)
		}
	}

	// L3: 远程 API（按优先级添加）
	// 优先级: IP2Location > ipapi.is > BigDataCloud > IPRegistry
	if cfg.IPIntel.IP2LocationKey != "" {
		registry.AddRemote(adapter.NewIP2LocationAdapter(cfg.IPIntel.IP2LocationKey))
		log.Println("Added IP2Location adapter")
	}

	if cfg.IPIntel.IPAPIISKey != "" {
		registry.AddRemote(adapter.NewIPAPIISAdapter(cfg.IPIntel.IPAPIISKey))
		log.Println("Added ipapi.is adapter")
	}

	if cfg.IPIntel.BigDataCloudKey != "" {
		registry.AddRemote(adapter.NewBigDataCloudAdapter(cfg.IPIntel.BigDataCloudKey))
		log.Println("Added BigDataCloud adapter")
	}

	if cfg.IPIntel.IPRegistryKey != "" {
		registry.AddRemote(adapter.NewIPRegistryAdapter(cfg.IPIntel.IPRegistryKey))
		log.Println("Added IPRegistry adapter")
	}

	// 初始化 HostingDetector
	var hostingDetector *service.HostingDetector
	if infra.SQLiteDB != nil && cfg.IPIntel.ASNDBPath != "" {
		db := infra.SQLiteDB.DB()
		cloudIPRepo := repository.NewCloudIPRangeRepository(db)
		providerRepo := repository.NewHostingProviderRepository(db)
		keywordRepo := repository.NewHostingKeywordRepository(db)

		var err error
		hostingDetector, err = service.NewHostingDetector(
			cfg.IPIntel.ASNDBPath,
			cloudIPRepo,
			providerRepo,
			keywordRepo,
		)
		if err != nil {
			log.Printf("Warning: Failed to create HostingDetector: %v", err)
		} else {
			stats := hostingDetector.GetStats()
			log.Printf("Initialized HostingDetector (providers: %v, keywords: %v)",
				stats["provider_count"], stats["keyword_count"])
		}
	}

	// 初始化 Cloudflare Radar 服务
	var radarService *service.RadarService
	if cfg.IPIntel.CloudflareRadarToken != "" {
		if infra.RedisCache != nil {
			radarService = service.NewRadarService(service.RadarConfig{
				Token:    cfg.IPIntel.CloudflareRadarToken,
				CacheTTL: 24 * time.Hour,
			}, infra.RedisCache.Client())
			log.Println("Initialized Cloudflare Radar service")
		} else {
			log.Printf("Warning: Cloudflare Radar service requires Redis for caching, skipped")
		}
	}

	return service.NewIPIntelService(registry, infra.RedisCache, hostingDetector, radarService, cnGeo)
}
