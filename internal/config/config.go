package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr string
	BaseURL  string
	Env      string

	DatabaseURL string

	SessionLifetime     time.Duration
	SessionCookieName   string
	SessionCookieSecure bool

	StorageEndpoint       string
	StoragePublicEndpoint string
	StorageAccessKey      string
	StorageSecretKey      string
	StorageBucket         string
	StorageRegion         string
	StoragePresignTTL     time.Duration
	StorageUseSSL         bool

	RealtimeChannel string
}

func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:              getEnv("ERREIA_HTTP_ADDR", ":8080"),
		BaseURL:               getEnv("ERREIA_BASE_URL", "http://localhost:8080"),
		Env:                   getEnv("ERREIA_ENV", "dev"),
		DatabaseURL:           os.Getenv("ERREIA_DATABASE_URL"),
		SessionLifetime:       getDuration("ERREIA_SESSION_LIFETIME", 720*time.Hour),
		SessionCookieName:     getEnv("ERREIA_SESSION_COOKIE_NAME", "erreia_session"),
		SessionCookieSecure:   getBool("ERREIA_SESSION_COOKIE_SECURE", false),
		StorageEndpoint:       getEnv("ERREIA_STORAGE_ENDPOINT", "localhost:9000"),
		StoragePublicEndpoint: getEnv("ERREIA_STORAGE_PUBLIC_ENDPOINT", "http://localhost:9000"),
		StorageAccessKey:      os.Getenv("ERREIA_STORAGE_ACCESS_KEY"),
		StorageSecretKey:      os.Getenv("ERREIA_STORAGE_SECRET_KEY"),
		StorageBucket:         getEnv("ERREIA_STORAGE_BUCKET", "erreia"),
		StorageRegion:         getEnv("ERREIA_STORAGE_REGION", "us-east-1"),
		StoragePresignTTL:     getDuration("ERREIA_STORAGE_PRESIGN_TTL", 15*time.Minute),
		StorageUseSSL:         getBool("ERREIA_STORAGE_USE_SSL", false),
		RealtimeChannel:       getEnv("ERREIA_REALTIME_CHANNEL", "erreia_board_events"),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("ERREIA_DATABASE_URL is required")
	}
	if c.StorageAccessKey == "" {
		return nil, fmt.Errorf("ERREIA_STORAGE_ACCESS_KEY is required")
	}
	if c.StorageSecretKey == "" {
		return nil, fmt.Errorf("ERREIA_STORAGE_SECRET_KEY is required")
	}
	return c, nil
}

func getEnv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func getBool(k string, def bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getDuration(k string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(k); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
