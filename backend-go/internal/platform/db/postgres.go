package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("database connected", "dsn", maskDSN(dsn))
	return pool, nil
}

// maskDSN hides the password in DSN for logging.
func maskDSN(dsn string) string {
	// Simple masking — replace password portion
	// postgres://user:password@host:port/db -> postgres://user:***@host:port/db
	start := -1
	atIdx := -1
	for i, c := range dsn {
		if c == ':' && start == -1 && i > 11 { // after "postgres://"
			start = i + 1
		}
		if c == '@' {
			atIdx = i
			break
		}
	}
	if start > 0 && atIdx > start {
		return dsn[:start] + "***" + dsn[atIdx:]
	}
	return dsn
}
