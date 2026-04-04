package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"alugil/internal/config"
	"alugil/internal/server"
)

func main() {
	configPath := flag.String("config", envOrDefault("ALUGIL_CONFIG", "config.yaml"), "Path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("load config %q: %v", *configPath, err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		fatalf("initialize server: %v", err)
	}
	defer srv.Close()

	fmt.Fprintf(os.Stdout, "INFO startup listen_addr=%s log_path=%s services=%v\n", cfg.ListenAddr, cfg.LogPath, cfg.ServiceNames())
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stdout, "ERROR server stopped listen_addr=%s error=%v\n", cfg.ListenAddr, err)
		fatalf("server stopped: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
