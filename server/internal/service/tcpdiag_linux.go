//go:build linux

package service

import "golang.org/x/sys/unix"

func querySockDiag(t TCPTuple) (TCPConnMeta, bool) {
	req, ipVersion, err := BuildSockDiagRequest(t)
	if err != nil {
		return TCPConnMeta{}, false
	}

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return TCPConnMeta{}, false
	}
	defer unix.Close(fd)

	tv := unix.Timeval{Sec: 0, Usec: 50000}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &tv)

	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return TCPConnMeta{}, false
	}

	buf := make([]byte, 8192)
	n, _, err := unix.Recvfrom(fd, buf, 0)
	if err != nil || n < 16 {
		return TCPConnMeta{}, false
	}

	meta, err := ParseSockDiagReply(buf[:n], ipVersion)
	if err != nil {
		return TCPConnMeta{}, false
	}
	ApplyServerMTU(&meta, lookupServerInterfaceMTU(t.ServerIP))
	return meta, true
}
