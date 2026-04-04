package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"alugil/internal/config"
)

type Server struct {
	cfg       config.Config
	logger    *slog.Logger
	logFile   *os.File
	transport http.RoundTripper
}

type route struct {
	service string
	port    int
	path    string
	url     *url.URL
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
	route, status, msg, err := s.parseRoute(r.URL.Path)
	if err != nil {
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

	proxy := httputil.NewSingleHostReverseProxy(route.url)
	proxy.Transport = s.transport
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
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
	}
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = route.path
		req.URL.RawPath = route.path
		req.URL.RawQuery = r.URL.RawQuery
		req.Host = route.url.Host
	}

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
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

func (s *Server) parseRoute(path string) (route, int, string, error) {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return route{}, http.StatusNotFound, "not found", errors.New("invalid route")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		return route{}, http.StatusBadRequest, "invalid port", errors.New("invalid port")
	}
	if !s.cfg.Allows(parts[0], port) {
		return route{}, http.StatusNotFound, "not found", errors.New("target not allowed")
	}

	upstreamPath := "/"
	if len(parts) == 3 {
		upstreamPath = "/" + parts[2]
	}

	return route{
		service: parts[0],
		port:    port,
		path:    upstreamPath,
		url: &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", parts[0], port),
			Path:   upstreamPath,
		},
	}, 0, "", nil
}

func (s *Server) log(level, msg string, fields map[string]any) {
	if s == nil || s.logger == nil {
		return
	}
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	if level == "warn" || level == "error" {
		fmt.Fprintf(os.Stdout, "%s %s", strings.ToUpper(level), msg)
		for k, v := range fields {
			fmt.Fprintf(os.Stdout, " %s=%v", k, v)
		}
		fmt.Fprintln(os.Stdout)
	}
	s.logger.Log(context.Background(), levelFromString(level), msg, args...)
}

func levelFromString(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func defaultTransport() http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
