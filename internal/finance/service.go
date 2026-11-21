package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type Service struct {
	q *sqlc.Queries
}

func NewService(db sqlc.DBTX) *Service { return &Service{q: sqlc.New(db)} }

type Report struct {
	Period    Period
	Actual    []Line
	Projected []Line
	ByClient  []GroupRow
	ByType    []GroupRow
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) actual(ctx context.Context, p Period) ([]Entry, error) {
	rows, err := s.q.ListRenewalsPaidBetween(ctx, sqlc.ListRenewalsPaidBetweenParams{StartDate: p.Start, EndDate: p.End})
	if err != nil {
		return nil, fmt.Errorf("list renewals: %w", err)
	}
	out := make([]Entry, len(rows))
	for i, r := range rows {
		out[i] = Entry{Date: r.PaidOn, Currency: money.Currency(r.Currency), Cost: r.CostPrice, Selling: r.SellingPrice,
			ClientID: deref(r.ClientID), ClientName: deref(r.ClientName), TypeID: r.AssetTypeID, TypeName: r.TypeName}
	}
	return out, nil
}

func (s *Service) projectable(ctx context.Context, end time.Time) ([]Projectable, error) {
	rows, err := s.q.ListProjectableAssets(ctx, end)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	out := make([]Projectable, len(rows))
	for i, r := range rows {
		out[i] = Projectable{Expiration: r.ExpirationDate, PeriodMonths: int(r.RenewalPeriodMonths), Currency: money.Currency(r.Currency),
			Cost: r.CostPrice, Selling: r.SellingPrice, ClientID: deref(r.ClientID), ClientName: deref(r.ClientName), TypeID: r.AssetTypeID, TypeName: r.TypeName}
	}
	return out, nil
}

func (s *Service) Report(ctx context.Context, p Period) (Report, error) {
	actual, err := s.actual(ctx, p)
	if err != nil {
		return Report{}, err
	}
	items, err := s.projectable(ctx, p.End)
	if err != nil {
		return Report{}, err
	}
	projected := Project(items, p)
	return Report{
		Period: p, Actual: Sum(actual), Projected: Sum(projected),
		ByClient: Breakdown(actual, projected, ByClient), ByType: Breakdown(actual, projected, ByType),
	}, nil
}

func (s *Service) Summary(ctx context.Context, p Period) ([]Line, []Line, error) {
	actual, err := s.actual(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	items, err := s.projectable(ctx, p.End)
	if err != nil {
		return nil, nil, err
	}
	return Sum(actual), Sum(Project(items, p)), nil
}

func (s *Service) Projection(ctx context.Context, first time.Time) ([]MonthRow, error) {
	start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	items, err := s.projectable(ctx, start.AddDate(1, 0, 0))
	if err != nil {
		return nil, err
	}
	return TwelveMonths(items, start), nil
}
