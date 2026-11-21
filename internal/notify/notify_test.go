package notify

import (
	"strings"
	"testing"
	"time"
)

func TestBuildReminder(t *testing.T) {
	m := BuildReminder(Reminder{
		AssetName: "example.com", TypeName: "Domain", Owner: "Acme", Provider: "Namecheap",
		Expiration: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC), DaysLeft: 7,
		CostPrice: "Rp 180.000", Link: "https://ops.example.com/assets/1",
	})
	if m.Subject != "Renewal in 7 days: example.com (Domain)" {
		t.Fatalf("subject = %q", m.Subject)
	}
	for _, want := range []string{"Owner: Acme", "Provider: Namecheap", "Expiration date: 28 Nov 2026", "Days left: 7", "Cost price: Rp 180.000", "Selling price: —", "https://ops.example.com/assets/1"} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if got := BuildReminder(Reminder{AssetName: "a", TypeName: "VPS", DaysLeft: 1}).Subject; got != "Renewal in 1 day: a (VPS)" {
		t.Fatalf("singular subject = %q", got)
	}
}
