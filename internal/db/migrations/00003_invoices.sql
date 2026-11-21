-- +goose Up
-- +goose StatementBegin
CREATE TABLE invoices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number text NOT NULL,
  client_id uuid REFERENCES clients(id) ON DELETE SET NULL,
  asset_id uuid REFERENCES assets(id) ON DELETE SET NULL,
  bill_to text NOT NULL,
  currency text NOT NULL CHECK (currency IN ('IDR','USD')),
  invoice_date date NOT NULL,
  due_date date,
  status text NOT NULL DEFAULT 'unpaid' CHECK (status IN ('unpaid','paid')),
  paid_date date,
  note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((status = 'paid') = (paid_date IS NOT NULL))
);
CREATE UNIQUE INDEX invoices_number_key ON invoices (upper(number));
CREATE INDEX invoices_client_idx ON invoices (client_id);
CREATE INDEX invoices_date_idx ON invoices (invoice_date DESC);

CREATE TABLE invoice_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id uuid NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  position int NOT NULL,
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  quantity numeric(12,2) NOT NULL CHECK (quantity > 0),
  rate numeric(15,2) NOT NULL CHECK (rate >= 0),
  tax_percent numeric(5,2) NOT NULL DEFAULT 0 CHECK (tax_percent >= 0 AND tax_percent <= 100),
  UNIQUE (invoice_id, position)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE invoice_items;
DROP TABLE invoices;
-- +goose StatementEnd
