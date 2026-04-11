package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	// register postgres driver for goose
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DBTX is a minimal query-runner interface satisfied by both *pgxpool.Pool and pgx.Tx.
// Repositories depend on DBTX so a single code path works for both pool-scoped and
// transaction-scoped operations.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxBeginner abstracts the Begin call so WithTx is unit-testable without a live pool.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx executes fn inside a DB transaction. It commits on success, rolls back on
// error, and rolls back + re-panics on panic.
func WithTx(ctx context.Context, pool TxBeginner, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			// Откат при панике — best-effort. Мы уже раскручиваем панику и
			// всё равно не можем вернуть ошибку вызывающему: важнее сохранить
			// исходное panic-значение, чем ошибку отката.
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			rbErr := tx.Rollback(ctx)
			// pgx.ErrTxClosed означает, что транзакция уже закрыта (например,
			// pgx сам откатил её при ошибке Commit) — это штатное состояние,
			// а не реальная ошибка отката.
			if rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

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
	d, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer d.Close() //nolint:errcheck

	if err = goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err = goose.Up(d, migrationsDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
