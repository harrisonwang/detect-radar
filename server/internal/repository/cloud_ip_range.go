package repository

import (
	"database/sql"
	"encoding/binary"
	"net"
)

// CloudIPRange 云厂商 IP 范围
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

// CloudIPRangeRepository 云厂商 IP 范围仓库
type CloudIPRangeRepository struct {
	db *sql.DB
}

// NewCloudIPRangeRepository 创建云厂商 IP 范围仓库
func NewCloudIPRangeRepository(db *sql.DB) *CloudIPRangeRepository {
	return &CloudIPRangeRepository{db: db}
}

// FindByIP 查询 IP 所属的云厂商
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

// Insert 插入云厂商 IP 范围
func (r *CloudIPRangeRepository) Insert(record *CloudIPRange) error {
	ipStart := ipToBytes(record.IPStart)
	ipEnd := ipToBytes(record.IPEnd)

	query := `
		INSERT OR REPLACE INTO cloud_ip_ranges
		(provider, cidr, ip_start, ip_end, region, service, ip_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		record.Provider, record.CIDR, ipStart, ipEnd,
		record.Region, record.Service, record.IPVersion,
	)
	return err
}

// Count 统计记录数
func (r *CloudIPRangeRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM cloud_ip_ranges").Scan(&count)
	return count, err
}

// ipToBytes 将 IP 转换为字节数组（用于比较）
func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}

// bytesToIP 将字节数组转换为 IP
func bytesToIP(b []byte, version int) net.IP {
	if version == 4 && len(b) == 4 {
		return net.IP(b)
	}
	if version == 6 && len(b) == 16 {
		return net.IP(b)
	}
	return nil
}

// IPRangeFromCIDR 从 CIDR 生成 IP 范围
func IPRangeFromCIDR(cidr string) (start, end net.IP, version int, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, 0, err
	}

	start = ipNet.IP
	end = make(net.IP, len(start))
	copy(end, start)

	// 计算结束 IP
	for i := range end {
		end[i] |= ^ipNet.Mask[i]
	}

	version = 4
	if start.To4() == nil {
		version = 6
	}

	return start, end, version, nil
}

// IPToUint32 将 IPv4 转换为 uint32（用于排序和比较）
func IPToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}
