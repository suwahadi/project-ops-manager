package auth

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func TestServiceLoginLockoutAndPasswordChange(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	clock := &fakeClock{t: time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)}
	svc := NewService(pool, clock.Now)

	created, err := svc.EnsureAdmin(ctx, "admin@example.com", "first-password")
	if err != nil || !created {
		t.Fatalf("ensure admin: %v %v", created, err)
	}
	created, err = svc.EnsureAdmin(ctx, "other@example.com", "ignored-password")
	if err != nil || created {
		t.Fatalf("second ensure admin should be a no-op: %v %v", created, err)
	}

	ip := netip.MustParseAddr("203.0.113.7")
	token, err := svc.Login(ctx, ip, "ADMIN@example.com", "first-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	sess, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	for i := 0; i < MaxFailedLogins; i++ {
		clock.t = clock.t.Add(time.Minute)
		if _, err := svc.Login(ctx, ip, "admin@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", i, err)
		}
	}
	if _, err := svc.Login(ctx, ip, "admin@example.com", "first-password"); !errors.Is(err, ErrLockedOut) {
		t.Fatalf("expected lockout, got %v", err)
	}
	other := netip.MustParseAddr("203.0.113.8")
	second, err := svc.Login(ctx, other, "admin@example.com", "first-password")
	if err != nil {
		t.Fatalf("other ip should not be locked: %v", err)
	}
	clock.t = clock.t.Add(LockoutDuration + time.Second)
	if _, err := svc.Login(ctx, ip, "admin@example.com", "first-password"); err != nil {
		t.Fatalf("expected lockout to expire: %v", err)
	}

	if err := svc.ChangePassword(ctx, sess, "wrong", "second-password"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("expected wrong password, got %v", err)
	}
	if err := svc.ChangePassword(ctx, sess, "first-password", "second-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); err != nil {
		t.Fatalf("current session should survive: %v", err)
	}
	if _, err := svc.Authenticate(ctx, second); !errors.Is(err, ErrNoSession) {
		t.Fatalf("other session should be ended, got %v", err)
	}

	clock.t = clock.t.Add(SessionIdleTimeout + time.Minute)
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("idle session should expire, got %v", err)
	}
}
