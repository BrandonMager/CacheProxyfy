package cache

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startRedis spins up a Redis container and returns its address.
func startRedis(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("failed to terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	return host + ":" + port.Port()
}

func TestNew_PingFailure(t *testing.T) {
	_, err := New(Config{Addr: "localhost:1"})
	if err == nil {
		t.Fatal("expected error for unreachable Redis, got nil")
	}
}

func TestNew_DefaultTTL(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	addr := startRedis(t)

	c, err := New(Config{Addr: addr})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	if c.ttl != 24*time.Hour {
		t.Errorf("expected default TTL 24h, got %v", c.ttl)
	}
}

func TestNew_CustomTTL(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	addr := startRedis(t)
	want := 5 * time.Minute

	c, err := New(Config{Addr: addr, TTL: want})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	if c.ttl != want {
		t.Errorf("expected TTL %v, got %v", want, c.ttl)
	}
}

func TestNew_ConnectionPool(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	addr := startRedis(t)

	c, err := New(Config{Addr: addr})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	if c.rdb.Options().PoolSize <= 0 {
		t.Errorf("expected pool size > 0, got %d", c.rdb.Options().PoolSize)
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	addr := startRedis(t)
	c, err := New(Config{Addr: addr})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestSetGet_ReturnsCorrectChecksum(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	c := newTestClient(t)
	ctx := context.Background()

	const ecosystem, name, version, checksum = "npm", "lodash", "4.17.21", "sha512-abc123"

	if err := c.Set(ctx, ecosystem, name, version, checksum); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := c.Get(ctx, ecosystem, name, version)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != checksum {
		t.Errorf("expected checksum %q, got %q", checksum, got)
	}
}

func TestGet_Miss(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	c := newTestClient(t)
	ctx := context.Background()

	_, err := c.Get(ctx, "npm", "lodash", "4.17.21")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestPing_Healthy(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	c := newTestClient(t)

	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("expected nil from healthy Redis, got: %v", err)
	}
}

func TestPing_Unhealthy(t *testing.T) {
	c := &Client{rdb: redis.NewClient(&redis.Options{Addr: "localhost:1"})}
	defer c.Close()

	if err := c.Ping(context.Background()); err == nil {
		t.Error("expected error from unreachable Redis, got nil")
	}
}

func TestDelete_EvictsKey(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	c := newTestClient(t)
	ctx := context.Background()

	const ecosystem, name, version, checksum = "npm", "lodash", "4.17.21", "sha512-abc123"

	if err := c.Set(ctx, ecosystem, name, version, checksum); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	if err := c.Delete(ctx, ecosystem, name, version); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := c.Get(ctx, ecosystem, name, version)
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss after delete, got %v", err)
	}
}
