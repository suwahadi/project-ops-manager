package finance

import (
	"math/big"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

const (
	SourceAsset     = "asset"
	SourceProject   = "project"
	ProjectTypeKey  = "project"
	ProjectTypeName = "Project"
)

func CostRecognized(cost, value, received money.Amount) money.Amount {
	if value <= 0 || received <= 0 {
		return 0
	}
	if received >= value {
		return cost
	}
	n := new(big.Int).Mul(big.NewInt(cost.Cents()), big.NewInt(received.Cents()))
	den := big.NewInt(value.Cents())
	q, r := new(big.Int).QuoRem(n, den, new(big.Int))
	if new(big.Int).Mul(r, big.NewInt(2)).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	return money.FromCents(q.Int64())
}

type ProjectPayment struct {
	ProjectID  string
	ReceivedOn time.Time
	Amount     money.Amount
	Value      money.Amount
	Cost       money.Amount
	Currency   money.Currency
	ClientID   string
	ClientName string
}

func ProjectActual(payments []ProjectPayment, p Period) []Entry {
	var out []Entry
	received := map[string]money.Amount{}
	for _, pay := range payments {
		before := received[pay.ProjectID]
		after := before + pay.Amount
		received[pay.ProjectID] = after
		if !p.Contains(pay.ReceivedOn) {
			continue
		}
		cost := CostRecognized(pay.Cost, pay.Value, after) - CostRecognized(pay.Cost, pay.Value, before)
		out = append(out, Entry{
			Date: pay.ReceivedOn, Currency: pay.Currency, Cost: cost, Selling: money.Some(pay.Amount),
			ClientID: pay.ClientID, ClientName: pay.ClientName, TypeID: ProjectTypeKey, TypeName: ProjectTypeName, Source: SourceProject,
		})
	}
	return out
}

type BillableProject struct {
	BillingDate time.Time
	Value       money.Amount
	Cost        money.Amount
	Received    money.Amount
	Currency    money.Currency
	ClientID    string
	ClientName  string
}

func (b BillableProject) dueDate(today time.Time) time.Time {
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if b.BillingDate.Before(monthStart) {
		return time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	}
	return b.BillingDate
}

func ProjectProjected(items []BillableProject, p Period, today time.Time) []Entry {
	var out []Entry
	for _, b := range items {
		if b.Received >= b.Value {
			continue
		}
		d := b.dueDate(today)
		if !p.Contains(d) {
			continue
		}
		cost := b.Cost - CostRecognized(b.Cost, b.Value, b.Received)
		out = append(out, Entry{
			Date: d, Currency: b.Currency, Cost: cost, Selling: money.Some(b.Value - b.Received),
			ClientID: b.ClientID, ClientName: b.ClientName, TypeID: ProjectTypeKey, TypeName: ProjectTypeName, Source: SourceProject,
		})
	}
	return out
}

func BySource(e Entry) (string, string) {
	if e.Source == SourceProject {
		return SourceProject, "Projects"
	}
	return SourceAsset, "Assets"
}
