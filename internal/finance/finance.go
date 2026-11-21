package finance

import (
	"sort"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type Period struct {
	Kind  string
	Start time.Time
	End   time.Time
}

func MonthPeriod(year int, month time.Month) Period {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	return Period{Kind: "month", Start: start, End: start.AddDate(0, 1, 0)}
}

func YearPeriod(year int) Period {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	return Period{Kind: "year", Start: start, End: start.AddDate(1, 0, 0)}
}

func (p Period) Label() string {
	if p.Kind == "year" {
		return p.Start.Format("2006")
	}
	return p.Start.Format("January 2006")
}

func (p Period) Contains(d time.Time) bool {
	d = asset.Date(d)
	return !d.Before(p.Start) && d.Before(p.End)
}

type Entry struct {
	Date       time.Time
	Currency   money.Currency
	Cost       money.Amount
	Selling    money.NullAmount
	ClientID   string
	ClientName string
	TypeID     string
	TypeName   string
}

type Projectable struct {
	Expiration   time.Time
	PeriodMonths int
	Currency     money.Currency
	Cost         money.Amount
	Selling      money.NullAmount
	ClientID     string
	ClientName   string
	TypeID       string
	TypeName     string
}

func Occurrences(expiration time.Time, months int, p Period) []time.Time {
	if months <= 0 {
		return nil
	}
	var out []time.Time
	for n := 0; ; n++ {
		d := asset.AddMonths(expiration, n*months)
		if !d.Before(p.End) {
			break
		}
		if !d.Before(p.Start) {
			out = append(out, d)
		}
		if n > 100000 {
			break
		}
	}
	return out
}

func Project(items []Projectable, p Period) []Entry {
	var out []Entry
	for _, it := range items {
		for _, d := range Occurrences(it.Expiration, it.PeriodMonths, p) {
			out = append(out, Entry{Date: d, Currency: it.Currency, Cost: it.Cost, Selling: it.Selling,
				ClientID: it.ClientID, ClientName: it.ClientName, TypeID: it.TypeID, TypeName: it.TypeName})
		}
	}
	return out
}

type Line struct {
	Currency money.Currency
	Cost     money.Amount
	Revenue  money.Amount
	Margin   money.Amount
	Count    int
}

func (l *Line) add(e Entry) {
	l.Cost += e.Cost
	l.Count++
	if e.Selling.Valid {
		l.Revenue += e.Selling.Amount
		l.Margin += e.Selling.Amount - e.Cost
	}
}

func Sum(entries []Entry) []Line {
	byCur := map[money.Currency]*Line{}
	for _, e := range entries {
		l, ok := byCur[e.Currency]
		if !ok {
			l = &Line{Currency: e.Currency}
			byCur[e.Currency] = l
		}
		l.add(e)
	}
	out := []Line{}
	for _, c := range money.Currencies {
		if l, ok := byCur[c]; ok {
			out = append(out, *l)
		}
	}
	return out
}

type MonthRow struct {
	Month time.Time
	Lines []Line
}

func TwelveMonths(items []Projectable, first time.Time) []MonthRow {
	start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows := make([]MonthRow, 0, 12)
	for i := 0; i < 12; i++ {
		m := start.AddDate(0, i, 0)
		p := MonthPeriod(m.Year(), m.Month())
		rows = append(rows, MonthRow{Month: m, Lines: Sum(Project(items, p))})
	}
	return rows
}

type GroupRow struct {
	Key       string
	Name      string
	Currency  money.Currency
	Actual    Line
	Projected Line
}

const InternalKey = "internal"

func Breakdown(actual, projected []Entry, by func(Entry) (string, string)) []GroupRow {
	type gk struct {
		key string
		cur money.Currency
	}
	rows := map[gk]*GroupRow{}
	get := func(e Entry) *GroupRow {
		key, name := by(e)
		k := gk{key, e.Currency}
		r, ok := rows[k]
		if !ok {
			r = &GroupRow{Key: key, Name: name, Currency: e.Currency, Actual: Line{Currency: e.Currency}, Projected: Line{Currency: e.Currency}}
			rows[k] = r
		}
		return r
	}
	for _, e := range actual {
		get(e).Actual.add(e)
	}
	for _, e := range projected {
		get(e).Projected.add(e)
	}
	out := make([]GroupRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Key == InternalKey) != (b.Key == InternalKey) {
			return a.Key == InternalKey
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Currency < b.Currency
	})
	return out
}

func ByClient(e Entry) (string, string) {
	if e.ClientID == "" {
		return InternalKey, "Internal"
	}
	return e.ClientID, e.ClientName
}

func ByType(e Entry) (string, string) { return e.TypeID, e.TypeName }
