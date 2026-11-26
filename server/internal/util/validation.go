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

func IsValidEmailAddress(value string) bool {
	if len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Name == "" && address.Address == value
}

func IsValidScanID(id string) bool {
	return scanIDRe.MatchString(id)
}

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
