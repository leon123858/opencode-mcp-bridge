package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddress  string
	OpenCodeURL    string
	Username       string
	Password       string
	RequestTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress:  envOrDefault("BRIDGE_LISTEN_ADDRESS", ":8080"),
		OpenCodeURL:    strings.TrimRight(envOrDefault("OPENCODE_BASE_URL", "http://127.0.0.1:4096"), "/"),
		Username:       os.Getenv("OPENCODE_SERVER_USERNAME"),
		Password:       os.Getenv("OPENCODE_SERVER_PASSWORD"),
		RequestTimeout: 30 * time.Second,
	}
	if value := os.Getenv("BRIDGE_REQUEST_TIMEOUT"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("BRIDGE_REQUEST_TIMEOUT must be a positive duration")
		}
		cfg.RequestTimeout = d
	}
	if cfg.OpenCodeURL == "" {
		return Config{}, fmt.Errorf("OPENCODE_BASE_URL cannot be empty")
	}
	parsedURL, err := url.Parse(cfg.OpenCodeURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return Config{}, fmt.Errorf("OPENCODE_BASE_URL must be an absolute HTTP(S) URL")
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
