// Command devtoken mints a JWT pair for local API testing:
//
//	go run ./cmd/devtoken -user <user-uuid> [-secret <jwt-secret>] [-ttl 1h]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mrhangz/beeroklog-backend/internal/middleware"
)

func main() {
	user := flag.String("user", "", "user id to embed as the sub claim (required)")
	secret := flag.String("secret", envOr("JWT_SECRET", "dev-secret"), "JWT signing secret")
	ttl := flag.Duration("ttl", time.Hour, "access token lifetime")
	flag.Parse()

	if *user == "" {
		flag.Usage()
		os.Exit(2)
	}

	pair, err := middleware.NewAuth(*secret).Issue(*user, *ttl, *ttl)
	if err != nil {
		log.Fatalf("issue: %v", err)
	}
	fmt.Println(pair.AccessToken)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
