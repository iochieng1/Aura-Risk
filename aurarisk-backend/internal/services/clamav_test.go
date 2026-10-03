package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
)

// fakeClamd accepts one connection, reads an INSTREAM payload, and replies
// FOUND when the payload contains the marker.
func fakeClamd(t *testing.T, marker string) (string, <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan []byte, 1)

	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		cmd := make([]byte, len("zINSTREAM\x00"))
		if _, err := io.ReadFull(conn, cmd); err != nil {
			return
		}
		var payload bytes.Buffer
		size := make([]byte, 4)
		for {
			if _, err := io.ReadFull(conn, size); err != nil {
				return
			}
			n := binary.BigEndian.Uint32(size)
			if n == 0 {
				break
			}
			if _, err := io.CopyN(&payload, conn, int64(n)); err != nil {
				return
			}
		}
		received <- payload.Bytes()

		if strings.Contains(payload.String(), marker) {
			conn.Write([]byte("stream: Eicar-Test-Signature FOUND\x00"))
		} else {
			conn.Write([]byte("stream: OK\x00"))
		}
	}()

	return ln.Addr().String(), received
}

func TestClamdScannerClean(t *testing.T) {
	addr, received := fakeClamd(t, "EICAR")
	data := bytes.Repeat([]byte("a"), clamdChunkSize*2+10)

	result, err := NewClamdScanner(addr).Scan(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clean {
		t.Fatalf("expected clean, got %+v", result)
	}
	if got := <-received; !bytes.Equal(got, data) {
		t.Fatalf("clamd received %d bytes, want %d", len(got), len(data))
	}
}

func TestClamdScannerInfected(t *testing.T) {
	addr, _ := fakeClamd(t, "EICAR")

	result, err := NewClamdScanner(addr).Scan(context.Background(), strings.NewReader("xxEICARxx"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Clean || result.Signature != "Eicar-Test-Signature" {
		t.Fatalf("expected infected with signature, got %+v", result)
	}
}

func TestParseClamdReplyError(t *testing.T) {
	if _, err := parseClamdReply("INSTREAM size limit exceeded. ERROR"); err == nil {
		t.Fatal("expected error")
	}
}
