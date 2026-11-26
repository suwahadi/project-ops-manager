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

This section is for a dedicated server where Ops Manager owns ports 80 and 443. If the server already hosts other websites behind Nginx, follow [Deploy on a shared VPS behind an existing Nginx](#deploy-on-a-shared-vps-behind-an-existing-nginx) instead.

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

## Deploy on a shared VPS behind an existing Nginx

Use this when the server already runs Nginx on ports 80 and 443 for other websites, for example an Ubuntu VPS with Nginx, Certbot and several sites. The app then runs from `deploy/compose.host-proxy.yaml`:

- There is no Caddy service.
- The app listens only on `127.0.0.1:8090`; change the port with `OPS_HOST_PORT` in `deploy/.env`.
- Container memory is capped for small servers: app 384 MB, database 512 MB, Chromium 640 MB.

Nginx gets one extra server block, and no other site is touched. Replace `ops.example.com` with your hostname everywhere below, and run every command on the server.

### 1. Check the server first (read-only)

```sh
sudo ss -tulpn | grep -E ':(80|443|8090)\b'
ls /etc/nginx/sites-enabled
certbot --version
sudo ls /etc/letsencrypt/live
free -h
```

What to look for:
- Ports 80 and 443 should belong to `nginx`, and port 8090 must be free.
- Look at one existing site in `sites-enabled` to see how it obtains certificates. The steps below use the Certbot `webroot` method.

### 2. Install Docker

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
echo '{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }' | sudo tee /etc/docker/daemon.json
sudo systemctl restart docker
```

The `daemon.json` setting caps container logs at 30 MB per container. Docker does not change Nginx, and because the app binds to `127.0.0.1` only, no new port is opened to the internet.

### 3. Clone the code and create `deploy/.env`

```sh
sudo mkdir -p /opt/opsmanager
sudo chown "$USER":"$USER" /opt/opsmanager
git clone https://github.com/suwahadi/project-ops-manager.git /opt/opsmanager
cd /opt/opsmanager
ADMIN_EMAIL="you@example.com"
KEY=$(openssl rand -base64 32)
PGPASS=$(openssl rand -hex 24)
ADMINPASS=$(openssl rand -base64 18)
cat > deploy/.env <<EOF
OPS_BASE_URL=https://ops.example.com
OPS_ENCRYPTION_KEY=$KEY
OPS_ADMIN_EMAIL=$ADMIN_EMAIL
OPS_ADMIN_PASSWORD=$ADMINPASS
POSTGRES_PASSWORD=$PGPASS
OPS_HOST_PORT=8090
EOF
chmod 600 deploy/.env
unset KEY PGPASS ADMINPASS
```

Then open the file with `sudo nano /opt/opsmanager/deploy/.env` and copy three values to a password manager **outside the server**:
- `OPS_ENCRYPTION_KEY`
- `OPS_ADMIN_PASSWORD`
- `POSTGRES_PASSWORD`

The database password is hexadecimal so that it is safe inside the connection URL.

### 4. Build and start

```sh
cd /opt/opsmanager
sudo docker compose -f deploy/compose.host-proxy.yaml pull db chromium
sudo docker compose -f deploy/compose.host-proxy.yaml build app
sudo docker compose -f deploy/compose.host-proxy.yaml up -d
sudo docker compose -f deploy/compose.host-proxy.yaml ps
curl -s -o /dev/null -w "healthz: %{http_code}\n" http://127.0.0.1:8090/healthz
```

The first build takes about 5–15 minutes on 2 vCPUs. When it is done:
- All three services should be `healthy`.
- `healthz` should print `200`.
- The app log (`sudo docker compose -f deploy/compose.host-proxy.yaml logs app`) should show `admin account created`.

### 5. Nginx: HTTP block for the certificate challenge

```sh
sudo mkdir -p /www/wwwroot/ops.example.com/.well-known/acme-challenge
sudo tee /etc/nginx/sites-available/ops.example.com > /dev/null <<'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name ops.example.com;
    location ^~ /.well-known/acme-challenge/ { root /www/wwwroot/ops.example.com; default_type text/plain; }
    location / { return 301 https://$host$request_uri; }
}
EOF
sudo ln -s /etc/nginx/sites-available/ops.example.com /etc/nginx/sites-enabled/ops.example.com
sudo nginx -t && sudo systemctl reload nginx
```

Use whichever webroot convention the server already follows; `/www/wwwroot/<host>` is only an example. Always run `nginx -t` before a reload: if the test fails, Nginx keeps serving the old configuration.

### 6. Certificate and the HTTPS block

```sh
sudo certbot certonly --webroot -w /www/wwwroot/ops.example.com -d ops.example.com --keep-until-expiring
sudo tee /etc/nginx/sites-available/ops.example.com > /dev/null <<'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name ops.example.com;
    location ^~ /.well-known/acme-challenge/ { root /www/wwwroot/ops.example.com; default_type text/plain; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ops.example.com;

    ssl_certificate /etc/letsencrypt/live/ops.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/ops.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:ops_ssl:10m;
    ssl_session_timeout 10m;

    client_max_body_size 256m;

    location ^~ /.well-known/acme-challenge/ { root /www/wwwroot/ops.example.com; default_type text/plain; }

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }

    access_log /var/log/nginx/ops.example.com.log;
    error_log /var/log/nginx/ops.example.com.error.log;
}
EOF
sudo nginx -t && sudo systemctl reload nginx
curl -sI https://ops.example.com/login | head -1
```

Notes on the HTTPS block:
- `client_max_body_size 256m` allows uploads of up to 10 files of 20 MB each.
- `proxy_read_timeout 120s` leaves time for large image conversions.

**Make renewals reload Nginx.** If `/etc/letsencrypt/renewal-hooks/deploy/` is empty and no other renewal hook exists, renewed certificates are only picked up at the next Nginx reload. Add a hook:

```sh
sudo grep -H -E 'renew_hook|deploy_hook|post_hook' /etc/letsencrypt/renewal/*.conf /etc/letsencrypt/cli.ini 2>/dev/null || echo "no hook"
printf '#!/bin/sh\nnginx -t -q && systemctl reload nginx\n' | sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx > /dev/null
sudo chmod 755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx
sudo certbot renew --dry-run
```

The hook applies to every site on the server. It runs only after a certificate is actually renewed, and it reloads Nginx only if the configuration test passes.

### 7. First sign-in

1. Open `https://ops.example.com` and sign in with `OPS_ADMIN_EMAIL` and `OPS_ADMIN_PASSWORD` from `deploy/.env`.
2. Change the password under **Settings → Account**. The admin variables are read only on the very first start; afterwards, editing them in `.env` does not change the login.
3. Check **Settings → General**, then configure **Settings → Notifications** (see [Notification setup](#notification-setup)).

### 8. Nightly backups

```sh
sudo tee /usr/local/bin/opsmanager-backup > /dev/null <<'EOF'
#!/bin/sh
set -eu
DIR=/var/backups/opsmanager
COMPOSE="docker compose -f /opt/opsmanager/deploy/compose.host-proxy.yaml"
DATE=$(date +%F)
mkdir -p "$DIR"
chmod 700 "$DIR"
$COMPOSE exec -T db pg_dump -U ops -Fc ops > "$DIR/db-$DATE.dump"
docker run --rm --user 0 --entrypoint tar -v opsmanager_uploads:/data:ro -v "$DIR":/backup opsmanager:local czf "/backup/uploads-$DATE.tar.gz" -C /data .
chmod 600 "$DIR"/*
find "$DIR" -type f -mtime +14 -delete
echo "$(date -Is) backup ok"
EOF
sudo chmod 755 /usr/local/bin/opsmanager-backup
sudo /usr/local/bin/opsmanager-backup
echo '30 19 * * * root /usr/local/bin/opsmanager-backup >> /var/log/opsmanager-backup.log 2>&1' | sudo tee /etc/cron.d/opsmanager-backup > /dev/null
```

What this sets up:
- The cron line runs at 19:30 UTC, which is 02:30 in Asia/Jakarta. Adjust it to your server's timezone.
- Backups are kept for 14 days.

To check that a dump can be restored, count its tables:

```sh
sudo sh -c 'docker compose -f /opt/opsmanager/deploy/compose.host-proxy.yaml exec -T db pg_restore --list < /var/backups/opsmanager/db-$(date +%F).dump | grep -c "TABLE DATA"'
```

Copy `/var/backups/opsmanager` off the server regularly, and keep `OPS_ENCRYPTION_KEY` in a separate safe place. A restore without the key cannot decrypt credentials or notification secrets.

### 9. Everyday commands

| Task | Command |
| --- | --- |
| Status | `sudo docker compose -f /opt/opsmanager/deploy/compose.host-proxy.yaml ps` |
| Logs | `sudo docker compose -f /opt/opsmanager/deploy/compose.host-proxy.yaml logs app --tail 50` |
| Warnings and errors | `sudo docker compose -f /opt/opsmanager/deploy/compose.host-proxy.yaml logs app --since 1h \| grep -E '"level":"(ERROR\|WARN)"'` |
| Memory | `sudo docker stats --no-stream` |
| Edit settings | `sudo nano /opt/opsmanager/deploy/.env`, then `cd /opt/opsmanager && sudo docker compose -f deploy/compose.host-proxy.yaml up -d` |
| Run a backup now | `sudo /usr/local/bin/opsmanager-backup` |

Never change `OPS_ENCRYPTION_KEY` by hand; use the rotation procedure in [Keep the encryption key safe](#keep-the-encryption-key-safe). Never change `POSTGRES_PASSWORD` after the first start.

### 10. Updating

```sh
cd /opt/opsmanager
git pull
sudo docker compose -f deploy/compose.host-proxy.yaml up -d --build
sudo docker image prune -f
curl -s -o /dev/null -w "healthz: %{http_code}\n" http://127.0.0.1:8090/healthz
```

What to expect:
- Migrations run automatically on start.
- The app is unavailable for a few seconds while the container is replaced.
- `image prune` removes only dangling images left over from earlier builds.

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

Then set `OPS_ENCRYPTION_KEY` in `deploy/.env` to the new key and start `app` again. On a shared VPS, use `-f deploy/compose.host-proxy.yaml` in these commands. Save the new key outside the VPS.

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
- **Invoices** (menu **Invoices**, or **Create invoice** on an asset or client):
  - Fill **Settings → Invoice** first: company name, website, number prefix and the default note (for example bank details).
  - Numbers are the prefix plus 8 random characters (`INV-DEV-HK87CZ02`). Press the refresh button for a new one, or type your own; numbers must be unique.
  - Each row has a bold item title, a multi-line description, quantity, rate and tax %. Totals are recalculated on the server when you save.
  - **Mark paid** asks for the payment date, adds a green PAID ribbon and sets Amount Due to zero. **Mark unpaid** reverses it.
  - **Download PDF** prints an A4 PDF through the Chromium container and names it after the invoice number. If Chromium is down, the page shows an error instead.
  - Invoices are not included in the Finance page totals; record project payments instead (below).
- **Project pricing** (client projects only):
  - Set the currency, project value, project cost and an optional billing date on the project form.
  - Use **Record payment** on the project page for every amount received (a deposit, then the rest). Payments cannot exceed the project value.
  - Finance counts each payment as revenue on its date, with the matching share of the project cost. The unpaid balance is projected on the billing date.
  - **Create invoice** on a project bills its remaining balance. When that invoice is marked paid, its page offers **Record payment** for each project on it.

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
