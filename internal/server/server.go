package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"alugil/internal/config"
)

type Server struct {
	cfg       config.Config
	logger    *slog.Logger
	logFile   *os.File
	transport http.RoundTripper
}

func New(cfg config.Config) (*Server, error) {
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", cfg.LogPath, err)
	}

	return &Server{
		cfg:       cfg,
		logger:    slog.New(slog.NewJSONHandler(logFile, nil)),
		logFile:   logFile,
		transport: defaultTransport(),
	}, nil
}

func (s *Server) Close() error {
	if s == nil || s.logFile == nil {
		return nil
	}
	return s.logFile.Close()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("/", s.proxy)
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	route, status, msg, err := parseRoute(r.URL.Path)
	if err != nil {
		svc, port, ok := parseContextCookie(r)
		if ok && s.cfg.Allows(svc, port) {
			route = buildRoute(svc, port, r.URL.Path)
			s.serveProxy(w, r, route, started)
			return
		}
		http.Error(w, msg, status)
		s.log("warn", "request rejected", map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      status,
			"duration_ms": time.Since(started).Milliseconds(),
			"error":       err.Error(),
		})
		return
	}
	if !s.cfg.Allows(route.service, route.port) {
		svc, port, ok := parseContextCookie(r)
		if ok && s.cfg.Allows(svc, port) {
			route = buildRoute(svc, port, r.URL.Path)
			s.serveProxy(w, r, route, started)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		s.log("warn", "request rejected", map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      http.StatusNotFound,
			"duration_ms": time.Since(started).Milliseconds(),
			"error":       "target not allowed",
		})
		return
	}

	s.serveProxy(w, r, route, started)
}

func (s *Server) serveProxy(w http.ResponseWriter, r *http.Request, route route, started time.Time) {
	proxy := &httputil.ReverseProxy{
		Transport: s.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = route.url.Scheme
			pr.Out.URL.Host = route.url.Host
			pr.Out.URL.Path = route.path
			pr.Out.URL.RawPath = route.path
			pr.Out.URL.RawQuery = r.URL.RawQuery
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			status := http.StatusBadGateway
			if errors.Is(proxyErr, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			http.Error(rw, http.StatusText(status), status)
			s.log("error", "proxy failed", map[string]any{
				"method":      r.Method,
				"service":     route.service,
				"port":        route.port,
				"path":        route.path,
				"upstream":    route.url.String(),
				"status":      status,
				"duration_ms": time.Since(started).Milliseconds(),
				"error":       proxyErr.Error(),
			})
		},
	}

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK, ctxService: route.service, ctxPort: route.port}
	proxy.ServeHTTP(rec, r)
	s.log("info", "request complete", map[string]any{
		"method":      r.Method,
		"service":     route.service,
		"port":        route.port,
		"path":        route.path,
		"status":      rec.status,
		"duration_ms": time.Since(started).Milliseconds(),
	})
}
