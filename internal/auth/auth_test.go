package auth

import (
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("correct horse battery", h)
	if err != nil || !ok {
		t.Fatalf("verify correct: %v %v", ok, err)
	}
	ok, err = VerifyPassword("wrong", h)
	if err != nil || ok {
		t.Fatalf("verify wrong: %v %v", ok, err)
	}
	if _, err := VerifyPassword("x", "$bcrypt$nope"); err == nil {
		t.Fatal("expected invalid hash error")
	}
}

func fails(base time.Time, offsets ...time.Duration) []Attempt {
	var out []Attempt
	for _, o := range offsets {
		out = append(out, Attempt{At: base.Add(o)})
	}
	return out
}

func TestLockedUntil(t *testing.T) {
	base := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	m := time.Minute
	cases := []struct {
		name     string
		attempts []Attempt
		now      time.Time
		locked   bool
	}{
		{"four failures", fails(base, 0, m, 2*m, 3*m), base.Add(4 * m), false},
		{"five failures in window", fails(base, 0, m, 2*m, 3*m, 4*m), base.Add(5 * m), true},
		{"lock lasts 15 minutes after fifth", fails(base, 0, m, 2*m, 3*m, 4*m), base.Add(18 * m), true},
		{"lock expires", fails(base, 0, m, 2*m, 3*m, 4*m), base.Add(19*m + time.Second), false},
		{"spread over more than window", fails(base, 0, 4*m, 8*m, 12*m, 16*m), base.Add(17 * m), false},
		{"success resets", append(append(fails(base, 0, m, 2*m), Attempt{Success: true, At: base.Add(3 * m)}), fails(base, 4*m, 5*m)...), base.Add(6 * m), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, locked := LockedUntil(c.attempts, c.now)
			if locked != c.locked {
				t.Fatalf("locked = %v, want %v", locked, c.locked)
			}
		})
	}
}
