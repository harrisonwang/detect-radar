//go:build !linux

package service

func querySockDiag(_ TCPTuple) (TCPConnMeta, bool) {
	return TCPConnMeta{}, false
}
