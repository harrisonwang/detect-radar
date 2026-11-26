package crawler

import (
	"database/sql"
	"fmt"
	"log"
)

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

func EnsureSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

func InsertRanges(db *sql.DB, provider string, ranges []Range) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err = tx.Exec("DELETE FROM cloud_ip_ranges WHERE provider = ?", provider); err != nil {
		return 0, fmt.Errorf("删除旧数据失败: %w", err)
	}

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
