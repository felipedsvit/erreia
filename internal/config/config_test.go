package config

import (
	"os"
	"testing"
	"time"
)

// TestLoadDefaults ensures we can load config from an empty environment
// using only the documented defaults. This is the contract every deploy
// can fall back to when no env vars are set.
func TestLoadDefaults(t *testing.T) {
	t.Setenv("ERREIA_DATABASE_URL", "postgres://x")
	t.Setenv("ERREIA_STORAGE_ACCESS_KEY", "testkey")
	t.Setenv("ERREIA_STORAGE_SECRET_KEY", "testsecret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default: got %q", cfg.HTTPAddr)
	}
	if cfg.SessionLifetime != 720*time.Hour {
		t.Errorf("SessionLifetime default: got %s", cfg.SessionLifetime)
	}
	if cfg.StorageBucket != "erreia" {
		t.Errorf("StorageBucket default: got %q", cfg.StorageBucket)
	}
	if cfg.RealtimeChannel != "erreia_board_events" {
		t.Errorf("RealtimeChannel default: got %q", cfg.RealtimeChannel)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// t.Setenv cannot unset a variable; we use os.Unsetenv and restore
	// the original value on test exit.
	orig, had := os.LookupEnv("ERREIA_DATABASE_URL")
	os.Unsetenv("ERREIA_DATABASE_URL")
	t.Cleanup(func() {
		if had {
			os.Setenv("ERREIA_DATABASE_URL", orig)
		} else {
			os.Unsetenv("ERREIA_DATABASE_URL")
		}
	})
	if _, err := Load(); err == nil {
		t.Fatal("expected error for empty DATABASE_URL")
	}
}

func TestLoadHonorsEnvOverrides(t *testing.T) {
	t.Setenv("ERREIA_DATABASE_URL", "postgres://x")
	t.Setenv("ERREIA_STORAGE_ACCESS_KEY", "testkey")
	t.Setenv("ERREIA_STORAGE_SECRET_KEY", "testsecret")
	t.Setenv("ERREIA_HTTP_ADDR", ":9000")
	t.Setenv("ERREIA_SESSION_LIFETIME", "15m")
	t.Setenv("ERREIA_STORAGE_PRESIGN_TTL", "1h")
	t.Setenv("ERREIA_SESSION_COOKIE_SECURE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9000" {
		t.Errorf("HTTPAddr override: got %q", cfg.HTTPAddr)
	}
	if cfg.SessionLifetime != 15*time.Minute {
		t.Errorf("SessionLifetime override: got %s", cfg.SessionLifetime)
	}
	if cfg.StoragePresignTTL != time.Hour {
		t.Errorf("StoragePresignTTL override: got %s", cfg.StoragePresignTTL)
	}
	if !cfg.SessionCookieSecure {
		t.Error("SessionCookieSecure override not applied")
	}
}

func TestGetDurationInvalidFallsBack(t *testing.T) {
	t.Setenv("ERREIA_SESSION_LIFETIME", "not-a-duration")
	if d := getDuration("ERREIA_SESSION_LIFETIME", time.Hour); d != time.Hour {
		t.Errorf("expected default, got %s", d)
	}
}

func TestGetBoolInvalidFallsBack(t *testing.T) {
	if b := getBool("ERREIA_NOT_SET", true); !b {
		t.Error("default true not preserved")
	}
	t.Setenv("ERREIA_TEST_BOOL", "wat")
	if b := getBool("ERREIA_TEST_BOOL", false); b {
		t.Error("default false not preserved for invalid bool")
	}
}
