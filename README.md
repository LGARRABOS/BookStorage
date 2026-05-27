# BookStorage

**BookStorage** is a personal reading tracker web application. Track your novels, manga, webtoons, light novels and more.

_🇫🇷 [Version française](./README.fr.md)_

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## About

BookStorage is a **self-hosted** web app to catalogue what you read and follow your progress over time. Ratings, notes, statistics, community libraries, dark mode, PWA, keyboard shortcuts — everything runs on **your** machine with a **SQLite** or **PostgreSQL** database.

### Key features

- Multi-format library (novels, manga, webtoons, light novels…)
- Ratings, notes, statistics, public community libraries
- Dark mode, multilingual UI (EN/FR/DE/ES/IT/PT), installable PWA
- Mobile PWA with simplified dashboard and quick chapter +/-
- Export/import (CSV, JSON) + MyAnimeList and AniList import
- AniList-powered recommendations, catalog integration
- Admin panel, Prometheus metrics, Google OAuth

---

## Quick start

**Requirements:** Go 1.22+, GCC (CGO for SQLite).

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Open **http://127.0.0.1:5000**

---

## Production (Linux)

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
bsctl start
```

`install.sh` generates `.env` with `BOOKSTORAGE_ENV=production`, a random secret key, and a random superadmin password (shown once). Also verify:

| Variable | Recommendation |
|----------|----------------|
| `BOOKSTORAGE_ENABLE_HSTS` | `true` behind HTTPS |
| `BOOKSTORAGE_TRUST_PROXY` | `true` when behind a trusted reverse proxy |
| `BOOKSTORAGE_POSTGRES_URL` | `sslmode=require` if the DB is on the public Internet; `disable` OK on private LAN IPs |

Post-install: rotate the superadmin password if needed, enable HSTS, run `./scripts/ci/security_smoke.sh` against the instance.

---

## Documentation

Full documentation is available on the **[Wiki](https://github.com/LGARRABOS/BookStorage/wiki)**:

- [Installation](https://github.com/LGARRABOS/BookStorage/wiki/Installation) — development and production setup
- [Configuration](https://github.com/LGARRABOS/BookStorage/wiki/Configuration) — environment variables, OAuth, PostgreSQL
- [Usage](https://github.com/LGARRABOS/BookStorage/wiki/Usage) — dashboard, PWA, export/import, shortcuts
- [API Reference](https://github.com/LGARRABOS/BookStorage/wiki/API-Reference) — REST API endpoints
- [OpenAPI spec](./docs/openapi.yaml) — machine-readable API schema (Bearer tokens, bulk, webhooks)
- [Architecture](https://github.com/LGARRABOS/BookStorage/wiki/Architecture) — tech stack, project structure
- [Database](https://github.com/LGARRABOS/BookStorage/wiki/Database) — schema, migrations, full-text search
- [Authentication & Security](https://github.com/LGARRABOS/BookStorage/wiki/Authentication-and-Security) — auth, sessions, hardening
- [CI / CD](https://github.com/LGARRABOS/BookStorage/wiki/CI-CD) — pipeline, deployment, bsctl CLI
- [Troubleshooting](https://github.com/LGARRABOS/BookStorage/wiki/Troubleshooting) — common issues and solutions

---

## License

[MIT License](./LICENSE)
