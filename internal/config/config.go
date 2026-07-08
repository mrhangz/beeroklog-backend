package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string

	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	GoogleClientIDs []string

	AppleTeamID        string
	AppleClientID      string
	AppleKeyID         string
	ApplePrivateKeyPath string

	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
}

func Load() *Config {
	return &Config{
		Port:        envOr("PORT", "8080"),
		DatabaseURL: envOr("DATABASE_URL", "postgres://beeroklog:beeroklog@localhost:5432/beeroklog?sslmode=disable"),

		JWTSecret:     envOr("JWT_SECRET", "change-me-in-production"),
		JWTAccessTTL:  parseDuration(envOr("JWT_ACCESS_TTL", "15m")),
		JWTRefreshTTL: parseDuration(envOr("JWT_REFRESH_TTL", "720h")),

		GoogleClientIDs: parseCommaSeparatedEnv("GOOGLE_CLIENT_IDS", "GOOGLE_CLIENT_ID"),

		AppleTeamID:         os.Getenv("APPLE_TEAM_ID"),
		AppleClientID:       os.Getenv("APPLE_CLIENT_ID"),
		AppleKeyID:          os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKeyPath: os.Getenv("APPLE_PRIVATE_KEY_PATH"),

		S3Endpoint:     envOr("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:       envOr("S3_REGION", "us-east-1"),
		S3Bucket:       envOr("S3_BUCKET", "beeroklog-photos"),
		S3AccessKey:    envOr("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    envOr("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle: envOr("S3_USE_PATH_STYLE", "true") == "true",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

func parseCommaSeparatedEnv(keys ...string) []string {
	for _, key := range keys {
		value := os.Getenv(key)
		if value == "" {
			continue
		}

		parts := strings.Split(value, ",")
		var cleaned []string
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			return cleaned
		}
	}

	return nil
}
