package repository

import (
	"database/sql"
	"strings"
	"unicode"
)

// HostingProvider 托管商信息
type HostingProvider struct {
	ID             int64
	ASN            uint64
	Name           string
	NormalizedName string
	Category       string
	Confidence     int
}

// HostingProviderRepository 托管商仓库
type HostingProviderRepository struct {
	db *sql.DB
}

// NewHostingProviderRepository 创建托管商仓库
func NewHostingProviderRepository(db *sql.DB) *HostingProviderRepository {
	return &HostingProviderRepository{db: db}
}

// FindByASN 根据 ASN 查询托管商
// 排除 hosting_provider_exclusions 里人工核实过的误收录 ASN（银行/学校等自有网络），
// 覆盖全部数据来源，不管这条记录是从 X4BNet、custom_asns 还是任何未来的导入路径进来的。
func (r *HostingProviderRepository) FindByASN(asn uint64) (*HostingProvider, error) {
	query := `
		SELECT id, asn, name, normalized_name, category, confidence
		FROM hosting_providers
		WHERE asn = ?
		  AND asn NOT IN (SELECT asn FROM hosting_provider_exclusions)
	`

	var result HostingProvider
	var category sql.NullString

	err := r.db.QueryRow(query, asn).Scan(
		&result.ID, &result.ASN, &result.Name,
		&result.NormalizedName, &category, &result.Confidence,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if category.Valid {
		result.Category = category.String
	}

	return &result, nil
}

// FindByNormalizedName 根据规范化名称精确匹配（同样排除 hosting_provider_exclusions）
func (r *HostingProviderRepository) FindByNormalizedName(name string) (*HostingProvider, error) {
	normalized := NormalizeName(name)

	query := `
		SELECT id, asn, name, normalized_name, category, confidence
		FROM hosting_providers
		WHERE normalized_name = ?
		  AND asn NOT IN (SELECT asn FROM hosting_provider_exclusions)
		LIMIT 1
	`

	var result HostingProvider
	var category sql.NullString

	err := r.db.QueryRow(query, normalized).Scan(
		&result.ID, &result.ASN, &result.Name,
		&result.NormalizedName, &category, &result.Confidence,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if category.Valid {
		result.Category = category.String
	}

	return &result, nil
}

// Insert 插入托管商
func (r *HostingProviderRepository) Insert(provider *HostingProvider) error {
	query := `
		INSERT OR REPLACE INTO hosting_providers
		(asn, name, normalized_name, category, confidence)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		provider.ASN, provider.Name, provider.NormalizedName,
		provider.Category, provider.Confidence,
	)
	return err
}

// InsertBatch 批量插入托管商
func (r *HostingProviderRepository) InsertBatch(providers []HostingProvider) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO hosting_providers
		(asn, name, normalized_name, category, confidence)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range providers {
		_, err := stmt.Exec(p.ASN, p.Name, p.NormalizedName, p.Category, p.Confidence)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Count 统计记录数
func (r *HostingProviderRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM hosting_providers").Scan(&count)
	return count, err
}

// HostingProviderExclusion 人工核实过的误收录 ASN
type HostingProviderExclusion struct {
	ASN    uint64
	Reason string
}

// AddExclusion 登记一条误收录 ASN：立即在 FindByASN/FindByNormalizedName/GetAll 生效，
// 不需要重新导入数据或重启服务
func (r *HostingProviderRepository) AddExclusion(asn uint64, reason string) error {
	_, err := r.db.Exec(
		`INSERT OR REPLACE INTO hosting_provider_exclusions (asn, reason) VALUES (?, ?)`,
		asn, reason,
	)
	return err
}

// ListExclusions 列出当前所有排除项（供排查/审计用）
func (r *HostingProviderRepository) ListExclusions() ([]HostingProviderExclusion, error) {
	rows, err := r.db.Query(`SELECT asn, reason FROM hosting_provider_exclusions ORDER BY asn`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HostingProviderExclusion
	for rows.Next() {
		var e HostingProviderExclusion
		if err := rows.Scan(&e.ASN, &e.Reason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetAll 获取所有托管商（用于缓存预热，同样排除 hosting_provider_exclusions）
func (r *HostingProviderRepository) GetAll() ([]HostingProvider, error) {
	query := `
		SELECT id, asn, name, normalized_name, category, confidence
		FROM hosting_providers
		WHERE asn NOT IN (SELECT asn FROM hosting_provider_exclusions)
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []HostingProvider
	for rows.Next() {
		var p HostingProvider
		var category sql.NullString
		if err := rows.Scan(&p.ID, &p.ASN, &p.Name, &p.NormalizedName, &category, &p.Confidence); err != nil {
			return nil, err
		}
		if category.Valid {
			p.Category = category.String
		}
		providers = append(providers, p)
	}

	return providers, rows.Err()
}

// NormalizeName 规范化名称（用于匹配）
// 去除标点、转小写、压缩空格
func NormalizeName(name string) string {
	var builder strings.Builder
	lastSpace := true // 避免开头空格

	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			lastSpace = false
		} else if unicode.IsSpace(r) && !lastSpace {
			builder.WriteRune(' ')
			lastSpace = true
		}
	}

	result := builder.String()
	return strings.TrimSpace(result)
}
