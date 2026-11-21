-- name: ListCredentials :many
SELECT id, asset_id, label, url, username, (password_ciphertext IS NOT NULL)::boolean AS has_password, notes, created_at, updated_at
FROM credentials WHERE asset_id = $1 ORDER BY lower(label);

-- name: GetCredential :one
SELECT * FROM credentials WHERE id = $1 AND asset_id = $2;

-- name: CreateCredential :one
INSERT INTO credentials (asset_id, label, url, username, password_ciphertext, password_nonce, key_version, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id;

-- name: UpdateCredential :exec
UPDATE credentials SET label = $3, url = $4, username = $5, notes = $6, updated_at = now()
WHERE id = $1 AND asset_id = $2;

-- name: UpdateCredentialPassword :exec
UPDATE credentials SET password_ciphertext = $3, password_nonce = $4, key_version = $5, updated_at = now()
WHERE id = $1 AND asset_id = $2;

-- name: DeleteCredential :exec
DELETE FROM credentials WHERE id = $1 AND asset_id = $2;
