package proxy

import (
	"io"
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
