package credential

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/crypto"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

var (
	ErrNotFound   = errors.New("credential not found")
	ErrNoPassword = errors.New("credential has no password")
)

type Input struct {
	Label    string
	URL      string
	Username string
	Password string
	Notes    string
}

type Service struct {
	q   *sqlc.Queries
	box *crypto.Box
}

func NewService(db sqlc.DBTX, box *crypto.Box) *Service {
	return &Service{q: sqlc.New(db), box: box}
}

func optional(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) List(ctx context.Context, assetID string) ([]sqlc.ListCredentialsRow, error) {
	return s.q.ListCredentials(ctx, assetID)
}

func (s *Service) Get(ctx context.Context, assetID, id string) (sqlc.Credential, error) {
	c, err := s.q.GetCredential(ctx, sqlc.GetCredentialParams{ID: id, AssetID: assetID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Service) seal(password string) ([]byte, []byte, *int32, error) {
	if password == "" {
		return nil, nil, nil, nil
	}
	sealed, err := s.box.SealString(password)
	if err != nil {
		return nil, nil, nil, err
	}
	v := sealed.KeyVersion
	return sealed.Ciphertext, sealed.Nonce, &v, nil
}

func (s *Service) Create(ctx context.Context, assetID string, in Input) error {
	ct, nonce, ver, err := s.seal(in.Password)
	if err != nil {
		return err
	}
	_, err = s.q.CreateCredential(ctx, sqlc.CreateCredentialParams{
		AssetID: assetID, Label: strings.TrimSpace(in.Label), Url: optional(in.URL), Username: optional(in.Username),
		PasswordCiphertext: ct, PasswordNonce: nonce, KeyVersion: ver, Notes: strings.TrimSpace(in.Notes),
	})
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

func (s *Service) Update(ctx context.Context, assetID, id string, in Input, changePassword bool) error {
	if err := s.q.UpdateCredential(ctx, sqlc.UpdateCredentialParams{
		ID: id, AssetID: assetID, Label: strings.TrimSpace(in.Label), Url: optional(in.URL), Username: optional(in.Username), Notes: strings.TrimSpace(in.Notes),
	}); err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	if !changePassword {
		return nil
	}
	ct, nonce, ver, err := s.seal(in.Password)
	if err != nil {
		return err
	}
	if err := s.q.UpdateCredentialPassword(ctx, sqlc.UpdateCredentialPasswordParams{ID: id, AssetID: assetID, PasswordCiphertext: ct, PasswordNonce: nonce, KeyVersion: ver}); err != nil {
		return fmt.Errorf("update credential password: %w", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, assetID, id string) error {
	return s.q.DeleteCredential(ctx, sqlc.DeleteCredentialParams{ID: id, AssetID: assetID})
}

func (s *Service) Reveal(ctx context.Context, assetID, id string) (string, error) {
	c, err := s.Get(ctx, assetID, id)
	if err != nil {
		return "", err
	}
	if c.PasswordCiphertext == nil || c.KeyVersion == nil {
		return "", ErrNoPassword
	}
	return s.box.OpenString(crypto.Sealed{Ciphertext: c.PasswordCiphertext, Nonce: c.PasswordNonce, KeyVersion: *c.KeyVersion})
}
