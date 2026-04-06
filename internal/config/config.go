package config

import (
	"fmt"
	"strings"
	"github.com/spf13/viper"
)

type Config struct {
	Proxy ProxyConfig `mapstructure:"proxy"`
	Cache CacheConfig `mapstructure:"cache"`
	Security SecurityConfig `mapstructure:"log"`
	Log LogConfig `mapstructure:"log"`
}

type ProxyConfig struct {
	Port int `mapstructure:"port"`
	Ecosystems []string `mapstructure:"ecosystems"`
}

type CacheConfig struct {
	Backend string `mapstructure:"backend"`
	LocalDir string `mapstructure:"local_dir"`
	TTLHours int `mapstructure:"ttl_hours"`
}

type SecurityConfig struct {
	CVEScanning bool `mapstructure:"cve_scanning"`
	Policy string `mapstructure:"policy"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
	Format string `mapstructure:"format"`
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
	v.SetDefault("cache.backend", "local")
	v.SetDefault("cache.local_dir", "./data/artifacts")
	v.SetDefault("cache.ttl_hours", 720)

	v.SetDefault("security.cve_scanning", false)
	v.SetDefault("security.policy", "warn")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

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