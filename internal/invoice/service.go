package invoice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

var (
	ErrNotFound    = errors.New("invoice not found")
	ErrNumberTaken = errors.New("invoice number is already used")
)

type Summary struct {
	sqlc.ListInvoicesRow
	Totals Totals
}

type Invoice struct {
	sqlc.GetInvoiceRow
	Items  []sqlc.InvoiceItem
	Totals Totals
}

func (s Summary) IsPaid() bool { return s.Status == StatusPaid }

func (i Invoice) IsPaid() bool { return i.Status == StatusPaid }

func (i Invoice) CurrencyCode() money.Currency { return money.Currency(i.Currency) }

func Overdue(status string, due *time.Time, today time.Time) bool {
	return status == StatusUnpaid && due != nil && due.Before(today)
}

func ItemLine(it sqlc.InvoiceItem) Line {
	return Line{Quantity: it.Quantity, Rate: it.Rate, TaxPercent: it.TaxPercent}
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlc.New(pool)}
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.q.ListInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	items, err := s.q.ListAllInvoiceItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoice items: %w", err)
	}
	lines := map[string][]Line{}
	for _, it := range items {
		lines[it.InvoiceID] = append(lines[it.InvoiceID], Line{Quantity: it.Quantity, Rate: it.Rate, TaxPercent: it.TaxPercent})
	}
	out := make([]Summary, len(rows))
	for i, r := range rows {
		out[i] = Summary{ListInvoicesRow: r, Totals: Compute(lines[r.ID], r.Status == StatusPaid)}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Invoice, error) {
	row, err := s.q.GetInvoice(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, fmt.Errorf("get invoice: %w", err)
	}
	items, err := s.q.ListInvoiceItems(ctx, id)
	if err != nil {
		return Invoice{}, fmt.Errorf("list invoice items: %w", err)
	}
	lines := make([]Line, len(items))
	for i, it := range items {
		lines[i] = ItemLine(it)
	}
	return Invoice{GetInvoiceRow: row, Items: items, Totals: Compute(lines, row.Status == StatusPaid)}, nil
}

func (s *Service) NumberTaken(ctx context.Context, number string, exclude *string) (bool, error) {
	return s.q.InvoiceNumberTaken(ctx, sqlc.InvoiceNumberTakenParams{Number: number, ExcludeID: exclude})
}

func (s *Service) UniqueNumber(ctx context.Context, prefix string) (string, error) {
	for range 10 {
		n, err := GenerateNumber(prefix)
		if err != nil {
			return "", err
		}
		taken, err := s.NumberTaken(ctx, n, nil)
		if err != nil {
			return "", fmt.Errorf("check invoice number: %w", err)
		}
		if !taken {
			return n, nil
		}
	}
	return "", errors.New("could not find a free invoice number")
}

func status(in Input) (string, *time.Time) {
	if in.Paid {
		return StatusPaid, in.PaidDate
	}
	return StatusUnpaid, nil
}

func (s *Service) Create(ctx context.Context, in Input) (string, error) {
	var id string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		if err := checkNumber(ctx, q, in.Number, nil); err != nil {
			return err
		}
		st, paid := status(in)
		var err error
		id, err = q.CreateInvoice(ctx, sqlc.CreateInvoiceParams{
			Number: in.Number, ClientID: in.ClientID, AssetID: in.AssetID, BillTo: in.BillTo, Currency: string(in.Currency),
			InvoiceDate: in.InvoiceDate, DueDate: in.DueDate, Status: st, PaidDate: paid, Note: in.Note,
		})
		if err != nil {
			return fmt.Errorf("create invoice: %w", err)
		}
		return writeItems(ctx, q, id, in.Items)
	})
	return id, err
}

func (s *Service) Update(ctx context.Context, id string, in Input) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		if err := checkNumber(ctx, q, in.Number, &id); err != nil {
			return err
		}
		st, paid := status(in)
		if err := q.UpdateInvoice(ctx, sqlc.UpdateInvoiceParams{
			ID: id, Number: in.Number, ClientID: in.ClientID, BillTo: in.BillTo, Currency: string(in.Currency),
			InvoiceDate: in.InvoiceDate, DueDate: in.DueDate, Status: st, PaidDate: paid, Note: in.Note,
		}); err != nil {
			return fmt.Errorf("update invoice: %w", err)
		}
		if err := q.DeleteInvoiceItems(ctx, id); err != nil {
			return fmt.Errorf("clear invoice items: %w", err)
		}
		return writeItems(ctx, q, id, in.Items)
	})
}

func (s *Service) MarkPaid(ctx context.Context, id string, paidDate time.Time) error {
	return s.setStatus(ctx, id, StatusPaid, &paidDate)
}

func (s *Service) MarkUnpaid(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusUnpaid, nil)
}

func (s *Service) setStatus(ctx context.Context, id, st string, paid *time.Time) error {
	n, err := s.q.SetInvoiceStatus(ctx, sqlc.SetInvoiceStatusParams{ID: id, Status: st, PaidDate: paid})
	if err != nil {
		return fmt.Errorf("set invoice status: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	n, err := s.q.DeleteInvoice(ctx, id)
	if err != nil {
		return fmt.Errorf("delete invoice: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func checkNumber(ctx context.Context, q *sqlc.Queries, number string, exclude *string) error {
	taken, err := q.InvoiceNumberTaken(ctx, sqlc.InvoiceNumberTakenParams{Number: number, ExcludeID: exclude})
	if err != nil {
		return fmt.Errorf("check invoice number: %w", err)
	}
	if taken {
		return ErrNumberTaken
	}
	return nil
}

func writeItems(ctx context.Context, q *sqlc.Queries, id string, items []Item) error {
	for i, it := range items {
		if err := q.AddInvoiceItem(ctx, sqlc.AddInvoiceItemParams{
			InvoiceID: id, Position: int32(i + 1), Title: it.Title, Description: it.Description,
			Quantity: it.Quantity, Rate: it.Rate, TaxPercent: it.TaxPercent,
		}); err != nil {
			return fmt.Errorf("add invoice item: %w", err)
		}
	}
	return nil
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
