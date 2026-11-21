-- name: ListRDAPAssets :many
SELECT a.id, a.name, a.expiration_date
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE t.supports_rdap AND a.rdap_enabled AND a.cancelled_at IS NULL
ORDER BY a.name;

-- name: GetRDAPAsset :one
SELECT a.id, a.name, a.expiration_date, a.rdap_enabled, a.cancelled_at, t.supports_rdap
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE a.id = $1;

-- name: SetRDAPResult :exec
UPDATE assets SET rdap_checked_at = $2, rdap_result = $3, rdap_error = $4 WHERE id = $1;

-- name: SetRDAPExpiration :exec
UPDATE assets SET expiration_date = $2, rdap_checked_at = $3, rdap_result = 'updated', rdap_error = NULL, updated_at = now() WHERE id = $1;

-- name: ListRDAPStatus :many
SELECT a.id, a.name, a.expiration_date, a.rdap_checked_at, a.rdap_result, a.rdap_error
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id
WHERE t.supports_rdap AND a.rdap_enabled AND a.cancelled_at IS NULL
ORDER BY (a.rdap_result = 'error') DESC NULLS LAST, a.name;
