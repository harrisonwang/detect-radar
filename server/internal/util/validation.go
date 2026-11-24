package util

import (
	"net"
	"net/mail"
	"net/netip"
	"regexp"
	"strings"
)

var (
	scanIDRe        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	cgnatPrefix     = netip.MustParsePrefix("100.64.0.0/10")
	benchmarkPrefix = netip.MustParsePrefix("198.18.0.0/15")
)

// CanonicalIP 将 IPv4/IPv6 规范化为标准文本格式（RFC 5952 压缩小写，并解包 ::ffff:a.b.c.d）。
// 非法 IP 原样返回去空白后的字符串，避免吞掉调用方上下文。
func CanonicalIP(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return s
	}
	return addr.Unmap().String()
}

// IsGlobalPublicIP 判断字符串是否为合法的公网单播 IP（排除内网、回环、链路本地、ULA、CGNAT 与 198.18/15 虚拟网段）。
func IsGlobalPublicIP(raw string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	if addr.Is4() && (cgnatPrefix.Contains(addr) || benchmarkPrefix.Contains(addr)) {
		return false
	}
	return true
}

// IsValidEmailAddress 只接受单个纯邮箱地址，不接受显示名称或邮件头。
func IsValidEmailAddress(value string) bool {
	if len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Name == "" && address.Address == value
}

// IsValidScanID 校验客户端提交的 scan_id：仅允许字母/数字/连字符/下划线，长度 1-64。
// 该 ID 会作为内存 store 的 key 并回显，须防止超长或注入类输入。
func IsValidScanID(id string) bool {
	return scanIDRe.MatchString(id)
}

// IsValidIP checks if a string is a valid IPv4 or IPv6 address
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsPrivateIP checks if an IP address is a private/local address
// This includes RFC 1918 (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16),
// loopback (127.0.0.0/8, ::1), link-local, and other non-routable addresses
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
