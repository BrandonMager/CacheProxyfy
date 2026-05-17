package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BrandonMager/CacheProxyfy/internal/ratelimit"
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

// TestLoadFromK8sConfigMap simulates the k8s deployment scenario:
// the ConfigMap YAML is written to a temp directory (standing in for
// /etc/cacheproxyfy/), Load() reads it, and secret env vars override
// the empty password fields — exactly as the k8s Secret injection does.
func TestLoadFromK8sConfigMap(t *testing.T) {
	// Exact content from deploy/k8s/01-configmap.yaml (comments stripped).
	configMapYAML := `
proxy:
  port: 8080
  ecosystems:
    - npm
    - pypi
    - maven

cache:
  backend: local
  local_dir: /app/data/artifacts
  ttl_hours: 720
  eviction_interval_hours: 1

s3:
  bucket: ""
  region: us-east-1
  endpoint: ""
  key_prefix: ""
  access_key_id: ""
  secret_access_key: ""

redis:
  addr: redis:6379
  db: 0
  password: ""

database:
  host: postgres
  port: 5432
  user: cacheproxyfy
  dbname: cacheproxyfy
  sslmode: disable
  password: ""

security:
  cve_scanning: true
  block_severity: CRITICAL
  warn_severity: HIGH

auth:
  enabled: false

log:
  level: info
  format: json

rate_limit:
  enabled: false
  requests_per_second: 10
  burst: 20
`

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cacheproxyfy.yaml"), []byte(configMapYAML), 0644); err != nil {
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

	// Simulate k8s Secret injection — passwords arrive as env vars.
	os.Setenv("CACHEPROXYFY_DATABASE_PASSWORD", "pg-secret")
	os.Setenv("CACHEPROXYFY_REDIS_PASSWORD", "redis-secret")
	defer os.Unsetenv("CACHEPROXYFY_DATABASE_PASSWORD")
	defer os.Unsetenv("CACHEPROXYFY_REDIS_PASSWORD")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// k8s-specific values that differ from local defaults.
	if cfg.Proxy.Port != 8080 {
		t.Errorf("proxy.port: got %d, want 8080", cfg.Proxy.Port)
	}
	wantEcosystems := []string{"npm", "pypi", "maven"}
	if len(cfg.Proxy.Ecosystems) != len(wantEcosystems) {
		t.Errorf("ecosystems: got %v, want %v", cfg.Proxy.Ecosystems, wantEcosystems)
	} else {
		for i, eco := range wantEcosystems {
			if cfg.Proxy.Ecosystems[i] != eco {
				t.Errorf("ecosystems[%d]: got %q, want %q", i, cfg.Proxy.Ecosystems[i], eco)
			}
		}
	}
	if cfg.Cache.LocalDir != "/app/data/artifacts" {
		t.Errorf("cache.local_dir: got %q, want /app/data/artifacts", cfg.Cache.LocalDir)
	}
	if cfg.Database.Host != "postgres" {
		t.Errorf("database.host: got %q, want postgres", cfg.Database.Host)
	}
	if cfg.Database.User != "cacheproxyfy" {
		t.Errorf("database.user: got %q, want cacheproxyfy", cfg.Database.User)
	}
	if cfg.Redis.Addr != "redis:6379" {
		t.Errorf("redis.addr: got %q, want redis:6379", cfg.Redis.Addr)
	}

	// Passwords must be overridden by the injected env vars, not left empty.
	if cfg.Database.Password != "pg-secret" {
		t.Errorf("database.password: got %q, want pg-secret (env override)", cfg.Database.Password)
	}
	if cfg.Redis.Password != "redis-secret" {
		t.Errorf("redis.password: got %q, want redis-secret (env override)", cfg.Redis.Password)
	}

	// S3 credentials stay empty — no env vars injected.
	if cfg.S3.AccessKeyID != "" {
		t.Errorf("s3.access_key_id: got %q, want empty", cfg.S3.AccessKeyID)
	}
	if cfg.S3.SecretAccessKey != "" {
		t.Errorf("s3.secret_access_key: got %q, want empty", cfg.S3.SecretAccessKey)
	}
}

// TestK8sConfigMap_RateLimitOverride simulates enabling a per-ecosystem rate limit
// override in the ConfigMap (maven: rps=2, burst=1) and rolling the deployment.
// It asserts that:
//   - maven is throttled at the override rate (burst=1, blocked on the 2nd call)
//   - npm uses the global default (burst=5, blocked only after the 5th call)
func TestK8sConfigMap_RateLimitOverride(t *testing.T) {
	configMapYAML := `
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 5
  overrides:
    maven:
      requests_per_second: 2
      burst: 1
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cacheproxyfy.yaml"), []byte(configMapYAML), 0644); err != nil {
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

	// Confirm the override was parsed from the ConfigMap YAML.
	mavenOv, ok := cfg.RateLimit.Overrides["maven"]
	if !ok {
		t.Fatal("maven override not found in parsed config")
	}
	if mavenOv.RequestsPerSecond != 2 {
		t.Errorf("maven override rps: got %v, want 2", mavenOv.RequestsPerSecond)
	}
	if mavenOv.Burst != 1 {
		t.Errorf("maven override burst: got %d, want 1", mavenOv.Burst)
	}

	// Build the limiter exactly as main.go does after a config reload.
	overrides := make(map[string]ratelimit.EcosystemLimit, len(cfg.RateLimit.Overrides))
	for eco, ov := range cfg.RateLimit.Overrides {
		overrides[eco] = ratelimit.EcosystemLimit{RPS: ov.RequestsPerSecond, Burst: ov.Burst}
	}
	limiter := ratelimit.New(cfg.RateLimit.Enabled, cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst, overrides)

	// --- maven: override burst=1 ---
	// First call consumes the single token.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	if err := limiter.Wait(ctx1, "maven"); err != nil {
		t.Fatalf("maven first call should be allowed: %v", err)
	}
	// Second call: next token takes 500ms (1/rps=2), deadline of 20ms fires first.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := limiter.Wait(ctx2, "maven"); err == nil {
		t.Error("maven second call should be rate-limited — override burst=1 exhausted")
	}

	// --- npm: global default burst=5 ---
	// All 5 burst tokens pass immediately.
	for i := 1; i <= 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := limiter.Wait(ctx, "npm")
		cancel()
		if err != nil {
			t.Fatalf("npm call %d/5 should be allowed by global burst=5: %v", i, err)
		}
	}
	// 6th call: global burst exhausted, next token takes 100ms (1/rps=10), 20ms deadline fires first.
	ctx6, cancel6 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel6()
	if err := limiter.Wait(ctx6, "npm"); err == nil {
		t.Error("npm 6th call should be rate-limited — global burst=5 exhausted")
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
