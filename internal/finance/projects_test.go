package finance

import (
	"testing"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

func TestCostRecognized(t *testing.T) {
	cases := []struct{ cost, value, received, want int64 }{
		{400, 1000, 500, 200},
		{400, 1000, 0, 0},
		{400, 1000, 1000, 400},
		{100, 300, 100, 33},
		{100, 300, 200, 67},
		{0, 1000, 500, 0},
		{400, 0, 0, 0},
	}
	for _, c := range cases {
		if got := CostRecognized(money.Amount(c.cost), money.Amount(c.value), money.Amount(c.received)); got != money.Amount(c.want) {
			t.Errorf("CostRecognized(%d,%d,%d) = %d, want %d", c.cost, c.value, c.received, got, c.want)
		}
	}
}

func TestProjectActual(t *testing.T) {
	pay := func(id, date string, amount int64) ProjectPayment {
		return ProjectPayment{ProjectID: id, ReceivedOn: d(date), Amount: money.Amount(amount), Value: 300, Cost: 100, Currency: money.IDR, ClientID: "c1", ClientName: "Acme"}
	}
	payments := []ProjectPayment{pay("p1", "2026-10-10", 100), pay("p1", "2026-11-05", 100), pay("p1", "2026-12-01", 100)}
	var costs, revenue money.Amount
	for _, m := range []Period{MonthPeriod(2026, 10), MonthPeriod(2026, 11), MonthPeriod(2026, 12)} {
		entries := ProjectActual(payments, m)
		if len(entries) != 1 || entries[0].Source != SourceProject || entries[0].TypeName != ProjectTypeName {
			t.Fatalf("%s: %+v", m.Label(), entries)
		}
		costs += entries[0].Cost
		revenue += entries[0].Selling.Amount
	}
	if costs != 100 || revenue != 300 {
		t.Fatalf("shares do not add up: cost %d revenue %d", costs, revenue)
	}
	nov := ProjectActual(payments, MonthPeriod(2026, 11))
	if nov[0].Cost != 34 {
		t.Fatalf("november cost share = %d", nov[0].Cost)
	}
}

func TestProjectProjected(t *testing.T) {
	today := d("2026-11-26")
	items := []BillableProject{
		{BillingDate: d("2026-12-15"), Value: 1000, Cost: 400, Received: 500, Currency: money.IDR},
		{BillingDate: d("2026-09-01"), Value: 200, Cost: 0, Received: 0, Currency: money.USD},
		{BillingDate: d("2026-12-20"), Value: 300, Cost: 100, Received: 300, Currency: money.IDR},
	}
	dec := ProjectProjected(items, MonthPeriod(2026, 12), today)
	if len(dec) != 1 || dec[0].Selling.Amount != 500 || dec[0].Cost != 200 {
		t.Fatalf("december: %+v", dec)
	}
	nov := ProjectProjected(items, MonthPeriod(2026, 11), today)
	if len(nov) != 1 || nov[0].Currency != money.USD || !nov[0].Date.Equal(today) {
		t.Fatalf("overdue billing should land today: %+v", nov)
	}
	if len(ProjectProjected(items, MonthPeriod(2026, 9), today)) != 0 {
		t.Fatal("past months must not show overdue projects")
	}
	rows := TwelveMonths(nil, items, today)
	if len(rows[0].Lines) != 1 || len(rows[1].Lines) != 1 {
		t.Fatalf("twelve months: %+v %+v", rows[0].Lines, rows[1].Lines)
	}
	g := Breakdown(nil, append(dec, nov...), BySource)
	if len(g) != 2 || g[0].Key != SourceProject {
		t.Fatalf("by source: %+v", g)
	}
}
