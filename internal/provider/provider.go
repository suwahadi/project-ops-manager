package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

var (
	ErrNotFound  = errors.New("provider not found")
	ErrInUse     = errors.New("provider has assets")
	ErrDuplicate = errors.New("a provider with this name already exists")
)

type Input struct {
	Name     string
	Website  string
	LoginURL string
	Notes    string
}

type Service struct {
	q *sqlc.Queries
}

func NewService(db sqlc.DBTX) *Service { return &Service{q: sqlc.New(db)} }

func optional(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) List(ctx context.Context) ([]sqlc.ListProvidersRow, error) {
	return s.q.ListProviders(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (sqlc.Provider, error) {
	p, err := s.q.GetProvider(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Service) Create(ctx context.Context, in Input) (sqlc.Provider, error) {
	p, err := s.q.CreateProvider(ctx, sqlc.CreateProviderParams{Name: strings.TrimSpace(in.Name), Website: optional(in.Website), LoginUrl: optional(in.LoginURL), Notes: optional(in.Notes)})
	if isUnique(err) {
		return p, ErrDuplicate
	}
	return p, err
}

func (s *Service) FindOrCreate(ctx context.Context, q *sqlc.Queries, name string) (string, error) {
	name = strings.TrimSpace(name)
	p, err := q.GetProviderByName(ctx, name)
	if err == nil {
		return p.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find provider: %w", err)
	}
	p, err = q.CreateProvider(ctx, sqlc.CreateProviderParams{Name: name})
	if err != nil {
		return "", fmt.Errorf("create provider: %w", err)
	}
	return p.ID, nil
}

func (s *Service) Update(ctx context.Context, id string, in Input) error {
	err := s.q.UpdateProvider(ctx, sqlc.UpdateProviderParams{ID: id, Name: strings.TrimSpace(in.Name), Website: optional(in.Website), LoginUrl: optional(in.LoginURL), Notes: optional(in.Notes)})
	if isUnique(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	n, err := s.q.CountAssetsByProvider(ctx, id)
	if err != nil {
		return fmt.Errorf("count assets: %w", err)
	}
	if n > 0 {
		return ErrInUse
	}
	return s.q.DeleteProvider(ctx, id)
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
