package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/notify"
	"github.com/suwahadi/project-ops-manager/internal/notify/email"
	"github.com/suwahadi/project-ops-manager/internal/notify/telegram"
	"github.com/suwahadi/project-ops-manager/internal/settings"
)

const lastRunKey = "reminder_last_run"

type TelegramSender interface {
	Send(ctx context.Context, cfg telegram.Config, msg notify.Message) error
}

type EmailSender interface {
	Send(ctx context.Context, cfg email.Config, msg notify.Message) error
}

type Dispatcher struct {
	pool     *pgxpool.Pool
	q        *sqlc.Queries
	settings *settings.Store
	telegram TelegramSender
	email    EmailSender
	baseURL  string
	logger   *slog.Logger
	now      func() time.Time
}

func NewDispatcher(pool *pgxpool.Pool, st *settings.Store, tg TelegramSender, em EmailSender, baseURL string, logger *slog.Logger, now func() time.Time) *Dispatcher {
	return &Dispatcher{pool: pool, q: sqlc.New(pool), settings: st, telegram: tg, email: em, baseURL: baseURL, logger: logger, now: now}
}

type channels struct {
	telegram    bool
	telegramCfg telegram.Config
	email       bool
	emailCfg    email.Config
}

func (c channels) enabled() []string {
	var out []string
	if c.telegram {
		out = append(out, ChannelTelegram)
	}
	if c.email {
		out = append(out, ChannelEmail)
	}
	return out
}

func (d *Dispatcher) loadChannels(ctx context.Context, all settings.All) (channels, error) {
	ch := channels{telegram: all.Telegram.Enabled, email: all.Email.Enabled}
	if ch.telegram {
		token, err := d.settings.Secret(ctx, settings.SecretTelegramBotToken)
		if err != nil {
			return ch, fmt.Errorf("load telegram token: %w", err)
		}
		ch.telegramCfg = telegram.Config{Token: token, ChatID: all.Telegram.ChatID}
	}
	if ch.email {
		pw, err := d.settings.Secret(ctx, settings.SecretSMTPPassword)
		if err != nil {
			return ch, fmt.Errorf("load smtp password: %w", err)
		}
		e := all.Email
		ch.emailCfg = email.Config{Host: e.Host, Port: e.Port, Username: e.Username, Password: pw, From: e.From, To: e.To}
	}
	return ch, nil
}

type RunResult struct {
	Scheduled int
	Sent      int
	Retried   int
	Failed    int
}

func (d *Dispatcher) Run(ctx context.Context) (RunResult, error) {
	var res RunResult
	all, err := d.settings.Load(ctx)
	if err != nil {
		return res, err
	}
	ch, err := d.loadChannels(ctx, all)
	if err != nil {
		return res, err
	}
	now := d.now()
	lastRun, err := d.settings.Time(ctx, lastRunKey)
	if err != nil {
		return res, err
	}
	h, m := all.General.SendClock()
	clock := Clock{Now: now, LastRun: lastRun, Location: all.General.Location(), Hour: h, Minute: m}
	if enabled := ch.enabled(); len(enabled) > 0 {
		n, err := d.schedule(ctx, clock, enabled)
		res.Scheduled = n
		if err != nil {
			return res, err
		}
	}
	if err := d.settings.SetTime(ctx, lastRunKey, now); err != nil {
		return res, err
	}
	sent, retried, failed, err := d.deliver(ctx, all, ch)
	res.Sent, res.Retried, res.Failed = sent, retried, failed
	return res, err
}

func (d *Dispatcher) Deliver(ctx context.Context) (RunResult, error) {
	var res RunResult
	all, err := d.settings.Load(ctx)
	if err != nil {
		return res, err
	}
	ch, err := d.loadChannels(ctx, all)
	if err != nil {
		return res, err
	}
	res.Sent, res.Retried, res.Failed, err = d.deliver(ctx, all, ch)
	return res, err
}

func (d *Dispatcher) schedule(ctx context.Context, clock Clock, enabled []string) (int, error) {
	rows, err := d.q.ListReminderCandidates(ctx, clock.Today())
	if err != nil {
		return 0, fmt.Errorf("list reminder candidates: %w", err)
	}
	inserted := 0
	for _, r := range rows {
		offset, ok := SelectOffset(r.ExpirationDate, r.ReminderOffsets, clock)
		if !ok {
			continue
		}
		for _, channel := range enabled {
			_, err := d.q.InsertReminderLog(ctx, sqlc.InsertReminderLogParams{
				AssetID: r.ID, ExpirationDate: r.ExpirationDate, OffsetDays: offset, Channel: channel, Now: clock.Now,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return inserted, fmt.Errorf("insert reminder log: %w", err)
			}
			inserted++
		}
	}
	return inserted, nil
}

func (d *Dispatcher) deliver(ctx context.Context, all settings.All, ch channels) (sent, retried, failed int, err error) {
	now := d.now()
	rows, err := d.q.ListDueReminderLogs(ctx, now)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list due reminders: %w", err)
	}
	today := asset.Today(now, all.General.Location())
	for _, r := range rows {
		if ctx.Err() != nil {
			return sent, retried, failed, ctx.Err()
		}
		cur, err := d.q.GetAssetForReminder(ctx, r.AssetID)
		stale := err != nil || cur.CancelledAt != nil || !cur.ExpirationDate.Equal(r.ExpirationDate) || !r.ExpirationDate.After(today)
		enabled := (r.Channel == ChannelTelegram && ch.telegram) || (r.Channel == ChannelEmail && ch.email)
		if stale || !enabled {
			if err := d.q.DeleteReminderLog(ctx, r.ID); err != nil {
				return sent, retried, failed, fmt.Errorf("drop stale reminder: %w", err)
			}
			continue
		}
		msg := d.message(r, today)
		sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		var sendErr error
		if r.Channel == ChannelTelegram {
			sendErr = d.telegram.Send(sendCtx, ch.telegramCfg, msg)
		} else {
			sendErr = d.email.Send(sendCtx, ch.emailCfg, msg)
		}
		cancel()
		now = d.now()
		if sendErr == nil {
			if err := d.q.MarkReminderSent(ctx, sqlc.MarkReminderSentParams{ID: r.ID, Now: now}); err != nil {
				return sent, retried, failed, fmt.Errorf("mark sent: %w", err)
			}
			sent++
			d.logger.Info("reminder sent", "asset_id", r.AssetID, "offset_days", r.OffsetDays, "channel", r.Channel)
			continue
		}
		errText := truncate(sendErr.Error(), 500)
		if next, ok := NextAttempt(int(r.Attempts)+1, now); ok {
			if err := d.q.MarkReminderRetry(ctx, sqlc.MarkReminderRetryParams{ID: r.ID, Error: &errText, NextAttemptAt: &next, Now: now}); err != nil {
				return sent, retried, failed, fmt.Errorf("mark retry: %w", err)
			}
			retried++
			d.logger.Warn("reminder send failed, will retry", "asset_id", r.AssetID, "channel", r.Channel, "attempt", r.Attempts+1, "error", errText)
			continue
		}
		if err := d.q.MarkReminderFailed(ctx, sqlc.MarkReminderFailedParams{ID: r.ID, Error: &errText, Now: now}); err != nil {
			return sent, retried, failed, fmt.Errorf("mark failed: %w", err)
		}
		failed++
		d.logger.Error("reminder send failed permanently", "asset_id", r.AssetID, "channel", r.Channel, "error", errText)
	}
	return sent, retried, failed, nil
}

func (d *Dispatcher) message(r sqlc.ListDueReminderLogsRow, today time.Time) notify.Message {
	cur := money.Currency(r.Currency)
	owner := "Internal"
	if r.ClientName != nil {
		owner = *r.ClientName
	}
	selling := ""
	if r.SellingPrice.Valid {
		selling = r.SellingPrice.Amount.Format(cur)
	}
	return notify.BuildReminder(notify.Reminder{
		AssetName: r.AssetName, TypeName: r.TypeName, Owner: owner, Provider: r.ProviderName,
		Expiration: r.ExpirationDate, DaysLeft: asset.DaysLeft(r.ExpirationDate, today),
		CostPrice: r.CostPrice.Format(cur), SellingPrice: selling,
		Link: strings.TrimRight(d.baseURL, "/") + "/assets/" + r.AssetID,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
