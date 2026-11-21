package asset

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestComputeStatus(t *testing.T) {
	today := d("2026-11-21")
	domain := []int32{30, 14, 7, 1}
	vps := []int32{14, 7, 1}
	cases := []struct {
		name      string
		cancelled bool
		exp       string
		offsets   []int32
		want      Status
	}{
		{"cancelled wins over expired", true, "2026-01-01", domain, StatusCancelled},
		{"cancelled wins over active", true, "2030-01-01", domain, StatusCancelled},
		{"expired yesterday", false, "2026-11-20", domain, StatusExpired},
		{"expires today is expiring soon", false, "2026-11-21", domain, StatusExpiringSoon},
		{"exactly largest offset", false, "2026-12-21", domain, StatusExpiringSoon},
		{"one day beyond largest offset", false, "2026-12-22", domain, StatusActive},
		{"vps uses its own largest offset", false, "2026-12-05", vps, StatusExpiringSoon},
		{"vps beyond 14 days", false, "2026-12-06", vps, StatusActive},
		{"far future", false, "2028-01-01", domain, StatusActive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ComputeStatus(c.cancelled, d(c.exp), today, c.offsets); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestTodayUsesTimezone(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 11, 21, 18, 30, 0, 0, time.UTC)
	if got := Today(now, jakarta); !got.Equal(d("2026-11-22")) {
		t.Fatalf("today in Jakarta = %s", got)
	}
	if got := Today(now, time.UTC); !got.Equal(d("2026-11-21")) {
		t.Fatalf("today in UTC = %s", got)
	}
}

func TestCountdown(t *testing.T) {
	today := d("2026-11-21")
	cases := map[string]string{
		"2026-12-21": "30 days left",
		"2026-11-22": "1 day left",
		"2026-11-21": "Expires today",
		"2026-11-20": "Expired 1 day ago",
		"2026-11-18": "Expired 3 days ago",
	}
	for exp, want := range cases {
		if got := Countdown(d(exp), today); got != want {
			t.Errorf("Countdown(%s) = %q, want %q", exp, got, want)
		}
	}
}

func TestAddMonths(t *testing.T) {
	cases := []struct {
		from   string
		months int
		want   string
	}{
		{"2026-01-31", 1, "2026-02-28"},
		{"2028-01-31", 1, "2028-02-29"},
		{"2026-03-15", 12, "2027-03-15"},
		{"2026-11-30", 3, "2027-02-28"},
		{"2026-05-31", 6, "2026-11-30"},
	}
	for _, c := range cases {
		if got := AddMonths(d(c.from), c.months); !got.Equal(d(c.want)) {
			t.Errorf("AddMonths(%s, %d) = %s, want %s", c.from, c.months, got.Format("2006-01-02"), c.want)
		}
	}
}
