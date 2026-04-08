package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache: miss")

type Client struct {
	rdb *redis.Client
	ttl time.Duration
}

//Redis connection params
type Config struct {
	Addr string
	Password string
	DB int // Redis logic DB index
	TTL time.Duration 
}

func New(cfg Config) (*Client, error){
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
		Password: cfg.Password,
		DB: cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cache: ping: %w", err)
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour //default
	}

	return &Client{rdb: rdb, ttl: ttl}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

// Format: "cpf:<ecosystem>:<name>:<version>"
func key(ecosystem, name, version string) string {
	return fmt.Sprintf("cpf:%s:%s:%s", ecosystem, name, version)
}

func (c *Client) Get(ctx context.Context, ecosystem, name, version string) (checksum string, err error) {
	val, err := c.rdb.Get(ctx, key(ecosystem, name, version)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}

	if err != nil {
		return "", fmt.Errorf("cache: get: %w", err)
	}

	return val, nil
}

func (c *Client) Set(ctx context.Context, ecosystem, name, version, checksum string) error {
	err := c.rdb.Set(ctx, key(ecosystem, name, version), checksum, c.ttl).Err()
	if err != nil {
		return fmt.Errorf("cache: set: %w", err)
	}

	return nil
}

func (c *Client) Delete(ctx context.Context, ecosystem, name, version string) error {
	err := c.rdb.Del(ctx, key(ecosystem, name, version)).Err()
	if err != nil {
		return fmt.Errorf("cache: delete: %w", err)
	}

	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}


