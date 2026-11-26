package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

const BriefTemplate = "## Overview\n\n\n## Tech Stack\n\n\n## Features\n\n"

var ErrNotFound = errors.New("project not found")

type Input struct {
	Name        string
	ClientID    *string
	URL         *string
	Brief       string
	AssetIDs    []string
	Currency    money.Currency
	Value       money.NullAmount
	Cost        money.NullAmount
	BillingDate *time.Time
}

func (in Input) currency() string {
	if in.Currency == "" {
		return string(money.IDR)
	}
	return string(in.Currency)
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), now: now}
}

func (s *Service) List(ctx context.Context) ([]sqlc.ListProjectsRow, error) {
	return s.q.ListProjects(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (sqlc.GetProjectRow, error) {
	p, err := s.q.GetProject(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Service) Assets(ctx context.Context, id string) ([]sqlc.ListProjectAssetsRow, error) {
	return s.q.ListProjectAssets(ctx, id)
}

func (s *Service) ByAsset(ctx context.Context, assetID string) ([]sqlc.ListProjectsByAssetRow, error) {
	return s.q.ListProjectsByAsset(ctx, assetID)
}

func (s *Service) Screenshot(ctx context.Context, id string) (*sqlc.ProjectScreenshot, error) {
	sc, err := s.q.GetProjectScreenshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (s *Service) inTx(ctx context.Context, fn func(*sqlc.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func setAssets(ctx context.Context, q *sqlc.Queries, projectID string, ids []string) error {
	if err := q.ClearProjectAssets(ctx, projectID); err != nil {
		return fmt.Errorf("clear assets: %w", err)
	}
	for _, a := range ids {
		if err := q.AddProjectAsset(ctx, sqlc.AddProjectAssetParams{ProjectID: projectID, AssetID: a}); err != nil {
			return fmt.Errorf("link asset: %w", err)
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in Input) (string, error) {
	var id string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		var err error
		id, err = q.CreateProject(ctx, sqlc.CreateProjectParams{
			ClientID: in.ClientID, Name: strings.TrimSpace(in.Name), Url: in.URL, Brief: in.Brief,
			Currency: in.currency(), ProjectValue: in.Value, ProjectCost: in.Cost, BillingDate: in.BillingDate,
		})
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		if err := setAssets(ctx, q, id, in.AssetIDs); err != nil {
			return err
		}
		if in.URL != nil {
			return q.RequestProjectScreenshot(ctx, sqlc.RequestProjectScreenshotParams{ProjectID: id, RequestedUrl: *in.URL, Now: s.now()})
		}
		return nil
	})
	return id, err
}

func (s *Service) Update(ctx context.Context, id string, in Input) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetProject(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}
		paid, err := q.ProjectReceived(ctx, id)
		if err != nil {
			return fmt.Errorf("sum payments: %w", err)
		}
		if err := checkMoneyChange(cur, paid.Received, paid.Payments, in); err != nil {
			return err
		}
		if err := q.UpdateProject(ctx, sqlc.UpdateProjectParams{
			ID: id, ClientID: in.ClientID, Name: strings.TrimSpace(in.Name), Url: in.URL, Brief: in.Brief,
			Currency: in.currency(), ProjectValue: in.Value, ProjectCost: in.Cost, BillingDate: in.BillingDate,
		}); err != nil {
			return fmt.Errorf("update project: %w", err)
		}
		if err := setAssets(ctx, q, id, in.AssetIDs); err != nil {
			return err
		}
		if in.URL != nil && (cur.Url == nil || *cur.Url != *in.URL) {
			return q.RequestProjectScreenshot(ctx, sqlc.RequestProjectScreenshotParams{ProjectID: id, RequestedUrl: *in.URL, Now: s.now()})
		}
		return nil
	})
}

func (s *Service) RequestScreenshot(ctx context.Context, id string) error {
	p, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.Url == nil {
		return errors.New("project has no website URL")
	}
	return s.q.RequestProjectScreenshot(ctx, sqlc.RequestProjectScreenshotParams{ProjectID: id, RequestedUrl: *p.Url, Now: s.now()})
}

func (s *Service) Delete(ctx context.Context, id string) ([]string, error) {
	var paths []string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		ps, err := q.ListScreenshotPathsByProject(ctx, id)
		if err != nil {
			return err
		}
		for _, p := range ps {
			if p != nil {
				paths = append(paths, *p)
			}
		}
		return q.DeleteProject(ctx, id)
	})
	return paths, err
}
