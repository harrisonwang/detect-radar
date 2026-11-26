package repository

import (
	"database/sql"
	"net"
)

type CloudIPRange struct {
	ID        int64
	Provider  string
	CIDR      string
	IPStart   net.IP
	IPEnd     net.IP
	Region    string
	Service   string
	IPVersion int
}

type CloudIPRangeRepository struct {
	db *sql.DB
}

func NewCloudIPRangeRepository(db *sql.DB) *CloudIPRangeRepository {
	return &CloudIPRangeRepository{db: db}
}

func (r *CloudIPRangeRepository) FindByIP(ip net.IP) (*CloudIPRange, error) {
	ipBytes := ipToBytes(ip)
	if ipBytes == nil {
		return nil, nil
	}

	ipVersion := 4
	if ip.To4() == nil {
		ipVersion = 6
	}

	query := `
		SELECT id, provider, cidr, ip_start, ip_end, region, service, ip_version
		FROM cloud_ip_ranges
		WHERE ip_version = ? AND ip_start <= ? AND ip_end >= ?
		LIMIT 1
	`

	var result CloudIPRange
	var ipStart, ipEnd []byte

	err := r.db.QueryRow(query, ipVersion, ipBytes, ipBytes).Scan(
		&result.ID, &result.Provider, &result.CIDR,
		&ipStart, &ipEnd, &result.Region, &result.Service, &result.IPVersion,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	result.IPStart = bytesToIP(ipStart, ipVersion)
	result.IPEnd = bytesToIP(ipEnd, ipVersion)

	return &result, nil
}

func (r *CloudIPRangeRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM cloud_ip_ranges").Scan(&count)
	return count, err
}

func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}

func bytesToIP(b []byte, version int) net.IP {
	if version == 4 && len(b) == 4 {
		return net.IP(b)
	}
	if version == 6 && len(b) == 16 {
		return net.IP(b)
	}
	return nil
}
