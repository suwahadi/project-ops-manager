package invoice

import (
	"cmp"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

type Option struct{ Value, Label string }

const ClientNone = "none"

var StatusOptions = []Option{
	{"", "All statuses"},
	{StatusUnpaid, "Unpaid"},
	{"overdue", "Overdue"},
	{StatusPaid, "Paid"},
}

var SortOptions = []Option{
	{"", "Invoice date (newest first)"},
	{"date_asc", "Invoice date (oldest first)"},
	{"due_asc", "Due date (soonest first)"},
	{"total_desc", "Total (highest first)"},
	{"number_asc", "Number (A–Z)"},
}

type Filter struct {
	Query  string
	Status string
	Client string
	Sort   string
}

func validOption(opts []Option, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return v
		}
	}
	return ""
}

func ParseFilter(q url.Values, isID func(string) bool) Filter {
	f := Filter{
		Query:  strings.TrimSpace(q.Get("q")),
		Status: validOption(StatusOptions, q.Get("status")),
		Client: q.Get("client"),
		Sort:   validOption(SortOptions, q.Get("sort")),
	}
	if f.Client != ClientNone && !isID(f.Client) {
		f.Client = ""
	}
	return f
}

func (f Filter) Filtered() bool { return f.Query != "" || f.Status != "" || f.Client != "" }

func (f Filter) Active() int {
	n := 0
	for _, v := range []string{f.Status, f.Client, f.Sort} {
		if v != "" {
			n++
		}
	}
	return n
}

func (f Filter) matches(s Summary, today time.Time) bool {
	switch f.Status {
	case StatusPaid:
		if s.Status != StatusPaid {
			return false
		}
	case StatusUnpaid:
		if s.Status != StatusUnpaid {
			return false
		}
	case "overdue":
		if !Overdue(s.Status, s.DueDate, today) {
			return false
		}
	}
	switch {
	case f.Client == ClientNone && s.ClientID != nil:
		return false
	case f.Client != "" && f.Client != ClientNone && (s.ClientID == nil || *s.ClientID != f.Client):
		return false
	}
	if f.Query == "" {
		return true
	}
	q := strings.ToLower(f.Query)
	return strings.Contains(strings.ToLower(s.Number+" "+s.BillTo), q)
}

func Apply(rows []Summary, f Filter, today time.Time) []Summary {
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		if f.matches(r, today) {
			out = append(out, r)
		}
	}
	newest := func(a, b Summary) int {
		return cmp.Or(b.InvoiceDate.Compare(a.InvoiceDate), b.CreatedAt.Compare(a.CreatedAt))
	}
	slices.SortStableFunc(out, func(a, b Summary) int {
		switch f.Sort {
		case "date_asc":
			return newest(b, a)
		case "due_asc":
			switch {
			case a.DueDate == nil && b.DueDate == nil:
				return newest(a, b)
			case a.DueDate == nil:
				return 1
			case b.DueDate == nil:
				return -1
			}
			return cmp.Or(a.DueDate.Compare(*b.DueDate), newest(a, b))
		case "total_desc":
			return cmp.Or(cmp.Compare(b.Totals.Total, a.Totals.Total), newest(a, b))
		case "number_asc":
			return cmp.Compare(a.Number, b.Number)
		}
		return newest(a, b)
	})
	return out
}

type CurrencyTotals struct {
	Currency money.Currency
	Unpaid   money.Amount
	Paid     money.Amount
	Overdue  int
}

func Summarize(rows []Summary, today time.Time) []CurrencyTotals {
	byCur := map[money.Currency]*CurrencyTotals{}
	for _, r := range rows {
		cur := money.Currency(r.Currency)
		t, ok := byCur[cur]
		if !ok {
			t = &CurrencyTotals{Currency: cur}
			byCur[cur] = t
		}
		if r.Status == StatusPaid {
			t.Paid += r.Totals.Total
		} else {
			t.Unpaid += r.Totals.Due
		}
		if Overdue(r.Status, r.DueDate, today) {
			t.Overdue++
		}
	}
	var out []CurrencyTotals
	for _, c := range money.Currencies {
		if t, ok := byCur[c]; ok {
			out = append(out, *t)
		}
	}
	return out
}
