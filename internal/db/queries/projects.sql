-- name: ListProjects :many
SELECT p.id, p.name, p.url, p.client_id, c.name AS client_name, p.updated_at,
  s.status AS screenshot_status, s.stored_path AS screenshot_path, s.captured_at AS screenshot_captured_at,
  (SELECT count(*) FROM project_assets pa WHERE pa.project_id = p.id)::bigint AS asset_count
FROM projects p
LEFT JOIN clients c ON c.id = p.client_id
LEFT JOIN project_screenshots s ON s.project_id = p.id
ORDER BY lower(p.name);

-- name: GetProject :one
SELECT p.*, c.name AS client_name FROM projects p LEFT JOIN clients c ON c.id = p.client_id WHERE p.id = $1;

-- name: CreateProject :one
INSERT INTO projects (client_id, name, url, brief) VALUES ($1, $2, $3, $4) RETURNING id;

-- name: UpdateProject :exec
UPDATE projects SET client_id = $2, name = $3, url = $4, brief = $5, updated_at = now() WHERE id = $1;

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
