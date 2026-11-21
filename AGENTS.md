# AGENTS.md — Ops Manager

Rules for every contributor working in this repository, human or AI. Read this file in full before writing code. A change that breaks any rule marked **MUST** is not done.

The product specification is [`docs/PRD.md`](docs/PRD.md). It is the source of truth for behaviour. This file is the source of truth for how the code is written, committed and verified. When the two disagree, the **Clarified decisions** section below wins, because it records the owner's answers to gaps in the PRD.

---

## 1. Non-negotiable rules

Check these before every commit.

### 1.1 No comments in code

- **MUST NOT** write comments of any kind in any source file: Go, SQL, HTML templates, CSS, JavaScript, Dockerfile, Compose, Caddyfile, Makefile, YAML, TOML, shell.
- This includes `TODO`, `FIXME`, doc comments, license headers, section dividers and commented-out code.
- Allowed exceptions, and only these, because the build needs them:
  - `//go:build`, `//go:embed`, `//go:generate`
  - sqlc query annotations: `-- name: GetAsset :one`
  - goose markers: `-- +goose Up`, `-- +goose Down`, `-- +goose StatementBegin`, `-- +goose StatementEnd`
- Markdown documentation (`README.md`, `AGENTS.md`, `CLAUDE.md`, `docs/`) is prose, not code, so the rule does not apply to it.
- Make code explain itself through clear names, small functions and tests.

### 1.2 Versions

- **MUST** use the latest **stable** release of every dependency at implementation time: Go toolchain, PostgreSQL, Debian base image, Caddy, Chromium image, htmx, Tailwind CLI, sqlc, goose, every Go module.
- **MUST NOT** use beta, RC, alpha, preview, nightly or pre-release versions.
- **MUST** check the official release page before adding or upgrading a dependency.
- **MUST** pin exact versions: `go.mod` / `go.sum` (including a `toolchain` line), full Docker image tags, the vendored htmx file name (e.g. `htmx-2.x.y.min.js`), the Tailwind CLI download URL, and tool versions in the Dockerfile. No `latest`, no ranges, no floating major tags.
- The local machine may have an older Go. Rely on the `toolchain` directive and `GOTOOLCHAIN=auto` rather than lowering the version.

### 1.3 Language

- **MUST** write everything in English: UI copy, identifiers, error messages, log messages, README, migration names, commit messages and docs.

### 1.4 Git and commits

- **MUST** commit directly to `main` and push to `origin main`. No feature branches, no pull requests.
- Commit messages: English, imperative mood, subject line ≤ 72 characters, e.g. `Add renewal dialog to asset detail page`. Add a body only when it adds information.
- **MUST NOT** add AI annotations of any kind: no `Co-Authored-By` trailers, no `Claude-Session` or session links, no "Generated with …" lines, no AI tool or model names, no emoji.
- **MUST** make each commit build and pass every quality gate. Make small, focused commits per logical change, and reference requirement IDs in the body when useful (e.g. `Implements AST-4`).
- **MUST NOT** commit `.env`, keys, uploads, database dumps, built binaries or `node_modules`.

### 1.5 Quality gates (run before every commit)

```sh
gofmt -l .                 # must print nothing
go vet ./...
sqlc generate && git diff --exit-code internal/db/sqlc
go test ./...
```

Also run `go build ./...`, and run `go mod tidy` with no resulting diff. `make check` runs the core gates. Set `OPS_TEST_DATABASE_URL` to also run the PostgreSQL integration tests.

### 1.6 Scope

- Build only what PRD section 2 lists as **In scope**. Everything under **Out of scope** is forbidden, even as a "small extra".
- No feature, setting, page or dependency without a requirement ID behind it, unless it is plumbing that a requirement needs.

---

## 2. Clarified decisions

The owner's answers to questions the PRD left open. They are as binding as the PRD itself.

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Repository layout | The repo root is the PRD's `opsmanager/` directory. `go.mod` is at the root. |
| D2 | Go module path | `github.com/suwahadi/project-ops-manager`. The binary and command are still named `opsmanager` (`cmd/opsmanager`). |
| D3 | Branch | Commit and push straight to `main` (PRD §10). |
| D4 | Delivery | Build milestones M1 to M7 in order, without stopping between them unless blocked. Each milestone ends deployable on `main`. |
| D5 | Recurring projection | FIN-3, FIN-2 "Projected" and DSH-3 "Projected" all extrapolate repeated renewals: an asset contributes at `expiration_date + n × renewal_period_months` for every n ≥ 0 that falls in the period, at current prices. One shared function computes this. |
| D6 | DSH-3 content | Shows both **Actual** and **Projected** (FIN-2 definitions) for this month and this year, per currency. |
| D7 | DSH-1 expired rows | Shows **all** non-cancelled expired assets at the top, however long ago they expired, followed by assets expiring in the next 30 days. |
| D8 | WebP encoder | A pure-Go **lossless** WebP encoder (e.g. `github.com/HugoSmits86/nativewebp`). The PRD's "quality 80" does not apply to uploads. Resizing to a 2560 px longest side and stripping metadata still apply. |
| D9 | WebP size | Always store JPEG, PNG and WebP uploads as WebP, even when the result is larger than the original (DOC-2 as written). |
| D10 | Screenshots | Screenshots keep CDP `Page.captureScreenshot` with format `webp`, quality 80 (PRJ-5). That is Chromium's encoder, not ours, so D8 does not affect it. |
| D11 | Integration tests | Tests that need PostgreSQL run only when `OPS_TEST_DATABASE_URL` is set and call `t.Skip` otherwise, so `go test ./...` passes anywhere. `internal/db/dbtest` creates a throwaway database per test from that URL. |
| D12 | Generated code | Files produced by `sqlc generate` keep the generator's `Code generated ... DO NOT EDIT.` header. Go tooling uses it to recognise generated files, so it counts as a tool directive. Never edit these files by hand. |
| D13 | Commit author | Commits are authored as `suwahadi <suwahadi@gmail.com>` (repository-local git config). |
| D14 | Extra packages | `internal/money` (decimal amounts as integer cents, pgx numeric scanning, IDR/USD formatting), `internal/settings` (settings table, encrypted secrets, key check and key rotation) and `internal/db/dbtest` (integration-test databases) were added to the PRD layout. Templates and static files embed themselves from `internal/web/templates` and `internal/web/static` to avoid import cycles. |
| D15 | Key check | On start the app decrypts a stored key-check secret. A wrong `OPS_ENCRYPTION_KEY` makes it refuse to start instead of failing later. `opsmanager rotate-key` re-encrypts every secret and credential password with `OPS_ENCRYPTION_KEY_NEW` under `key_version + 1`. |
| D17 | Client totals (CLI-2) | "Per year" means annualised recurring totals of non-cancelled assets: each price × 12 / renewal period months, per currency. |
| D18 | Internal selling price | An Internal asset with a selling price fails validation with an inline error. It is never silently cleared. |
| D19 | Markdown package | `internal/markdown` wraps goldmark (GFM) and bluemonday. Notes and project briefs share it. |
| D20 | Custom field keys | A field key is set on creation (derived from the label if empty) and cannot change afterwards. Label, type, options, required flag and order stay editable. |
| D21 | Reminder window (REM-2, REM-4) | Each offset has a send moment: its due day at the configured send time in the Settings timezone. A run picks the most recent offset whose moment has passed and that is either due today or fell after the previous successful run (`reminder_last_run` setting). This sends today's reminder once, sends only the latest missed offset after downtime, and does not backfill offsets that passed before an asset was created. |
| D22 | Reminder delivery | The dispatcher runs every 5 minutes. A separate retry job runs every minute, so the 1/5/15-minute backoff (REM-6) is honoured; both share one advisory lock. A pending log is dropped, not sent, if its asset is cancelled, its expiration date changed, the date passed or the channel was disabled. A log reaches `failed` after the initial attempt plus 3 retries. |
| D23 | SMTP security | Port 465 uses implicit TLS. Every other port requires STARTTLS (`TLSMandatory`); plaintext SMTP is not supported. |
| D24 | Dashboard failure banner (DSH-5) | The banner shows when the most recently finished reminder log is `failed`, linking to that asset's Reminders tab. Migration 00002 adds `reminder_logs.updated_at` and a `job_runs` table for job history. |
| D25 | Aggregate margin | In every finance total (FIN-2/3/4, DSH-3), margin is the sum of `selling − cost` over items that have a selling price. Cost still includes Internal items, so a total's margin is not simply revenue − cost. |
| D26 | Projection start | Projected occurrences start at an asset's current expiration date (n = 0), even if that date has already passed. An expired but uncancelled asset therefore still counts in the period that contains its expiration date. |
| D27 | Uploads | Converted images keep their uploaded base name with a `.webp` extension. JPEG EXIF orientation is applied before metadata is stripped. At most 10 files per upload. Images over 60 megapixels are rejected. |
| D28 | Renewals | The new expiration date must be after the current one. The renewal keeps the asset's currency. Documents uploaded in the renewal dialog belong to the renewal and are listed on the Renewals tab. |
| D29 | RDAP details | The asset name is normalised (lowercase; scheme, path and `www.` stripped) and must be an ASCII domain. Internationalised names give an `error` result. The `expiration` event date is taken as its UTC calendar date. A 404 is an `error` ("domain not found at the registry"). The bootstrap cache and per-server rate limiter live in memory in one shared client used by the daily job and Check now. |
| D30 | RDAP runs and banner | Each daily run is recorded in `job_runs` (job `rdap`). A run is `failed` if it aborts or any domain returns `error`. The dashboard banner shows when the latest finished run failed and links to `/rdap` (recent runs plus per-domain results). Job runs older than 90 days are removed by the daily cleanup. |
| D16 | Chromium network | The Compose network named `internal` publishes no ports but is not a Docker `internal: true` network, because Chromium must reach public websites to take screenshots. |

When a new ambiguity appears, record the decision here in the same commit that implements it.

---

## 3. Architecture

- One statically linked Go binary (`CGO_ENABLED=0`) serves the UI, the htmx endpoints and all background jobs.
- PostgreSQL is the only datastore. There is no Redis, no external queue and no in-memory state that must survive a restart.
- Headless Chromium runs as a separate container, reached over CDP at `OPS_CHROMIUM_URL`.
- Caddy terminates TLS and is the only service with published ports.
- Background jobs (reminder dispatcher, RDAP sync, screenshot worker, session cleanup) are goroutines. Each job takes its own PostgreSQL advisory lock per run.

### Project structure

```text
cmd/opsmanager/main.go
internal/
  config/        env parsing and validation
  db/
    migrations/  goose SQL files, embedded
    queries/     sqlc query files
    sqlc/        generated code; never edited by hand
  auth/  crypto/  asset/  assettype/  client/  provider/  project/
  finance/  document/  credential/  reminder/
  notify/telegram/  notify/email/
  rdap/  screenshot/  imaging/  jobs/
  web/
    handlers/  middleware/  templates/  static/
deploy/Dockerfile  deploy/compose.yaml  deploy/Caddyfile
sqlc.yaml  go.mod  README.md
docs/PRD.md
```

- Organise packages by feature. `web/handlers` stays thin: it parses requests, calls feature packages and renders templates. Business rules live in feature packages.
- **MUST NOT** use global mutable state. Pass dependencies explicitly through constructors.
- **MUST NOT** add a package outside this layout without a clear reason. Recording the reason in section 2 is required.

---

## 4. Coding conventions

### Go

- Use the standard library first: `net/http` method and path patterns, `html/template`, `embed`, `log/slog`, `crypto/aes` + `crypto/cipher`.
- Approved third-party modules: pgx, goose, goldmark, bluemonday, chromedp, `golang.org/x/crypto` (Argon2id), an SMTP client library, a pure-Go WebP encoder (D8) and image resizing from `golang.org/x/image`. Adding anything else needs a requirement that cannot reasonably be met without it.
- Pass `context.Context` as the first parameter of every function that does I/O.
- Wrap errors with `fmt.Errorf("verb noun: %w", err)`. Never panic in request handling.
- Model money as decimal values, never as `float64`. Use `pgtype.Numeric` or a decimal type at the edges and fixed-point integer cents internally.
- Dates without a time are calendar dates. Compute "today" in the Settings timezone, never in the server's local time or UTC.
- Compute status at read time from `cancelled_at`, `expiration_date` and the type's offsets (PRD §4). Never store it.
- Keep functions small and names descriptive enough that no comment is needed.

### SQL and database

- All SQL lives in `internal/db/queries/*.sql` and goes through sqlc-generated, parameterised functions. No string-built SQL.
- Schema changes are new goose migrations under `internal/db/migrations/`. **MUST NOT** edit a migration that has already been committed. Name migrations in English (`00002_add_x.sql`).
- Seed data is idempotent (`ON CONFLICT DO NOTHING`).
- Multi-step writes (renewals AST-4, deletes AST-6, key rotation) run in one transaction.
- Reminder idempotency relies on the unique key `(asset_id, expiration_date, offset_days, channel)`. Insert first and send second, so the database, not memory, prevents duplicates.

### Templates, htmx and CSS

- Server-rendered `html/template` pages. htmx handles partial updates. No SPA framework and no other JavaScript framework.
- htmx is vendored under `web/static/` with its exact version in the file name and embedded. Load nothing from a CDN or external font.
- No inline `<script>` and no inline event handlers (`onclick=` etc.). This keeps the CSP of `default-src 'self'` working. Any small custom JS goes in an embedded static file.
- Tailwind CSS is built with the standalone CLI during the Docker build, and the output is embedded.
- Every page includes `<meta name="robots" content="noindex, nofollow">`.
- UI follows PRD §6: badges always carry text, colours meet WCAG AA in both themes, layouts work at 375 px, tables become cards below 768 px, and every list has an empty state with a primary action.
- Formats: dates as `12 Nov 2026`, IDR as `Rp 180.000`, USD as `$12.00`. Never add IDR and USD together or convert between them.

### Security

- Passwords are hashed with Argon2id. Credential passwords, the Telegram bot token and the SMTP password are encrypted with AES-256-GCM, using a random 12-byte nonce and storing `key_version`.
- **MUST NOT** let secrets, passwords, tokens, session IDs or decrypted values appear in logs, error messages, notifications, list views or HTML before a Reveal.
- Every state-changing request needs a CSRF token. htmx sends it as a header.
- Set these headers on every response: CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`, `X-Robots-Tag: noindex, nofollow`.
- Uploads: detect the type by content sniffing, limit size to 20 MB, use random UUID filenames, and serve files only through the authenticated handler with `Content-Disposition`.
- Screenshot URLs: allow only `http` and `https`. Reject private, loopback and link-local addresses after DNS resolution.
- Render Markdown with goldmark (GFM) and always sanitise it with bluemonday.

### Logging

- Write structured JSON logs to stdout with `log/slog`. Messages are short English phrases with key-value attributes.

---

## 5. Testing

- Required unit tests (PRD §10): status computation, reminder scheduling and catch-up, the idempotency key, RDAP response parsing, the AES-GCM round trip, finance aggregation (including the D5 extrapolation), and WebP conversion.
- Prefer table-driven tests. Inject a clock (`func() time.Time`) and timezone wherever "today" matters; never call `time.Now()` deep in business logic.
- Integration tests that touch PostgreSQL follow D11.
- Network-facing code (Telegram, SMTP, RDAP, Chromium) is tested with `httptest` servers or interfaces, never against real external services.

---

## 6. Definition of done

A change is done only when all of these hold:

1. It does what its requirement ID says, and nothing out of scope.
2. No comments were added (section 1.1). Check with `grep` before committing.
3. All quality gates in section 1.5 pass.
4. New logic has tests, and required tests exist.
5. Any new dependency is the latest stable version and is pinned exactly.
6. Any decision the PRD did not cover is recorded in section 2.
7. The commit message follows section 1.4, and the change is pushed to `origin main`.

A milestone (PRD §12) is done when every one of its acceptance boxes can be ticked.
