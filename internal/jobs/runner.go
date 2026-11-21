package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db"
)

type Schedule interface {
	Next(after time.Time) time.Time
}

type Every time.Duration

func (e Every) Next(after time.Time) time.Time { return after.Add(time.Duration(e)) }

type DailyAt struct {
	Hour     int
	Minute   int
	Location func() *time.Location
}

func (d DailyAt) Next(after time.Time) time.Time {
	loc := d.Location()
	local := after.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), d.Hour, d.Minute, 0, 0, loc)
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month(), local.Day()+1, d.Hour, d.Minute, 0, 0, loc)
	}
	return next
}

type Job struct {
	Name       string
	LockKey    int64
	Schedule   Schedule
	RunAtStart bool
	Run        func(context.Context) error
}

type Runner struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	jobs   []Job
	wg     sync.WaitGroup
}

func NewRunner(pool *pgxpool.Pool, logger *slog.Logger, jobs ...Job) *Runner {
	return &Runner{pool: pool, logger: logger, jobs: jobs}
}

func (r *Runner) Start(ctx context.Context) {
	for _, j := range r.jobs {
		r.wg.Add(1)
		go func(j Job) {
			defer r.wg.Done()
			r.loop(ctx, j)
		}(j)
	}
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) loop(ctx context.Context, j Job) {
	if j.RunAtStart {
		r.runOnce(ctx, j)
	}
	for {
		now := time.Now()
		wait := time.Until(j.Schedule.Next(now))
		if wait > time.Hour {
			wait = time.Hour
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if due := j.Schedule.Next(now); time.Now().Before(due) {
			continue
		}
		r.runOnce(ctx, j)
	}
}

func (r *Runner) runOnce(ctx context.Context, j Job) {
	start := time.Now()
	ran, err := db.WithAdvisoryLock(ctx, r.pool, j.LockKey, j.Run)
	switch {
	case err != nil && ctx.Err() == nil:
		r.logger.Error("job failed", "job", j.Name, "error", err, "duration_ms", time.Since(start).Milliseconds())
	case !ran:
		r.logger.Info("job skipped, already running elsewhere", "job", j.Name)
	}
}
