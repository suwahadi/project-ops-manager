package invoice

import (
	"math/big"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

type Line struct {
	Quantity   money.Amount
	Rate       money.Amount
	TaxPercent money.Amount
}

type Totals struct {
	Subtotal money.Amount
	Tax      money.Amount
	Total    money.Amount
	Paid     money.Amount
	Due      money.Amount
}

func roundDiv(n *big.Int, d int64) money.Amount {
	den := big.NewInt(d)
	q, r := new(big.Int).QuoRem(n, den, new(big.Int))
	if new(big.Int).Mul(r, big.NewInt(2)).CmpAbs(den) >= 0 {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return money.FromCents(q.Int64())
}

const maxLineCents = 999999999999999

func (l Line) Fits() bool {
	n := new(big.Int).Mul(big.NewInt(l.Quantity.Cents()), big.NewInt(l.Rate.Cents()))
	return n.Cmp(new(big.Int).Mul(big.NewInt(maxLineCents), big.NewInt(100))) <= 0
}

func (l Line) Amount() money.Amount {
	return roundDiv(new(big.Int).Mul(big.NewInt(l.Quantity.Cents()), big.NewInt(l.Rate.Cents())), 100)
}

func (l Line) Tax() money.Amount {
	return roundDiv(new(big.Int).Mul(big.NewInt(l.Amount().Cents()), big.NewInt(l.TaxPercent.Cents())), 10000)
}

func Compute(lines []Line, paid bool) Totals {
	var t Totals
	for _, l := range lines {
		t.Subtotal += l.Amount()
		t.Tax += l.Tax()
	}
	t.Total = t.Subtotal + t.Tax
	if paid {
		t.Paid = t.Total
	}
	t.Due = t.Total - t.Paid
	return t
}

func FormatDecimal(a money.Amount) string {
	c := a.Cents()
	s := a.String()
	switch {
	case c%100 == 0:
		return s[:len(s)-3]
	case c%10 == 0:
		return s[:len(s)-1]
	}
	return s
}
