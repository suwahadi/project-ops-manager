-- name: CreateDocument :one
INSERT INTO documents (asset_id, renewal_id, original_name, stored_path, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetDocument :one
SELECT d.*, coalesce(d.asset_id, r.asset_id)::uuid AS owner_asset_id
FROM documents d LEFT JOIN renewals r ON r.id = d.renewal_id
WHERE d.id = $1;

-- name: ListAssetDocuments :many
SELECT * FROM documents WHERE asset_id = $1 ORDER BY created_at DESC;

-- name: ListRenewalDocumentsByAsset :many
SELECT d.* FROM documents d JOIN renewals r ON r.id = d.renewal_id
WHERE r.asset_id = $1 ORDER BY d.created_at DESC;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id = $1;
