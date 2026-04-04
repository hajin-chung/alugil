package config

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	defaultListenAddr = ":8080"
	defaultLogPath    = "./alugil.log"
)

type Config struct {
	ListenAddr string           `yaml:"listen_addr"`
	LogPath    string           `yaml:"log_path"`
	Services   map[string][]int `yaml:"services"`
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = defaultListenAddr
	}
	if c.LogPath == "" {
		c.LogPath = defaultLogPath
	}
}

func (c Config) Validate() error {
	if len(c.Services) == 0 {
		return errors.New("services map must not be empty")
	}

	for service, ports := range c.Services {
		if service == "" {
			return errors.New("service name must not be empty")
		}
		if len(ports) == 0 {
			return fmt.Errorf("service %q must allow at least one port", service)
		}

		seen := make(map[int]struct{}, len(ports))
		for _, port := range ports {
			if port < 1 || port > 65535 {
				return fmt.Errorf("service %q has invalid port %d", service, port)
			}
			if _, ok := seen[port]; ok {
				return fmt.Errorf("service %q has duplicate port %d", service, port)
			}
			seen[port] = struct{}{}
		}
	}

	return nil
}

func (c Config) Allows(service string, port int) bool {
	ports, ok := c.Services[service]
	if !ok {
		return false
	}
	for _, allowed := range ports {
		if allowed == port {
			return true
		}
	}
	return false
}

func (c Config) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for service := range c.Services {
		names = append(names, service)
	}
	sort.Strings(names)
	return names
}
