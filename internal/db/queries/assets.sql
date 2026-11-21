-- name: ListAssets :many
SELECT a.id, a.name, a.expiration_date, a.cancelled_at, a.currency, a.cost_price, a.selling_price,
  a.renewal_period_months, a.client_id, a.asset_type_id, a.provider_id,
  t.name AS type_name, t.icon AS type_icon, t.reminder_offsets,
  p.name AS provider_name, c.name AS client_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
JOIN providers p ON p.id = a.provider_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE (sqlc.narg(type_id)::uuid IS NULL OR a.asset_type_id = sqlc.narg(type_id))
  AND (sqlc.narg(provider_id)::uuid IS NULL OR a.provider_id = sqlc.narg(provider_id))
  AND (NOT @internal_only::boolean OR a.client_id IS NULL)
  AND (sqlc.narg(client_id)::uuid IS NULL OR a.client_id = sqlc.narg(client_id))
  AND (@query::text = '' OR a.name ILIKE '%' || @query::text || '%')
  AND (@status::text = '' OR @status::text = (
    CASE
      WHEN a.cancelled_at IS NOT NULL THEN 'cancelled'
      WHEN a.expiration_date < @today::date THEN 'expired'
      WHEN a.expiration_date - @today::date <= (SELECT max(o) FROM unnest(t.reminder_offsets) o) THEN 'expiring'
      ELSE 'active'
    END))
ORDER BY
  CASE WHEN @sort::text = 'expiration_desc' THEN a.expiration_date END DESC,
  CASE WHEN @sort::text = 'name_asc' THEN lower(a.name) END ASC,
  CASE WHEN @sort::text = 'name_desc' THEN lower(a.name) END DESC,
  CASE WHEN @sort::text = 'created_desc' THEN a.created_at END DESC,
  a.expiration_date ASC, lower(a.name) ASC;

-- name: GetAsset :one
SELECT a.*, t.name AS type_name, t.icon AS type_icon, t.reminder_offsets, t.supports_rdap,
  p.name AS provider_name, c.name AS client_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
JOIN providers p ON p.id = a.provider_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE a.id = $1;

-- name: CreateAsset :one
INSERT INTO assets (asset_type_id, provider_id, client_id, name, url, order_date, expiration_date,
  renewal_period_months, auto_renew, cost_price, selling_price, currency, notes, custom_fields, rdap_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: UpdateAsset :exec
UPDATE assets SET asset_type_id = $2, provider_id = $3, client_id = $4, name = $5, url = $6, order_date = $7,
  expiration_date = $8, renewal_period_months = $9, auto_renew = $10, cost_price = $11, selling_price = $12,
  currency = $13, custom_fields = $14, rdap_enabled = $15, updated_at = now()
WHERE id = $1;

-- name: UpdateAssetNotes :exec
UPDATE assets SET notes = $2, updated_at = now() WHERE id = $1;

-- name: SetAssetCancelled :exec
UPDATE assets SET cancelled_at = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAsset :exec
DELETE FROM assets WHERE id = $1;

-- name: ListAssetDocumentPaths :many
SELECT d.stored_path FROM documents d WHERE d.asset_id = $1
UNION ALL
SELECT d.stored_path FROM documents d JOIN renewals r ON r.id = d.renewal_id WHERE r.asset_id = $1;

-- name: ListAssetsByClient :many
SELECT a.id, a.name, a.expiration_date, a.cancelled_at, a.currency, a.cost_price, a.selling_price,
  a.renewal_period_months, t.name AS type_name, t.icon AS type_icon, t.reminder_offsets
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE a.client_id = $1
ORDER BY a.expiration_date, lower(a.name);

-- name: ListAssetOptions :many
SELECT a.id, a.name, t.name AS type_name, c.name AS client_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
LEFT JOIN clients c ON c.id = a.client_id
ORDER BY lower(a.name);

-- name: SearchAssets :many
SELECT a.id, a.name, t.name AS type_name, t.icon AS type_icon FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE a.name ILIKE '%' || @query::text || '%' ORDER BY lower(a.name) LIMIT 20;

-- name: SearchClients :many
SELECT id, name FROM clients WHERE name ILIKE '%' || @query::text || '%' ORDER BY lower(name) LIMIT 20;

-- name: SearchProjects :many
SELECT id, name FROM projects WHERE name ILIKE '%' || @query::text || '%' ORDER BY lower(name) LIMIT 20;
