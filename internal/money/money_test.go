package money

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParse(t *testing.T) {
	cases := map[string]int64{
		"180000":   18000000,
		"12":       1200,
		"12.5":     1250,
		"12.05":    1205,
		"1,250.75": 125075,
		"0":        0,
		" 99.99 ":  9999,
		"-3.10":    -310,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got.Cents() != want {
			t.Errorf("Parse(%q) = %d, %v; want %d", in, got.Cents(), err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1.234", "1.2.3", "12a", "99999999999999"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) expected error", bad)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		a    Amount
		cur  Currency
		want string
	}{
		{18000000, IDR, "Rp 180.000"},
		{100, IDR, "Rp 1"},
		{123456789, IDR, "Rp 1.234.567,89"},
		{1200, USD, "$12.00"},
		{123456, USD, "$1,234.56"},
		{-550, USD, "-$5.50"},
	}
	for _, c := range cases {
		if got := c.a.Format(c.cur); got != c.want {
			t.Errorf("Format(%d, %s) = %q; want %q", c.a, c.cur, got, c.want)
		}
	}
}

func TestNumericRoundTrip(t *testing.T) {
	for _, a := range []Amount{0, 1, 1250, 18000000, 999999999999999} {
		n, err := a.NumericValue()
		if err != nil {
			t.Fatal(err)
		}
		var back Amount
		if err := back.ScanNumeric(n); err != nil || back != a {
			t.Fatalf("round trip %d -> %d (%v)", a, back, err)
		}
	}
	var na NullAmount
	if err := na.ScanNumeric(pgtype.Numeric{}); err != nil || na.Valid {
		t.Fatalf("null scan: %+v %v", na, err)
	}
	var fromExp Amount
	if err := fromExp.ScanNumeric(pgtype.Numeric{Int: bigInt(18), Exp: 4, Valid: true}); err != nil || fromExp != 18000000 {
		t.Fatalf("exp scan = %d %v", fromExp, err)
	}
}

func bigInt(v int64) *big.Int { return big.NewInt(v) }
