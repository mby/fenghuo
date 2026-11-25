//go:build linux

package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"syscall"
	"time"
)

func icmpLatency(ctx context.Context, addr string) (float64, string) {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return -1, "local"
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.IPPROTO_ICMP)
	if err != nil {
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
			return -1, "permission"
		}
		if errors.Is(err, syscall.EPROTONOSUPPORT) {
			return -1, "unsupported"
		}
		return -1, "local"
	}
	peer := &syscall.SockaddrInet4{}
	copy(peer.Addr[:], ip)
	if err = syscall.Connect(fd, peer); err != nil {
		syscall.Close(fd)
		return -1, "local"
	}
	f := os.NewFile(uintptr(fd), "icmp-ping")
	conn, err := net.FileConn(f)
	f.Close()
	if err != nil {
		return -1, "local"
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	payload := make([]byte, 56)
	if _, err = rand.Read(payload); err != nil {
		return -1, "local"
	}
	request := echoRequest(payload)
	start := time.Now()
	if _, err = conn.Write(request); err != nil {
		return -1, ""
	}
	buffer := make([]byte, 1500)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return -1, ""
		}
		if validEcho(buffer[:n], request) {
			return float64(time.Since(start).Microseconds()) / 1000, ""
		}
	}
}
