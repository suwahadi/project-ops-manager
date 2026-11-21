package reminder_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/crypto"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/notify"
	"github.com/suwahadi/project-ops-manager/internal/notify/email"
	"github.com/suwahadi/project-ops-manager/internal/notify/telegram"
	"github.com/suwahadi/project-ops-manager/internal/reminder"
	"github.com/suwahadi/project-ops-manager/internal/settings"
)

type fakeSender struct {
	mu       sync.Mutex
	subjects []string
	fail     bool
}

func (f *fakeSender) record(msg notify.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("smtp: connection refused")
	}
	f.subjects = append(f.subjects, msg.Subject)
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subjects)
}

type fakeTelegram struct{ fakeSender }

func (f *fakeTelegram) Send(_ context.Context, _ telegram.Config, m notify.Message) error {
	return f.record(m)
}

type fakeEmail struct{ fakeSender }

func (f *fakeEmail) Send(_ context.Context, _ email.Config, m notify.Message) error {
	return f.record(m)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestDispatcher(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	box, _ := crypto.NewBox(bytes.Repeat([]byte{1}, 32), 1)
	store := settings.NewStore(pool, box)
	if err := store.SaveTelegram(ctx, settings.Telegram{Enabled: true, ChatID: "1"}, "token"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmail(ctx, settings.Email{Enabled: true, Host: "smtp", Port: 587, From: "a@b.c", To: "d@e.f"}, ""); err != nil {
		t.Fatal(err)
	}
	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, jakarta)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	c := &clock{t: at("2026-12-01 08:00")}
	tg, em := &fakeTelegram{}, &fakeEmail{}
	d := reminder.NewDispatcher(pool, store, tg, em, "https://ops.example.com", slog.New(slog.NewTextHandler(io.Discard, nil)), c.now)

	domain, err := q.GetAssetTypeBySlug(ctx, "domain")
	if err != nil {
		t.Fatal(err)
	}
	assets := asset.NewService(pool, t.TempDir())
	mk := func(name, exp string) string {
		e, _ := time.Parse("2006-01-02", exp)
		id, err := assets.Create(ctx, asset.Input{TypeID: domain.ID, ProviderName: "Reg", Name: name, Expiration: e, RenewalMonths: 12, Cost: money.FromCents(100), Currency: money.IDR, Custom: assettype.Values{}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	mk("due7.com", "2026-12-08")
	cancelledID := mk("cancelled.com", "2026-12-08")
	mk("expired.com", "2026-11-30")
	if err := assets.Cancel(ctx, cancelledID, c.t); err != nil {
		t.Fatal(err)
	}

	run := func() reminder.RunResult {
		t.Helper()
		res, err := d.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	run()
	if tg.count() != 0 || em.count() != 0 {
		t.Fatal("nothing may be sent before the send time")
	}
	c.t = at("2026-12-01 09:05")
	run()
	if tg.count() != 1 || em.count() != 1 {
		t.Fatalf("expected one message per channel, got telegram=%d email=%d", tg.count(), em.count())
	}
	if em.subjects[0] != "Renewal in 7 days: due7.com (Domain)" {
		t.Fatalf("subject = %q", em.subjects[0])
	}
	c.t = at("2026-12-01 09:10")
	run()
	d2 := reminder.NewDispatcher(pool, store, tg, em, "https://ops.example.com", slog.New(slog.NewTextHandler(io.Discard, nil)), c.now)
	if _, err := d2.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if tg.count() != 1 || em.count() != 1 {
		t.Fatal("reminders must not be resent after a restart")
	}

	c.t = at("2026-12-07 15:00")
	run()
	if tg.count() != 2 || em.subjects[1] != "Renewal in 1 day: due7.com (Domain)" {
		t.Fatalf("H-1 not sent: %v", em.subjects)
	}
	c.t = at("2026-12-08 09:05")
	run()
	if tg.count() != 2 {
		t.Fatal("nothing may be sent on the expiration date")
	}

	catchup := mk("catchup.com", "2026-12-25")
	c.t = at("2026-12-09 09:05")
	run()
	c.t = at("2026-12-20 10:00")
	run()
	logs, err := q.ListReminderLogsByAsset(ctx, catchup)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].OffsetDays != 7 {
		t.Fatalf("catch-up should send only the 7-day offset once per channel, got %+v", logs)
	}

	failing := mk("failing.com", "2027-01-04")
	em.fail = true
	c.t = at("2026-12-21 09:00")
	run()
	for _, step := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute} {
		c.t = c.t.Add(step)
		if _, err := d.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
	}
	logs, err = q.ListReminderLogsByAsset(ctx, failing)
	if err != nil {
		t.Fatal(err)
	}
	var emailLog sqlc.ReminderLog
	for _, l := range logs {
		if l.Channel == reminder.ChannelEmail {
			emailLog = l
		}
	}
	if emailLog.Status != "failed" || emailLog.Attempts != 4 || emailLog.Error == nil {
		t.Fatalf("expected failed after 3 retries, got %+v", emailLog)
	}
	latest, err := q.LatestFinishedReminderLog(ctx)
	if err != nil || latest.Status != "failed" {
		t.Fatalf("latest finished log should be the failure: %+v %v", latest, err)
	}

	cancelledLogs, _ := q.ListReminderLogsByAsset(ctx, cancelledID)
	if len(cancelledLogs) != 0 {
		t.Fatal("cancelled assets must not receive reminders")
	}
}
