package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/crypto"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

const (
	SecretTelegramBotToken = "telegram_bot_token"
	SecretSMTPPassword     = "smtp_password"
	secretKeyCheck         = "key_check"
	keyCheckPlaintext      = "ops-manager-key-check"
)

var ErrKeyMismatch = errors.New("OPS_ENCRYPTION_KEY does not match the key used for stored secrets")

type General struct {
	Timezone        string
	SendTime        string
	DefaultCurrency money.Currency
}

type Telegram struct {
	Enabled  bool
	ChatID   string
	TopicID  int64
	HasToken bool
}

type Email struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	From        string
	FromName    string
	To          string
	HasPassword bool
}

type Invoice struct {
	CompanyName  string
	CompanyURL   string
	NumberPrefix string
	DefaultNote  string
}

type All struct {
	General  General
	Telegram Telegram
	Email    Email
	Invoice  Invoice
}

func (g General) Location() *time.Location {
	loc, err := time.LoadLocation(g.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (g General) SendClock() (int, int) {
	h, m, _ := ParseClock(g.SendTime)
	return h, m
}

var clockPattern = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

func ParseClock(s string) (int, int, error) {
	m := clockPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 9, 0, fmt.Errorf("invalid time %q", s)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	return h, mi, nil
}

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	box  *crypto.Box
}

func NewStore(pool *pgxpool.Pool, box *crypto.Box) *Store {
	return &Store{pool: pool, q: sqlc.New(pool), box: box}
}

func (s *Store) Load(ctx context.Context) (All, error) {
	rows, err := s.q.ListSettings(ctx)
	if err != nil {
		return All{}, fmt.Errorf("list settings: %w", err)
	}
	raw := map[string][]byte{}
	for _, r := range rows {
		raw[r.Key] = r.Value
	}
	str := func(k, def string) string {
		var v string
		if json.Unmarshal(raw[k], &v) != nil {
			return def
		}
		return v
	}
	boolean := func(k string) bool {
		var v bool
		_ = json.Unmarshal(raw[k], &v)
		return v
	}
	integer := func(k string, def int) int {
		var v int
		if json.Unmarshal(raw[k], &v) != nil {
			return def
		}
		return v
	}
	cur, err := money.ParseCurrency(str("default_currency", "IDR"))
	if err != nil {
		cur = money.IDR
	}
	all := All{
		General: General{
			Timezone:        str("timezone", "Asia/Jakarta"),
			SendTime:        str("reminder_send_time", "09:00"),
			DefaultCurrency: cur,
		},
		Telegram: Telegram{
			Enabled: boolean("telegram_enabled"),
			ChatID:  str("telegram_chat_id", ""),
			TopicID: int64(integer("telegram_topic_id", 0)),
		},
		Email: Email{
			Enabled:  boolean("email_enabled"),
			Host:     str("smtp_host", ""),
			Port:     integer("smtp_port", 587),
			Username: str("smtp_username", ""),
			From:     str("smtp_from", ""),
			FromName: str("smtp_from_name", ""),
			To:       str("smtp_to", ""),
		},
		Invoice: Invoice{
			CompanyName:  str("invoice_company_name", ""),
			CompanyURL:   str("invoice_company_url", ""),
			NumberPrefix: str("invoice_number_prefix", "INV-DEV-"),
			DefaultNote:  str("invoice_default_note", ""),
		},
	}
	all.Telegram.HasToken, err = s.hasSecret(ctx, SecretTelegramBotToken)
	if err != nil {
		return All{}, err
	}
	all.Email.HasPassword, err = s.hasSecret(ctx, SecretSMTPPassword)
	if err != nil {
		return All{}, err
	}
	return all, nil
}

func (s *Store) hasSecret(ctx context.Context, key string) (bool, error) {
	_, err := s.q.GetSecret(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get secret: %w", err)
	}
	return true, nil
}

func (s *Store) set(ctx context.Context, q *sqlc.Queries, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := q.UpsertSetting(ctx, sqlc.UpsertSettingParams{Key: key, Value: b}); err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}

func (s *Store) SaveGeneral(ctx context.Context, g General) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		for k, v := range map[string]any{"timezone": g.Timezone, "reminder_send_time": g.SendTime, "default_currency": string(g.DefaultCurrency)} {
			if err := s.set(ctx, q, k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) SaveTelegram(ctx context.Context, t Telegram, newToken string) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		if err := s.set(ctx, q, "telegram_enabled", t.Enabled); err != nil {
			return err
		}
		if err := s.set(ctx, q, "telegram_chat_id", t.ChatID); err != nil {
			return err
		}
		if err := s.set(ctx, q, "telegram_topic_id", t.TopicID); err != nil {
			return err
		}
		if newToken != "" {
			return s.putSecret(ctx, q, SecretTelegramBotToken, newToken)
		}
		return nil
	})
}

func (s *Store) SaveEmail(ctx context.Context, e Email, newPassword string) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		values := map[string]any{
			"email_enabled": e.Enabled, "smtp_host": e.Host, "smtp_port": e.Port,
			"smtp_username": e.Username, "smtp_from": e.From, "smtp_from_name": e.FromName, "smtp_to": e.To,
		}
		for k, v := range values {
			if err := s.set(ctx, q, k, v); err != nil {
				return err
			}
		}
		if newPassword != "" {
			return s.putSecret(ctx, q, SecretSMTPPassword, newPassword)
		}
		return nil
	})
}

func (s *Store) SaveInvoice(ctx context.Context, inv Invoice) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		values := map[string]any{
			"invoice_company_name": inv.CompanyName, "invoice_company_url": inv.CompanyURL,
			"invoice_number_prefix": inv.NumberPrefix, "invoice_default_note": inv.DefaultNote,
		}
		for k, v := range values {
			if err := s.set(ctx, q, k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) putSecret(ctx context.Context, q *sqlc.Queries, key, value string) error {
	sealed, err := s.box.SealString(value)
	if err != nil {
		return err
	}
	return q.UpsertSecret(ctx, sqlc.UpsertSecretParams{Key: key, Ciphertext: sealed.Ciphertext, Nonce: sealed.Nonce, KeyVersion: sealed.KeyVersion})
}

func (s *Store) Secret(ctx context.Context, key string) (string, error) {
	row, err := s.q.GetSecret(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get secret: %w", err)
	}
	return s.box.OpenString(crypto.Sealed{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion})
}

func (s *Store) inTx(ctx context.Context, fn func(*sqlc.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func OpenBox(ctx context.Context, pool *pgxpool.Pool, key []byte) (*crypto.Box, error) {
	q := sqlc.New(pool)
	row, err := q.GetSecret(ctx, secretKeyCheck)
	if errors.Is(err, pgx.ErrNoRows) {
		box, err := crypto.NewBox(key, 1)
		if err != nil {
			return nil, err
		}
		sealed, err := box.SealString(keyCheckPlaintext)
		if err != nil {
			return nil, err
		}
		if err := q.UpsertSecret(ctx, sqlc.UpsertSecretParams{Key: secretKeyCheck, Ciphertext: sealed.Ciphertext, Nonce: sealed.Nonce, KeyVersion: sealed.KeyVersion}); err != nil {
			return nil, fmt.Errorf("store key check: %w", err)
		}
		return box, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get key check: %w", err)
	}
	box, err := crypto.NewBox(key, row.KeyVersion)
	if err != nil {
		return nil, err
	}
	plain, err := box.OpenString(crypto.Sealed{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion})
	if err != nil || plain != keyCheckPlaintext {
		return nil, ErrKeyMismatch
	}
	return box, nil
}

func (s *Store) Time(ctx context.Context, key string) (*time.Time, error) {
	raw, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get setting %s: %w", key, err)
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil || v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil, nil
	}
	return &t, nil
}

func (s *Store) SetTime(ctx context.Context, key string, t time.Time) error {
	return s.set(ctx, s.q, key, t.UTC().Format(time.RFC3339Nano))
}
