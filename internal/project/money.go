package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

var (
	ErrNoValue          = errors.New("project has no value")
	ErrOverpaid         = errors.New("payment exceeds the remaining value")
	ErrValueBelowPaid   = errors.New("project value is below the amount received")
	ErrCurrencyLocked   = errors.New("currency cannot change after payments")
	ErrInternalPayments = errors.New("a project with payments must keep its client")
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrAlreadyRecorded  = errors.New("this invoice payment is already recorded")
)

const (
	PaymentNone    = "none"
	PaymentUnpaid  = "unpaid"
	PaymentPartial = "partial"
	PaymentPaid    = "paid"
)

type Money struct {
	Currency money.Currency
	Value    money.NullAmount
	Cost     money.NullAmount
	Received money.Amount
}

func (m Money) Remaining() money.Amount {
	if !m.Value.Valid || m.Received >= m.Value.Amount {
		return 0
	}
	return m.Value.Amount - m.Received
}

func (m Money) MarginValue() money.NullAmount {
	if !m.Value.Valid {
		return money.NullAmount{}
	}
	cost := money.Amount(0)
	if m.Cost.Valid {
		cost = m.Cost.Amount
	}
	return money.Some(m.Value.Amount - cost)
}

func (m Money) Status() string {
	switch {
	case !m.Value.Valid:
		return PaymentNone
	case m.Received <= 0 && m.Value.Amount > 0:
		return PaymentUnpaid
	case m.Received < m.Value.Amount:
		return PaymentPartial
	}
	return PaymentPaid
}

func (m Money) PercentReceived() int {
	if !m.Value.Valid || m.Value.Amount <= 0 {
		return 0
	}
	p := int(m.Received.Cents() * 100 / m.Value.Amount.Cents())
	return min(p, 100)
}

func StatusLabel(status string) string {
	switch status {
	case PaymentUnpaid:
		return "Unpaid"
	case PaymentPartial:
		return "Partially paid"
	case PaymentPaid:
		return "Paid"
	}
	return "No value"
}

type PaymentInput struct {
	ReceivedOn time.Time
	Amount     money.Amount
	Note       string
	InvoiceID  *string
}

func checkMoneyChange(cur sqlc.GetProjectRow, received money.Amount, payments int64, in Input) error {
	if payments == 0 {
		return nil
	}
	switch {
	case in.ClientID == nil:
		return ErrInternalPayments
	case string(in.Currency) != cur.Currency:
		return ErrCurrencyLocked
	case !in.Value.Valid || in.Value.Amount < received:
		return ErrValueBelowPaid
	}
	return nil
}

func (s *Service) Money(ctx context.Context, p sqlc.GetProjectRow) (Money, error) {
	r, err := s.q.ProjectReceived(ctx, p.ID)
	if err != nil {
		return Money{}, fmt.Errorf("sum payments: %w", err)
	}
	return Money{Currency: money.Currency(p.Currency), Value: p.ProjectValue, Cost: p.ProjectCost, Received: r.Received}, nil
}

func (s *Service) Payments(ctx context.Context, id string) ([]sqlc.ListProjectPaymentsRow, error) {
	return s.q.ListProjectPayments(ctx, id)
}

func (s *Service) InvoicePayments(ctx context.Context, invoiceID string) (map[string]bool, error) {
	ids, err := s.q.ListInvoiceProjectPayments(ctx, &invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list invoice payments: %w", err)
	}
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (s *Service) RecordPayment(ctx context.Context, id string, in PaymentInput) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		p, err := q.LockProject(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock project: %w", err)
		}
		if !p.ProjectValue.Valid || p.ClientID == nil {
			return ErrNoValue
		}
		r, err := q.ProjectReceived(ctx, id)
		if err != nil {
			return fmt.Errorf("sum payments: %w", err)
		}
		if r.Received+in.Amount > p.ProjectValue.Amount {
			return ErrOverpaid
		}
		if in.InvoiceID != nil {
			recorded, err := q.ListInvoiceProjectPayments(ctx, in.InvoiceID)
			if err != nil {
				return fmt.Errorf("list invoice payments: %w", err)
			}
			for _, pid := range recorded {
				if pid == id {
					return ErrAlreadyRecorded
				}
			}
		}
		_, err = q.CreateProjectPayment(ctx, sqlc.CreateProjectPaymentParams{
			ProjectID: id, ReceivedOn: in.ReceivedOn, Amount: in.Amount, Note: in.Note, InvoiceID: in.InvoiceID,
		})
		if err != nil {
			return fmt.Errorf("record payment: %w", err)
		}
		return nil
	})
}

func (s *Service) DeletePayment(ctx context.Context, id, paymentID string) error {
	n, err := s.q.DeleteProjectPayment(ctx, sqlc.DeleteProjectPaymentParams{ID: paymentID, ProjectID: id})
	if err != nil {
		return fmt.Errorf("delete payment: %w", err)
	}
	if n == 0 {
		return ErrPaymentNotFound
	}
	return nil
}

func (s *Service) ClientProjects(ctx context.Context, clientID string) ([]sqlc.ListProjectsByClientWithMoneyRow, error) {
	return s.q.ListProjectsByClientWithMoney(ctx, &clientID)
}

type CurrencyTotal struct {
	Currency money.Currency
	Value    money.Amount
	Received money.Amount
	Due      money.Amount
}

func ClientTotals(rows []sqlc.ListProjectsByClientWithMoneyRow) []CurrencyTotal {
	by := map[money.Currency]*CurrencyTotal{}
	for _, r := range rows {
		if !r.ProjectValue.Valid {
			continue
		}
		cur := money.Currency(r.Currency)
		t, ok := by[cur]
		if !ok {
			t = &CurrencyTotal{Currency: cur}
			by[cur] = t
		}
		m := Money{Currency: cur, Value: r.ProjectValue, Received: r.Received}
		t.Value += r.ProjectValue.Amount
		t.Received += r.Received
		t.Due += m.Remaining()
	}
	var out []CurrencyTotal
	for _, c := range money.Currencies {
		if t, ok := by[c]; ok {
			out = append(out, *t)
		}
	}
	return out
}
