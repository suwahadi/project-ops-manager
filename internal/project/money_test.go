package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/finance"
	"github.com/suwahadi/project-ops-manager/internal/invoice"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/project"
)

func TestPaymentsIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	svc := project.NewService(pool, time.Now)
	clients := client.NewService(pool)
	acme, err := clients.Create(ctx, client.Input{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	day := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}
	billing := day("2026-12-15")
	in := project.Input{Name: "Website", ClientID: &acme, Currency: money.IDR,
		Value: money.Some(money.FromCents(1000000000)), Cost: money.Some(money.FromCents(400000000)), BillingDate: &billing}
	id, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	internalID, err := svc.Create(ctx, project.Input{Name: "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordPayment(ctx, internalID, project.PaymentInput{ReceivedOn: day("2026-11-01"), Amount: 100}); !errors.Is(err, project.ErrNoValue) {
		t.Fatalf("internal payment: %v", err)
	}

	inv := invoice.NewService(pool)
	invID, err := inv.Create(ctx, invoice.Input{Number: "INV-1", ClientID: &acme, BillTo: "Acme", Currency: money.IDR, InvoiceDate: day("2026-11-10"),
		Items: []invoice.Item{{ProjectID: &id, Title: "Website", Quantity: money.FromCents(100), Rate: money.FromCents(500000000)}}})
	if err != nil {
		t.Fatal(err)
	}
	dp := project.PaymentInput{ReceivedOn: day("2026-11-10"), Amount: money.FromCents(500000000), InvoiceID: &invID}
	if err := svc.RecordPayment(ctx, id, dp); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordPayment(ctx, id, dp); !errors.Is(err, project.ErrAlreadyRecorded) {
		t.Fatalf("second invoice payment: %v", err)
	}
	if err := svc.RecordPayment(ctx, id, project.PaymentInput{ReceivedOn: day("2026-11-20"), Amount: money.FromCents(500000001)}); !errors.Is(err, project.ErrOverpaid) {
		t.Fatalf("overpayment: %v", err)
	}
	recorded, err := svc.InvoicePayments(ctx, invID)
	if err != nil || !recorded[id] {
		t.Fatalf("invoice payments: %v %v", recorded, err)
	}

	p, _ := svc.Get(ctx, id)
	m, err := svc.Money(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status() != project.PaymentPartial || m.Remaining() != money.FromCents(500000000) || m.PercentReceived() != 50 {
		t.Fatalf("money: %+v", m)
	}

	edit := in
	edit.Value = money.Some(money.FromCents(400000000))
	if err := svc.Update(ctx, id, edit); !errors.Is(err, project.ErrValueBelowPaid) {
		t.Fatalf("lower value: %v", err)
	}
	edit = in
	edit.Currency = money.USD
	if err := svc.Update(ctx, id, edit); !errors.Is(err, project.ErrCurrencyLocked) {
		t.Fatalf("currency: %v", err)
	}
	edit = in
	edit.ClientID = nil
	if err := svc.Update(ctx, id, edit); !errors.Is(err, project.ErrInternalPayments) {
		t.Fatalf("internal: %v", err)
	}

	fin := finance.NewService(pool)
	today := day("2026-11-26")
	rep, err := fin.Report(ctx, finance.MonthPeriod(2026, time.November), today)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Actual) != 1 || rep.Actual[0].Revenue != money.FromCents(500000000) || rep.Actual[0].Cost != money.FromCents(200000000) {
		t.Fatalf("november actual: %+v", rep.Actual)
	}
	dec, err := fin.Report(ctx, finance.MonthPeriod(2026, time.December), today)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec.Projected) != 1 || dec.Projected[0].Revenue != money.FromCents(500000000) || dec.Projected[0].Cost != money.FromCents(200000000) {
		t.Fatalf("december projected: %+v", dec.Projected)
	}

	if err := clients.UnlinkProject(ctx, acme, id); !errors.Is(err, client.ErrHasPayments) {
		t.Fatalf("unlink paid project: %v", err)
	}
	payments, _ := svc.Payments(ctx, id)
	if len(payments) != 1 || payments[0].InvoiceNumber == nil || *payments[0].InvoiceNumber != "INV-1" {
		t.Fatalf("payments: %+v", payments)
	}
	if err := svc.DeletePayment(ctx, id, payments[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePayment(ctx, id, payments[0].ID); !errors.Is(err, project.ErrPaymentNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	totals := project.ClientTotals(must(svc.ClientProjects(ctx, acme)))
	if len(totals) != 1 || totals[0].Due != money.FromCents(1000000000) || totals[0].Received != 0 {
		t.Fatalf("client totals: %+v", totals)
	}
	if err := clients.UnlinkProject(ctx, acme, id); err != nil {
		t.Fatalf("unlink unpaid project: %v", err)
	}
	p, _ = svc.Get(ctx, id)
	if p.ClientID != nil || p.ProjectValue.Valid || p.ProjectCost.Valid || p.BillingDate != nil {
		t.Fatalf("unlink should clear pricing: %+v", p)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
