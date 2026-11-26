package finance

import (
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestOccurrencesExtrapolate(t *testing.T) {
	year := YearPeriod(2027)
	if got := Occurrences(d("2026-12-15"), 1, year); len(got) != 12 || !got[0].Equal(d("2027-01-15")) || !got[11].Equal(d("2027-12-15")) {
		t.Fatalf("monthly in a year = %v", got)
	}
	if got := Occurrences(d("2027-03-31"), 12, year); len(got) != 1 {
		t.Fatalf("yearly = %v", got)
	}
	if got := Occurrences(d("2026-01-31"), 1, MonthPeriod(2026, time.February)); len(got) != 1 || !got[0].Equal(d("2026-02-28")) {
		t.Fatalf("month-end clamping = %v", got)
	}
	if got := Occurrences(d("2028-01-01"), 12, year); len(got) != 0 {
		t.Fatalf("future = %v", got)
	}
	if got := Occurrences(d("2027-02-10"), 24, YearPeriod(2028)); len(got) != 0 {
		t.Fatalf("24-month cycle should skip 2028: %v", got)
	}
}

func TestSumAndMarginNeverMixCurrencies(t *testing.T) {
	entries := []Entry{
		{Currency: money.IDR, Cost: 18000000, Selling: money.Some(25000000)},
		{Currency: money.IDR, Cost: 5000000},
		{Currency: money.USD, Cost: 1200, Selling: money.Some(2000)},
	}
	lines := Sum(entries)
	if len(lines) != 2 || lines[0].Currency != money.IDR || lines[1].Currency != money.USD {
		t.Fatalf("lines = %+v", lines)
	}
	idr := lines[0]
	if idr.Cost != 23000000 || idr.Revenue != 25000000 || idr.Margin != 7000000 || idr.Count != 2 {
		t.Fatalf("idr = %+v", idr)
	}
	if lines[1].Cost != 1200 || lines[1].Margin != 800 {
		t.Fatalf("usd = %+v", lines[1])
	}
	if len(Sum(nil)) != 0 {
		t.Fatal("empty sum")
	}
}

func TestTwelveMonthsAndBreakdown(t *testing.T) {
	items := []Projectable{
		{Expiration: d("2026-12-05"), PeriodMonths: 1, Currency: money.USD, Cost: 1000, Selling: money.Some(1500), ClientID: "c1", ClientName: "Acme", TypeID: "vps", TypeName: "VPS"},
		{Expiration: d("2027-03-01"), PeriodMonths: 12, Currency: money.IDR, Cost: 180000, TypeID: "dom", TypeName: "Domain"},
	}
	rows := TwelveMonths(items, nil, d("2026-11-21"))
	if len(rows) != 12 || !rows[0].Month.Equal(d("2026-11-01")) {
		t.Fatalf("rows start = %v", rows[0].Month)
	}
	if len(rows[0].Lines) != 0 {
		t.Fatalf("november should be empty: %+v", rows[0].Lines)
	}
	if len(rows[1].Lines) != 1 || rows[1].Lines[0].Cost != 1000 {
		t.Fatalf("december = %+v", rows[1].Lines)
	}
	if len(rows[4].Lines) != 2 {
		t.Fatalf("march should have both currencies: %+v", rows[4].Lines)
	}

	year := YearPeriod(2027)
	proj := Project(items, year)
	actual := []Entry{{Date: d("2027-02-01"), Currency: money.IDR, Cost: 50000, TypeID: "dom", TypeName: "Domain"}}
	byClient := Breakdown(actual, proj, ByClient)
	if len(byClient) != 2 || byClient[0].Name != "Internal" || byClient[1].Name != "Acme" {
		t.Fatalf("by client = %+v", byClient)
	}
	if byClient[0].Actual.Cost != 50000 || byClient[0].Projected.Cost != 180000 {
		t.Fatalf("internal row = %+v", byClient[0])
	}
	if byClient[1].Projected.Count != 12 || byClient[1].Projected.Margin != 6000 {
		t.Fatalf("acme row = %+v", byClient[1])
	}
	byType := Breakdown(actual, proj, ByType)
	if len(byType) != 2 {
		t.Fatalf("by type = %+v", byType)
	}
}

func TestAnnualize(t *testing.T) {
	got := Annualize([]Recurring{
		{Currency: "IDR", Cost: 100000, Selling: money.Some(150000), PeriodMonths: 1},
		{Currency: "IDR", Cost: 240000, PeriodMonths: 24},
		{Currency: "USD", Cost: 1000, Selling: money.Some(1200), PeriodMonths: 12},
	})
	if len(got) != 2 || got[0].Cost != 1320000 || got[0].Revenue != 1800000 || got[0].Margin != 600000 {
		t.Fatalf("annualize = %+v", got)
	}
	if got[1].Margin != 200 {
		t.Fatalf("usd = %+v", got[1])
	}
}
