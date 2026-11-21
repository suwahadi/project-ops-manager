package reminder

import (
	"testing"
	"time"
)

var jakarta, _ = time.LoadLocation("Asia/Jakarta")

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, jakarta)
	if err != nil {
		panic(err)
	}
	return t
}

func clock(now string, lastRun string) Clock {
	c := Clock{Now: at(now), Location: jakarta, Hour: 9, Minute: 0}
	if lastRun != "" {
		lr := at(lastRun)
		c.LastRun = &lr
	}
	return c
}

func TestSelectOffset(t *testing.T) {
	offsets := []int32{30, 14, 7, 1}
	exp := day("2026-12-08")
	cases := []struct {
		name   string
		exp    time.Time
		clock  Clock
		want   int32
		wantOK bool
	}{
		{"before send time on due day", exp, clock("2026-12-01 08:59", "2026-12-01 08:55"), 0, false},
		{"at send time on due day", exp, clock("2026-12-01 09:00", "2026-12-01 08:55"), 7, true},
		{"later the same day still due", exp, clock("2026-12-01 17:00", "2026-12-01 16:55"), 7, true},
		{"not a due day", exp, clock("2026-12-02 09:00", "2026-12-02 08:55"), 0, false},
		{"H-1", exp, clock("2026-12-07 09:05", "2026-12-07 09:00"), 1, true},
		{"nothing on expiration day", exp, clock("2026-12-08 09:05", "2026-12-08 09:00"), 0, false},
		{"nothing after expiration", exp, clock("2026-12-10 09:05", "2026-12-10 09:00"), 0, false},
		{"catch-up only latest missed offset", exp, clock("2026-12-03 10:00", "2026-11-01 10:00"), 7, true},
		{"catch-up after downtime spanning send time", exp, clock("2026-11-24 10:00", "2026-11-24 08:30"), 14, true},
		{"catch-up skipped when expiry passed", exp, clock("2026-12-09 10:00", "2026-11-01 10:00"), 0, false},
		{"missed moment before last run is not resent", exp, clock("2026-11-26 10:00", "2026-11-25 10:00"), 0, false},
		{"fresh install sends only today's due", exp, clock("2026-12-03 10:00", ""), 0, false},
		{"fresh install on due day", exp, clock("2026-12-01 10:00", ""), 7, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := SelectOffset(c.exp, offsets, c.clock)
			if ok != c.wantOK || got != c.want {
				t.Fatalf("got %d,%v want %d,%v", got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestNewExpirationProducesNewSchedule(t *testing.T) {
	offsets := []int32{30, 14, 7, 1}
	c := clock("2026-12-01 09:00", "2026-12-01 08:55")
	if o, ok := SelectOffset(day("2026-12-08"), offsets, c); !ok || o != 7 {
		t.Fatalf("old schedule: %d %v", o, ok)
	}
	if _, ok := SelectOffset(day("2027-12-08"), offsets, c); ok {
		t.Fatal("renewed expiration should not be due")
	}
	k1 := Key{AssetID: "a", Expiration: day("2026-12-08"), Offset: 7, Channel: ChannelEmail}
	k2 := Key{AssetID: "a", Expiration: day("2027-12-08"), Offset: 7, Channel: ChannelEmail}
	if k1 == k2 {
		t.Fatal("idempotency key must include the expiration date")
	}
	k3 := k1
	k3.Channel = ChannelTelegram
	if k1 == k3 {
		t.Fatal("idempotency key must include the channel")
	}
}

func TestPlanAndBackoff(t *testing.T) {
	p := Plan(day("2026-12-08"), []int32{1, 30, 7, 14})
	if len(p) != 4 || p[0].Offset != 30 || !p[0].Due.Equal(day("2026-11-08")) || !p[3].Due.Equal(day("2026-12-07")) {
		t.Fatalf("plan = %+v", p)
	}
	now := at("2026-12-01 09:00")
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		next, ok := NextAttempt(i+1, now)
		if !ok || next.Sub(now) != w {
			t.Fatalf("attempt %d: %v %v", i+1, next.Sub(now), ok)
		}
	}
	if _, ok := NextAttempt(4, now); ok {
		t.Fatal("no retry after the third retry")
	}
}
