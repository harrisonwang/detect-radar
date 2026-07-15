package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SQLiteDB SQLite 数据库管理
type SQLiteDB struct {
	db   *sql.DB
	path string
}

// SQLiteConfig SQLite 配置
type SQLiteConfig struct {
	Path string // 数据库文件路径
}

// NewSQLiteDB 创建 SQLite 数据库连接
func NewSQLiteDB(cfg SQLiteConfig) (*SQLiteDB, error) {
	// 确保目录存在
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.Path+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(1) // SQLite 只支持单写
	db.SetMaxIdleConns(1)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	sqlite := &SQLiteDB{
		db:   db,
		path: cfg.Path,
	}

	// 初始化表结构
	if err := sqlite.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return sqlite, nil
}

// initTables 初始化表结构
func (s *SQLiteDB) initTables() error {
	schema := `
	-- 云厂商 IP 范围（留空，爬虫填充）
	CREATE TABLE IF NOT EXISTS cloud_ip_ranges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider VARCHAR(100) NOT NULL,
		cidr VARCHAR(50) NOT NULL,
		ip_start BLOB NOT NULL,
		ip_end BLOB NOT NULL,
		region VARCHAR(100),
		service VARCHAR(100),
		ip_version TINYINT DEFAULT 4,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(provider, cidr)
	);

	CREATE INDEX IF NOT EXISTS idx_cloud_ip_v4 ON cloud_ip_ranges(ip_start, ip_end) WHERE ip_version = 4;
	CREATE INDEX IF NOT EXISTS idx_cloud_ip_v6 ON cloud_ip_ranges(ip_start, ip_end) WHERE ip_version = 6;

	-- 托管商名称（从 hosting_asns.go 迁移）
	CREATE TABLE IF NOT EXISTS hosting_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		asn INTEGER NOT NULL UNIQUE,
		name VARCHAR(255) NOT NULL,
		normalized_name VARCHAR(255) NOT NULL,
		category VARCHAR(50),
		confidence INTEGER DEFAULT 100,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_providers_asn ON hosting_providers(asn);
	CREATE INDEX IF NOT EXISTS idx_providers_name ON hosting_providers(normalized_name);

	-- 品牌关键词
	CREATE TABLE IF NOT EXISTS hosting_keywords (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		keyword VARCHAR(100) NOT NULL UNIQUE,
		provider VARCHAR(100) NOT NULL,
		weight INTEGER DEFAULT 50,
		is_standalone BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_keywords ON hosting_keywords(keyword);

	-- 人工核实过确属误收录的 ASN（银行/学校等机构自有网络，不是机房/VPN/托管商）。
	-- HostingProviderRepository 的查询层统一排除，覆盖全部数据来源；
	-- 线上发现新的误判可直接 INSERT 一行修复，不需要重新导入数据或重启服务。
	CREATE TABLE IF NOT EXISTS hosting_provider_exclusions (
		asn INTEGER PRIMARY KEY,
		reason VARCHAR(255) NOT NULL,
		excluded_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// DB 获取底层数据库连接
func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}

// Close 关闭数据库连接
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Path 获取数据库文件路径
func (s *SQLiteDB) Path() string {
	return s.path
}

// Stats 获取数据库统计信息
func (s *SQLiteDB) Stats() (map[string]int64, error) {
	stats := make(map[string]int64)

	tables := []string{"cloud_ip_ranges", "hosting_providers", "hosting_keywords", "hosting_provider_exclusions"}
	for _, table := range tables {
		var count int64
		err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			return nil, err
		}
		stats[table] = count
	}

	return stats, nil
}
