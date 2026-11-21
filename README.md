# Ops Manager

Self-hosted, single-user web app that tracks every web-development asset with an expiration date (domains, hosting, VPS, SSL certificates, email hosting, software licenses and custom types) and reminds you before it expires.

The full specification lives in [`docs/PRD.md`](docs/PRD.md). Contributor rules live in [`AGENTS.md`](AGENTS.md).

## Quick start (Docker Compose)

```sh
cp deploy/.env.example deploy/.env
openssl rand -base64 32
docker compose -f deploy/compose.yaml up -d --build
```

Put the generated key into `OPS_ENCRYPTION_KEY` in `deploy/.env` and fill in the other values before starting.
