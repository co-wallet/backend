package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	// register postgres driver for goose
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to db: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

func RunMigrations(databaseURL, migrationsDir string) error {
	db, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer db.Close()

	if err = goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err = goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
