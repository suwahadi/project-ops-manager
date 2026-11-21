package invoice

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

func amt(s string) money.Amount {
	a, err := money.Parse(s)
	if err != nil {
		panic(err)
	}
	return a
}

func TestCompute(t *testing.T) {
	lines := []Line{
		{Quantity: amt("1"), Rate: amt("785000"), TaxPercent: 0},
		{Quantity: amt("2.5"), Rate: amt("100.10"), TaxPercent: amt("11")},
		{Quantity: amt("3"), Rate: amt("0.33"), TaxPercent: amt("12.5")},
	}
	if got := lines[1].Amount(); got != amt("250.25") {
		t.Fatalf("line amount = %s", got)
	}
	if got := lines[1].Tax(); got != amt("27.53") {
		t.Fatalf("line tax = %s", got)
	}
	if got := lines[2].Tax(); got != amt("0.12") {
		t.Fatalf("rounded tax = %s", got)
	}
	unpaid := Compute(lines, false)
	want := Totals{Subtotal: amt("785251.24"), Tax: amt("27.65"), Total: amt("785278.89"), Paid: 0, Due: amt("785278.89")}
	if unpaid != want {
		t.Fatalf("unpaid totals = %+v, want %+v", unpaid, want)
	}
	paid := Compute(lines, true)
	if paid.Paid != want.Total || paid.Due != 0 {
		t.Fatalf("paid totals = %+v", paid)
	}
	if (Line{Quantity: amt("1000000"), Rate: money.FromCents(9999999999999)}).Fits() {
		t.Fatal("oversized line fits")
	}
}

func TestFormatDecimal(t *testing.T) {
	for in, want := range map[string]string{"1": "1", "2.5": "2.5", "0.33": "0.33", "11": "11", "12.50": "12.5"} {
		if got := FormatDecimal(amt(in)); got != want {
			t.Errorf("FormatDecimal(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestGenerateNumber(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		n, err := GenerateNumber("inv-dev-")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(n, "INV-DEV-") || len(n) != len("INV-DEV-")+8 || !ValidNumber(n) {
			t.Fatalf("bad number %q", n)
		}
		seen[n] = true
	}
	if len(seen) < 199 {
		t.Fatalf("numbers repeat: %d unique of 200", len(seen))
	}
	for _, bad := range []string{"", "INV DEV", "INV/01", "-INV", strings.Repeat("A", 41)} {
		if ValidNumber(bad) {
			t.Errorf("ValidNumber(%q) = true", bad)
		}
	}
	if !ValidPrefix("") || !ValidPrefix("INV-") || ValidPrefix("INV/") {
		t.Error("ValidPrefix")
	}
}

func TestValidate(t *testing.T) {
	clients := map[string]string{"c1": "RL-Indo"}
	lookup := func(id string) (string, bool) { n, ok := clients[id]; return n, ok }
	f := Form{
		Number: " inv-dev-hk87cz02 ", ClientID: "c1", Currency: "IDR", InvoiceDate: "2026-11-21", DueDate: "2026-11-28",
		Status: StatusPaid, PaidDate: "2026-11-22", Note: "BCA 123\n",
		Items: []ItemForm{
			{Title: "Renew rl-indo.com", Description: "Line one\nLine two", Quantity: "", Rate: "785000", Tax: "11%"},
			{},
		},
	}
	in, errs := f.Validate(lookup)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if in.Number != "INV-DEV-HK87CZ02" || in.BillTo != "RL-Indo" || !in.Paid || in.PaidDate == nil || len(in.Items) != 1 {
		t.Fatalf("input: %+v", in)
	}
	if in.Items[0].Quantity != amt("1") || in.Items[0].TaxPercent != amt("11") || in.Items[0].Description != "Line one\nLine two" || in.Note != "BCA 123" {
		t.Fatalf("item: %+v", in.Items[0])
	}

	bad := Form{Number: "INV/1", ClientID: "zz", Currency: "EUR", InvoiceDate: "2026-11-21", DueDate: "2026-11-01", Status: StatusPaid,
		Items: []ItemForm{{Title: "", Quantity: "0", Rate: "-1", Tax: "101"}}}
	_, errs = bad.Validate(lookup)
	for _, k := range []string{"number", "client_id", "bill_to", "currency", "due_date", "paid_date", "item_title.0", "item_quantity.0", "item_rate.0", "item_tax.0"} {
		if errs[k] == "" {
			t.Errorf("missing error %s (got %v)", k, errs)
		}
	}
	_, errs = Form{Number: "A", BillTo: "X", Currency: "USD", InvoiceDate: "2026-01-01"}.Validate(lookup)
	if errs["items"] == "" {
		t.Error("empty invoice accepted")
	}
}

func TestFilter(t *testing.T) {
	today := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	past := today.AddDate(0, 0, -3)
	c1 := "0f8fad5b-d9cb-469f-a165-70867728950e"
	rows := []Summary{
		{Totals: Totals{Total: amt("100"), Due: amt("100")}},
		{Totals: Totals{Total: amt("300"), Paid: amt("300")}},
		{Totals: Totals{Total: amt("200"), Due: amt("200")}},
	}
	rows[0].Number, rows[0].BillTo, rows[0].Status, rows[0].Currency, rows[0].InvoiceDate, rows[0].DueDate = "INV-A", "RL-Indo", StatusUnpaid, "IDR", past, &past
	rows[0].ClientID = &c1
	rows[1].Number, rows[1].BillTo, rows[1].Status, rows[1].Currency, rows[1].InvoiceDate = "INV-B", "Walk-in", StatusPaid, "IDR", today
	rows[2].Number, rows[2].BillTo, rows[2].Status, rows[2].Currency, rows[2].InvoiceDate = "INV-C", "Acme", StatusUnpaid, "USD", today.AddDate(0, 0, -1)
	isID := func(s string) bool { return s == c1 }
	cases := map[string][]string{
		"":                 {"INV-B", "INV-C", "INV-A"},
		"status=overdue":   {"INV-A"},
		"status=paid":      {"INV-B"},
		"status=unpaid":    {"INV-C", "INV-A"},
		"client=" + c1:     {"INV-A"},
		"client=none":      {"INV-B", "INV-C"},
		"q=rl-":            {"INV-A"},
		"sort=total_desc":  {"INV-B", "INV-C", "INV-A"},
		"sort=due_asc":     {"INV-A", "INV-B", "INV-C"},
		"sort=date_asc":    {"INV-A", "INV-C", "INV-B"},
		"status=bogus&x=1": {"INV-B", "INV-C", "INV-A"},
	}
	for q, want := range cases {
		v, _ := url.ParseQuery(q)
		var got []string
		for _, r := range Apply(rows, ParseFilter(v, isID), today) {
			got = append(got, r.Number)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: got %v, want %v", q, got, want)
		}
	}
	sum := Summarize(rows, today)
	if len(sum) != 2 || sum[0].Currency != money.IDR || sum[0].Unpaid != amt("100") || sum[0].Paid != amt("300") || sum[0].Overdue != 1 || sum[1].Unpaid != amt("200") {
		t.Fatalf("summary: %+v", sum)
	}
}
