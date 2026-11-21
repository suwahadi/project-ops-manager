-- name: ListEncryptedCredentialPasswords :many
SELECT id, password_ciphertext, password_nonce, key_version FROM credentials
WHERE password_ciphertext IS NOT NULL
FOR UPDATE;

-- name: UpdateCredentialPasswordCipher :exec
UPDATE credentials SET password_ciphertext = $2, password_nonce = $3, key_version = $4 WHERE id = $1;

-- name: ListSecretsForUpdate :many
SELECT * FROM secrets ORDER BY key FOR UPDATE;
