-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(@email::text);

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUserEmail :exec
UPDATE users SET email = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSession :one
SELECT * FROM sessions WHERE token_hash = $1 AND expires_at > @now::timestamptz;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = $2, expires_at = $3 WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = $1 AND token_hash <> @keep_token_hash::text;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= @now::timestamptz;

-- name: RecordLoginAttempt :exec
INSERT INTO login_attempts (ip, success, attempted_at) VALUES ($1, $2, $3);

-- name: ListRecentLoginAttempts :many
SELECT success, attempted_at FROM login_attempts
WHERE ip = $1 AND attempted_at > @since::timestamptz
ORDER BY attempted_at;

-- name: DeleteLoginAttemptsBefore :execrows
DELETE FROM login_attempts WHERE attempted_at < @before::timestamptz;
