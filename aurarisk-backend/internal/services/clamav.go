package services

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"aurarisk-backend/internal/metrics"
)

const clamdChunkSize = 64 * 1024

type ScanResult struct {
	Clean     bool
	Signature string
}

// ClamdScanner streams data to clamd using the INSTREAM command.
type ClamdScanner struct {
	Addr    string
	Timeout time.Duration
}

func NewClamdScanner(addr string) *ClamdScanner {
	return &ClamdScanner{Addr: addr, Timeout: 60 * time.Second}
}

func (s *ClamdScanner) Scan(ctx context.Context, r io.Reader) (_ ScanResult, err error) {
	defer metrics.ObserveProvider(metrics.ProviderClamAV, "scan", time.Now(), &err)

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return ScanResult{}, fmt.Errorf("connect to clamd: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(s.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return ScanResult{}, fmt.Errorf("send INSTREAM: %w", err)
	}

	buf := make([]byte, clamdChunkSize)
	size := make([]byte, 4)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			binary.BigEndian.PutUint32(size, uint32(n))
			if _, err := conn.Write(size); err != nil {
				return ScanResult{}, fmt.Errorf("send chunk size: %w", err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return ScanResult{}, fmt.Errorf("send chunk: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return ScanResult{}, readErr
		}
	}
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return ScanResult{}, fmt.Errorf("terminate stream: %w", err)
	}

	reply, err := bufio.NewReader(conn).ReadString(0)
	if err != nil && err != io.EOF {
		return ScanResult{}, fmt.Errorf("read clamd reply: %w", err)
	}
	return parseClamdReply(strings.TrimRight(reply, "\x00\n"))
}

// parseClamdReply handles replies such as "stream: OK" and
// "stream: Eicar-Signature FOUND".
func parseClamdReply(reply string) (ScanResult, error) {
	switch {
	case strings.HasSuffix(reply, " OK"):
		return ScanResult{Clean: true}, nil
	case strings.HasSuffix(reply, " FOUND"):
		signature := strings.TrimSuffix(reply, " FOUND")
		if idx := strings.Index(signature, ": "); idx >= 0 {
			signature = signature[idx+2:]
		}
		return ScanResult{Clean: false, Signature: signature}, nil
	default:
		return ScanResult{}, fmt.Errorf("clamd error: %s", reply)
	}
}
