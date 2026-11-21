package asset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/provider"
)

var (
	ErrNotFound      = errors.New("asset not found")
	ErrTypeLocked    = errors.New("the asset type cannot change while custom field values exist")
	ErrClientMissing = errors.New("client not found")
)

type Filter struct {
	TypeID     string
	Status     string
	Owner      string
	ProviderID string
	Query      string
	Sort       string
}

var SortOptions = []struct{ Value, Label string }{
	{"", "Expiration date (soonest first)"},
	{"expiration_desc", "Expiration date (latest first)"},
	{"name_asc", "Name (A–Z)"},
	{"name_desc", "Name (Z–A)"},
	{"created_desc", "Recently added"},
}

type ListItem struct {
	ID           string
	Name         string
	Expiration   time.Time
	Status       Status
	Countdown    string
	DaysLeft     int
	TypeName     string
	TypeIcon     string
	ProviderName string
	ClientID     *string
	ClientName   *string
	Currency     string
	Cost         money.Amount
	Selling      money.NullAmount
}

type Detail struct {
	sqlc.GetAssetRow
	Status    Status
	Countdown string
	DaysLeft  int
	Custom    assettype.Values
}

func (d Detail) Margin() (money.Amount, bool) {
	if !d.SellingPrice.Valid {
		return 0, false
	}
	return d.SellingPrice.Amount - d.CostPrice, true
}

type Service struct {
	pool      *pgxpool.Pool
	q         *sqlc.Queries
	providers *provider.Service
	uploads   string
}

func NewService(pool *pgxpool.Pool, uploadsDir string) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), providers: provider.NewService(pool), uploads: uploadsDir}
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func optionalID(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) List(ctx context.Context, f Filter, today time.Time) ([]ListItem, error) {
	params := sqlc.ListAssetsParams{
		TypeID:     optionalID(f.TypeID),
		ProviderID: optionalID(f.ProviderID),
		Query:      escapeLike(strings.TrimSpace(f.Query)),
		Status:     f.Status,
		Today:      today,
		Sort:       f.Sort,
	}
	switch f.Owner {
	case "":
	case OwnerInternal:
		params.InternalOnly = true
	default:
		params.ClientID = optionalID(f.Owner)
	}
	rows, err := s.q.ListAssets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	out := make([]ListItem, len(rows))
	for i, r := range rows {
		out[i] = ListItem{
			ID: r.ID, Name: r.Name, Expiration: r.ExpirationDate,
			Status:    ComputeStatus(r.CancelledAt != nil, r.ExpirationDate, today, r.ReminderOffsets),
			Countdown: Countdown(r.ExpirationDate, today), DaysLeft: DaysLeft(r.ExpirationDate, today),
			TypeName: r.TypeName, TypeIcon: r.TypeIcon, ProviderName: r.ProviderName,
			ClientID: r.ClientID, ClientName: r.ClientName, Currency: r.Currency, Cost: r.CostPrice, Selling: r.SellingPrice,
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string, today time.Time) (Detail, error) {
	r, err := s.q.GetAsset(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("get asset: %w", err)
	}
	return Detail{
		GetAssetRow: r,
		Status:      ComputeStatus(r.CancelledAt != nil, r.ExpirationDate, today, r.ReminderOffsets),
		Countdown:   Countdown(r.ExpirationDate, today),
		DaysLeft:    DaysLeft(r.ExpirationDate, today),
		Custom:      assettype.DecodeValues(r.CustomFields),
	}, nil
}

func (s *Service) resolveProvider(ctx context.Context, q *sqlc.Queries, in Input) (string, error) {
	if in.ProviderName != "" {
		return s.providers.FindOrCreate(ctx, q, in.ProviderName)
	}
	if _, err := q.GetProvider(ctx, in.ProviderID); err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	return in.ProviderID, nil
}

func (s *Service) Create(ctx context.Context, in Input) (string, error) {
	var id string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		providerID, err := s.resolveProvider(ctx, q, in)
		if err != nil {
			return err
		}
		id, err = q.CreateAsset(ctx, sqlc.CreateAssetParams{
			AssetTypeID: in.TypeID, ProviderID: providerID, ClientID: in.ClientID, Name: in.Name, Url: in.URL,
			OrderDate: in.OrderDate, ExpirationDate: in.Expiration, RenewalPeriodMonths: int32(in.RenewalMonths),
			AutoRenew: in.AutoRenew, CostPrice: in.Cost, SellingPrice: in.Selling, Currency: string(in.Currency),
			Notes: "", CustomFields: in.Custom.Encode(), RdapEnabled: in.RDAPEnabled,
		})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("create asset: %w", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id string, in Input) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetAsset(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get asset: %w", err)
		}
		if cur.AssetTypeID != in.TypeID && HasCustomValues(assettype.DecodeValues(cur.CustomFields)) {
			return ErrTypeLocked
		}
		providerID, err := s.resolveProvider(ctx, q, in)
		if err != nil {
			return err
		}
		return q.UpdateAsset(ctx, sqlc.UpdateAssetParams{
			ID: id, AssetTypeID: in.TypeID, ProviderID: providerID, ClientID: in.ClientID, Name: in.Name, Url: in.URL,
			OrderDate: in.OrderDate, ExpirationDate: in.Expiration, RenewalPeriodMonths: int32(in.RenewalMonths),
			AutoRenew: in.AutoRenew, CostPrice: in.Cost, SellingPrice: in.Selling, Currency: string(in.Currency),
			CustomFields: in.Custom.Encode(), RdapEnabled: in.RDAPEnabled,
		})
	})
}

func (s *Service) UpdateNotes(ctx context.Context, id, notes string) error {
	return s.q.UpdateAssetNotes(ctx, sqlc.UpdateAssetNotesParams{ID: id, Notes: notes})
}

func (s *Service) Cancel(ctx context.Context, id string, at time.Time) error {
	return s.q.SetAssetCancelled(ctx, sqlc.SetAssetCancelledParams{ID: id, CancelledAt: &at})
}

func (s *Service) Restore(ctx context.Context, id string) error {
	return s.q.SetAssetCancelled(ctx, sqlc.SetAssetCancelledParams{ID: id, CancelledAt: nil})
}

func (s *Service) Delete(ctx context.Context, id string) ([]string, error) {
	var paths []string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		var err error
		paths, err = q.ListAssetDocumentPaths(ctx, &id)
		if err != nil {
			return fmt.Errorf("list documents: %w", err)
		}
		return q.DeleteAsset(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	var failed []string
	for _, p := range paths {
		if err := os.Remove(filepath.Join(s.uploads, filepath.Clean("/"+p))); err != nil && !errors.Is(err, os.ErrNotExist) {
			failed = append(failed, p)
		}
	}
	return failed, nil
}

func (s *Service) Options(ctx context.Context) ([]sqlc.ListAssetOptionsRow, error) {
	return s.q.ListAssetOptions(ctx)
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

type SearchResults struct {
	Assets   []sqlc.SearchAssetsRow
	Clients  []sqlc.SearchClientsRow
	Projects []sqlc.SearchProjectsRow
}

func (s *Service) Search(ctx context.Context, query string) (SearchResults, error) {
	q := escapeLike(strings.TrimSpace(query))
	var res SearchResults
	if q == "" {
		return res, nil
	}
	var err error
	if res.Assets, err = s.q.SearchAssets(ctx, q); err != nil {
		return res, fmt.Errorf("search assets: %w", err)
	}
	if res.Clients, err = s.q.SearchClients(ctx, q); err != nil {
		return res, fmt.Errorf("search clients: %w", err)
	}
	if res.Projects, err = s.q.SearchProjects(ctx, q); err != nil {
		return res, fmt.Errorf("search projects: %w", err)
	}
	return res, nil
}
