-- name: ListReminderCandidates :many
SELECT a.id, a.expiration_date, t.reminder_offsets
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE a.cancelled_at IS NULL
  AND a.expiration_date > @today::date
  AND a.expiration_date - @today::date <= (SELECT max(o) FROM unnest(t.reminder_offsets) o);

-- name: InsertReminderLog :one
INSERT INTO reminder_logs (asset_id, expiration_date, offset_days, channel, status, attempts, next_attempt_at, updated_at)
VALUES ($1, $2, $3, $4, 'pending', 0, @now::timestamptz, @now::timestamptz)
ON CONFLICT (asset_id, expiration_date, offset_days, channel) DO NOTHING
RETURNING id;

-- name: ListDueReminderLogs :many
SELECT l.id, l.asset_id, l.expiration_date, l.offset_days, l.channel, l.attempts,
  a.name AS asset_name, a.cost_price, a.selling_price, a.currency, a.cancelled_at,
  t.name AS type_name, p.name AS provider_name, c.name AS client_name
FROM reminder_logs l
JOIN assets a ON a.id = l.asset_id
JOIN asset_types t ON t.id = a.asset_type_id
JOIN providers p ON p.id = a.provider_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE l.status = 'pending' AND l.next_attempt_at <= @now::timestamptz
ORDER BY l.next_attempt_at
LIMIT 200;

-- name: MarkReminderSent :exec
UPDATE reminder_logs SET status = 'sent', attempts = attempts + 1, sent_at = @now::timestamptz, error = NULL,
  next_attempt_at = NULL, updated_at = @now::timestamptz
WHERE id = $1;

-- name: MarkReminderRetry :exec
UPDATE reminder_logs SET attempts = attempts + 1, error = $2, next_attempt_at = $3, updated_at = @now::timestamptz
WHERE id = $1;

-- name: MarkReminderFailed :exec
UPDATE reminder_logs SET status = 'failed', attempts = attempts + 1, error = $2, next_attempt_at = NULL, updated_at = @now::timestamptz
WHERE id = $1;

-- name: ListReminderLogsByAsset :many
SELECT * FROM reminder_logs WHERE asset_id = $1 ORDER BY created_at DESC, channel LIMIT 100;

-- name: LatestFinishedReminderLog :one
SELECT * FROM reminder_logs WHERE status <> 'pending' ORDER BY updated_at DESC LIMIT 1;

-- name: StartJobRun :one
INSERT INTO job_runs (job, started_at, status) VALUES ($1, $2, 'running') RETURNING id;

-- name: FinishJobRun :exec
UPDATE job_runs SET finished_at = $2, status = $3, summary = $4, error = $5 WHERE id = $1;

-- name: LatestJobRun :one
SELECT * FROM job_runs WHERE job = $1 AND status <> 'running' ORDER BY started_at DESC LIMIT 1;

-- name: ListJobRuns :many
SELECT * FROM job_runs WHERE job = $1 ORDER BY started_at DESC LIMIT $2;

-- name: DeleteJobRunsBefore :exec
DELETE FROM job_runs WHERE started_at < $1;

-- name: DeleteReminderLog :exec
DELETE FROM reminder_logs WHERE id = $1 AND status = 'pending';

-- name: GetAssetForReminder :one
SELECT a.expiration_date, a.cancelled_at FROM assets a WHERE a.id = $1;
