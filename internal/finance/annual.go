package finance

import (
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type Recurring struct {
	Currency     string
	Cost         money.Amount
	Selling      money.NullAmount
	PeriodMonths int
}

type AnnualTotal struct {
	Currency money.Currency
	Cost     money.Amount
	Revenue  money.Amount
	Margin   money.Amount
}

func perYear(a money.Amount, months int) money.Amount {
	if months <= 0 {
		return 0
	}
	c := a.Cents() * 12
	q := c / int64(months)
	if r := c % int64(months); r*2 >= int64(months) {
		q++
	}
	return money.FromCents(q)
}

func Annualize(items []Recurring) []AnnualTotal {
	totals := map[money.Currency]*AnnualTotal{}
	for _, it := range items {
		cur := money.Currency(it.Currency)
		t, ok := totals[cur]
		if !ok {
			t = &AnnualTotal{Currency: cur}
			totals[cur] = t
		}
		cost := perYear(it.Cost, it.PeriodMonths)
		t.Cost += cost
		if it.Selling.Valid {
			rev := perYear(it.Selling.Amount, it.PeriodMonths)
			t.Revenue += rev
			t.Margin += rev - cost
		}
	}
	var out []AnnualTotal
	for _, c := range money.Currencies {
		if t, ok := totals[c]; ok {
			out = append(out, *t)
		}
	}
	return out
}
