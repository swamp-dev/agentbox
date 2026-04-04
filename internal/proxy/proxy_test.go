package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAllowed(t *testing.T) {
	p := &EgressProxy{
		AllowedHosts: map[string]bool{
			"api.anthropic.com:443":   true,
			"api.openai.com:443":      true,
			"custom.example.com:8080": true,
		},
	}

	tests := []struct {
		name string
		host string
		want bool
	}{
		{"allowed with port", "api.anthropic.com:443", true},
		{"allowed without port defaults to 443", "api.anthropic.com", true},
		{"allowed openai", "api.openai.com:443", true},
		{"allowed custom port", "custom.example.com:8080", true},
		{"blocked host", "evil.com:443", false},
		{"blocked host no port", "evil.com", false},
		{"wrong port for allowed host", "api.anthropic.com:80", false},
		{"empty host", "", false},
		{"subdomain not matched", "sub.api.anthropic.com:443", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.isAllowed(tt.host); got != tt.want {
				t.Errorf("isAllowed(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestHandleHTTPAllowed(t *testing.T) {
	// Set up a backend server to proxy to.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	p := &EgressProxy{
		AllowedHosts: map[string]bool{
			backend.Listener.Addr().String(): true,
		},
	}

	// Build a request that looks like a proxy request (absolute URI).
	req := httptest.NewRequest(http.MethodGet, backend.URL+"/test", nil)
	req.Host = backend.Listener.Addr().String()

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "hello from backend" {
		t.Errorf("expected 'hello from backend', got %q", string(body))
	}
	if rr.Header().Get("X-Backend") != "ok" {
		t.Error("expected X-Backend header to be forwarded")
	}
}

func TestHandleHTTPBlocked(t *testing.T) {
	p := &EgressProxy{
		AllowedHosts: map[string]bool{},
	}

	req := httptest.NewRequest(http.MethodGet, "http://evil.com/steal", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestHandleCONNECTBlocked(t *testing.T) {
	p := &EgressProxy{
		AllowedHosts: map[string]bool{
			"api.anthropic.com:443": true,
		},
	}

	req := httptest.NewRequest(http.MethodConnect, "evil.com:443", nil)
	req.Host = "evil.com:443"
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blocked CONNECT, got %d", rr.Code)
	}
}

func TestEmptyAllowlist(t *testing.T) {
	p := &EgressProxy{
		AllowedHosts: map[string]bool{},
	}

	req := httptest.NewRequest(http.MethodGet, "http://anything.com/path", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 with empty allowlist, got %d", rr.Code)
	}
}

// startTestProxy starts an EgressProxy on a random port and returns its address.
// The proxy is shut down when the test finishes.
func startTestProxy(t *testing.T, allowed map[string]bool) string {
	t.Helper()
	p := &EgressProxy{AllowedHosts: allowed}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: p}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func TestHandleCONNECTAllowed(t *testing.T) {
	// Start a TCP server that reads a line, echoes it back, then closes.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()

	go func() {
		for {
			conn, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()

	echoAddr := echoLn.Addr().String()

	// Start proxy that allows the echo server.
	proxyAddr := startTestProxy(t, map[string]bool{echoAddr: true})

	// Connect to the proxy and issue a CONNECT request.
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", echoAddr, echoAddr)
	if err != nil {
		t.Fatal(err)
	}

	// Read the HTTP response line manually to avoid http.ReadResponse draining the tunnel.
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if statusLine != "HTTP/1.1 200 OK\r\n" {
		t.Fatalf("expected 200 OK status line, got %q", statusLine)
	}
	// Read remaining headers until blank line.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}

	// The tunnel is now open — send data through and verify the echo.
	msg := "hello through tunnel"
	_, err = fmt.Fprint(conn, msg)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(br, buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != msg {
		t.Errorf("expected echoed %q, got %q", msg, string(buf))
	}
}

func TestHandleCONNECTBlockedTunnel(t *testing.T) {
	// Start proxy that allows nothing.
	proxyAddr := startTestProxy(t, map[string]bool{})

	tests := []struct {
		name       string
		target     string
		wantStatus int
	}{
		{"blocked host", "evil.com:443", http.StatusForbidden},
		{"unlisted host with port", "notlisted.com:8080", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", tt.target, tt.target)
			if err != nil {
				t.Fatal(err)
			}

			br := bufio.NewReader(conn)
			resp, err := http.ReadResponse(br, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("CONNECT %s: expected %d, got %d", tt.target, tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestHandleCONNECTMalformed(t *testing.T) {
	// Start proxy allowing everything (to ensure rejection is due to malformed request).
	proxyAddr := startTestProxy(t, map[string]bool{"*:443": true})

	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send a malformed HTTP request (not valid HTTP).
	_, err = fmt.Fprint(conn, "NOT-HTTP GARBAGE\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		// Server closed connection or returned unparseable response — both acceptable
		// for a malformed request.
		return
	}
	resp.Body.Close()

	// If we got a response, it should be an error status.
	if resp.StatusCode == http.StatusOK {
		t.Error("expected error status for malformed request, got 200")
	}
}
