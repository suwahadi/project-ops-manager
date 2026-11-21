package invoice_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/invoice"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

func TestServiceIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	svc := invoice.NewService(pool)
	clients := client.NewService(pool)
	acme, err := clients.Create(ctx, client.Input{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	in := invoice.Input{
		Number: "INV-DEV-AAAA0001", ClientID: &acme, BillTo: "Acme", Currency: money.IDR, InvoiceDate: day, Note: "Bank",
		Items: []invoice.Item{
			{Title: "Renew acme.com", Description: "1 year", Quantity: money.FromCents(100), Rate: money.FromCents(78500000)},
			{Title: "Support", Quantity: money.FromCents(200), Rate: money.FromCents(10000000), TaxPercent: money.FromCents(1100)},
		},
	}
	id, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	dup := in
	dup.Number = "inv-dev-aaaa0001"
	if _, err := svc.Create(ctx, dup); !errors.Is(err, invoice.ErrNumberTaken) {
		t.Fatalf("duplicate number: got %v", err)
	}
	inv, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 2 || inv.Items[0].Title != "Renew acme.com" || inv.Items[1].Position != 2 {
		t.Fatalf("items: %+v", inv.Items)
	}
	if inv.Totals.Total != money.FromCents(100700000) || inv.Totals.Due != inv.Totals.Total || inv.IsPaid() {
		t.Fatalf("totals: %+v", inv.Totals)
	}

	in.Items = in.Items[1:]
	in.DueDate = &day
	if err := svc.Update(ctx, id, in); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkPaid(ctx, id, day); err != nil {
		t.Fatal(err)
	}
	inv, err = svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !inv.IsPaid() || len(inv.Items) != 1 || inv.Totals.Paid != money.FromCents(22200000) || inv.Totals.Due != 0 {
		t.Fatalf("after update and payment: %+v %+v", inv.GetInvoiceRow, inv.Totals)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Totals.Total != money.FromCents(22200000) || deref(list[0].ClientName) != "Acme" {
		t.Fatalf("list: %+v", list)
	}
	if err := svc.MarkUnpaid(ctx, id); err != nil {
		t.Fatal(err)
	}
	if inv, _ = svc.Get(ctx, id); inv.IsPaid() || inv.PaidDate != nil {
		t.Fatal("still paid")
	}
	if n, err := svc.UniqueNumber(ctx, "INV-"); err != nil || len(n) != 12 {
		t.Fatalf("unique number %q %v", n, err)
	}
	if err := clients.Delete(ctx, acme); err != nil {
		t.Fatalf("delete client with invoice: %v", err)
	}
	if inv, _ = svc.Get(ctx, id); inv.ClientID != nil || inv.BillTo != "Acme" {
		t.Fatal("client delete should keep Bill To and clear the link")
	}
	if err := svc.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, id); !errors.Is(err, invoice.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
