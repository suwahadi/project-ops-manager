package invoice

import (
	"strconv"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

type RenewalSource struct {
	Name       string
	TypeName   string
	OrderDate  *time.Time
	Expiration time.Time
	Due        bool
	Months     int
	Cost       money.Amount
	Selling    money.NullAmount
	Currency   money.Currency
	Cancelled  bool
}

func RenewalPeriod(months int) string {
	switch {
	case months == 12:
		return "1 year"
	case months > 0 && months%12 == 0:
		return strconv.Itoa(months/12) + " years"
	case months == 1:
		return "1 month"
	}
	return strconv.Itoa(months) + " months"
}

func RenewalItem(src RenewalSource) ItemForm {
	rate := src.Cost
	if src.Selling.Valid {
		rate = src.Selling.Amount
	}
	item := ItemForm{Quantity: "1", Rate: FormatDecimal(rate), Tax: "0"}
	start, end := src.CurrentTerm()
	item.Title = src.Name
	if src.Due {
		start, end = src.Expiration, src.Expiration.AddDate(0, src.Months, -1)
		item.Title = "Renew " + src.Name
	}
	item.Description = src.TypeName + " " + src.Name + " – " + RenewalPeriod(src.Months) + " (" + start.Format("02 Jan 2006") + " – " + end.Format("02 Jan 2006") + ")"
	return item
}

func (src RenewalSource) CurrentTerm() (time.Time, time.Time) {
	start := src.Expiration.AddDate(0, -src.Months, 0)
	if src.OrderDate != nil && src.OrderDate.Before(src.Expiration) && !src.OrderDate.Before(start.AddDate(0, -1, 0)) {
		start = *src.OrderDate
	}
	return start, src.Expiration
}

type ProjectSource struct {
	ID        string
	Name      string
	Currency  money.Currency
	Value     money.Amount
	Remaining money.Amount
}

func ProjectItem(p ProjectSource) ItemForm {
	desc := "Project payment"
	if p.Remaining < p.Value {
		desc = "Remaining balance"
	}
	return ItemForm{ProjectID: p.ID, Title: p.Name, Description: desc, Quantity: "1", Rate: FormatDecimal(p.Remaining), Tax: "0"}
}

func ClientItems(assets []RenewalSource, projects []ProjectSource, preferred money.Currency) (money.Currency, []ItemForm) {
	counts := map[money.Currency]int{}
	for _, s := range assets {
		if !s.Cancelled {
			counts[s.Currency]++
		}
	}
	for _, p := range projects {
		if p.Remaining > 0 {
			counts[p.Currency]++
		}
	}
	cur := preferred
	best := counts[preferred]
	for _, c := range money.Currencies {
		if counts[c] > best {
			cur, best = c, counts[c]
		}
	}
	var items []ItemForm
	for _, s := range assets {
		if !s.Cancelled && s.Currency == cur {
			items = append(items, RenewalItem(s))
		}
	}
	for _, p := range projects {
		if p.Remaining > 0 && p.Currency == cur {
			items = append(items, ProjectItem(p))
		}
	}
	return cur, items
}
