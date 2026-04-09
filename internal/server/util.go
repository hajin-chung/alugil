package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type route struct {
	service string
	port    int
	path    string
	url     *url.URL
}

type statusRecorder struct {
	http.ResponseWriter
	status     int
	cookieSet  bool
	ctxService string
	ctxPort    int
}

func parseRoute(path string) (route, int, string, error) {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return route{}, http.StatusNotFound, "not found", fmt.Errorf("invalid route")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		return route{}, http.StatusBadRequest, "invalid port", fmt.Errorf("invalid port")
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
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.cookieSet {
		setContextCookie(r.ResponseWriter, r.ctxService, r.ctxPort)
		r.cookieSet = true
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.cookieSet {
		setContextCookie(r.ResponseWriter, r.ctxService, r.ctxPort)
		r.cookieSet = true
	}
	return r.ResponseWriter.Write(b)
}

func parseContextCookie(r *http.Request) (string, int, bool) {
	c, err := r.Cookie("alugil_ctx")
	if err != nil {
		return "", 0, false
	}
	parts := strings.SplitN(c.Value, ":", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	return parts[0], port, true
}

func setContextCookie(w http.ResponseWriter, service string, port int) {
	http.SetCookie(w, &http.Cookie{
		Name:  "alugil_ctx",
		Value: fmt.Sprintf("%s:%d", service, port),
		Path:  "/",
	})
}

func buildRoute(service string, port int, path string) route {
	upstreamPath := "/"
	if len(path) > 0 {
		upstreamPath = path
	}
	return route{
		service: service,
		port:    port,
		path:    upstreamPath,
		url: &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", service, port),
			Path:   upstreamPath,
		},
	}
}
