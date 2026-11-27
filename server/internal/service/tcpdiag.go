package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sockDiagByFamily = 20
	nlmsgTypeError   = 2
	nlmFRequest      = 1
	afInet           = 2
	afInet6          = 10
	ipProtoTCP       = 6
	inetDiagInfoExt  = 1 << 1
	inetDiagInfoAttr = 2

	minPlausibleSndMSS  = 500
	maxPlausibleSndMSS  = 9000
	minPlausibleEstMTU  = 576
	maxPlausibleEstMTU  = 9000
	standardEthernetMTU = 1500
	ifaceMTUCacheTTL    = 60 * time.Second
)

type TCPTuple struct {
	ClientIP   string
	ClientPort int
	ServerIP   string
	ServerPort int
}

type TCPConnMeta struct {
	PeerMSS    int
	EstMTU     int
	IPVersion  int
	SndMSS     int
	PMTU       int
	Advmss     int
	ServerMTU  int
	MTUClamped bool
	Timestamps bool
	PeerWScale int
}

type ifaceMTUCacheState struct {
	mu        sync.Mutex
	updatedAt time.Time
	byIP      map[string]int
}

var ifaceMTUCache ifaceMTUCacheState

var lookupTCPConnMeta = querySockDiag

func InspectTCPConnection(clientIP, clientPort, serverIP, serverPort string) (TCPConnMeta, bool) {
	tuple, ok := ParseTCPTupleHeaders(clientIP, clientPort, serverIP, serverPort)
	if !ok {
		return TCPConnMeta{}, false
	}
	return lookupTCPConnMeta(tuple)
}

func ParseTCPTupleHeaders(clientIP, clientPort, serverIP, serverPort string) (TCPTuple, bool) {
	cli := net.ParseIP(strings.TrimSpace(clientIP))
	srv := net.ParseIP(strings.TrimSpace(serverIP))
	if cli == nil || srv == nil {
		return TCPTuple{}, false
	}
	cPort, err := strconv.Atoi(strings.TrimSpace(clientPort))
	if err != nil || cPort < 1 || cPort > 65535 {
		return TCPTuple{}, false
	}
	sPort, err := strconv.Atoi(strings.TrimSpace(serverPort))
	if err != nil || sPort < 1 || sPort > 65535 {
		return TCPTuple{}, false
	}
	c4, s4 := cli.To4(), srv.To4()
	if (c4 != nil) != (s4 != nil) {
		return TCPTuple{}, false
	}
	if c4 != nil {
		return TCPTuple{
			ClientIP:   c4.String(),
			ClientPort: cPort,
			ServerIP:   s4.String(),
			ServerPort: sPort,
		}, true
	}
	return TCPTuple{
		ClientIP:   cli.To16().String(),
		ClientPort: cPort,
		ServerIP:   srv.To16().String(),
		ServerPort: sPort,
	}, true
}

func BuildSockDiagRequest(t TCPTuple) ([]byte, int, error) {
	cli := net.ParseIP(t.ClientIP)
	srv := net.ParseIP(t.ServerIP)
	if cli == nil || srv == nil {
		return nil, 0, errors.New("invalid ip in tuple")
	}
	if t.ClientPort < 1 || t.ClientPort > 65535 || t.ServerPort < 1 || t.ServerPort > 65535 {
		return nil, 0, errors.New("invalid port in tuple")
	}

	c4, s4 := cli.To4(), srv.To4()
	if (c4 != nil) != (s4 != nil) {
		return nil, 0, errors.New("address family mismatch")
	}

	var family uint8
	var ipVersion int
	var srcBytes, dstBytes [16]byte
	if c4 != nil {
		family = afInet
		ipVersion = 4
		copy(srcBytes[:4], s4)
		copy(dstBytes[:4], c4)
	} else {
		family = afInet6
		ipVersion = 6
		copy(srcBytes[:], srv.To16())
		copy(dstBytes[:], cli.To16())
	}

	req := make([]byte, 72)
	binary.LittleEndian.PutUint32(req[0:4], 72)
	binary.LittleEndian.PutUint16(req[4:6], sockDiagByFamily)
	binary.LittleEndian.PutUint16(req[6:8], nlmFRequest)
	binary.LittleEndian.PutUint32(req[8:12], 1)

	req[16] = family
	req[17] = ipProtoTCP
	req[18] = inetDiagInfoExt
	binary.LittleEndian.PutUint32(req[20:24], 0xffffffff)

	binary.BigEndian.PutUint16(req[24:26], uint16(t.ServerPort))
	binary.BigEndian.PutUint16(req[26:28], uint16(t.ClientPort))
	copy(req[28:44], srcBytes[:])
	copy(req[44:60], dstBytes[:])
	for i := 64; i < 72; i++ {
		req[i] = 0xff
	}
	return req, ipVersion, nil
}

func DecodeTCPInfo(info []byte, ipVersion int) (TCPConnMeta, bool) {
	if len(info) < 88 || (ipVersion != 4 && ipVersion != 6) {
		return TCPConnMeta{}, false
	}
	options := info[5]
	ts := (options & 1) != 0
	peerWScale := int(info[6] & 0x0f)
	sndMSS := int(binary.LittleEndian.Uint32(info[16:20]))
	if sndMSS < minPlausibleSndMSS || sndMSS > maxPlausibleSndMSS {
		return TCPConnMeta{}, false
	}
	pmtu := int(binary.LittleEndian.Uint32(info[60:64]))
	if pmtu < minPlausibleEstMTU || pmtu > maxPlausibleEstMTU {
		pmtu = 0
	}
	advmss := int(binary.LittleEndian.Uint32(info[84:88]))
	if advmss < minPlausibleSndMSS || advmss > maxPlausibleSndMSS {
		advmss = 0
	}
	peerMSS := sndMSS
	if ts {
		peerMSS += 12
	}
	hdrBytes := 40
	if ipVersion == 6 {
		hdrBytes = 60
	}
	estMTU := peerMSS + hdrBytes
	if estMTU < minPlausibleEstMTU || estMTU > maxPlausibleEstMTU {
		return TCPConnMeta{}, false
	}
	return TCPConnMeta{
		PeerMSS:    peerMSS,
		EstMTU:     estMTU,
		IPVersion:  ipVersion,
		SndMSS:     sndMSS,
		PMTU:       pmtu,
		Advmss:     advmss,
		Timestamps: ts,
		PeerWScale: peerWScale,
	}, true
}

func ApplyServerMTU(meta *TCPConnMeta, ifaceMTU int) {
	if meta == nil {
		return
	}
	ceiling := ifaceMTU
	if ceiling < minPlausibleEstMTU || ceiling > maxPlausibleEstMTU {
		ceiling = meta.PMTU
	}
	if ceiling < minPlausibleEstMTU || ceiling > maxPlausibleEstMTU {
		return
	}
	meta.ServerMTU = ceiling
	if ceiling < standardEthernetMTU && meta.EstMTU >= ceiling-4 {
		meta.MTUClamped = true
	}
}

func canonicalIPKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.To16().String()
}

func lookupServerInterfaceMTU(serverIP string) int {
	parsed := net.ParseIP(strings.TrimSpace(serverIP))
	key := canonicalIPKey(parsed)
	if key == "" {
		return 0
	}

	now := time.Now()
	ifaceMTUCache.mu.Lock()
	defer ifaceMTUCache.mu.Unlock()

	if ifaceMTUCache.byIP == nil || now.Sub(ifaceMTUCache.updatedAt) >= ifaceMTUCacheTTL {
		ifaceMTUCache.byIP = collectInterfaceMTUMap()
		ifaceMTUCache.updatedAt = now
	}
	return ifaceMTUCache.byIP[key]
}

func collectInterfaceMTUMap() map[string]int {
	ifaces, err := net.Interfaces()
	if err != nil {
		return map[string]int{}
	}
	byIP := make(map[string]int)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.MTU < minPlausibleEstMTU || iface.MTU > maxPlausibleEstMTU {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if key := canonicalIPKey(ip); key != "" {
				byIP[key] = iface.MTU
			}
		}
	}
	return byIP
}

func ParseSockDiagReply(buf []byte, ipVersion int) (TCPConnMeta, error) {
	if len(buf) < 16 {
		return TCPConnMeta{}, fmt.Errorf("short netlink reply: %d", len(buf))
	}
	nlLen := int(binary.LittleEndian.Uint32(buf[0:4]))
	nlType := binary.LittleEndian.Uint16(buf[4:6])
	if nlLen < 16 || nlLen > len(buf) {
		return TCPConnMeta{}, fmt.Errorf("invalid netlink length: %d (buf %d)", nlLen, len(buf))
	}
	if nlType == nlmsgTypeError {
		if nlLen < 20 {
			return TCPConnMeta{}, errors.New("short netlink error")
		}
		code := int32(binary.LittleEndian.Uint32(buf[16:20]))
		return TCPConnMeta{}, fmt.Errorf("netlink error code %d", code)
	}
	if nlType != sockDiagByFamily {
		return TCPConnMeta{}, fmt.Errorf("unexpected netlink message type: %d", nlType)
	}
	if nlLen < 88 {
		return TCPConnMeta{}, fmt.Errorf("short inet_diag_msg: %d", nlLen)
	}

	attrs := buf[88:nlLen]
	for len(attrs) >= 4 {
		rtaLen := int(binary.LittleEndian.Uint16(attrs[0:2]))
		rtaType := binary.LittleEndian.Uint16(attrs[2:4])
		if rtaLen < 4 || rtaLen > len(attrs) {
			break
		}
		if rtaType == inetDiagInfoAttr {
			meta, ok := DecodeTCPInfo(attrs[4:rtaLen], ipVersion)
			if !ok {
				return TCPConnMeta{}, errors.New("invalid tcp_info payload")
			}
			return meta, nil
		}
		step := (rtaLen + 3) &^ 3
		if step > len(attrs) {
			break
		}
		attrs = attrs[step:]
	}
	return TCPConnMeta{}, errors.New("INET_DIAG_INFO attribute missing")
}
