package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Listen   string
	Upstream string
	Timeout  time.Duration
	TTL      time.Duration
	Cache    CacheConfig
}

// CacheConfig holds cache-specific configuration
type CacheConfig struct {
	// KeyHeaders is a list of HTTP headers to include in cache key
	// This allows caching different responses for different header values
	KeyHeaders []string
}

// FileConfig represents the structure of the YAML config file
type FileConfig struct {
	Server struct {
		Listen   string `yaml:"listen"`
		Upstream string `yaml:"upstream"`
		Timeout  string `yaml:"timeout"`
	} `yaml:"server"`
	Cache struct {
		TTL        string   `yaml:"ttl"`
		KeyHeaders []string `yaml:"key_headers"`
	} `yaml:"cache"`
}

// Load loads configuration from YAML file
func Load() *Config {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load config file
	fileConfig, err := loadConfigFile(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Parse durations
	timeout, err := parseDuration(fileConfig.Server.Timeout, 1*time.Second)
	if err != nil {
		log.Fatalf("invalid timeout in config: %v", err)
	}

	ttl, err := parseDuration(fileConfig.Cache.TTL, 0)
	if err != nil {
		log.Fatalf("invalid ttl in config: %v", err)
	}

	return &Config{
		Listen:   fileConfig.Server.Listen,
		Upstream: fileConfig.Server.Upstream,
		Timeout:  timeout,
		TTL:      ttl,
		Cache: CacheConfig{
			KeyHeaders: fileConfig.Cache.KeyHeaders,
		},
	}
}

func loadConfigFile(path string) (FileConfig, error) {
	var fc FileConfig

	// Try specified path first
	if path != "" {
		if fileExists(path) {
			data, err := os.ReadFile(path)
			if err != nil {
				return fc, fmt.Errorf("failed to read config file %s: %w", path, err)
			}

			if err := yaml.Unmarshal(data, &fc); err != nil {
				return fc, fmt.Errorf("failed to parse config file %s: %w", path, err)
			}

			log.Printf("loaded config from %s", path)
			return fc, nil
		}
	}

	// Fallback: try default locations
	defaultPaths := []string{"config.yaml", "aegis.yaml", "/etc/aegis/config.yaml"}
	for _, p := range defaultPaths {
		if !fileExists(p) {
			continue
		}

		data, err := os.ReadFile(p)
		if err != nil {
			log.Printf("warning: failed to read config file %s: %v", p, err)
			continue
		}

		if err := yaml.Unmarshal(data, &fc); err != nil {
			log.Printf("warning: failed to parse config file %s: %v", p, err)
			continue
		}

		log.Printf("loaded config from %s", p)
		return fc, nil
	}

	return fc, fmt.Errorf("no config file found (tried: %s, %v)", path, defaultPaths)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseDuration(value string, defaultValue time.Duration) (time.Duration, error) {
	if value == "" {
		return defaultValue, nil
	}
	return time.ParseDuration(value)
}
