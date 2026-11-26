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
	BySource  []GroupRow
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
			ClientID: deref(r.ClientID), ClientName: deref(r.ClientName), TypeID: r.AssetTypeID, TypeName: r.TypeName, Source: SourceAsset}
	}
	pays, err := s.q.ListProjectPaymentsForFinance(ctx, p.End)
	if err != nil {
		return nil, fmt.Errorf("list project payments: %w", err)
	}
	payments := make([]ProjectPayment, len(pays))
	for i, r := range pays {
		cost := money.Amount(0)
		if r.ProjectCost.Valid {
			cost = r.ProjectCost.Amount
		}
		payments[i] = ProjectPayment{ProjectID: r.ProjectID, ReceivedOn: r.ReceivedOn, Amount: r.Amount, Value: r.ProjectValue.Amount,
			Cost: cost, Currency: money.Currency(r.Currency), ClientID: deref(r.ClientID), ClientName: deref(r.ClientName)}
	}
	return append(out, ProjectActual(payments, p)...), nil
}

func (s *Service) billable(ctx context.Context) ([]BillableProject, error) {
	rows, err := s.q.ListBillableProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billable projects: %w", err)
	}
	out := make([]BillableProject, 0, len(rows))
	for _, r := range rows {
		cost := money.Amount(0)
		if r.ProjectCost.Valid {
			cost = r.ProjectCost.Amount
		}
		out = append(out, BillableProject{BillingDate: *r.BillingDate, Value: r.ProjectValue.Amount, Cost: cost, Received: r.Received,
			Currency: money.Currency(r.Currency), ClientID: deref(r.ClientID), ClientName: deref(r.ClientName)})
	}
	return out, nil
}

func (s *Service) projected(ctx context.Context, p Period, today time.Time) ([]Entry, error) {
	items, err := s.projectable(ctx, p.End)
	if err != nil {
		return nil, err
	}
	projects, err := s.billable(ctx)
	if err != nil {
		return nil, err
	}
	return append(Project(items, p), ProjectProjected(projects, p, today)...), nil
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

func (s *Service) Report(ctx context.Context, p Period, today time.Time) (Report, error) {
	actual, err := s.actual(ctx, p)
	if err != nil {
		return Report{}, err
	}
	projected, err := s.projected(ctx, p, today)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Period: p, Actual: Sum(actual), Projected: Sum(projected),
		ByClient: Breakdown(actual, projected, ByClient), ByType: Breakdown(actual, projected, ByType),
		BySource: Breakdown(actual, projected, BySource),
	}, nil
}

func (s *Service) Summary(ctx context.Context, p Period, today time.Time) ([]Line, []Line, error) {
	actual, err := s.actual(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	projected, err := s.projected(ctx, p, today)
	if err != nil {
		return nil, nil, err
	}
	return Sum(actual), Sum(projected), nil
}

func (s *Service) Projection(ctx context.Context, today time.Time) ([]MonthRow, error) {
	start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	items, err := s.projectable(ctx, start.AddDate(1, 0, 0))
	if err != nil {
		return nil, err
	}
	projects, err := s.billable(ctx)
	if err != nil {
		return nil, err
	}
	return TwelveMonths(items, projects, today), nil
}
