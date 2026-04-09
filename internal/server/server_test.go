package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alugil/internal/config"
)

func newTestServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	if cfg.LogPath == "" {
		cfg.LogPath = filepath.Join(t.TempDir(), "alugil.log")
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestHealth(t *testing.T) {
	s := newTestServer(t, config.Config{Services: map[string][]int{"docmost": {3000}}})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !body["ok"] {
		t.Fatalf("body = %#v, want ok=true", body)
	}
}

func TestProxyRouting(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Path", r.URL.Path)
		w.Header().Set("X-Got-Query", r.URL.RawQuery)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	serviceName := "docmost"
	servicePort := 3000
	s := newTestServer(t, config.Config{Services: map[string][]int{serviceName: {servicePort}}})
	s.transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == net.JoinHostPort(serviceName, strconv.Itoa(servicePort)) {
				addr = backendAddr
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/docmost/3000/api/health?foo=bar", strings.NewReader("payload"))
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if rr.Header().Get("X-Got-Path") != "/api/health" {
		t.Fatalf("forwarded path = %q", rr.Header().Get("X-Got-Path"))
	}
	if rr.Header().Get("X-Got-Query") != "foo=bar" {
		t.Fatalf("forwarded query = %q", rr.Header().Get("X-Got-Query"))
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestRejectsInvalidTargets(t *testing.T) {
	s := newTestServer(t, config.Config{Services: map[string][]int{"docmost": {3000}}})

	tests := []struct {
		path   string
		status int
		body   string
	}{
		{path: "/nope/3000/health", status: http.StatusNotFound, body: "not found\n"},
		{path: "/docmost/9999/health", status: http.StatusNotFound, body: "not found\n"},
		{path: "/docmost/abc/health", status: http.StatusBadRequest, body: "invalid port\n"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)

		if rr.Code != tt.status {
			t.Fatalf("path %s: status = %d, want %d", tt.path, rr.Code, tt.status)
		}
		if rr.Body.String() != tt.body {
			t.Fatalf("path %s: body = %q, want %q", tt.path, rr.Body.String(), tt.body)
		}
	}
}

func TestCookieContextSet(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	serviceName := "docmost"
	servicePort := 3000
	s := newTestServer(t, config.Config{Services: map[string][]int{serviceName: {servicePort}}})
	s.transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == net.JoinHostPort(serviceName, strconv.Itoa(servicePort)) {
				addr = backendAddr
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/docmost/3000/api/health", nil)
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	cookie := rr.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected cookie to be set")
	}
	if cookie[0].Name != "alugil_ctx" {
		t.Fatalf("cookie name = %q, want %q", cookie[0].Name, "alugil_ctx")
	}
	if cookie[0].Value != "docmost:3000" {
		t.Fatalf("cookie value = %q, want %q", cookie[0].Value, "docmost:3000")
	}
}

func TestCookieContextFallback(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	serviceName := "filebrowser"
	servicePort := 80
	s := newTestServer(t, config.Config{Services: map[string][]int{"filebrowser": {80}, "docmost": {3000}}})
	s.transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == net.JoinHostPort(serviceName, strconv.Itoa(servicePort)) {
				addr = backendAddr
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
	req.AddCookie(&http.Cookie{Name: "alugil_ctx", Value: "filebrowser:80"})
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Header().Get("X-Got-Path") != "/assets/index.js" {
		t.Fatalf("forwarded path = %q, want %q", rr.Header().Get("X-Got-Path"), "/assets/index.js")
	}
}

func TestCookieContextDisallowedService(t *testing.T) {
	s := newTestServer(t, config.Config{Services: map[string][]int{"docmost": {3000}}})

	req := httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
	req.AddCookie(&http.Cookie{Name: "alugil_ctx", Value: "filebrowser:80"})
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCookieContextDisallowedServiceValidRoute(t *testing.T) {
	s := newTestServer(t, config.Config{Services: map[string][]int{"docmost": {3000}}})

	req := httptest.NewRequest(http.MethodGet, "/filebrowser/80/api", nil)
	req.AddCookie(&http.Cookie{Name: "alugil_ctx", Value: "filebrowser:80"})
	rr := httptest.NewRecorder()

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
