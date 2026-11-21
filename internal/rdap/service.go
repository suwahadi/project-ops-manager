package rdap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

const JobName = "rdap"

const (
	ResultUpdated     = "updated"
	ResultUnchanged   = "unchanged"
	ResultUnsupported = "unsupported"
	ResultError       = "error"
)

var (
	ErrNotEligible = errors.New("RDAP sync applies only to non-cancelled Domain assets with RDAP sync on")
	ErrAssetGone   = errors.New("asset not found")
)

type Lookup interface {
	Expiration(ctx context.Context, name string) (time.Time, error)
}

type Service struct {
	q      *sqlc.Queries
	lookup Lookup
	logger *slog.Logger
	now    func() time.Time
}

func NewService(pool *pgxpool.Pool, lookup Lookup, logger *slog.Logger, now func() time.Time) *Service {
	return &Service{q: sqlc.New(pool), lookup: lookup, logger: logger, now: now}
}

type Outcome struct {
	Result        string
	Error         string
	OldExpiration time.Time
	NewExpiration time.Time
}

func (s *Service) check(ctx context.Context, id, name string, current time.Time) (Outcome, error) {
	out := Outcome{OldExpiration: current, NewExpiration: current}
	exp, err := s.lookup.Expiration(ctx, name)
	now := s.now()
	switch {
	case errors.Is(err, ErrUnsupported):
		out.Result = ResultUnsupported
		out.Error = "The registry for this TLD has no RDAP service or returned no expiration date."
	case err != nil:
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Result = ResultError
		out.Error = truncate(err.Error(), 500)
	case !exp.Equal(current):
		out.Result = ResultUpdated
		out.NewExpiration = exp
		if err := s.q.SetRDAPExpiration(ctx, sqlc.SetRDAPExpirationParams{ID: id, ExpirationDate: exp, RdapCheckedAt: &now}); err != nil {
			return out, fmt.Errorf("update expiration: %w", err)
		}
		s.logger.Info("rdap expiration updated", "asset_id", id, "old", current.Format("2006-01-02"), "new", exp.Format("2006-01-02"))
		return out, nil
	default:
		out.Result = ResultUnchanged
	}
	var errText *string
	if out.Error != "" {
		errText = &out.Error
	}
	result := out.Result
	if err := s.q.SetRDAPResult(ctx, sqlc.SetRDAPResultParams{ID: id, RdapCheckedAt: &now, RdapResult: &result, RdapError: errText}); err != nil {
		return out, fmt.Errorf("store rdap result: %w", err)
	}
	return out, nil
}

func (s *Service) CheckAsset(ctx context.Context, id string) (Outcome, error) {
	a, err := s.q.GetRDAPAsset(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Outcome{}, ErrAssetGone
	}
	if err != nil {
		return Outcome{}, fmt.Errorf("get asset: %w", err)
	}
	if !a.SupportsRdap || !a.RdapEnabled || a.CancelledAt != nil {
		return Outcome{}, ErrNotEligible
	}
	return s.check(ctx, a.ID, a.Name, a.ExpirationDate)
}

type RunSummary struct {
	Checked     int
	Updated     int
	Unchanged   int
	Unsupported int
	Errors      int
}

func (r RunSummary) String() string {
	return fmt.Sprintf("%d checked: %d updated, %d unchanged, %d unsupported, %d errors", r.Checked, r.Updated, r.Unchanged, r.Unsupported, r.Errors)
}

func (s *Service) RunAll(ctx context.Context) (RunSummary, error) {
	var sum RunSummary
	start := s.now()
	runID, err := s.q.StartJobRun(ctx, sqlc.StartJobRunParams{Job: JobName, StartedAt: start})
	if err != nil {
		return sum, fmt.Errorf("start job run: %w", err)
	}
	runErr := s.runAll(ctx, &sum)
	status := "success"
	var errText *string
	if runErr != nil {
		status = "failed"
		msg := truncate(runErr.Error(), 500)
		errText = &msg
	} else if sum.Errors > 0 {
		status = "failed"
		msg := fmt.Sprintf("%d %s could not be checked", sum.Errors, plural(sum.Errors, "domain", "domains"))
		errText = &msg
	}
	finish := s.now()
	if err := s.q.FinishJobRun(context.WithoutCancel(ctx), sqlc.FinishJobRunParams{ID: runID, FinishedAt: &finish, Status: status, Summary: sum.String(), Error: errText}); err != nil {
		return sum, fmt.Errorf("finish job run: %w", err)
	}
	return sum, runErr
}

func (s *Service) runAll(ctx context.Context, sum *RunSummary) error {
	rows, err := s.q.ListRDAPAssets(ctx)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	for _, a := range rows {
		out, err := s.check(ctx, a.ID, a.Name, a.ExpirationDate)
		if err != nil {
			return err
		}
		sum.Checked++
		switch out.Result {
		case ResultUpdated:
			sum.Updated++
		case ResultUnchanged:
			sum.Unchanged++
		case ResultUnsupported:
			sum.Unsupported++
		default:
			sum.Errors++
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
