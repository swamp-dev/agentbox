// Package proxy provides an HTTP CONNECT proxy that restricts egress to allowlisted hosts.
package proxy

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// hopByHopHeaders are headers that must be removed before forwarding per RFC 7230 §6.1.
var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Proxy-Authorization",
	"Proxy-Authenticate",
	"Transfer-Encoding",
	"TE",
	"Trailers",
	"Upgrade",
	"Keep-Alive",
}

// transport used for plain HTTP forwarding. Proxy is explicitly nil to prevent
// proxy loops when the proxy container itself has HTTP_PROXY set.
var forwardTransport = &http.Transport{
	Proxy: nil,
}

// EgressProxy is an HTTP proxy that only allows CONNECT tunnels to allowlisted host:port pairs.
type EgressProxy struct {
	AllowedHosts map[string]bool
	Addr         string
}

// ServeHTTP handles both CONNECT (HTTPS) and plain HTTP requests.
func (p *EgressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *EgressProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	if !p.isAllowed(r.Host) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	dest, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer dest.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(dest, clientConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, dest)
		done <- struct{}{}
	}()
	// Wait for both directions to complete to avoid goroutine leaks.
	<-done
	<-done
}

func (p *EgressProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.isAllowed(r.Host) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Strip hop-by-hop headers before forwarding per RFC 7230 §6.1.
	for _, h := range hopByHopHeaders {
		r.Header.Del(h)
	}
	r.RequestURI = ""

	resp, err := forwardTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Strip hop-by-hop headers from response.
	for _, h := range hopByHopHeaders {
		resp.Header.Del(h)
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *EgressProxy) isAllowed(host string) bool {
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}
	return p.AllowedHosts[host]
}

// ListenAndServe starts the proxy server.
func (p *EgressProxy) ListenAndServe() error {
	server := &http.Server{
		Addr:              p.Addr,
		Handler:           p,
		ReadHeaderTimeout: 30 * time.Second,
	}
	return server.ListenAndServe()
}
