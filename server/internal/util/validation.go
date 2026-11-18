package util

import (
	"net"
	"net/mail"
	"regexp"
	"strings"
)

var scanIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

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
