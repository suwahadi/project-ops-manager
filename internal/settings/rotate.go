package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/crypto"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

type RotationResult struct {
	Secrets     int
	Credentials int
	NewVersion  int32
}

func RotateKey(ctx context.Context, pool *pgxpool.Pool, oldKey, newKey []byte) (RotationResult, error) {
	oldBox, err := OpenBox(ctx, pool, oldKey)
	if err != nil {
		return RotationResult{}, err
	}
	newVersion := oldBox.CurrentVersion() + 1
	newBox, err := crypto.NewBox(newKey, newVersion)
	if err != nil {
		return RotationResult{}, err
	}
	reseal := func(s crypto.Sealed) (crypto.Sealed, error) {
		plain, err := oldBox.Open(s)
		if err != nil {
			return crypto.Sealed{}, err
		}
		return newBox.Seal(plain)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return RotationResult{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)

	res := RotationResult{NewVersion: newVersion}
	secrets, err := q.ListSecretsForUpdate(ctx)
	if err != nil {
		return res, fmt.Errorf("list secrets: %w", err)
	}
	for _, s := range secrets {
		sealed, err := reseal(crypto.Sealed{Ciphertext: s.Ciphertext, Nonce: s.Nonce, KeyVersion: s.KeyVersion})
		if err != nil {
			return res, fmt.Errorf("decrypt secret %s: %w", s.Key, err)
		}
		if err := q.UpsertSecret(ctx, sqlc.UpsertSecretParams{Key: s.Key, Ciphertext: sealed.Ciphertext, Nonce: sealed.Nonce, KeyVersion: sealed.KeyVersion}); err != nil {
			return res, fmt.Errorf("update secret %s: %w", s.Key, err)
		}
		if s.Key != secretKeyCheck {
			res.Secrets++
		}
	}
	creds, err := q.ListEncryptedCredentialPasswords(ctx)
	if err != nil {
		return res, fmt.Errorf("list credentials: %w", err)
	}
	for _, c := range creds {
		if c.KeyVersion == nil {
			return res, errors.New("credential has a password without a key version")
		}
		sealed, err := reseal(crypto.Sealed{Ciphertext: c.PasswordCiphertext, Nonce: c.PasswordNonce, KeyVersion: *c.KeyVersion})
		if err != nil {
			return res, fmt.Errorf("decrypt credential %s: %w", c.ID, err)
		}
		v := sealed.KeyVersion
		if err := q.UpdateCredentialPasswordCipher(ctx, sqlc.UpdateCredentialPasswordCipherParams{ID: c.ID, PasswordCiphertext: sealed.Ciphertext, PasswordNonce: sealed.Nonce, KeyVersion: &v}); err != nil {
			return res, fmt.Errorf("update credential %s: %w", c.ID, err)
		}
		res.Credentials++
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}
