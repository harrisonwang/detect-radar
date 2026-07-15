package repository

import (
	"database/sql"
	"strings"
)

// HostingKeyword 托管关键词
type HostingKeyword struct {
	ID           int64
	Keyword      string
	Provider     string
	Weight       int
	IsStandalone bool
}

// HostingKeywordRepository 托管关键词仓库
type HostingKeywordRepository struct {
	db       *sql.DB
	keywords []HostingKeyword // 内存缓存（关键词数量少，全量加载）
}

// NewHostingKeywordRepository 创建托管关键词仓库
func NewHostingKeywordRepository(db *sql.DB) *HostingKeywordRepository {
	return &HostingKeywordRepository{db: db}
}

// LoadAll 加载所有关键词到内存
func (r *HostingKeywordRepository) LoadAll() error {
	query := `SELECT id, keyword, provider, weight, is_standalone FROM hosting_keywords`

	rows, err := r.db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	r.keywords = nil
	for rows.Next() {
		var k HostingKeyword
		if err := rows.Scan(&k.ID, &k.Keyword, &k.Provider, &k.Weight, &k.IsStandalone); err != nil {
			return err
		}
		r.keywords = append(r.keywords, k)
	}

	return rows.Err()
}

// MatchTokens 匹配 Token（切分后匹配）
// 返回匹配的关键词和总权重
func (r *HostingKeywordRepository) MatchTokens(org string) (matches []HostingKeyword, totalWeight int) {
	tokens := tokenize(org)

	for _, kw := range r.keywords {
		if kw.IsStandalone {
			// 独立关键词：必须完整匹配一个 token
			for _, token := range tokens {
				if token == kw.Keyword {
					matches = append(matches, kw)
					totalWeight += kw.Weight
					break
				}
			}
		} else {
			// 子串匹配
			orgLower := strings.ToLower(org)
			if strings.Contains(orgLower, kw.Keyword) {
				matches = append(matches, kw)
				totalWeight += kw.Weight
			}
		}
	}

	return matches, totalWeight
}

// GetBestMatch 获取最佳匹配
func (r *HostingKeywordRepository) GetBestMatch(org string) *HostingKeyword {
	matches, _ := r.MatchTokens(org)
	if len(matches) == 0 {
		return nil
	}

	// 返回权重最高的
	best := &matches[0]
	for i := 1; i < len(matches); i++ {
		if matches[i].Weight > best.Weight {
			best = &matches[i]
		}
	}

	return best
}

// Insert 插入关键词
func (r *HostingKeywordRepository) Insert(keyword *HostingKeyword) error {
	query := `
		INSERT OR REPLACE INTO hosting_keywords
		(keyword, provider, weight, is_standalone)
		VALUES (?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		keyword.Keyword, keyword.Provider, keyword.Weight, keyword.IsStandalone,
	)
	return err
}

// InsertBatch 批量插入关键词
func (r *HostingKeywordRepository) InsertBatch(keywords []HostingKeyword) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO hosting_keywords
		(keyword, provider, weight, is_standalone)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range keywords {
		_, err := stmt.Exec(k.Keyword, k.Provider, k.Weight, k.IsStandalone)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Count 统计记录数
func (r *HostingKeywordRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM hosting_keywords").Scan(&count)
	return count, err
}

// tokenize 切分组织名称为 tokens
func tokenize(org string) []string {
	// 转小写并按非字母数字字符分割
	org = strings.ToLower(org)
	var tokens []string
	var current strings.Builder

	for _, r := range org {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// InitDefaultKeywords 初始化默认关键词
// 极保守策略：宁可漏判，不可错判
// 只保留 ASN 名称中出现时 100% 确定是机房的关键词
func (r *HostingKeywordRepository) InitDefaultKeywords() error {
	keywords := []HostingKeyword{
		// 只有这 3 个词出现在 ASN 名称中时，可以 100% 确定是机房
		{Keyword: "datacenter", Provider: "Datacenter", Weight: 90, IsStandalone: false},
		{Keyword: "data center", Provider: "Datacenter", Weight: 90, IsStandalone: false},
		{Keyword: "colocation", Provider: "Colocation", Weight: 90, IsStandalone: false},
	}

	return r.InsertBatch(keywords)
}
