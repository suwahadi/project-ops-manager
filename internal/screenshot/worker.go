package screenshot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

const maxAttempts = 3

type Shooter interface {
	Capture(ctx context.Context, target string) ([]byte, error)
}

type FileStore interface {
	WriteFile(rel string, data []byte) error
	Remove(rels ...string)
}

type Worker struct {
	q       *sqlc.Queries
	shooter Shooter
	files   FileStore
	logger  *slog.Logger
	now     func() time.Time
	newID   func() (string, error)
}

func NewWorker(pool *pgxpool.Pool, shooter Shooter, files FileStore, logger *slog.Logger, now func() time.Time, newID func() (string, error)) *Worker {
	return &Worker{q: sqlc.New(pool), shooter: shooter, files: files, logger: logger, now: now, newID: newID}
}

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	rows, err := w.q.ClaimPendingScreenshots(ctx)
	if err != nil {
		return 0, fmt.Errorf("claim screenshots: %w", err)
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		w.process(ctx, r)
	}
	return len(rows), nil
}

func (w *Worker) fail(ctx context.Context, r sqlc.ProjectScreenshot, msg string) {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if _, err := w.q.FailScreenshot(ctx, sqlc.FailScreenshotParams{ProjectID: r.ProjectID, Error: &msg, RequestedAt: r.RequestedAt}); err != nil {
		w.logger.Error("store screenshot failure", "project_id", r.ProjectID, "error", err)
	}
	w.logger.Warn("screenshot failed", "project_id", r.ProjectID, "error", msg)
}

func (w *Worker) process(ctx context.Context, r sqlc.ProjectScreenshot) {
	if r.Attempts > maxAttempts {
		w.fail(ctx, r, "the capture did not finish after several attempts")
		return
	}
	data, err := w.shooter.Capture(ctx, r.RequestedUrl)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.fail(ctx, r, err.Error())
		return
	}
	id, err := w.newID()
	if err != nil {
		w.fail(ctx, r, "could not create a file name")
		return
	}
	now := w.now()
	rel := fmt.Sprintf("screenshots/%04d/%02d/%s.webp", now.Year(), int(now.Month()), id)
	if err := w.files.WriteFile(rel, data); err != nil {
		w.fail(ctx, r, "could not store the screenshot")
		return
	}
	n, err := w.q.CompleteScreenshot(ctx, sqlc.CompleteScreenshotParams{ProjectID: r.ProjectID, StoredPath: &rel, CapturedAt: &now, RequestedAt: r.RequestedAt})
	if err != nil || n == 0 {
		w.files.Remove(rel)
		if err != nil {
			w.logger.Error("store screenshot", "project_id", r.ProjectID, "error", err)
		}
		return
	}
	if r.StoredPath != nil && *r.StoredPath != rel {
		w.files.Remove(*r.StoredPath)
	}
	w.logger.Info("screenshot captured", "project_id", r.ProjectID, "bytes", len(data))
}
