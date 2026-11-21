package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

const (
	SessionIdleTimeout  = 30 * 24 * time.Hour
	sessionTouchEvery   = time.Minute
	attemptRetention    = 24 * time.Hour
	lockoutLookbackSpan = LockoutWindow + LockoutDuration
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrLockedOut          = errors.New("too many failed logins")
	ErrNoSession          = errors.New("no valid session")
	ErrWrongPassword      = errors.New("current password is incorrect")
)

type Session struct {
	TokenHash string
	UserID    string
	CSRFToken string
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), now: now}
}

func (s *Service) EnsureAdmin(ctx context.Context, email, password string) (bool, error) {
	n, err := s.q.CountUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return false, nil
	}
	if email == "" || password == "" {
		return false, errors.New("OPS_ADMIN_EMAIL and OPS_ADMIN_PASSWORD are required on first start")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := s.q.CreateUser(ctx, sqlc.CreateUserParams{Email: strings.TrimSpace(email), PasswordHash: hash}); err != nil {
		return false, fmt.Errorf("create admin: %w", err)
	}
	return true, nil
}

func (s *Service) Login(ctx context.Context, ip netip.Addr, email, password string) (string, error) {
	now := s.now()
	rows, err := s.q.ListRecentLoginAttempts(ctx, sqlc.ListRecentLoginAttemptsParams{Ip: ip, Since: now.Add(-lockoutLookbackSpan)})
	if err != nil {
		return "", fmt.Errorf("list login attempts: %w", err)
	}
	attempts := make([]Attempt, len(rows))
	for i, r := range rows {
		attempts[i] = Attempt{Success: r.Success, At: r.AttemptedAt}
	}
	if _, locked := LockedUntil(attempts, now); locked {
		return "", ErrLockedOut
	}

	user, err := s.q.GetUserByEmail(ctx, strings.TrimSpace(email))
	ok := false
	if err == nil {
		ok, err = VerifyPassword(password, user.PasswordHash)
		if err != nil {
			return "", fmt.Errorf("verify password: %w", err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("get user: %w", err)
	} else {
		_, _ = VerifyPassword(password, dummyHash)
	}
	if err := s.q.RecordLoginAttempt(ctx, sqlc.RecordLoginAttemptParams{Ip: ip, Success: ok, AttemptedAt: now}); err != nil {
		return "", fmt.Errorf("record login attempt: %w", err)
	}
	if !ok {
		return "", ErrInvalidCredentials
	}
	return s.createSession(ctx, s.q, user.ID)
}

var dummyHash = func() string {
	h, _ := HashPassword("dummy-password-for-timing")
	return h
}()

func (s *Service) createSession(ctx context.Context, q *sqlc.Queries, userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	err = q.CreateSession(ctx, sqlc.CreateSessionParams{
		TokenHash:  HashToken(token),
		UserID:     userID,
		CsrfToken:  csrf,
		ExpiresAt:  now.Add(SessionIdleTimeout),
		LastSeenAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNoSession
	}
	now := s.now()
	hash := HashToken(token)
	row, err := s.q.GetSession(ctx, sqlc.GetSessionParams{TokenHash: hash, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	if now.Sub(row.LastSeenAt) >= sessionTouchEvery {
		if err := s.q.TouchSession(ctx, sqlc.TouchSessionParams{TokenHash: hash, LastSeenAt: now, ExpiresAt: now.Add(SessionIdleTimeout)}); err != nil {
			return Session{}, fmt.Errorf("touch session: %w", err)
		}
	}
	return Session{TokenHash: hash, UserID: row.UserID, CSRFToken: row.CsrfToken}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, HashToken(token))
}

func (s *Service) User(ctx context.Context, id string) (sqlc.User, error) {
	return s.q.GetUserByID(ctx, id)
}

func (s *Service) ChangeEmail(ctx context.Context, userID, email string) error {
	return s.q.UpdateUserEmail(ctx, sqlc.UpdateUserEmailParams{ID: userID, Email: strings.TrimSpace(email)})
}

func (s *Service) ChangePassword(ctx context.Context, sess Session, current, next string) error {
	user, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	ok, err := VerifyPassword(current, user.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return ErrWrongPassword
	}
	if err := ValidateNewPassword(next); err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if err := q.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if err := q.DeleteOtherSessions(ctx, sqlc.DeleteOtherSessionsParams{UserID: user.ID, KeepTokenHash: sess.TokenHash}); err != nil {
		return fmt.Errorf("delete other sessions: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) Cleanup(ctx context.Context) (int64, int64, error) {
	now := s.now()
	sessions, err := s.q.DeleteExpiredSessions(ctx, now)
	if err != nil {
		return 0, 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	attempts, err := s.q.DeleteLoginAttemptsBefore(ctx, now.Add(-attemptRetention))
	if err != nil {
		return sessions, 0, fmt.Errorf("delete login attempts: %w", err)
	}
	return sessions, attempts, nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func RandomToken() (string, error) { return randomToken() }
