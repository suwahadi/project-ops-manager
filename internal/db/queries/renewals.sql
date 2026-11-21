-- name: CreateRenewal :one
INSERT INTO renewals (asset_id, paid_on, previous_expiration_date, new_expiration_date, cost_price, selling_price, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id;

-- name: ApplyRenewalToAsset :exec
UPDATE assets SET expiration_date = $2, cost_price = $3, selling_price = $4, updated_at = now() WHERE id = $1;

-- name: ListRenewalsByAsset :many
SELECT * FROM renewals WHERE asset_id = $1 ORDER BY paid_on DESC, created_at DESC;

-- name: GetAssetForUpdate :one
SELECT id, expiration_date, client_id, currency FROM assets WHERE id = $1 FOR UPDATE;
