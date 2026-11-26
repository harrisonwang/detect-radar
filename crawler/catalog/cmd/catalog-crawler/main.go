package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	catalogsync "github.com/harrisonwang/detect-radar/crawler/catalog/internal/sync"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "hosting.db", "SQLite 数据库路径")
	seedPath := flag.String("seed", "", "可选种子 JSON 路径（如 data/catalog_seed.json）")
	enablePeeringDB := flag.Bool("peeringdb", true, "是否同步 PeeringDB（默认增量 ?since=）")
	enableBGPTools := flag.Bool("bgptools", true, "是否同步 bgp.tools/asns.csv（默认 ETag 差量）")
	enableWikidata := flag.Bool("wikidata", false, "是否用 Wikidata SPARQL 增量补全接入网中文名")
	wikidataLimit := flag.Int("wikidata-limit", 200, "单次 Wikidata 补全的最大 ASN 数量")
	full := flag.Bool("full", false, "忽略增量游标执行全量拉取")
	peeringDBKey := flag.String("peeringdb-key", "", "PeeringDB API Key（默认读环境变量 PEERINGDB_API_KEY）")
	userAgent := flag.String("user-agent", "", "自定义 HTTP User-Agent（默认读环境变量 CRAWLER_USER_AGENT）")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("catalog-crawler", catalogsync.Version)
		return
	}

	apiKey := strings.TrimSpace(*peeringDBKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("PEERINGDB_API_KEY"))
	}
	ua := strings.TrimSpace(*userAgent)
	if ua == "" {
		ua = strings.TrimSpace(os.Getenv("CRAWLER_USER_AGENT"))
	}

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("无法打开数据库 %s: %v", *dbPath, err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := catalogsync.EnsureSchema(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	totalChanged := 0
	hadSource := false
	failed := 0

	if s := strings.TrimSpace(*seedPath); s != "" {
		hadSource = true
		rawSeed, err := os.ReadFile(s)
		if err != nil {
			log.Fatalf("读取种子文件 %s 失败: %v", s, err)
		}
		sum := sha256.Sum256(rawSeed)
		seedHash := hex.EncodeToString(sum[:])
		meta, err := catalogsync.GetSyncMeta(db, "seed")
		if err != nil {
			log.Fatalf("读取 seed 游标失败: %v", err)
		}
		if !*full && meta.Cursor == seedHash {
			log.Printf("[seed] 种子哈希未变化，跳过重复写入")
		} else {
			seed, err := catalogsync.LoadSeedFile(s)
			if err != nil {
				log.Fatalf("加载种子文件 %s 失败: %v", s, err)
			}
			n, err := catalogsync.ApplySeed(db, seed)
			if err != nil {
				log.Fatalf("写入种子数据失败: %v", err)
			}
			_ = catalogsync.SetSyncMeta(db, "seed", seedHash, int64(n))
			log.Printf("[seed] 已导入/更新 %d 条基础记录", n)
			totalChanged += n
		}
	}

	if *enablePeeringDB {
		hadSource = true
		n, err := catalogsync.SyncPeeringDB(db, apiKey, *full, ua)
		if err != nil {
			log.Printf("[peeringdb] 同步失败: %v", err)
			failed++
		} else {
			log.Printf("[peeringdb] 增量同步完成，变更 %d 条", n)
			totalChanged += n
		}
	}

	if *enableBGPTools {
		hadSource = true
		n, notModified, err := catalogsync.SyncBGPTools(db, *full, ua)
		if err != nil {
			log.Printf("[bgptools] 同步失败: %v", err)
			failed++
		} else if notModified {
			log.Printf("[bgptools] 304 Not Modified，无变更")
		} else {
			log.Printf("[bgptools] 差量同步完成，变更 %d 条", n)
			totalChanged += n
		}
	}

	if *enableWikidata {
		hadSource = true
		n, err := catalogsync.SyncWikidata(db, *wikidataLimit, ua)
		if err != nil {
			log.Printf("[wikidata] 补全失败: %v", err)
			failed++
		} else {
			log.Printf("[wikidata] 中文名增量补全 %d 条", n)
			totalChanged += n
		}
	}

	if !hadSource {
		log.Fatal("至少启用一个同步源（--seed / --peeringdb / --bgptools / --wikidata）")
	}

	if totalChanged > 0 {
		ver, err := catalogsync.BumpCatalogVersion(db)
		if err != nil {
			log.Fatalf("推进 catalog_version 失败: %v", err)
		}
		log.Printf("[catalog] 总变更 %d 条，catalog_version=%d", totalChanged, ver)
	} else {
		log.Printf("[catalog] 无新增变更")
	}

	if failed > 0 {
		os.Exit(1)
	}
}
