package crawler

import (
	"database/sql"
	"fmt"
	"log"
)

// (provider, cidr) 上的 UNIQUE 不是可选装饰：InsertRanges 的 INSERT OR IGNORE 靠它
// 去掉源数据里「同一 CIDR 挂多个 service/region 标签」的重复行。去掉约束就会插入重复。
//
// 两个部分索引按 ip_version 切分，是因为 IPv4 存 4 字节、IPv6 存 16 字节，
// 混在一个索引里做 BLOB 范围比较没有意义。
const schemaSQL = `
CREATE TABLE IF NOT EXISTS cloud_ip_ranges (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    provider   VARCHAR(100) NOT NULL,
    cidr       VARCHAR(50)  NOT NULL,
    ip_start   BLOB NOT NULL,
    ip_end     BLOB NOT NULL,
    region     VARCHAR(100),
    service    VARCHAR(100),
    ip_version TINYINT DEFAULT 4,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, cidr)
);
CREATE INDEX IF NOT EXISTS idx_cloud_ip_v4 ON cloud_ip_ranges(ip_start, ip_end) WHERE ip_version = 4;
CREATE INDEX IF NOT EXISTS idx_cloud_ip_v6 ON cloud_ip_ranges(ip_start, ip_end) WHERE ip_version = 6;
`

// EnsureSchema 幂等建表，使工具能对着一个不存在的文件直接开跑
func EnsureSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

// InsertRanges 用整表事务替换该厂商的全部数据：先删旧、再插新，避免下架的网段残留
func InsertRanges(db *sql.DB, provider string, ranges []Range) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // Commit 成功后此处是 no-op

	if _, err = tx.Exec("DELETE FROM cloud_ip_ranges WHERE provider = ?", provider); err != nil {
		return 0, fmt.Errorf("删除旧数据失败: %w", err)
	}

	// 部分厂商（Azure/AWS 尤其明显）在源数据里会把同一 CIDR 挂在多个 service/region 标签下
	// 重复列出；(provider, cidr) 上的 UNIQUE 约束本就只想留一行，这是预期的重复，不是错误。
	// 用 INSERT OR IGNORE 让 SQLite 静默跳过冲突行，而不是把每条重复都当"插入失败"刷屏
	// （曾把 Azure 78544 条里的 35182 条当失败逐行打日志）。
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO cloud_ip_ranges
		(provider, cidr, ip_start, ip_end, region, service, ip_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	inserted, duplicates := 0, 0
	for _, r := range ranges {
		res, err := stmt.Exec(r.Provider, r.CIDR, ipToBytes(r.IPStart), ipToBytes(r.IPEnd), r.Region, r.Service, r.IPVersion)
		if err != nil {
			log.Printf("插入失败 %s: %v", r.CIDR, err)
			continue
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			inserted++
		} else {
			duplicates++
		}
	}
	if duplicates > 0 {
		log.Printf("%s 源数据内有 %d 条重复 CIDR（同一网段挂了多个 service/region 标签），已去重保留 1 条", provider, duplicates)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return inserted, nil
}
