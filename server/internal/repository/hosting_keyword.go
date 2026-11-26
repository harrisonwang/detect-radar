package repository

import (
	"database/sql"
	"strings"
)

type HostingKeyword struct {
	ID           int64
	Keyword      string
	Provider     string
	Weight       int
	IsStandalone bool
}

type HostingKeywordRepository struct {
	db       *sql.DB
	keywords []HostingKeyword
}

func NewHostingKeywordRepository(db *sql.DB) *HostingKeywordRepository {
	return &HostingKeywordRepository{db: db}
}

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

func (r *HostingKeywordRepository) MatchTokens(org string) (matches []HostingKeyword, totalWeight int) {
	tokens := tokenize(org)

	for _, kw := range r.keywords {
		if kw.IsStandalone {
			for _, token := range tokens {
				if token == kw.Keyword {
					matches = append(matches, kw)
					totalWeight += kw.Weight
					break
				}
			}
		} else {
			orgLower := strings.ToLower(org)
			if strings.Contains(orgLower, kw.Keyword) {
				matches = append(matches, kw)
				totalWeight += kw.Weight
			}
		}
	}

	return matches, totalWeight
}

func (r *HostingKeywordRepository) GetBestMatch(org string) *HostingKeyword {
	matches, _ := r.MatchTokens(org)
	if len(matches) == 0 {
		return nil
	}

	best := &matches[0]
	for i := 1; i < len(matches); i++ {
		if matches[i].Weight > best.Weight {
			best = &matches[i]
		}
	}

	return best
}

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

func (r *HostingKeywordRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM hosting_keywords").Scan(&count)
	return count, err
}

func tokenize(org string) []string {
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

func (r *HostingKeywordRepository) InitDefaultKeywords() error {
	keywords := []HostingKeyword{
		{Keyword: "datacenter", Provider: "Datacenter", Weight: 90, IsStandalone: false},
		{Keyword: "data center", Provider: "Datacenter", Weight: 90, IsStandalone: false},
		{Keyword: "colocation", Provider: "Colocation", Weight: 90, IsStandalone: false},
	}

	return r.InsertBatch(keywords)
}
