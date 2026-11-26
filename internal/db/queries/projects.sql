-- name: ListProjects :many
SELECT p.id, p.name, p.url, p.client_id, c.name AS client_name, p.updated_at,
  s.status AS screenshot_status, s.stored_path AS screenshot_path, s.captured_at AS screenshot_captured_at,
  (SELECT count(*) FROM project_assets pa WHERE pa.project_id = p.id)::bigint AS asset_count,
  p.currency, p.project_value, p.project_cost, p.billing_date,
  (SELECT COALESCE(sum(pp.amount), 0) FROM project_payments pp WHERE pp.project_id = p.id)::numeric(15,2) AS received
FROM projects p
LEFT JOIN clients c ON c.id = p.client_id
LEFT JOIN project_screenshots s ON s.project_id = p.id
ORDER BY lower(p.name);

-- name: GetProject :one
SELECT p.*, c.name AS client_name FROM projects p LEFT JOIN clients c ON c.id = p.client_id WHERE p.id = $1;

-- name: CreateProject :one
INSERT INTO projects (client_id, name, url, brief, currency, project_value, project_cost, billing_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id;

-- name: UpdateProject :exec
UPDATE projects SET client_id = $2, name = $3, url = $4, brief = $5, currency = $6, project_value = $7,
  project_cost = $8, billing_date = $9, updated_at = now()
WHERE id = $1;

-- name: LockProject :one
SELECT id, client_id, currency, project_value FROM projects WHERE id = $1 FOR UPDATE;

-- name: ProjectReceived :one
SELECT COALESCE(sum(amount), 0)::numeric(15,2) AS received, count(*)::bigint AS payments
FROM project_payments WHERE project_id = $1;

-- name: ListProjectPayments :many
SELECT pp.*, i.number AS invoice_number
FROM project_payments pp
LEFT JOIN invoices i ON i.id = pp.invoice_id
WHERE pp.project_id = $1
ORDER BY pp.received_on, pp.created_at;

-- name: CreateProjectPayment :one
INSERT INTO project_payments (project_id, received_on, amount, note, invoice_id)
VALUES ($1, $2, $3, $4, $5) RETURNING id;

-- name: DeleteProjectPayment :execrows
DELETE FROM project_payments WHERE id = $1 AND project_id = $2;

-- name: ListInvoiceProjectPayments :many
SELECT project_id FROM project_payments WHERE invoice_id = $1;

-- name: ListProjectsByClientWithMoney :many
SELECT p.id, p.name, p.currency, p.project_value, p.project_cost, p.billing_date,
  (SELECT COALESCE(sum(pp.amount), 0) FROM project_payments pp WHERE pp.project_id = p.id)::numeric(15,2) AS received
FROM projects p
WHERE p.client_id = $1
ORDER BY lower(p.name);

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: ClearProjectAssets :exec
DELETE FROM project_assets WHERE project_id = $1;

-- name: AddProjectAsset :exec
INSERT INTO project_assets (project_id, asset_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListProjectAssets :many
SELECT a.id, a.name, a.expiration_date, a.cancelled_at, t.name AS type_name, t.icon AS type_icon, t.reminder_offsets
FROM project_assets pa
JOIN assets a ON a.id = pa.asset_id
JOIN asset_types t ON t.id = a.asset_type_id
WHERE pa.project_id = $1
ORDER BY lower(a.name);

-- name: ListProjectsByAsset :many
SELECT p.id, p.name, p.url, c.name AS client_name
FROM project_assets pa
JOIN projects p ON p.id = pa.project_id
LEFT JOIN clients c ON c.id = p.client_id
WHERE pa.asset_id = $1
ORDER BY lower(p.name);

-- name: GetProjectScreenshot :one
SELECT * FROM project_screenshots WHERE project_id = $1;

-- name: RequestProjectScreenshot :exec
INSERT INTO project_screenshots (project_id, status, requested_url, attempts, requested_at, error)
VALUES ($1, 'pending', $2, 0, @now::timestamptz, NULL)
ON CONFLICT (project_id) DO UPDATE SET status = 'pending', requested_url = EXCLUDED.requested_url,
  attempts = 0, requested_at = EXCLUDED.requested_at, error = NULL;

-- name: ClaimPendingScreenshots :many
UPDATE project_screenshots SET attempts = attempts + 1
WHERE project_id IN (
  SELECT project_id FROM project_screenshots WHERE status = 'pending' ORDER BY requested_at LIMIT 5
)
RETURNING *;

-- name: CompleteScreenshot :execrows
UPDATE project_screenshots SET status = 'done', stored_path = $2, captured_at = $3, error = NULL
WHERE project_id = $1 AND requested_at = @requested_at::timestamptz;

-- name: FailScreenshot :execrows
UPDATE project_screenshots SET status = 'failed', error = $2
WHERE project_id = $1 AND requested_at = @requested_at::timestamptz;

-- name: ListScreenshotPathsByProject :many
SELECT stored_path FROM project_screenshots WHERE project_id = $1 AND stored_path IS NOT NULL;
