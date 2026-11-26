package repository

import (
	"database/sql"
	"strings"
	"unicode"
)

type HostingProvider struct {
	ID             int64
	ASN            uint64
	Name           string
	NormalizedName string
	Category       string
	Confidence     int
}

type HostingProviderRepository struct {
	db *sql.DB
}

func NewHostingProviderRepository(db *sql.DB) *HostingProviderRepository {
	return &HostingProviderRepository{db: db}
}

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

func (r *HostingProviderRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM hosting_providers").Scan(&count)
	return count, err
}

func NormalizeName(name string) string {
	var builder strings.Builder
	lastSpace := true

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
