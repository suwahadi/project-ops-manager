package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

const (
	LockReminders  int64 = 7301
	LockRDAP       int64 = 7302
	LockScreenshot int64 = 7303
	LockCleanup    int64 = 7304
)

func WithAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, key int64, fn func(context.Context) error) (bool, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	q := sqlc.New(conn)
	locked, err := q.TryAdvisoryLock(ctx, key)
	if err != nil {
		return false, fmt.Errorf("try advisory lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		_, _ = q.AdvisoryUnlock(context.WithoutCancel(ctx), key)
	}()
	return true, fn(ctx)
}
