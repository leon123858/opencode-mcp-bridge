package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OPENCODE_BASE_URL", "")
	t.Setenv("BRIDGE_LISTEN_ADDRESS", "")
	t.Setenv("BRIDGE_REQUEST_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeURL != "http://127.0.0.1:4096" || cfg.ListenAddress != ":8080" || cfg.RequestTimeout != 30*time.Second {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestLoadCustomValues(t *testing.T) {
	t.Setenv("OPENCODE_BASE_URL", "https://opencode.example.test/")
	t.Setenv("BRIDGE_LISTEN_ADDRESS", "127.0.0.1:9000")
	t.Setenv("BRIDGE_REQUEST_TIMEOUT", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeURL != "https://opencode.example.test" || cfg.ListenAddress != "127.0.0.1:9000" || cfg.RequestTimeout != 45*time.Second {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		timeout string
	}{
		{name: "relative URL", baseURL: "localhost:4096"},
		{name: "unsupported URL scheme", baseURL: "ftp://localhost:4096"},
		{name: "invalid timeout", baseURL: "http://localhost:4096", timeout: "later"},
		{name: "non-positive timeout", baseURL: "http://localhost:4096", timeout: "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENCODE_BASE_URL", tt.baseURL)
			t.Setenv("BRIDGE_REQUEST_TIMEOUT", tt.timeout)
			if _, err := Load(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
