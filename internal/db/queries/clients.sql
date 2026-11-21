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
