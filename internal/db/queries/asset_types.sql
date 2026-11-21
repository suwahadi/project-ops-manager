-- name: ListAssetTypes :many
SELECT t.*, (SELECT count(*) FROM assets a WHERE a.asset_type_id = t.id)::bigint AS asset_count
FROM asset_types t ORDER BY t.sort_order, lower(t.name);

-- name: GetAssetType :one
SELECT * FROM asset_types WHERE id = $1;

-- name: GetAssetTypeBySlug :one
SELECT * FROM asset_types WHERE slug = $1;

-- name: CreateAssetType :one
INSERT INTO asset_types (name, slug, icon, reminder_offsets, sort_order)
VALUES ($1, $2, $3, $4, (SELECT coalesce(max(sort_order), 0) + 1 FROM asset_types))
RETURNING *;

-- name: UpdateAssetType :exec
UPDATE asset_types SET name = $2, icon = $3, reminder_offsets = $4, updated_at = now() WHERE id = $1;

-- name: DeleteAssetType :exec
DELETE FROM asset_types WHERE id = $1 AND NOT is_builtin;

-- name: CountAssetsByType :one
SELECT count(*) FROM assets WHERE asset_type_id = $1;

-- name: ListAssetTypeFields :many
SELECT * FROM asset_type_fields WHERE asset_type_id = $1 ORDER BY sort_order, lower(label);

-- name: ListAllAssetTypeFields :many
SELECT * FROM asset_type_fields ORDER BY asset_type_id, sort_order, lower(label);

-- name: GetAssetTypeField :one
SELECT * FROM asset_type_fields WHERE id = $1;

-- name: CreateAssetTypeField :one
INSERT INTO asset_type_fields (asset_type_id, key, label, field_type, options, is_required, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateAssetTypeField :exec
UPDATE asset_type_fields SET label = $2, field_type = $3, options = $4, is_required = $5, sort_order = $6 WHERE id = $1;

-- name: DeleteAssetTypeField :exec
DELETE FROM asset_type_fields WHERE id = $1;

-- name: CountAssetsWithFieldValue :one
SELECT count(*) FROM assets WHERE asset_type_id = $1 AND custom_fields ? @key::text;

-- name: RemoveFieldValues :exec
UPDATE assets SET custom_fields = custom_fields - @key::text, updated_at = now()
WHERE asset_type_id = $1 AND custom_fields ? @key::text;
