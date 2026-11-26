package crawler

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParsePeeringDBAccessASNs(t *testing.T) {
	sample := []byte(`{
		"data": [
			{"asn": 7922, "name": "Comcast Cable", "info_type": "Cable/DSL/ISP"},
			{"asn": 99999, "name": "Nordic Mobile Broadband", "info_type": "Cable/DSL/ISP"},
			{"asn": 16509, "name": "Amazon AWS", "info_type": "Content"}
		]
	}`)

	items, err := ParsePeeringDBAccessASNs(sample)
	if err != nil {
		t.Fatalf("ParsePeeringDBAccessASNs failed: %v", err)
	}

	byASN := make(map[uint64]AccessASN, len(items))
	for _, item := range items {
		byASN[item.ASN] = item
	}

	if got, ok := byASN[7922]; !ok || got.Country != "US" || got.Source != "builtin" {
		t.Fatalf("builtin should take precedence for AS7922, got %+v", got)
	}
	if got, ok := byASN[99999]; !ok || got.NetworkKind != "mobile" || got.Source != "peeringdb" {
		t.Fatalf("peeringdb mobile item mismatch: %+v", got)
	}
	if _, ok := byASN[16509]; ok {
		t.Fatal("non-ISP Content ASN must be filtered out")
	}
}

func TestInsertAccessASNs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := EnsureSchema(db); err != nil {
		t.Fatal(err)
	}

	n, err := InsertAccessASNs(db, BuiltinAccessASNs())
	if err != nil {
		t.Fatalf("InsertAccessASNs failed: %v", err)
	}
	if n != len(BuiltinAccessASNs()) {
		t.Fatalf("inserted = %d, want %d", n, len(BuiltinAccessASNs()))
	}

	if _, err := InsertAccessASNs(db, nil); err == nil {
		t.Fatal("empty input must return error without wiping table")
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM access_asns").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != n {
		t.Fatalf("table count = %d, want %d", count, n)
	}
}
