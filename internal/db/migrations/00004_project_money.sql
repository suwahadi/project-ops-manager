-- +goose Up
-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN currency text NOT NULL DEFAULT 'IDR' CHECK (currency IN ('IDR','USD'));
ALTER TABLE projects ADD COLUMN project_value numeric(15,2) CHECK (project_value >= 0);
ALTER TABLE projects ADD COLUMN project_cost numeric(15,2) CHECK (project_cost >= 0);
ALTER TABLE projects ADD COLUMN billing_date date;
ALTER TABLE projects ADD CONSTRAINT projects_cost_needs_value CHECK (project_cost IS NULL OR project_value IS NOT NULL);

CREATE TABLE project_payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  received_on date NOT NULL,
  amount numeric(15,2) NOT NULL CHECK (amount > 0),
  note text NOT NULL DEFAULT '',
  invoice_id uuid REFERENCES invoices(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_payments_project_idx ON project_payments (project_id, received_on);
CREATE UNIQUE INDEX project_payments_invoice_key ON project_payments (invoice_id, project_id) WHERE invoice_id IS NOT NULL;

ALTER TABLE invoice_items ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE invoice_items DROP COLUMN project_id;
DROP TABLE project_payments;
ALTER TABLE projects DROP CONSTRAINT projects_cost_needs_value;
ALTER TABLE projects DROP COLUMN billing_date;
ALTER TABLE projects DROP COLUMN project_cost;
ALTER TABLE projects DROP COLUMN project_value;
ALTER TABLE projects DROP COLUMN currency;
-- +goose StatementEnd
