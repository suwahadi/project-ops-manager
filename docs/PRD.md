# Ops Manager — Product Requirements Document

Nov 21, 2026 · @Ambon

## 1. Overview

Ops Manager is a self-hosted, single-user web app that tracks every web-development asset with an expiration date and sends reminders before it expires. Guiding principle: **anything with an expiration date, a cost and a renewal belongs here.**

**Problem.** Domain, hosting and related renewals for both the company and its clients are tracked from memory. Missed renewals cause downtime, lost domains and unbilled costs.

**Goals**

- Never miss a renewal: every asset has an expiration date and a reminder schedule.
- Know cost, selling price and margin per asset, per month and per year.
- Keep asset notes, credentials, documents and project briefs in one place.

**Success criteria**

- Every non-cancelled asset has an expiration date and resolves to a reminder schedule.
- Each reminder is sent once per asset, per offset, per channel, on its scheduled day at the configured send time.
- Dashboard renders in under 1 second with 500 assets.
- A new asset can be created in under 1 minute.

## 2. Scope

The MVP covers assets, clients, projects, finance summaries, documents, credentials, reminders and RDAP sync for one user. Anything not listed under In scope is out of scope and must not be built.

**In scope (MVP)**

- Dashboard summary: renewals in the next 30 days, asset count per status, cost and margin for this month and this year, asset count per type.
- Assets: built-in types (Domain, Hosting, VPS, SSL Certificate, Email Hosting, Software License) plus user-defined custom types with custom fields.
- Ownership: each asset is Internal or belongs to one client.
- Clients: lightweight records (name and contact details).
- Providers: registrars, hosting companies and vendors.
- Pricing: cost price and selling price per asset, renewal history with prices.
- Finance summary: cost, revenue and margin per month and year, plus a 12-month renewal projection.
- Projects: Markdown brief (overview, tech stack, features), linked client and assets, automatic website screenshot on save plus manual refresh.
- Documents: invoice and receipt uploads attached to an asset or a renewal; images auto-converted to WebP.
- Notes and credentials per asset; passwords encrypted at rest.
- Reminders: default schedule per asset type, delivered via Telegram and email, last reminder at H-1.
- RDAP sync: periodic expiration check for domains that overwrites the expiration date automatically.
- Light, dark and system themes with a manual toggle.
- No search-engine indexing.

**Out of scope**

- Multiple users, roles, permissions or registration.
- Invoices or billing sent to clients; payment tracking for clients.
- Reminders not attached to an asset.
- WhatsApp, Slack or other channels beyond Telegram and email.
- CRM features, sales pipeline, client portal.
- Task management, Kanban, milestones, time tracking.
- Reminders after the expiration date (no H-0 or overdue repeats).
- Automatic renewal through provider APIs; server or uptime monitoring.
- Currency conversion between IDR and USD.
- In-app backup and restore.

## 3. Users and access

There is exactly one user account; there is no sign-up page and no role system.

- The account is created on first start from `OPS_ADMIN_EMAIL` and `OPS_ADMIN_PASSWORD`. If the account already exists, these variables are ignored.
- Passwords are hashed with Argon2id.
- Login uses a server-side session stored in PostgreSQL, sent as a cookie with `HttpOnly`, `Secure` and `SameSite=Lax`. Sessions expire after 30 days of inactivity.
- After 5 failed logins within 15 minutes from one IP, that IP is locked out for 15 minutes.
- Settings allows changing the email and password; changing the password ends all other sessions.
- Every page except `/login`, `/healthz` and `/robots.txt` requires a valid session.

## 4. Domain model

The Asset is the central entity: everything else (clients, providers, renewals, documents, credentials, reminders, projects) hangs off it.

**Entities**

| Entity | Purpose | Key relationships |
| --- | --- | --- |
| Asset | Anything with an expiration date | Belongs to one asset type, one provider, zero or one client |
| Asset type | Built-in or custom kind of asset, with default reminder offsets and custom field definitions | Has many assets and field definitions |
| Custom field definition | Extra field for one asset type | Belongs to one asset type |
| Client | Owner of client assets and projects | Has many assets and projects |
| Provider | Registrar, host or vendor | Has many assets |
| Renewal | One past renewal of an asset, with prices | Belongs to one asset; has many documents |
| Document | Uploaded invoice, receipt or image | Belongs to one asset or one renewal |
| Credential | Login for a panel or service; password encrypted | Belongs to one asset |
| Project | Website or app with a Markdown brief and screenshot | Belongs to zero or one client; linked to many assets |
| Reminder log | One sent (or failed) reminder | Belongs to one asset |
| Setting | Key-value app configuration | None |

**Asset core fields**

| Field | Type | Rules |
| --- | --- | --- |
| Name | text, required | e.g. example.com, Production VPS |
| Asset type | reference, required | Cannot change after creation if custom field values exist |
| Provider | reference, required | Created inline if missing |
| Owner | Internal or a client | `client_id` null means Internal |
| Order date | date, optional | Original purchase date |
| Expiration date | date, required | Drives status and reminders |
| Renewal period | integer months, required | 1, 3, 6, 12, 24, 36 or custom |
| Auto-renew | boolean | Informational only; reminders still send |
| Cost price | decimal(15,2), required | Price paid to the provider per renewal period |
| Selling price | decimal(15,2), optional | Price charged to the client; empty for Internal assets |
| Currency | IDR or USD | Per asset; summaries are grouped per currency |
| URL | text, optional | Panel or service URL |
| RDAP sync | boolean | Only on Domain-type assets; default on |
| Notes | Markdown | DNS records, configuration, anything else |
| Cancelled at | timestamp, optional | Set when the user cancels the asset |

Margin = selling price − cost price, shown only when a selling price exists.

**Status rules.** Status is computed at read time from the expiration date and stored only as `cancelled_at`, so it never goes stale.

| Status | Condition | Badge colour |
| --- | --- | --- |
| Cancelled | `cancelled_at` is set | Gray |
| Expired | Expiration date is before today | Red |
| Expiring Soon | Days left ≤ the largest reminder offset of its type | Amber |
| Active | Otherwise | Green |

Conditions are evaluated top to bottom; the first match wins. "Today" uses the timezone in Settings (default `Asia/Jakarta`).

**Built-in asset types.** Seeded on first start. Offsets and fields are editable; built-in types cannot be deleted.

| Type | Default reminder offsets (days before) | Default custom fields |
| --- | --- | --- |
| Domain | 30, 14, 7, 1 | Nameservers, registrar account |
| Hosting | 30, 14, 7, 1 | Plan name, control panel URL |
| VPS | 14, 7, 1 | IP address, region, specs |
| SSL Certificate | 14, 7, 1 | Issuer, covered hostnames |
| Email Hosting | 30, 14, 7, 1 | Plan name, mailbox count |
| Software License | 30, 14, 7, 1 | Seats, account email |

Custom field types: text, long text, number, date, URL, boolean, select (with options). Each field has a label, key, type, required flag and sort order. A custom type with assets cannot be deleted.

## 5. Functional requirements

Each requirement has an ID so code, tests and commits can reference it.

### 5.1 Dashboard

- **DSH-1** Upcoming renewals: non-cancelled assets expiring in the next 30 days, sorted by days left, showing name, type badge, owner, status badge and countdown badge. Expired assets appear at the top.
- **DSH-2** Assets per status: four counters (Active, Expiring Soon, Expired, Cancelled); each links to the filtered asset list.
- **DSH-3** Cost and margin: this month and this year, per currency: cost, revenue and margin (definitions in FIN-2).
- **DSH-4** Assets per type: count per asset type; each links to the filtered asset list.
- **DSH-5** Banner when the last reminder delivery or RDAP run failed, linking to its log.

### 5.2 Assets

- **AST-1** List view with filters (type, status, owner/client, provider), text search on name, and sort (expiration date ascending by default). Filters are reflected in the URL query string.
- **AST-2** Create and edit form: core fields plus the custom fields of the selected type, rendered dynamically with htmx when the type changes.
- **AST-3** Detail page with tabs: Overview, Renewals, Documents, Notes & Credentials, Reminders, Projects.
- **AST-4** Mark as renewed: dialog with new expiration date (default = current expiration + renewal period), cost price, selling price (both default to current), paid-on date (default today) and optional documents. Saving creates a Renewal row and updates the asset's expiration date and prices in one transaction.
- **AST-5** Cancel: sets `cancelled_at` after confirmation; reminders and RDAP sync stop. Restore clears it.
- **AST-6** Delete: hard delete after typing the asset name to confirm; removes renewals, documents (including files), credentials and logs.
- **AST-7** Duplicate: copies an asset into a new create form without renewals, documents or credentials.

### 5.3 Asset types and custom fields

- **TYP-1** CRUD for custom types in Settings; built-in types are editable but not deletable.
- **TYP-2** Each type has a name, a reminder offset list (positive integers, unique, at least one) and ordered custom field definitions.
- **TYP-3** Removing a field definition deletes its values after confirmation showing how many assets are affected.

### 5.4 Clients and providers

- **CLI-1** Client CRUD: name (required), contact person, email, phone, notes.
- **CLI-2** Client detail lists its assets, its projects, and per-currency totals of cost, selling price and margin per year.
- **CLI-3** A client with assets or projects cannot be deleted.
- **PRV-1** Provider CRUD: name (required), website, account login URL, notes. A provider with assets cannot be deleted.

### 5.5 Projects

- **PRJ-1** Project CRUD: name, owner (Internal or a client), website URL, brief, linked assets (many-to-many, pick from existing assets).
- **PRJ-2** The brief is Markdown, edited in a textarea with a live preview tab. New projects start from this template: `## Overview`, `## Tech Stack`, `## Features`.
- **PRJ-3** Markdown is rendered server-side with GitHub-flavoured Markdown and sanitized HTML.
- **PRJ-4** Screenshot is captured asynchronously when a project is created with a URL and when its URL changes on save. A **Refresh screenshot** button captures it again on demand.
- **PRJ-5** Capture settings: viewport 1440×900, above the fold only, WebP quality 80, timeout 30 seconds. The page shows a "Capturing" badge while pending and the capture time when done.
- **PRJ-6** On failure, the previous screenshot stays and an error badge shows the reason.

### 5.6 Finance

- **FIN-1** Renewal history per asset: renewed-on date, previous and new expiration dates, cost price, selling price, margin, attached documents.
- **FIN-2** Finance page for a chosen month or year, per currency. **Actual** = sum of renewals whose paid-on date falls in the period: cost, revenue (selling price), margin. **Projected** = non-cancelled assets whose expiration date falls in the period, at current prices.
- **FIN-3** 12-month projection table: one row per month with projected cost, revenue and margin, per currency.
- **FIN-4** Breakdowns of the selected period by client (Internal as its own row) and by asset type.
- **FIN-5** IDR and USD are never added together or converted.

### 5.7 Documents

- **DOC-1** Upload one or more files to an asset or a renewal. Accepted: PDF, JPEG, PNG, WebP; maximum 20 MB per file. Type is checked by content sniffing, not file extension.
- **DOC-2** JPEG and PNG are converted to WebP (quality 80, longest side capped at 2560 px, metadata stripped). WebP uploads are re-encoded with the same settings. PDFs are stored unchanged.
- **DOC-3** Files are stored on the uploads volume as `/data/uploads/{yyyy}/{mm}/{uuid}.{ext}` and served only through an authenticated handler.
- **DOC-4** Documents show filename, size, upload date and an inline preview for images; PDFs open in a new tab.

### 5.8 Notes and credentials

- **NOT-1** One Markdown notes field per asset, rendered like the project brief.
- **CRD-1** Credential CRUD per asset: label, URL, username, password, notes.
- **CRD-2** Passwords are encrypted at rest (see Security) and masked in the UI. **Reveal** fetches and shows the plaintext for 30 seconds; **Copy** copies it without displaying it.
- **CRD-3** Passwords never appear in list views, logs, notifications, error messages or HTML until revealed.

### 5.9 Reminders

- **REM-1** A reminder is due for an asset on the day `expiration_date − offset` for each offset of its type. All offsets are ≥ 1, so the last reminder is H-1 and nothing is sent on or after the expiration date.
- **REM-2** The dispatcher runs every 5 minutes. It sends every due reminder whose day is today and whose send time (Settings, default 09:00) has passed, for non-cancelled assets, on every enabled channel.
- **REM-3** Idempotency: `reminder_logs` has a unique key on (asset, expiration date, offset, channel). A reminder is sent at most once per key, even after restarts.
- **REM-4** Catch-up: if the app was down on a reminder day, only the most recent missed offset is sent on the next run, and only if the expiration date is still in the future.
- **REM-5** When the expiration date changes (renewal, edit or RDAP), the new date produces a new schedule automatically, because the date is part of the key.
- **REM-6** Failed sends retry up to 3 times with 1, 5 and 15 minute backoff; the final result is logged with the error.
- **REM-7** Channels: Telegram (bot token and chat ID) and email (SMTP host, port, username, password, from address, to address). Each channel can be enabled separately and has a **Send test** button in Settings.
- **REM-8** Message content: asset name, type, owner, provider, expiration date, days left, cost price, selling price, and a link to the asset. Example subject: `Renewal in 7 days: example.com (Domain)`.
- **REM-9** The asset's Reminders tab lists its upcoming schedule and its delivery log.

### 5.10 RDAP sync

- **RDP-1** Applies only to non-cancelled Domain-type assets with RDAP sync on.
- **RDP-2** A daily run at 03:00 (Settings timezone) checks each domain; the asset page has a **Check now** button.
- **RDP-3** The RDAP server is found through the IANA DNS bootstrap file, cached for 24 hours. Requests are limited to 1 per second per RDAP server, with a 15-second timeout.
- **RDP-4** The `expiration` event date replaces the asset's expiration date automatically whenever it differs. No confirmation, no separate field, no status hold.
- **RDP-5** The asset stores the last check time, the last result (`updated`, `unchanged`, `unsupported`, `error`) and the last error message, shown on the Overview tab.
- **RDP-6** TLDs without RDAP support, or responses without an expiration event, return `unsupported` and leave the date untouched.

### 5.11 Settings

- **SET-1** Account: email and password.
- **SET-2** General: timezone (default `Asia/Jakarta`), reminder send time (default 09:00), default currency (default IDR).
- **SET-3** Notifications: Telegram and email configuration with test buttons. Secrets are encrypted at rest like credentials.
- **SET-4** Asset types and custom fields (TYP-1 to TYP-3).
- **SET-5** Providers (PRV-1).

## 6. UI and UX

The interface is simple, clean and modern, and every status is readable at a glance through consistent badges. All UI copy is English.

**Layout**

- Left sidebar: Dashboard, Assets, Clients, Projects, Finance, Settings. It collapses into a drawer below 1024 px.
- Top bar: page title, global search (assets, clients, projects by name), theme toggle.
- Content: max width 1280 px; tables become stacked cards below 768 px.
- Server-rendered pages; htmx handles partial updates (filters, dialogs, form sections, screenshot status). No client-side SPA framework.

**Badges**

| Badge | Values | Style |
| --- | --- | --- |
| Status | Active, Expiring Soon, Expired, Cancelled | Green, amber, red, gray; filled pill, always with text |
| Countdown | `30 days left`, `1 day left`, `Expired 3 days ago` | Same colour as the asset's status |
| Asset type | Domain, Hosting, VPS, … | Neutral outline pill with an icon |
| Owner | `Internal` or the client name | Neutral filled pill; Internal uses a distinct muted tone |
| RDAP | Updated, Unchanged, Unsupported, Error | Small outline pill; Error uses red |
| Job | Capturing, Failed | Small outline pill on project screenshots |

Colour never carries meaning alone: every badge shows text, and colours meet WCAG AA contrast in both themes.

**Themes.** Light, dark and system. The toggle in the top bar cycles through them; the choice is stored in a cookie so the server renders the right theme with no flash on load.

**Interaction patterns**

- Destructive actions (delete, cancel, remove field) use a confirmation dialog.
- Success and error feedback appears as a toast that dismisses after 4 seconds.
- Every list has an empty state with a primary action, e.g. "No assets yet — Add asset".
- Forms validate on the server and show errors inline next to each field.
- Dates display as `12 Nov 2026`. IDR displays as `Rp 180.000`, USD as `$12.00`.
- Typography: system font stack; no external font or CDN requests.

## 7. Technical architecture

One statically linked Go binary serves the UI, the HTTP API for htmx and all background jobs. PostgreSQL holds the data, a headless Chromium container takes screenshots, and Caddy terminates TLS.

&#91;embedded content: System architecture · 4 containers, 3 external services\]

Only Caddy is reachable from the internet; PostgreSQL and Chromium sit on the internal network, and the app makes outbound calls to Telegram, SMTP and RDAP.

**Stack**

| Layer | Choice | Notes |
| --- | --- | --- |
| Language | Go | `CGO_ENABLED=0`; one static binary |
| HTTP routing | Go standard library `net/http` | Method and path patterns; no web framework |
| Templates | `html/template` + `embed` | Templates and static files embedded in the binary |
| Interactivity | htmx | Vendored file, embedded; no CDN |
| CSS | Tailwind CSS standalone CLI | Built in the Docker build stage; output embedded |
| Database | PostgreSQL | Only datastore |
| DB access | pgx + sqlc | Type-safe generated queries; no ORM |
| Migrations | goose | Embedded; run automatically on start |
| Markdown | goldmark (GFM) + bluemonday | Render, then sanitize |
| Screenshots | chromedp → headless Chromium container | CDP `Page.captureScreenshot` with format `webp` |
| WebP encoding | Pure-Go encoder (no CGO) | For uploaded JPEG/PNG/WebP |
| Email | SMTP client library | STARTTLS or implicit TLS |
| Telegram | Bot API over HTTPS | Plain `net/http`; no SDK |
| Password hashing | Argon2id (`golang.org/x/crypto`) |  |
| Encryption | AES-256-GCM (standard library) | See Security |
| Reverse proxy | Caddy | Automatic HTTPS |

Versions follow the rule in Engineering rules: latest stable release at implementation time.

**Background jobs.** Jobs run as goroutines inside the same process; there is no Redis or external queue.

| Job | Trigger | Behaviour |
| --- | --- | --- |
| Reminder dispatcher | Every 5 minutes | REM-1 to REM-6 |
| RDAP sync | Daily 03:00 and on demand | RDP-1 to RDP-6 |
| Screenshot worker | Polls `project_screenshots` with status `pending` every 10 seconds | PRJ-4 to PRJ-6 |
| Session cleanup | Daily | Deletes expired sessions and stale login-attempt rows |

State lives in PostgreSQL, so a restart loses no work. A PostgreSQL advisory lock per job guarantees one instance of each job runs at a time.

**Storage.** Uploaded files and screenshots live on the `uploads` Docker volume mounted at `/data/uploads`. The database stores only paths and metadata.

**Configuration.** All configuration comes from environment variables (listed under Deployment). Runtime preferences such as timezone and notification channels live in the `settings` table.

## 8. Security and privacy

Stored passwords and notification secrets are the most sensitive data, so they are encrypted with a key that never touches the database.

**Encryption at rest**

- AES-256-GCM with a random 12-byte nonce per value. Stored as `ciphertext`, `nonce` and `key_version` columns.
- The 32-byte key comes from `OPS_ENCRYPTION_KEY` (base64). The app refuses to start if it is missing or the wrong length.
- Encrypted fields: credential passwords, Telegram bot token, SMTP password.
- Key rotation: `opsmanager rotate-key` re-encrypts all values with `OPS_ENCRYPTION_KEY_NEW` and bumps `key_version`.
- Losing the key makes encrypted values unrecoverable; the README states this and tells the user to store the key outside the VPS.

**Web security**

- HTTPS only through Caddy; HSTS enabled.
- CSRF token on every state-changing request, including htmx requests (sent as a header).
- Content Security Policy: `default-src 'self'`; images also allow `data:`; no inline scripts.
- Headers: `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`.
- All SQL goes through sqlc parameterised queries; all HTML output is escaped by `html/template`; rendered Markdown is sanitized.
- Uploads: content sniffing, size limit, random filenames, served with `Content-Disposition` and the sniffed content type.
- The screenshot service only accepts `http` and `https` URLs, rejects private, loopback and link-local addresses, and runs on an internal Docker network.

**No search-engine indexing.** Three layers, all required:

- `<meta name="robots" content="noindex, nofollow">` on every page.
- `X-Robots-Tag: noindex, nofollow` header on every response.
- `/robots.txt` returns `User-agent: *` and `Disallow: /`.

**Logging.** Structured JSON logs to stdout. Logs never contain passwords, tokens, session IDs or decrypted values.

## 9. Database schema

The initial goose migration creates the schema below. Custom field values live in `assets.custom_fields` (JSONB), validated in Go against `asset_type_fields`; this avoids a table per asset type. Money uses `numeric(15,2)`, calendar dates use `date`, moments use `timestamptz`.

```sql
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
```

Seed data (built-in asset types and their fields, default settings) is inserted by the same migration with `ON CONFLICT DO NOTHING`.

## 10. Engineering rules

These rules are mandatory for every contributor, including AI coding agents. A change that breaks one is not done.

**Versions**

- Use the latest **stable** release of every dependency at implementation time: Go, PostgreSQL, Debian base images, Caddy, Chromium image, htmx, Tailwind CLI, every Go module and every build tool.
- Beta, RC, alpha, preview, nightly and pre-release versions are forbidden.
- Check the current stable version on the official release page before adding or upgrading a dependency.
- Pin exact versions everywhere: `go.mod` / `go.sum`, Docker image tags, vendored htmx file name, Tailwind CLI download. No `latest` tags, no version ranges.

**Language**

- Everything is English: UI copy, identifiers, error messages, log messages, README, migration names and commit messages.

**No comments in code**

- No comments of any kind in any source file: Go, SQL, HTML templates, CSS, JavaScript, Dockerfile, Compose, Caddyfile, Makefile, YAML. This includes `TODO`, `FIXME`, doc comments and commented-out code.
- Code explains itself through clear names, small functions and tests.
- The only exceptions are tool directives the build cannot work without: `//go:build`, `//go:embed`, `//go:generate`, sqlc query annotations (`-- name: … :one`) and goose markers (`-- +goose Up`, `-- +goose Down`).

**Git and commits**

- Commit directly to `main`. No feature branches, no pull requests.
- Commit messages are English, imperative mood, subject line ≤ 72 characters, e.g. `Add renewal dialog to asset detail page`.
- No AI annotations anywhere in commits: no `Co-Authored-By` trailers, no "Generated with …" lines, no AI tool names, no emoji.
- Each commit builds and passes tests.

**Quality gates (run before every commit)**

- `gofmt` produces no changes; `go vet ./...` passes.
- `sqlc generate` produces no changes.
- `go test ./...` passes. Required unit tests: status computation, reminder scheduling and catch-up, idempotency key, RDAP response parsing, AES-GCM encryption round trip, finance aggregation, WebP conversion.

**Project structure**

```text
opsmanager/
  cmd/opsmanager/main.go
  internal/
    config/
    db/
      migrations/
      queries/
      sqlc/
    auth/
    crypto/
    asset/
    assettype/
    client/
    provider/
    project/
    finance/
    document/
    credential/
    reminder/
    notify/
      telegram/
      email/
    rdap/
    screenshot/
    imaging/
    jobs/
    web/
      handlers/
      middleware/
      templates/
      static/
  deploy/
    Dockerfile
    compose.yaml
    Caddyfile
  sqlc.yaml
  go.mod
  README.md
```

Packages are organised by feature; `web/handlers` stays thin and calls feature packages. No global mutable state; dependencies are passed explicitly.

## 11. Deployment

The app runs on a Debian 13 VPS as four Docker Compose services; only Caddy publishes ports.

**Host**

- Debian 13 with Docker Engine and the Docker Compose plugin installed from Docker's official apt repository.
- Firewall allows only 22, 80 and 443.
- A DNS A record points the chosen hostname to the VPS.

**Services (`deploy/compose.yaml`)**

| Service | Image | Ports | Volumes | Networks |
| --- | --- | --- | --- | --- |
| `caddy` | Official Caddy | 80, 443 (published) | `caddy_data`, `caddy_config` | `web` |
| `app` | Built from `deploy/Dockerfile` | 8080 (internal) | `uploads` → `/data/uploads` | `web`, `internal` |
| `db` | Official PostgreSQL | 5432 (internal) | `pg_data` | `internal` |
| `chromium` | Headless Chromium for chromedp | 9222 (internal) | none | `internal` |

- `app` image: multi-stage build. Stage 1 builds CSS with the Tailwind CLI; stage 2 builds the Go binary with `CGO_ENABLED=0`; the final stage is a minimal Debian-based or distroless image running as a non-root user.
- Every service has `restart: unless-stopped` and a health check; `app` exposes `GET /healthz`.
- Migrations run automatically when `app` starts.

**Environment variables (`.env`, never committed)**

| Variable | Required | Purpose |
| --- | --- | --- |
| `OPS_BASE_URL` | Yes | Public URL used in reminder links, e.g. `https://ops.example.com` |
| `OPS_DATABASE_URL` | Yes | PostgreSQL connection string |
| `OPS_ENCRYPTION_KEY` | Yes | Base64-encoded 32-byte key |
| `OPS_ADMIN_EMAIL` | First start | Initial account email |
| `OPS_ADMIN_PASSWORD` | First start | Initial account password |
| `OPS_CHROMIUM_URL` | Yes | DevTools endpoint, e.g. `ws://chromium:9222` |
| `OPS_UPLOADS_DIR` | No | Default `/data/uploads` |
| `OPS_DOMAIN` | Yes | Hostname for Caddy's certificate |
| `POSTGRES_PASSWORD` | Yes | Database password |

**Backups.** Backups are outside the app. The README documents a nightly host cron job that runs `pg_dump` and archives the `uploads` volume, and reminds the user to keep `OPS_ENCRYPTION_KEY` in a separate safe place.

## 12. Milestones and acceptance criteria

Build in seven milestones, in order; each ends in a deployable state on `main`. A milestone is done when every box under it can be ticked.

### M1 — Foundation

- [ ] `docker compose up` on a fresh Debian 13 VPS serves the login page over HTTPS.
- [ ] Migrations and seed data run on start; restarting is safe.
- [ ] Login, logout, lockout after 5 failures and password change work.
- [ ] Layout, sidebar, light/dark/system toggle with no flash on load.
- [ ] `robots.txt`, `X-Robots-Tag` and meta robots present on every page.
- [ ] App refuses to start without a valid `OPS_ENCRYPTION_KEY`.

### M2 — Assets

- [ ] Providers, clients and asset types (built-in and custom, with custom fields) can be managed.
- [ ] Assets can be created, edited, duplicated, cancelled, restored and deleted.
- [ ] Status badge follows the rules in section 4 for every case, covered by tests.
- [ ] List filters, search and sort work and survive a page reload.
- [ ] Notes render as sanitized Markdown; credentials are stored encrypted and reveal or copy on demand.

### M3 — Reminders

- [ ] Telegram and email can be configured and tested from Settings.
- [ ] Each offset sends exactly once per channel at the send time; restarting does not resend.
- [ ] Catch-up sends only the latest missed offset.
- [ ] Nothing is sent on or after the expiration date, or for cancelled assets.
- [ ] Failed sends retry 3 times and appear in the log and the dashboard banner.

### M4 — Renewals, finance and documents

- [ ] Mark as renewed creates a renewal row and moves the expiration date and schedule.
- [ ] Finance page shows actual and projected cost, revenue and margin per currency, plus the 12-month projection and breakdowns.
- [ ] JPEG and PNG uploads are stored as WebP; PDFs stay unchanged; files are only reachable when logged in.

### M5 — RDAP sync

- [ ] Daily run and Check now update the expiration date of supported domains.
- [ ] Unsupported TLDs and errors are shown on the asset without changing the date.
- [ ] Rate limit and timeout hold during a run of 100 domains.

### M6 — Projects

- [ ] Projects can be created with a Markdown brief from the template, an owner and linked assets.
- [ ] Saving with a new URL captures a WebP screenshot; Refresh screenshot captures again.
- [ ] Private and loopback URLs are rejected; a failed capture keeps the previous image.

### M7 — Dashboard and polish

- [ ] Dashboard shows all four widgets (DSH-1 to DSH-4) and the failure banner.
- [ ] Dashboard renders in under 1 second with 500 seeded assets.
- [ ] Every list has an empty state; every page works at 375 px width.
- [ ] README covers setup, environment variables, backups and key safety.

## 13. Decisions and accepted risks

These decisions were made deliberately during scoping; do not reverse them without the owner's approval.

| Decision | Choice | Consequence |
| --- | --- | --- |
| RDAP overwrite | Overwrite the expiration date automatically, no safeguard | **Accepted risk:** registries often auto-extend gTLD expiration dates by one year before the registrar is paid. The asset can then look Active and reminders move a year ahead while the domain is actually unpaid. |
| Reminder end | Last reminder at H-1; none after expiry | A missed H-1 message has no follow-up; the Expired badge on the dashboard is the only signal. |
| Users | Single user | No roles, permissions or audit trail by user. |
| Credentials | Passwords stored encrypted in the app | Losing `OPS_ENCRYPTION_KEY` loses every stored password. |
| Currencies | IDR and USD per asset, never converted | Totals are shown per currency, side by side. |
| Custom field storage | JSONB on `assets`, validated in Go | Simple schema; reporting on custom fields needs JSONB queries. |
| Background jobs | In-process goroutines, state in PostgreSQL | No Redis; one app instance only. |
| Screenshots | Separate headless Chromium container | App stays a single static binary; one extra container to run. |
| Comments rule | No comments in any code | Tool directives (`go:embed`, sqlc and goose annotations) are the only exception, because the build needs them. |
| Defaults | Timezone `Asia/Jakarta`, send time 09:00, currency IDR | All editable in Settings. |
