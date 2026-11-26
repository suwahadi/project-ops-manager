-- name: ListClients :many
SELECT c.*,
  (SELECT count(*) FROM assets a WHERE a.client_id = c.id)::bigint AS asset_count,
  (SELECT count(*) FROM projects p WHERE p.client_id = c.id)::bigint AS project_count
FROM clients c ORDER BY lower(c.name);

-- name: ListClientOptions :many
SELECT id, name FROM clients ORDER BY lower(name);

-- name: GetClient :one
SELECT * FROM clients WHERE id = $1;

-- name: CreateClient :one
INSERT INTO clients (name, contact_person, email, phone, notes) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: UpdateClient :exec
UPDATE clients SET name = $2, contact_person = $3, email = $4, phone = $5, notes = $6, updated_at = now() WHERE id = $1;

-- name: CountClientDependents :one
SELECT
  (SELECT count(*) FROM assets a WHERE a.client_id = @client_id::uuid)::bigint AS assets,
  (SELECT count(*) FROM projects p WHERE p.client_id = @client_id::uuid)::bigint AS projects;

-- name: DeleteClient :exec
DELETE FROM clients WHERE id = $1;

-- name: ListProjectsByClient :many
SELECT id, name, url FROM projects WHERE client_id = $1 ORDER BY lower(name);

-- name: ListAssetsLinkableToClient :many
SELECT a.id, a.name, t.name AS type_name, c.name AS client_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE a.client_id IS DISTINCT FROM @client_id::uuid
ORDER BY lower(a.name);

-- name: ListProjectsLinkableToClient :many
SELECT p.id, p.name, c.name AS client_name
FROM projects p
LEFT JOIN clients c ON c.id = p.client_id
WHERE p.client_id IS DISTINCT FROM @client_id::uuid
ORDER BY lower(p.name);

-- name: LinkAssetsToClient :execrows
UPDATE assets SET client_id = @client_id::uuid, updated_at = now()
WHERE id = ANY(@ids::uuid[]) AND client_id IS DISTINCT FROM @client_id::uuid;

-- name: LinkProjectsToClient :execrows
UPDATE projects SET client_id = @client_id::uuid, updated_at = now()
WHERE id = ANY(@ids::uuid[]) AND client_id IS DISTINCT FROM @client_id::uuid;

-- name: UnlinkAssetFromClient :execrows
UPDATE assets SET client_id = NULL, selling_price = NULL, updated_at = now()
WHERE id = @asset_id::uuid AND client_id = @client_id::uuid;

-- name: UnlinkProjectFromClient :execrows
UPDATE projects SET client_id = NULL, project_value = NULL, project_cost = NULL, billing_date = NULL, updated_at = now()
WHERE id = @project_id::uuid AND client_id = @client_id::uuid
  AND NOT EXISTS (SELECT 1 FROM project_payments pp WHERE pp.project_id = @project_id::uuid);

-- name: ProjectHasPayments :one
SELECT EXISTS (SELECT 1 FROM project_payments WHERE project_id = $1);
