-- name: ListUpcomingRenewals :many
SELECT a.id, a.name, a.expiration_date, t.name AS type_name, t.icon AS type_icon, t.reminder_offsets, c.name AS client_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE a.cancelled_at IS NULL AND a.expiration_date <= @until::date
ORDER BY a.expiration_date, lower(a.name);

-- name: CountAssetsByStatus :one
SELECT
  count(*) FILTER (WHERE a.cancelled_at IS NOT NULL)::bigint AS cancelled,
  count(*) FILTER (WHERE a.cancelled_at IS NULL AND a.expiration_date < @today::date)::bigint AS expired,
  count(*) FILTER (WHERE a.cancelled_at IS NULL AND a.expiration_date >= @today::date
    AND a.expiration_date - @today::date <= (SELECT max(o) FROM unnest(t.reminder_offsets) o))::bigint AS expiring,
  count(*) FILTER (WHERE a.cancelled_at IS NULL AND a.expiration_date - @today::date > (SELECT max(o) FROM unnest(t.reminder_offsets) o))::bigint AS active
FROM assets a JOIN asset_types t ON t.id = a.asset_type_id;

-- name: CountAssetsPerType :many
SELECT t.id, t.name, t.icon, count(a.id)::bigint AS asset_count
FROM asset_types t LEFT JOIN assets a ON a.asset_type_id = t.id
GROUP BY t.id, t.name, t.icon, t.sort_order
ORDER BY t.sort_order, lower(t.name);
