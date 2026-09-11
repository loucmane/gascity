package platforminstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type metadataScriptedConnection struct {
	response  *strings.Reader
	writes    bytes.Buffer
	closed    atomic.Bool
	deadlines int
}

func (connection *metadataScriptedConnection) Read(data []byte) (int, error) {
	return connection.response.Read(data)
}

func (connection *metadataScriptedConnection) Write(data []byte) (int, error) {
	if connection.closed.Load() {
		return 0, net.ErrClosed
	}
	return connection.writes.Write(data)
}

func (connection *metadataScriptedConnection) Close() error {
	connection.closed.Store(true)
	return nil
}

func (connection *metadataScriptedConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4401}
}

func (connection *metadataScriptedConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4400}
}

func (connection *metadataScriptedConnection) SetDeadline(time.Time) error {
	connection.deadlines++
	return nil
}
func (connection *metadataScriptedConnection) SetReadDeadline(time.Time) error  { return nil }
func (connection *metadataScriptedConnection) SetWriteDeadline(time.Time) error { return nil }

func TestMetadataTransportRetainsOneConnectionAndBracketsReplies(t *testing.T) {
	script := &metadataScriptedConnection{}
	connection := &metadataConnection{conn: script}
	checks := 0
	for _, action := range []string{"health", "begin", "check"} {
		method, path, err := metadataConnectionRoute("fixture", action)
		if err != nil {
			t.Fatal(err)
		}
		script.response = strings.NewReader("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nCache-Control: no-store\r\n\r\n{}")
		request, err := http.NewRequest(method, "http://127.0.0.1:4400"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := connection.exchange(context.Background(), request, func(ctx context.Context) error {
			if script.closed.Load() {
				return errors.New("closed before inspection")
			}
			checks++
			return ctx.Err()
		})
		if err != nil || string(data) != "{}" {
			t.Fatalf("%s: %q %v", action, data, err)
		}
	}
	if checks != 6 || script.deadlines != 3 || script.closed.Load() {
		t.Fatalf("checks=%d deadlines=%d closed=%v", checks, script.deadlines, script.closed.Load())
	}
	wire := script.writes.String()
	if strings.Count(wire, "HTTP/1.1\r\n") != 3 || strings.Contains(wire, "Upgrade:") || strings.Contains(wire, "CONNECT ") {
		t.Fatal(wire)
	}
}

func TestMetadataTransportRefusalPoisonsConnectionWithoutRetry(t *testing.T) {
	for _, failure := range []string{"status", "redirect", "upgrade", "close", "http10", "chunked", "length", "truncated", "headers", "unsolicited", "cache", "identity-before", "identity-after", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			response := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nCache-Control: no-store\r\n\r\n{}"
			switch failure {
			case "status":
				response = strings.Replace(response, "200 OK", "503 Unavailable", 1)
			case "redirect":
				response = strings.Replace(response, "200 OK", "302 Found", 1)
			case "upgrade":
				response = strings.Replace(response, "Content-Length", "Upgrade: websocket\r\nContent-Length", 1)
			case "close":
				response = strings.Replace(response, "Content-Length", "Connection: close\r\nContent-Length", 1)
			case "http10":
				response = strings.Replace(response, "HTTP/1.1", "HTTP/1.0", 1)
			case "chunked":
				response = "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\n{}\r\n0\r\n\r\n"
			case "length":
				response = strings.Replace(response, "Length: 2", fmt.Sprintf("Length: %d", metadataFrameLimit+1), 1)
			case "truncated":
				response = strings.TrimSuffix(response, "}")
			case "headers":
				response = strings.Replace(response, "Content-Length", "X-Huge: "+strings.Repeat("a", 8192)+"\r\nContent-Length", 1)
			case "unsolicited":
				response += "extra reply"
			case "cache":
				response = strings.Replace(response, "no-store", "public", 1)
			}
			script := &metadataScriptedConnection{response: strings.NewReader(response)}
			connection := &metadataConnection{conn: script}
			request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:4400/v0/city/fixture/platform/metadata-lease/check", nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			checks := 0
			inspect := func(ctx context.Context) error {
				checks++
				if failure == "identity-before" || failure == "identity-after" && checks == 2 {
					return errors.New("identity lost")
				}
				return ctx.Err()
			}
			if _, err := connection.exchange(ctx, request, inspect); err == nil {
				t.Fatal("accepted failure")
			}
			writes := script.writes.Len()
			if !script.closed.Load() || !connection.failed {
				t.Fatal("connection not poisoned")
			}
			if _, err := connection.exchange(context.Background(), request, inspect); err == nil {
				t.Fatal("retried failed connection")
			}
			if script.writes.Len() != writes {
				t.Fatal("second request sent")
			}
		})
	}
}

func TestMetadataConnectionRefusesUnavailableKernelEvidence(t *testing.T) {
	connection := &metadataScriptedConnection{response: strings.NewReader("")}
	if _, err := metadataReadSocket(connection); err == nil {
		t.Fatal("accepted a connection without kernel evidence")
	}
	if _, err := io.ReadAll(connection); err != nil {
		t.Fatal(err)
	}
}
