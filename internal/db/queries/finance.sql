-- name: ListRenewalsPaidBetween :many
SELECT r.paid_on, r.cost_price, r.selling_price, r.currency, a.client_id, c.name AS client_name,
  a.asset_type_id, t.name AS type_name
FROM renewals r
JOIN assets a ON a.id = r.asset_id
JOIN asset_types t ON t.id = a.asset_type_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE r.paid_on >= @start_date::date AND r.paid_on < @end_date::date;

-- name: ListProjectableAssets :many
SELECT a.id, a.expiration_date, a.renewal_period_months, a.cost_price, a.selling_price, a.currency,
  a.client_id, c.name AS client_name, a.asset_type_id, t.name AS type_name
FROM assets a
JOIN asset_types t ON t.id = a.asset_type_id
LEFT JOIN clients c ON c.id = a.client_id
WHERE a.cancelled_at IS NULL AND a.expiration_date < @end_date::date;
