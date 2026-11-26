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

-- name: ListProjectPaymentsForFinance :many
SELECT pp.project_id, pp.received_on, pp.amount, p.project_value, p.project_cost, p.currency,
  p.client_id, c.name AS client_name
FROM project_payments pp
JOIN projects p ON p.id = pp.project_id
LEFT JOIN clients c ON c.id = p.client_id
WHERE p.project_value IS NOT NULL AND pp.received_on < @end_date::date
ORDER BY pp.project_id, pp.received_on, pp.created_at;

-- name: ListBillableProjects :many
SELECT p.id, p.billing_date, p.project_value, p.project_cost, p.currency, p.client_id, c.name AS client_name,
  (SELECT COALESCE(sum(pp.amount), 0) FROM project_payments pp WHERE pp.project_id = p.id)::numeric(15,2) AS received
FROM projects p
LEFT JOIN clients c ON c.id = p.client_id
WHERE p.project_value IS NOT NULL AND p.billing_date IS NOT NULL;
