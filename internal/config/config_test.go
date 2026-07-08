package config

import (
	"reflect"
	"testing"
)

func TestParseCommaSeparatedEnv(t *testing.T) {
	t.Run("splits and trims", func(t *testing.T) {
		t.Setenv("TEST_IDS", "a.apps.example, b.apps.example ,, c")
		got := parseCommaSeparatedEnv("TEST_IDS")
		want := []string{"a.apps.example", "b.apps.example", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("falls back to legacy key", func(t *testing.T) {
		t.Setenv("TEST_IDS", "")
		t.Setenv("TEST_ID", "legacy-client")
		got := parseCommaSeparatedEnv("TEST_IDS", "TEST_ID")
		want := []string{"legacy-client"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("empty when unset", func(t *testing.T) {
		if got := parseCommaSeparatedEnv("TEST_UNSET_KEY"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestLoadDefaults(t *testing.T) {
	cfg := Load()
	if cfg.Port == "" {
		t.Error("Port default missing")
	}
	if cfg.AppEnv == "" {
		t.Error("AppEnv default missing")
	}
	if cfg.JWTAccessTTL <= 0 || cfg.JWTRefreshTTL <= 0 {
		t.Error("token TTL defaults missing")
	}
}
