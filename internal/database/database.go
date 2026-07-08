package database

import (
	"context"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error

	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			log.Printf("db connect attempt %d/30: %v", attempt, err)
			time.Sleep(time.Second)
			continue
		}
		if err = pool.Ping(ctx); err != nil {
			pool.Close()
			log.Printf("db ping attempt %d/30: %v", attempt, err)
			time.Sleep(time.Second)
			continue
		}
		log.Printf("db connected on attempt %d", attempt)
		return pool, nil
	}
	return nil, fmt.Errorf("gave up after 30 attempts: %w", err)
}

func Migrate(databaseURL string) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
