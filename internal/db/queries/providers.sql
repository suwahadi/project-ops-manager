-- name: ListProviders :many
SELECT p.*, (SELECT count(*) FROM assets a WHERE a.provider_id = p.id)::bigint AS asset_count
FROM providers p ORDER BY lower(p.name);

-- name: GetProvider :one
SELECT * FROM providers WHERE id = $1;

-- name: GetProviderByName :one
SELECT * FROM providers WHERE lower(name) = lower(@name::text);

-- name: CreateProvider :one
INSERT INTO providers (name, website, login_url, notes) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateProvider :exec
UPDATE providers SET name = $2, website = $3, login_url = $4, notes = $5, updated_at = now() WHERE id = $1;

-- name: CountAssetsByProvider :one
SELECT count(*) FROM assets WHERE provider_id = $1;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = $1;
