package service

import (
	"encoding/binary"
	"errors"
	"log"
	"net"
	"net/netip"
	"sync"
)

const (
	stunBindingRequest         uint16 = 0x0001
	stunBindingSuccessResponse uint16 = 0x0101
	stunMagicCookie            uint32 = 0x2112A442
	stunAttrXORMappedAddress   uint16 = 0x0020
	stunFamilyIPv4             byte   = 0x01
	stunFamilyIPv6             byte   = 0x02
)

type STUNService struct {
	mu      sync.Mutex
	conn    net.PacketConn
	running bool
	done    chan struct{}
}

func NewSTUNService() *STUNService {
	return &STUNService{}
}

func (s *STUNService) Start(addr string) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.conn = pc
	s.running = true
	s.done = done
	s.mu.Unlock()

	log.Printf("[STUN] Listening on %s (UDP)", addr)
	go s.serve(pc, done)
	return nil
}

func (s *STUNService) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr()
}

func (s *STUNService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	pc := s.conn
	done := s.done
	s.conn = nil
	s.mu.Unlock()

	if pc != nil {
		_ = pc.Close()
	}
	if done != nil {
		<-done
	}
}

func (s *STUNService) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *STUNService) serve(pc net.PacketConn, done chan struct{}) {
	defer close(done)

	buf := make([]byte, 1500)
	var out [44]byte
	for {
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			if !s.isRunning() || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		udpAddr, ok := raddr.(*net.UDPAddr)
		if !ok {
			continue
		}
		resp := writeSTUNBindingResponse(out[:], buf[:n], udpAddr)
		if len(resp) > 0 {
			_, _ = pc.WriteTo(resp, raddr)
		}
	}
}

func writeSTUNBindingResponse(dst, req []byte, remote *net.UDPAddr) []byte {
	if len(req) < 20 || remote == nil || remote.Port < 0 || remote.Port > 65535 {
		return nil
	}
	if binary.BigEndian.Uint16(req[0:2]) != stunBindingRequest {
		return nil
	}
	msgLen := binary.BigEndian.Uint16(req[2:4])
	if msgLen%4 != 0 || 20+int(msgLen) > len(req) {
		return nil
	}
	if binary.BigEndian.Uint32(req[4:8]) != stunMagicCookie {
		return nil
	}

	ip, ok := netip.AddrFromSlice(remote.IP)
	if !ok {
		return nil
	}
	ip = ip.Unmap()
	xPort := uint16(remote.Port) ^ uint16(stunMagicCookie>>16)

	if ip.Is4() {
		if len(dst) < 32 {
			return nil
		}
		binary.BigEndian.PutUint16(dst[0:2], stunBindingSuccessResponse)
		binary.BigEndian.PutUint16(dst[2:4], 12)
		binary.BigEndian.PutUint32(dst[4:8], stunMagicCookie)
		copy(dst[8:20], req[8:20])

		binary.BigEndian.PutUint16(dst[20:22], stunAttrXORMappedAddress)
		binary.BigEndian.PutUint16(dst[22:24], 8)
		dst[24] = 0
		dst[25] = stunFamilyIPv4
		binary.BigEndian.PutUint16(dst[26:28], xPort)
		ip4 := ip.As4()
		for i := 0; i < 4; i++ {
			dst[28+i] = ip4[i] ^ req[4+i]
		}
		return dst[:32]
	}

	if ip.Is6() {
		if len(dst) < 44 {
			return nil
		}
		binary.BigEndian.PutUint16(dst[0:2], stunBindingSuccessResponse)
		binary.BigEndian.PutUint16(dst[2:4], 24)
		binary.BigEndian.PutUint32(dst[4:8], stunMagicCookie)
		copy(dst[8:20], req[8:20])

		binary.BigEndian.PutUint16(dst[20:22], stunAttrXORMappedAddress)
		binary.BigEndian.PutUint16(dst[22:24], 20)
		dst[24] = 0
		dst[25] = stunFamilyIPv6
		binary.BigEndian.PutUint16(dst[26:28], xPort)
		ip6 := ip.As16()
		for i := 0; i < 16; i++ {
			dst[28+i] = ip6[i] ^ req[4+i]
		}
		return dst[:44]
	}

	return nil
}
