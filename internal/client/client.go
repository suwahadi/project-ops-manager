package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

var (
	ErrNotFound  = errors.New("client not found")
	ErrInUse     = errors.New("client has assets or projects")
	ErrNotLinked = errors.New("item does not belong to this client")
)

type Input struct {
	Name          string
	ContactPerson string
	Email         string
	Phone         string
	Notes         string
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

func (s *Service) List(ctx context.Context) ([]sqlc.ListClientsRow, error) {
	return s.q.ListClients(ctx)
}

func (s *Service) Options(ctx context.Context) ([]sqlc.ListClientOptionsRow, error) {
	return s.q.ListClientOptions(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (sqlc.Client, error) {
	c, err := s.q.GetClient(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Service) Create(ctx context.Context, in Input) (string, error) {
	c, err := s.q.CreateClient(ctx, sqlc.CreateClientParams{Name: strings.TrimSpace(in.Name), ContactPerson: optional(in.ContactPerson), Email: optional(in.Email), Phone: optional(in.Phone), Notes: optional(in.Notes)})
	if err != nil {
		return "", fmt.Errorf("create client: %w", err)
	}
	return c.ID, nil
}

func (s *Service) Update(ctx context.Context, id string, in Input) error {
	return s.q.UpdateClient(ctx, sqlc.UpdateClientParams{ID: id, Name: strings.TrimSpace(in.Name), ContactPerson: optional(in.ContactPerson), Email: optional(in.Email), Phone: optional(in.Phone), Notes: optional(in.Notes)})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	deps, err := s.q.CountClientDependents(ctx, id)
	if err != nil {
		return fmt.Errorf("count dependents: %w", err)
	}
	if deps.Assets > 0 || deps.Projects > 0 {
		return ErrInUse
	}
	return s.q.DeleteClient(ctx, id)
}

func (s *Service) Assets(ctx context.Context, id string) ([]sqlc.ListAssetsByClientRow, error) {
	return s.q.ListAssetsByClient(ctx, &id)
}

func (s *Service) Projects(ctx context.Context, id string) ([]sqlc.ListProjectsByClientRow, error) {
	return s.q.ListProjectsByClient(ctx, &id)
}

func (s *Service) LinkableAssets(ctx context.Context, id string) ([]sqlc.ListAssetsLinkableToClientRow, error) {
	return s.q.ListAssetsLinkableToClient(ctx, id)
}

func (s *Service) LinkableProjects(ctx context.Context, id string) ([]sqlc.ListProjectsLinkableToClientRow, error) {
	return s.q.ListProjectsLinkableToClient(ctx, id)
}

func (s *Service) LinkAssets(ctx context.Context, id string, assetIDs []string) (int64, error) {
	n, err := s.q.LinkAssetsToClient(ctx, sqlc.LinkAssetsToClientParams{ClientID: id, Ids: assetIDs})
	if err != nil {
		return 0, fmt.Errorf("link assets: %w", err)
	}
	return n, nil
}

func (s *Service) LinkProjects(ctx context.Context, id string, projectIDs []string) (int64, error) {
	n, err := s.q.LinkProjectsToClient(ctx, sqlc.LinkProjectsToClientParams{ClientID: id, Ids: projectIDs})
	if err != nil {
		return 0, fmt.Errorf("link projects: %w", err)
	}
	return n, nil
}

func (s *Service) UnlinkAsset(ctx context.Context, id, assetID string) error {
	n, err := s.q.UnlinkAssetFromClient(ctx, sqlc.UnlinkAssetFromClientParams{AssetID: assetID, ClientID: id})
	if err != nil {
		return fmt.Errorf("unlink asset: %w", err)
	}
	if n == 0 {
		return ErrNotLinked
	}
	return nil
}

func (s *Service) UnlinkProject(ctx context.Context, id, projectID string) error {
	n, err := s.q.UnlinkProjectFromClient(ctx, sqlc.UnlinkProjectFromClientParams{ProjectID: projectID, ClientID: id})
	if err != nil {
		return fmt.Errorf("unlink project: %w", err)
	}
	if n == 0 {
		return ErrNotLinked
	}
	return nil
}
