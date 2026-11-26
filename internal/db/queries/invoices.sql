-- name: ListInvoices :many
SELECT i.id, i.number, i.client_id, i.bill_to, i.currency, i.invoice_date, i.due_date, i.status, i.paid_date, i.created_at,
  c.name AS client_name
FROM invoices i
LEFT JOIN clients c ON c.id = i.client_id
ORDER BY i.invoice_date DESC, i.created_at DESC;

-- name: ListAllInvoiceItems :many
SELECT invoice_id, quantity, rate, tax_percent FROM invoice_items;

-- name: GetInvoice :one
SELECT i.*, c.name AS client_name
FROM invoices i
LEFT JOIN clients c ON c.id = i.client_id
WHERE i.id = $1;

-- name: ListInvoiceItems :many
SELECT ii.*, p.name AS project_name
FROM invoice_items ii
LEFT JOIN projects p ON p.id = ii.project_id
WHERE ii.invoice_id = $1
ORDER BY ii.position;

-- name: InvoiceNumberTaken :one
SELECT EXISTS (
  SELECT 1 FROM invoices WHERE upper(number) = upper(@number::text) AND id IS DISTINCT FROM sqlc.narg(exclude_id)::uuid
);

-- name: CreateInvoice :one
INSERT INTO invoices (number, client_id, asset_id, bill_to, currency, invoice_date, due_date, status, paid_date, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: UpdateInvoice :exec
UPDATE invoices SET number = $2, client_id = $3, bill_to = $4, currency = $5, invoice_date = $6, due_date = $7,
  status = $8, paid_date = $9, note = $10, updated_at = now()
WHERE id = $1;

-- name: SetInvoiceStatus :execrows
UPDATE invoices SET status = $2, paid_date = $3, updated_at = now() WHERE id = $1;

-- name: DeleteInvoiceItems :exec
DELETE FROM invoice_items WHERE invoice_id = $1;

-- name: AddInvoiceItem :exec
INSERT INTO invoice_items (invoice_id, position, title, description, quantity, rate, tax_percent, project_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DeleteInvoice :execrows
DELETE FROM invoices WHERE id = $1;
