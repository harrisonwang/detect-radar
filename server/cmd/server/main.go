package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"

	"detect-radar/internal/adapter"
	"detect-radar/internal/cache"
	"detect-radar/internal/config"
	"detect-radar/internal/database"
	"detect-radar/internal/handler"
	"detect-radar/internal/middleware"
	"detect-radar/internal/repository"
	"detect-radar/internal/service"
	"detect-radar/internal/telemetry"
)

type Services struct {
	IPIntel     *service.IPIntelService
	Scan        *service.ScanService
	Consistency *service.ConsistencyService
	DNS         *service.DNSService
	DNSTap      *service.DNSTapService
	STUN        *service.STUNService
	Reputation  *service.ReputationService
	RTTProbes   *service.RTTProbeStore
	Journal     *service.Journal
}

type Infrastructure struct {
	RedisCache *cache.RedisCache
	SQLiteDB   *database.SQLiteDB
	Catalog    *service.NetworkCatalog
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("加载配置失败：", err)
	}

	infra := initInfrastructure(cfg)
	defer cleanupInfrastructure(infra)

	ipIntelService := initIPIntelService(cfg, infra)

	store, err := telemetry.Open(telemetry.Options{
		Path: cfg.Telemetry.Path, RetentionEnabled: cfg.Telemetry.RetentionEnabled,
		ScanRetention:     time.Duration(cfg.Telemetry.ScanRetentionDays) * 24 * time.Hour,
		FeedbackRetention: time.Duration(cfg.Telemetry.FeedbackRetentionDays) * 24 * time.Hour,
	})
	if err != nil {
		log.Fatal("打开遥测数据库失败：", err)
	}
	defer store.Close()
	var mailer *service.Mailer
	if cfg.Mail.Enabled() {
		outbox, err := repository.NewMailOutbox(store.DB())
		if err != nil {
			log.Fatal("初始化邮件队列失败：", err)
		}
		mailer = service.NewMailer(outbox, cfg.Mail)
		mailCtx, cancelMail := context.WithCancel(context.Background())
		mailDone := make(chan struct{})
		go func() {
			defer close(mailDone)
			mailer.Run(mailCtx)
		}()
		defer func() { cancelMail(); <-mailDone }()
		log.Println("[邮件] Cloudflare 发信已启用")
	}
	journal := service.NewJournal(store, mailer)
	if cfg.Telemetry.RetentionEnabled {
		cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
		cleanupDone := make(chan struct{})
		go func() {
			defer close(cleanupDone)
			cleanupTelemetry(cleanupCtx, store, cfg.Telemetry.CleanupInterval)
		}()
		defer func() { cancelCleanup(); <-cleanupDone }()
	}

	consistencyService := service.NewConsistencyService()
	reputationService := service.NewReputationService(cfg.Reputation)
	dnsService := service.NewDNSService(cfg.DNS, ipIntelService, journal)
	dnsService.SetCatalog(infra.Catalog)
	dnsTapService := service.NewDNSTapService(cfg.DNS.Domain, dnsService)
	stunService := service.NewSTUNService()
	rttProbes := service.NewRTTProbeStore()

	services := &Services{
		IPIntel:     ipIntelService,
		Consistency: consistencyService,
		Reputation:  reputationService,
		DNS:         dnsService,
		DNSTap:      dnsTapService,
		STUN:        stunService,
		RTTProbes:   rttProbes,
		Scan:        service.NewScanService(ipIntelService, consistencyService, reputationService, cfg.RTT.ServerLat, cfg.RTT.ServerLon, journal),
		Journal:     journal,
	}

	dnsService.SetScanService(services.Scan)
	services.Scan.SetRTTProbeStore(rttProbes)
	defer rttProbes.Close()
	defer dnsService.Close()
	defer services.Scan.Close()

	if cfg.DNS.DNSTapAddr != "" {
		if err := dnsTapService.Start(cfg.DNS.DNSTapAddr); err != nil {
			log.Printf("警告：启动 DNSTap 服务失败：%v", err)
		}
	}
	if cfg.STUN.Addr != "" {
		if err := stunService.Start(cfg.STUN.Addr); err != nil {
			log.Printf("警告：启动 STUN 服务失败：%v", err)
		}
	}

	app := fiber.New(fiber.Config{
		AppName:                 "Detect Radar API",
		ReadTimeout:             cfg.Server.ReadTimeout,
		WriteTimeout:            cfg.Server.WriteTimeout,
		BodyLimit:               256 * 1024,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          cfg.Server.TrustedProxies,
		ProxyHeader:             cfg.Server.ProxyHeader,
		EnableIPValidation:      true,
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(middleware.CORS(cfg.CORS))
	app.Use(middleware.RateLimit(cfg.RateLimit))

	setupRoutes(app, services)

	go func() {
		if err := app.Listen(cfg.Server.Port); err != nil {
			log.Fatal("启动服务失败：", err)
		}
	}()

	log.Printf("服务已启动，监听端口：%s", cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务……")
	if services.DNSTap != nil {
		services.DNSTap.Stop()
	}
	if services.STUN != nil {
		services.STUN.Stop()
	}
	if err := app.Shutdown(); err != nil {
		log.Fatal("服务关闭失败：", err)
	}

	log.Println("服务已退出")
}

func cleanupTelemetry(ctx context.Context, store *telemetry.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			n, err := store.Prune(ctx, time.Now())
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("[遥测] 清理过期数据失败：%v", err)
				}
				break
			}
			if n < 1000 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func setupRoutes(app *fiber.App, services *Services) {
	api := app.Group("/api/v1")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "healthy",
			"version":   "1.0.1",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	api.Get("/ping", func(c *fiber.Ctx) error {
		if services.RTTProbes != nil && c.IsProxyTrusted() {
			if probeID := c.Query("probe_id"); probeID != "" {
				services.RTTProbes.Record(probeID, c.Get("X-TCP-RTT"))
			}
		}
		c.Set("Cache-Control", "no-store")
		return c.SendStatus(fiber.StatusNoContent)
	})

	ipIntelHandler := handler.NewIPIntelHandler(services.IPIntel)
	reputationHandler := handler.NewReputationHandler(services.Reputation)
	api.Get("/ip", ipIntelHandler.GetMyIPIntel)
	api.Get("/ip/text", ipIntelHandler.GetMyIPText)
	api.Get("/ip/reputation", reputationHandler.GetCurrentReputation)

	scanHandler := handler.NewScanHandler(services.Scan, services.Journal)
	api.Post("/scans", scanHandler.CreateScan)
	api.Get("/scans/:id", scanHandler.GetScan)
	api.Post("/scans/:id/feedback", scanHandler.SubmitFeedback)

	dnsHandler := handler.NewDNSHandler(services.DNS)
	api.Post("/leaks/dns", dnsHandler.CreateLeakTest)
	api.Post("/leaks/dns/assessment", dnsHandler.Assess)
	api.Get("/leaks/dns/:id", dnsHandler.GetLeakResult)

	webrtcHandler := handler.NewWebRTCHandler(services.IPIntel)
	api.Post("/leaks/webrtc/assessment", webrtcHandler.Assess)
}

func initInfrastructure(cfg *config.Config) *Infrastructure {
	infra := &Infrastructure{}

	redisCache, err := cache.NewRedisCache(cache.RedisConfig{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, cfg.IPIntel.CacheTTL)
	if err != nil {
		log.Printf("警告：连接 Redis 失败：%v（已禁用缓存）", err)
	} else {
		infra.RedisCache = redisCache
		log.Printf("已连接 Redis：%s", cfg.Redis.Addr)
	}

	sqliteDB, err := database.NewSQLiteDB(database.SQLiteConfig{
		Path: cfg.SQLite.Path,
	})
	if err != nil {
		log.Printf("警告：打开 SQLite 数据库失败：%v", err)
	} else {
		infra.SQLiteDB = sqliteDB
		stats, _ := sqliteDB.Stats()
		log.Printf("已打开 SQLite 数据库：%s（服务商：%d，关键词：%d）",
			cfg.SQLite.Path, stats["hosting_providers"], stats["hosting_keywords"])
	}

	var catalogRepo *repository.CatalogRepository
	if infra.SQLiteDB != nil {
		catalogRepo = repository.NewCatalogRepository(infra.SQLiteDB.DB())
	}
	var redisClient *redis.Client
	if infra.RedisCache != nil {
		redisClient = infra.RedisCache.Client()
	}
	catalog, err := service.NewNetworkCatalog(catalogRepo, redisClient, cfg.SQLite.SeedPath)
	if err != nil {
		log.Printf("警告：初始化网络目录失败：%v", err)
	} else if catalog != nil {
		infra.Catalog = catalog
		service.SetDefaultCatalog(catalog)
		catalog.Start()
		cstats := catalog.Stats()
		log.Printf("已初始化网络目录（ASN：%d，接入网：%d，别名：%d，公共 DNS：%d，地名：%d）",
			cstats["asn_count"], cstats["access_asn_count"], cstats["alias_count"], cstats["public_dns_count"], cstats["geo_name_count"])
	}

	return infra
}

func cleanupInfrastructure(infra *Infrastructure) {
	if infra.Catalog != nil {
		infra.Catalog.Close()
	}
	if infra.RedisCache != nil {
		infra.RedisCache.Close()
	}
	if infra.SQLiteDB != nil {
		infra.SQLiteDB.Close()
	}
}

func initIPIntelService(cfg *config.Config, infra *Infrastructure) *service.IPIntelService {
	registry := adapter.NewRegistry()

	if cfg.IPIntel.CityDBPath != "" || cfg.IPIntel.ASNDBPath != "" {
		local, err := adapter.NewLocalAdapter(cfg.IPIntel.CityDBPath, cfg.IPIntel.ASNDBPath)
		if err != nil {
			log.Printf("警告：加载本地 IP 数据库失败：%v", err)
		} else {
			registry.SetLocal(local)
			log.Printf("已加载本地 IP 数据库：城市库=%s，ASN 库=%s", cfg.IPIntel.CityDBPath, cfg.IPIntel.ASNDBPath)
		}
	}

	var cnGeo *adapter.IP2RegionCN
	if cfg.IPIntel.CNV4DBPath != "" || cfg.IPIntel.CNV6DBPath != "" {
		cn, err := adapter.NewIP2RegionCN(cfg.IPIntel.CNV4DBPath, cfg.IPIntel.CNV6DBPath)
		if err != nil {
			log.Printf("警告：加载 ip2region 中国地理数据库失败：%v", err)
		} else if cn != nil {
			cnGeo = cn
			log.Printf("已加载 ip2region 中国地理数据库：IPv4=%s，IPv6=%s", cfg.IPIntel.CNV4DBPath, cfg.IPIntel.CNV6DBPath)
		}
	}

	if cfg.IPIntel.IP2LocationKey != "" {
		registry.AddRemote(adapter.NewIP2LocationAdapter(cfg.IPIntel.IP2LocationKey))
		log.Println("已添加 IP2Location 适配器")
	}

	if cfg.IPIntel.IPAPIISKey != "" {
		registry.AddRemote(adapter.NewIPAPIISAdapter(cfg.IPIntel.IPAPIISKey))
		log.Println("已添加 ipapi.is 适配器")
	}

	if cfg.IPIntel.BigDataCloudKey != "" {
		registry.AddRemote(adapter.NewBigDataCloudAdapter(cfg.IPIntel.BigDataCloudKey))
		log.Println("已添加 BigDataCloud 适配器")
	}

	if cfg.IPIntel.IPRegistryKey != "" {
		registry.AddRemote(adapter.NewIPRegistryAdapter(cfg.IPIntel.IPRegistryKey))
		log.Println("已添加 IPRegistry 适配器")
	}

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
			log.Printf("警告：创建机房检测器失败：%v", err)
		} else {
			hostingDetector.SetCatalog(infra.Catalog)
			stats := hostingDetector.GetStats()
			log.Printf("已初始化机房检测器（服务商：%v，关键词：%v，接入网 ASN：%v）",
				stats["provider_count"], stats["keyword_count"], stats["access_asn_count"])
		}
	}

	var radarService *service.RadarService
	if cfg.IPIntel.CloudflareRadarToken != "" {
		if infra.RedisCache != nil {
			radarService = service.NewRadarService(service.RadarConfig{
				Token:    cfg.IPIntel.CloudflareRadarToken,
				CacheTTL: 24 * time.Hour,
			}, infra.RedisCache.Client())
			log.Println("已初始化 Cloudflare Radar 服务")
		} else {
			log.Printf("警告：Cloudflare Radar 服务依赖 Redis 缓存，已跳过初始化")
		}
	}

	svc := service.NewIPIntelService(registry, infra.RedisCache, hostingDetector, radarService, cnGeo)
	svc.SetL3DailyLimit(cfg.IPIntel.L3DailyLimit)
	if cfg.IPIntel.L3DailyLimit > 0 {
		log.Printf("已启用 L3 远程 API 每日熔断保护：上限 %d 次/天", cfg.IPIntel.L3DailyLimit)
	}
	return svc
}
