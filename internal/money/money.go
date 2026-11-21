package money

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type Currency string

const (
	IDR Currency = "IDR"
	USD Currency = "USD"
)

var Currencies = []Currency{IDR, USD}

func ParseCurrency(s string) (Currency, error) {
	switch Currency(strings.ToUpper(strings.TrimSpace(s))) {
	case IDR:
		return IDR, nil
	case USD:
		return USD, nil
	}
	return "", fmt.Errorf("unsupported currency %q", s)
}

type Amount int64

const maxCents = 9999999999999

func FromCents(c int64) Amount { return Amount(c) }

func (a Amount) Cents() int64 { return int64(a) }

func Parse(s string) (Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("amount is empty")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	whole = strings.ReplaceAll(whole, ",", "")
	if whole == "" {
		whole = "0"
	}
	if !hasFrac {
		frac = "00"
	}
	if len(frac) == 1 {
		frac += "0"
	}
	if len(frac) != 2 || !allDigits(whole) || !allDigits(frac) {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	if w > maxCents/100 {
		return 0, fmt.Errorf("amount %q is too large", s)
	}
	c := w*100 + f
	if neg {
		c = -c
	}
	return Amount(c), nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (a Amount) String() string {
	c := int64(a)
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

func (a Amount) Format(cur Currency) string {
	c := int64(a)
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	switch cur {
	case IDR:
		s := "Rp " + groupThousands(c/100, ".")
		if c%100 != 0 {
			s += fmt.Sprintf(",%02d", c%100)
		}
		return sign + s
	default:
		return fmt.Sprintf("%s$%s.%02d", sign, groupThousands(c/100, ","), c%100)
	}
}

func groupThousands(n int64, sep string) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func (a *Amount) ScanNumeric(n pgtype.Numeric) error {
	if !n.Valid {
		return errors.New("cannot scan NULL into money.Amount")
	}
	c, err := numericToCents(n)
	if err != nil {
		return err
	}
	*a = Amount(c)
	return nil
}

func (a Amount) NumericValue() (pgtype.Numeric, error) {
	return pgtype.Numeric{Int: big.NewInt(int64(a)), Exp: -2, Valid: true}, nil
}

func (a Amount) Value() (driver.Value, error) { return a.String(), nil }

type NullAmount struct {
	Amount Amount
	Valid  bool
}

func Some(a Amount) NullAmount { return NullAmount{Amount: a, Valid: true} }

func (n *NullAmount) ScanNumeric(v pgtype.Numeric) error {
	if !v.Valid {
		*n = NullAmount{}
		return nil
	}
	c, err := numericToCents(v)
	if err != nil {
		return err
	}
	*n = NullAmount{Amount: Amount(c), Valid: true}
	return nil
}

func (n NullAmount) NumericValue() (pgtype.Numeric, error) {
	if !n.Valid {
		return pgtype.Numeric{}, nil
	}
	return n.Amount.NumericValue()
}

func (n NullAmount) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Amount.String(), nil
}

func numericToCents(n pgtype.Numeric) (int64, error) {
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return 0, errors.New("money value is not finite")
	}
	i := new(big.Int).Set(n.Int)
	exp := int(n.Exp) + 2
	ten := big.NewInt(10)
	for ; exp > 0; exp-- {
		i.Mul(i, ten)
	}
	for ; exp < 0; exp++ {
		i.Quo(i, ten)
	}
	if !i.IsInt64() {
		return 0, errors.New("money value out of range")
	}
	return i.Int64(), nil
}

type Totals map[Currency]Amount

func (t Totals) Add(cur Currency, a Amount) { t[cur] += a }
