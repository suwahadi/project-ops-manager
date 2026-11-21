package finance_test

import (
	"context"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/document"
	"github.com/suwahadi/project-ops-manager/internal/finance"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestRenewAndReport(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	vps, err := q.GetAssetTypeBySlug(ctx, "vps")
	if err != nil {
		t.Fatal(err)
	}
	assets := asset.NewService(pool, t.TempDir())
	clientID, err := q.CreateClient(ctx, sqlc.CreateClientParams{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	cid := clientID.ID
	id, err := assets.Create(ctx, asset.Input{TypeID: vps.ID, ProviderName: "Host", ClientID: &cid, Name: "vps-1", Expiration: day("2026-11-25"),
		RenewalMonths: 1, Cost: money.FromCents(1000), Selling: money.Some(money.FromCents(1500)), Currency: money.USD, Custom: assettype.Values{}})
	if err != nil {
		t.Fatal(err)
	}
	docs := []document.Prepared{{OriginalName: "inv.pdf", StoredPath: "2026/11/x.pdf", ContentType: "application/pdf", Size: 10}}
	if _, err := assets.Renew(ctx, id, asset.RenewInput{NewExpiration: day("2026-12-25"), Cost: money.FromCents(1100), Selling: money.Some(money.FromCents(1600)), PaidOn: day("2026-11-20")}, docs); err != nil {
		t.Fatal(err)
	}
	d, err := assets.Get(ctx, id, day("2026-11-21"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.ExpirationDate.Equal(day("2026-12-25")) || d.CostPrice != 1100 || d.SellingPrice.Amount != 1600 {
		t.Fatalf("asset not updated: %+v", d.GetAssetRow)
	}
	ren, err := assets.Renewals(ctx, id)
	if err != nil || len(ren) != 1 || !ren[0].PreviousExpirationDate.Equal(day("2026-11-25")) {
		t.Fatalf("renewal row: %+v %v", ren, err)
	}

	svc := finance.NewService(pool)
	rep, err := svc.Report(ctx, finance.MonthPeriod(2026, time.November))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Actual) != 1 || rep.Actual[0].Cost != 1100 || rep.Actual[0].Margin != 500 {
		t.Fatalf("actual = %+v", rep.Actual)
	}
	if len(rep.Projected) != 0 {
		t.Fatalf("november projection should be empty after renewal: %+v", rep.Projected)
	}
	year, err := svc.Report(ctx, finance.YearPeriod(2027))
	if err != nil {
		t.Fatal(err)
	}
	if len(year.Projected) != 1 || year.Projected[0].Count != 12 || year.Projected[0].Cost != 13200 {
		t.Fatalf("2027 projection = %+v", year.Projected)
	}
	if len(year.ByClient) != 1 || year.ByClient[0].Name != "Acme" {
		t.Fatalf("by client = %+v", year.ByClient)
	}
	if err := assets.Cancel(ctx, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	year, _ = svc.Report(ctx, finance.YearPeriod(2027))
	if len(year.Projected) != 0 {
		t.Fatal("cancelled assets must not be projected")
	}
}
