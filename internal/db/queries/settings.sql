-- name: ListSettings :many
SELECT key, value FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: GetSecret :one
SELECT * FROM secrets WHERE key = $1;

-- name: UpsertSecret :exec
INSERT INTO secrets (key, ciphertext, nonce, key_version, updated_at) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (key) DO UPDATE SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, key_version = EXCLUDED.key_version, updated_at = now();

-- name: DeleteSecret :exec
DELETE FROM secrets WHERE key = $1;

-- name: ListSecrets :many
SELECT * FROM secrets ORDER BY key;

-- name: TryAdvisoryLock :one
SELECT pg_try_advisory_lock($1);

-- name: AdvisoryUnlock :one
SELECT pg_advisory_unlock($1);

-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;
