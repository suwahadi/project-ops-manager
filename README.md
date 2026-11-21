# Ops Manager

Ops Manager is a self-hosted web app for a single user. It tracks every web-development asset that has an expiration date (domains, hosting, VPS, SSL certificates, email hosting, software licenses and your own custom types) and reminds you on Telegram and by email before each one expires.

- **Assets:** expiration status, prices and margin, custom fields, renewal history, invoices and receipts, encrypted credentials and Markdown notes.
- **Reminders:** sent at H-30, H-14, H-7 and H-1 (configurable per type), exactly once per channel.
- **RDAP sync:** domain expiration dates are updated automatically every night.
- **Finance:** actual and projected cost, revenue and margin, per currency (IDR and USD are never mixed).
- **Projects:** a Markdown brief, linked assets and an automatic website screenshot.

The product specification is in [`docs/PRD.md`](docs/PRD.md). Contributor rules are in [`AGENTS.md`](AGENTS.md).

## Architecture

| Service | Image | Purpose |
| --- | --- | --- |
| `caddy` | `caddy:2.11.9-alpine` | HTTPS with automatic certificates. The only service with published ports (80, 443). |
| `app` | built from `deploy/Dockerfile` | One static Go binary: the UI, the htmx endpoints and all background jobs. |
| `db` | `postgres:19.0-trixie` | The only datastore. |
| `chromium` | `chromedp/headless-shell:159.0.8154.13` | Headless browser for project screenshots. |

Uploaded documents and screenshots are stored on the `uploads` volume at `/data/uploads`.

## Setup on a Debian 13 VPS

1. **Install Docker.** Install Docker Engine and the Compose plugin from Docker's official apt repository (see docs.docker.com, "Install Docker Engine on Debian").
2. **Open the firewall** for SSH, HTTP and HTTPS only:

   ```sh
   apt install ufw
   ufw allow 22/tcp && ufw allow 80/tcp && ufw allow 443/tcp
   ufw enable
   ```

3. **Point DNS** at the server: create an A record for your hostname, for example `ops.example.com`.
4. **Clone the repository and create the environment file:**

   ```sh
   git clone https://github.com/suwahadi/project-ops-manager.git
   cd project-ops-manager
   cp deploy/.env.example deploy/.env
   chmod 600 deploy/.env
   ```

5. **Generate the secrets** and put them into `deploy/.env`:

   ```sh
   openssl rand -base64 32
   openssl rand -base64 24
   ```

   Use the first value for `OPS_ENCRYPTION_KEY` and the second for `POSTGRES_PASSWORD`. Choose an admin password of at least 10 characters.

6. **Start the stack:**

   ```sh
   docker compose -f deploy/compose.yaml up -d --build
   ```

7. **Sign in** at `https://<your hostname>` with `OPS_ADMIN_EMAIL` and `OPS_ADMIN_PASSWORD`.
8. **Configure the app** under Settings:
   - **General:** timezone and reminder send time.
   - **Notifications:** Telegram and email, then press **Send test** for each channel.
   - **Asset types and Providers:** adjust as needed.

Migrations and seed data run automatically every time `app` starts, so restarting is always safe. Check that every service is healthy with `docker compose -f deploy/compose.yaml ps`.

### Updating

```sh
git pull
docker compose -f deploy/compose.yaml up -d --build
```

## Notification setup

Configure both channels under **Settings → Notifications**. **Send test** uses the values currently in the form, so you can test before saving; press **Save** to keep them. A secret field left empty keeps the stored secret.

### Email (SMTP relay)

| Field | Value |
| --- | --- |
| SMTP host | The relay host from your email provider's SMTP settings. |
| Port | `587` (STARTTLS, recommended) or `465` (implicit TLS). Plaintext SMTP is refused; port `2525` works if it offers STARTTLS. |
| Username | The **SMTP login** from the provider's SMTP settings. It is often not your account email. |
| Password | The **SMTP key** generated in the provider's dashboard, not your account password. Stored encrypted. |
| From name | Optional display name, e.g. `Ops Manager`. |
| From address | Only the email address, e.g. `ops@example.com`. It must be a sender or domain verified with the provider, or the relay rejects the message. |
| To address | The mailbox that receives reminders. |

### Telegram bot

1. Chat with **@BotFather**, send `/newbot` and copy the bot token (`123456789:ABC…`).
2. Find the chat ID:
   - **Private chat:** open your bot and press **Start**; a bot cannot message you until you do. Then open `https://api.telegram.org/bot<TOKEN>/getUpdates` and copy `message.chat.id` (a positive number).
   - **Group:** add the bot to the group, send any message there, then read `chat.id` from `getUpdates` (a negative number, usually starting with `-100`).
   - **Channel:** make the bot an administrator, then use `@channelname` or the `-100…` ID.
3. **Topic ID** (optional): for a group with Topics enabled, open a message in the topic, choose **Copy link**, and use the last number of `https://t.me/c/<group>/<topic>/<message>`. Leave it empty to post in the General topic.
4. Enter the bot token and chat ID, press **Send test**, then tick **Send reminders via Telegram** and **Save**.

Common errors shown by **Send test**:
- `401 Unauthorized`: wrong token.
- `400 chat not found`: wrong chat ID, or the bot is not in the group.
- `403 Forbidden`: you never pressed Start, or the bot was blocked or removed.

## Environment variables

Put these in `deploy/.env`. Never commit that file.

| Variable | Required | Purpose |
| --- | --- | --- |
| `OPS_DOMAIN` | Yes | Hostname that Caddy requests a certificate for, e.g. `ops.example.com`. |
| `OPS_BASE_URL` | Yes | Public URL used in reminder links, e.g. `https://ops.example.com`. |
| `OPS_ENCRYPTION_KEY` | Yes | Base64-encoded 32-byte key for credentials and notification secrets. The app refuses to start if it is missing, the wrong length, or does not match the stored data. |
| `OPS_ADMIN_EMAIL` | First start | Email of the single account. Ignored once the account exists. |
| `OPS_ADMIN_PASSWORD` | First start | Initial password. Ignored once the account exists; change it in Settings. |
| `POSTGRES_PASSWORD` | Yes | Database password. Compose builds `OPS_DATABASE_URL` from it. |
| `OPS_DATABASE_URL` | Set by Compose | PostgreSQL connection string. Set it yourself only when running the binary directly. |
| `OPS_CHROMIUM_URL` | Set by Compose | DevTools endpoint. Defaults to `ws://chromium:9222`. |
| `OPS_UPLOADS_DIR` | No | Defaults to `/data/uploads`. |
| `OPS_LISTEN_ADDR` | No | Defaults to `:8080`. |
| `OPS_ENCRYPTION_KEY_NEW` | Key rotation only | New key for `opsmanager rotate-key`. |

## Keep the encryption key safe

`OPS_ENCRYPTION_KEY` encrypts every stored credential password, the Telegram bot token and the SMTP password. The key is never stored in the database.

- **Store a copy outside the VPS,** for example in your password manager. If the key is lost, every encrypted value is lost with it. There is no recovery.
- **Do not change the key in `.env` by hand.** The app will refuse to start because the key no longer matches the stored data.

To rotate the key:

```sh
openssl rand -base64 32
docker compose -f deploy/compose.yaml stop app
docker compose -f deploy/compose.yaml run --rm -e OPS_ENCRYPTION_KEY_NEW='<new key>' app rotate-key
```

Then set `OPS_ENCRYPTION_KEY` in `deploy/.env` to the new key and start `app` again. Save the new key outside the VPS.

## Backups

Backups happen outside the app. The cron job below dumps the database and archives the uploads volume every night and keeps 14 days:

```sh
mkdir -p /var/backups/opsmanager
cat > /etc/cron.d/opsmanager-backup <<'CRON'
30 2 * * * root cd /root/project-ops-manager && docker compose -f deploy/compose.yaml exec -T db pg_dump -U ops -Fc ops > /var/backups/opsmanager/db-$(date +\%F).dump && docker run --rm -v opsmanager_uploads:/data:ro -v /var/backups/opsmanager:/backup debian:trixie-20261104-slim tar czf /backup/uploads-$(date +\%F).tar.gz -C /data . && find /var/backups/opsmanager -type f -mtime +14 -delete
CRON
```

Adjust the repository path if you cloned it elsewhere. Copy `/var/backups/opsmanager` off the server regularly, and keep `OPS_ENCRYPTION_KEY` separately: a database dump without the key cannot decrypt credentials.

To restore:

```sh
docker compose -f deploy/compose.yaml exec -T db pg_restore -U ops -d ops --clean --if-exists < db-YYYY-MM-DD.dump
docker run --rm -v opsmanager_uploads:/data -v /var/backups/opsmanager:/backup debian:trixie-20261104-slim tar xzf /backup/uploads-YYYY-MM-DD.tar.gz -C /data
```

## How it behaves

- **Status** is computed at read time from the expiration date and the Settings timezone:
  - Cancelled
  - Expired (before today)
  - Expiring Soon (days left ≤ the type's largest reminder offset)
  - Active
- **Reminders** are checked every 5 minutes and go out at the configured send time:
  - Each offset is sent once per channel and is never resent after a restart.
  - After downtime, only the most recent missed reminder is sent.
  - Nothing is sent on or after the expiration date, or for cancelled assets.
  - Failed sends retry after 1, 5 and 15 minutes.
- **RDAP sync** runs daily at 03:00 and overwrites a domain's expiration date when the registry reports a different one.
  - Registries often extend gTLD dates before the registrar is paid, so a domain can look renewed while it is not (an accepted risk, PRD §13).
- **Screenshots** are 1440×900 WebP captures. Private, loopback and link-local addresses are refused.

## Development

You need Go (the version in `go.mod`), PostgreSQL, the Tailwind CSS standalone CLI and sqlc.

```sh
make css
make generate
OPS_BASE_URL=http://localhost:8080 \
OPS_DATABASE_URL=postgres://localhost/ops \
OPS_ENCRYPTION_KEY=$(openssl rand -base64 32) \
OPS_ADMIN_EMAIL=admin@example.com OPS_ADMIN_PASSWORD=change-me-now \
OPS_CHROMIUM_URL=ws://127.0.0.1:9222 OPS_UPLOADS_DIR=./data \
go run ./cmd/opsmanager
```

Before every commit, run the quality gates:

```sh
make check
```

Set `OPS_TEST_DATABASE_URL=postgres://localhost/postgres` to also run the PostgreSQL integration tests. They create and drop throwaway databases.
