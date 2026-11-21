-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf_token text NOT NULL,
  expires_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE login_attempts (
  id bigserial PRIMARY KEY,
  ip inet NOT NULL,
  success boolean NOT NULL,
  attempted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_attempts_ip_time_idx ON login_attempts (ip, attempted_at);

CREATE TABLE settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE secrets (
  key text PRIMARY KEY,
  ciphertext bytea NOT NULL,
  nonce bytea NOT NULL,
  key_version integer NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE clients (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  contact_person text,
  email text,
  phone text,
  notes text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE providers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  website text,
  login_url text,
  notes text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE asset_types (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  slug text NOT NULL UNIQUE,
  icon text NOT NULL,
  is_builtin boolean NOT NULL DEFAULT false,
  supports_rdap boolean NOT NULL DEFAULT false,
  reminder_offsets integer[] NOT NULL CHECK (cardinality(reminder_offsets) >= 1 AND 1 <= ALL (reminder_offsets)),
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE asset_type_fields (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_type_id uuid NOT NULL REFERENCES asset_types(id) ON DELETE CASCADE,
  key text NOT NULL,
  label text NOT NULL,
  field_type text NOT NULL CHECK (field_type IN ('text','long_text','number','date','url','boolean','select')),
  options jsonb NOT NULL DEFAULT '[]',
  is_required boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0,
  UNIQUE (asset_type_id, key)
);

CREATE TABLE assets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_type_id uuid NOT NULL REFERENCES asset_types(id) ON DELETE RESTRICT,
  provider_id uuid NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
  client_id uuid REFERENCES clients(id) ON DELETE RESTRICT,
  name text NOT NULL,
  url text,
  order_date date,
  expiration_date date NOT NULL,
  renewal_period_months integer NOT NULL CHECK (renewal_period_months > 0),
  auto_renew boolean NOT NULL DEFAULT false,
  cost_price numeric(15,2) NOT NULL CHECK (cost_price >= 0),
  selling_price numeric(15,2) CHECK (selling_price >= 0),
  currency text NOT NULL CHECK (currency IN ('IDR','USD')),
  notes text NOT NULL DEFAULT '',
  custom_fields jsonb NOT NULL DEFAULT '{}',
  rdap_enabled boolean NOT NULL DEFAULT true,
  rdap_checked_at timestamptz,
  rdap_result text CHECK (rdap_result IN ('updated','unchanged','unsupported','error')),
  rdap_error text,
  cancelled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX assets_expiration_idx ON assets (expiration_date) WHERE cancelled_at IS NULL;
CREATE INDEX assets_type_idx ON assets (asset_type_id);
CREATE INDEX assets_client_idx ON assets (client_id);
CREATE INDEX assets_provider_idx ON assets (provider_id);

CREATE TABLE renewals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  paid_on date NOT NULL,
  previous_expiration_date date NOT NULL,
  new_expiration_date date NOT NULL,
  cost_price numeric(15,2) NOT NULL CHECK (cost_price >= 0),
  selling_price numeric(15,2) CHECK (selling_price >= 0),
  currency text NOT NULL CHECK (currency IN ('IDR','USD')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX renewals_asset_idx ON renewals (asset_id);
CREATE INDEX renewals_paid_on_idx ON renewals (paid_on);

CREATE TABLE documents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_id uuid REFERENCES assets(id) ON DELETE CASCADE,
  renewal_id uuid REFERENCES renewals(id) ON DELETE CASCADE,
  original_name text NOT NULL,
  stored_path text NOT NULL UNIQUE,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((asset_id IS NULL) <> (renewal_id IS NULL))
);

CREATE TABLE credentials (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  label text NOT NULL,
  url text,
  username text,
  password_ciphertext bytea,
  password_nonce bytea,
  key_version integer,
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  client_id uuid REFERENCES clients(id) ON DELETE RESTRICT,
  name text NOT NULL,
  url text,
  brief text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_assets (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  PRIMARY KEY (project_id, asset_id)
);

CREATE TABLE project_screenshots (
  project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  status text NOT NULL CHECK (status IN ('pending','done','failed')),
  requested_url text NOT NULL,
  stored_path text,
  captured_at timestamptz,
  error text,
  attempts integer NOT NULL DEFAULT 0,
  requested_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reminder_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  expiration_date date NOT NULL,
  offset_days integer NOT NULL,
  channel text NOT NULL CHECK (channel IN ('telegram','email')),
  status text NOT NULL CHECK (status IN ('pending','sent','failed')),
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz,
  error text,
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (asset_id, expiration_date, offset_days, channel)
);

INSERT INTO asset_types (name, slug, icon, is_builtin, supports_rdap, reminder_offsets, sort_order) VALUES
  ('Domain', 'domain', 'globe', true, true, '{30,14,7,1}', 1),
  ('Hosting', 'hosting', 'server', true, false, '{30,14,7,1}', 2),
  ('VPS', 'vps', 'cpu', true, false, '{14,7,1}', 3),
  ('SSL Certificate', 'ssl-certificate', 'lock', true, false, '{14,7,1}', 4),
  ('Email Hosting', 'email-hosting', 'mail', true, false, '{30,14,7,1}', 5),
  ('Software License', 'software-license', 'key', true, false, '{30,14,7,1}', 6)
ON CONFLICT DO NOTHING;

INSERT INTO asset_type_fields (asset_type_id, key, label, field_type, sort_order)
SELECT t.id, f.key, f.label, f.field_type, f.sort_order
FROM (VALUES
  ('domain', 'nameservers', 'Nameservers', 'long_text', 1),
  ('domain', 'registrar_account', 'Registrar account', 'text', 2),
  ('hosting', 'plan_name', 'Plan name', 'text', 1),
  ('hosting', 'control_panel_url', 'Control panel URL', 'url', 2),
  ('vps', 'ip_address', 'IP address', 'text', 1),
  ('vps', 'region', 'Region', 'text', 2),
  ('vps', 'specs', 'Specs', 'text', 3),
  ('ssl-certificate', 'issuer', 'Issuer', 'text', 1),
  ('ssl-certificate', 'covered_hostnames', 'Covered hostnames', 'long_text', 2),
  ('email-hosting', 'plan_name', 'Plan name', 'text', 1),
  ('email-hosting', 'mailbox_count', 'Mailbox count', 'number', 2),
  ('software-license', 'seats', 'Seats', 'number', 1),
  ('software-license', 'account_email', 'Account email', 'text', 2)
) AS f(slug, key, label, field_type, sort_order)
JOIN asset_types t ON t.slug = f.slug
ON CONFLICT DO NOTHING;

INSERT INTO settings (key, value) VALUES
  ('timezone', '"Asia/Jakarta"'),
  ('reminder_send_time', '"09:00"'),
  ('default_currency', '"IDR"'),
  ('telegram_enabled', 'false'),
  ('telegram_chat_id', '""'),
  ('email_enabled', 'false'),
  ('smtp_host', '""'),
  ('smtp_port', '587'),
  ('smtp_username', '""'),
  ('smtp_from', '""'),
  ('smtp_to', '""')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE reminder_logs;
DROP TABLE project_screenshots;
DROP TABLE project_assets;
DROP TABLE projects;
DROP TABLE credentials;
DROP TABLE documents;
DROP TABLE renewals;
DROP TABLE assets;
DROP TABLE asset_type_fields;
DROP TABLE asset_types;
DROP TABLE providers;
DROP TABLE clients;
DROP TABLE secrets;
DROP TABLE settings;
DROP TABLE login_attempts;
DROP TABLE sessions;
DROP TABLE users;
-- +goose StatementEnd
