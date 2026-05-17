package config

import (
	"fmt"
	"strings"
	"github.com/spf13/viper"
)

type Config struct {
	Proxy     ProxyConfig     `mapstructure:"proxy"`
	Cache     CacheConfig     `mapstructure:"cache"`
	S3        S3Config        `mapstructure:"s3"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Security  SecurityConfig  `mapstructure:"security"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Log       LogConfig       `mapstructure:"log"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
}

type AuthConfig struct {
	Enabled bool         `mapstructure:"enabled"`
	Users   []UserConfig `mapstructure:"users"`
}

type UserConfig struct {
	Username     string `mapstructure:"username"`
	PasswordHash string `mapstructure:"password_hash"`
}

type ProxyConfig struct {
	Port int `mapstructure:"port"`
	Ecosystems []string `mapstructure:"ecosystems"`
}

type CacheConfig struct {
	Backend               string `mapstructure:"backend"`
	LocalDir              string `mapstructure:"local_dir"`
	TTLHours              int    `mapstructure:"ttl_hours"`
	EvictionIntervalHours int    `mapstructure:"eviction_interval_hours"`
}

type S3Config struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`
	Endpoint        string `mapstructure:"endpoint"`
	KeyPrefix       string `mapstructure:"key_prefix"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type SecurityConfig struct {
	CVEScanning   bool   `mapstructure:"cve_scanning"`
	BlockSeverity string `mapstructure:"block_severity"`
	WarnSeverity  string `mapstructure:"warn_severity"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// RateLimitConfig controls how aggressively the proxy throttles upstream fetches.
// Independent token buckets are maintained per ecosystem, so a flood of npm requests
// does not consume tokens from the PyPI bucket.
type RateLimitConfig struct {
	Enabled            bool    `mapstructure:"enabled"`
	RequestsPerSecond  float64 `mapstructure:"requests_per_second"`
	Burst              int     `mapstructure:"burst"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("cacheproxyfy")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	v.SetEnvPrefix("CACHEPROXYFY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("proxy.port", 8080)
	v.SetDefault("proxy.ecosystems", []string{})
	v.SetDefault("cache.backend", "local")
	v.SetDefault("cache.local_dir", "./data/artifacts")
	v.SetDefault("cache.ttl_hours", 720)

	v.SetDefault("s3.region", "us-east-1")

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "postgres")
	v.SetDefault("database.sslmode", "disable")

	v.SetDefault("cache.eviction_interval_hours", 1)

	v.SetDefault("security.cve_scanning", false)
	v.SetDefault("security.block_severity", "CRITICAL")
	v.SetDefault("security.warn_severity", "HIGH")
	v.SetDefault("auth.enabled", false)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("rate_limit.enabled", false)
	v.SetDefault("rate_limit.requests_per_second", 10.0)
	v.SetDefault("rate_limit.burst", 20)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}