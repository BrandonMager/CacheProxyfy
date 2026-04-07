package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Proxy.Port != 8080 {
		t.Errorf("expected proxy.port=8080, got %d", cfg.Proxy.Port)
	}
	if cfg.Cache.Backend != "local" {
		t.Errorf("expected cache.backend=local, got %s", cfg.Cache.Backend)
	}
	if cfg.Cache.LocalDir != "./data/artifacts" {
		t.Errorf("expected cache.local_dir=./data/artifacts, got %s", cfg.Cache.LocalDir)
	}
	if cfg.Cache.TTLHours != 720 {
		t.Errorf("expected cache.ttl_hours=720, got %d", cfg.Cache.TTLHours)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected log.level=info, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected log.format=json, got %s", cfg.Log.Format)
	}
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
proxy:
  port: 7070
  ecosystems:
    - npm
    - pypi
cache:
  backend: local
  local_dir: /tmp/cache
  ttl_hours: 48
log:
  level: debug
  format: text
`
	if err := os.WriteFile(filepath.Join(dir, "cacheproxyfy.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Proxy.Port != 7070 {
		t.Errorf("expected proxy.port=7070, got %d", cfg.Proxy.Port)
	}
	if len(cfg.Proxy.Ecosystems) != 2 || cfg.Proxy.Ecosystems[0] != "npm" || cfg.Proxy.Ecosystems[1] != "pypi" {
		t.Errorf("expected ecosystems=[npm pypi], got %v", cfg.Proxy.Ecosystems)
	}
	if cfg.Cache.LocalDir != "/tmp/cache" {
		t.Errorf("expected cache.local_dir=/tmp/cache, got %s", cfg.Cache.LocalDir)
	}
	if cfg.Cache.TTLHours != 48 {
		t.Errorf("expected cache.ttl_hours=48, got %d", cfg.Cache.TTLHours)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log.level=debug, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("expected log.format=text, got %s", cfg.Log.Format)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	os.Setenv("CACHEPROXYFY_PROXY_PORT", "9090")
	defer os.Unsetenv("CACHEPROXYFY_PROXY_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Proxy.Port != 9090 {
		t.Errorf("expected proxy.port=9090 from env, got %d", cfg.Proxy.Port)
	}
}
