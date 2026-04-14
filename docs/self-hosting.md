# Self-hosting BookStorage

Run BookStorage on your own Linux machine. For local development and CI, see [Development](development.md).

---

## Table of contents

- [Production installation (Linux)](#production-installation-linux)
- [bsctl — service and updates](#bsctl--service-and-updates)
- [Configuration](#configuration)
- [Prometheus metrics (optional)](#prometheus-metrics-optional)
- [Using the app](#using-the-app)
- [Troubleshooting](#troubleshooting)

---

## Production installation (Linux)

### Automatic installation

```bash
# Clone and install (as root)
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

The script installs:

- Compiled application
- `bsctl` CLI to manage the service
- systemd service (loads optional `EnvironmentFile=-/opt/bookstorage/.env`)
- Firewall configuration

**Prometheus (optional):** set `INSTALL_WITH_PROMETHEUS=1` when running the installer to install the distribution `prometheus` package, generate `BOOKSTORAGE_METRICS_TOKEN` if missing, and enable the `bookstorage-prometheus` systemd unit (Prometheus UI on `http://127.0.0.1:9091`, scrape via bearer token file). After a token is added to `.env`, run `systemctl restart bookstorage`. `bsctl update` does **not** re-run this step; on the server run `INSTALL_APP_DIR=/opt/bookstorage bash /opt/bookstorage/deploy/setup-bookstorage-prometheus.sh` (use `bash` so execute permission on the script is not required).

### Start the service

```bash
bsctl start
```

---

## bsctl — service and updates

`bsctl` (BookStorage Control) is the CLI to manage the running service and apply updates. For **development** commands (`build`, `run`, `clean`, …), see [Development — bsctl for development](development.md#bsctl-for-development).

```bash
bsctl help     # Show help
```

### Service commands

| Command        | Description          |
|----------------|----------------------|
| `bsctl start`  | Start the service    |
| `bsctl stop`   | Stop the service     |
| `bsctl restart`| Restart the service  |
| `bsctl status` | Show status          |
| `bsctl logs`   | Show real-time logs  |

### Production / maintenance commands

| Command            | Description                          |
|--------------------|--------------------------------------|
| `bsctl install`    | Install systemd service              |
| `bsctl uninstall`  | Uninstall service                    |
| `bsctl update`     | Interactive release: **1** / **2** = last two **major** tags `vX.0.0`, **3** = type any tag; or `BSCTL_UPDATE_TAG=vX.Y.Z` for non-interactive + build + restart |
| `bsctl update main` | Update from `origin/main` (fast-forward) + build + restart |
| `bsctl update <branch>` | Advanced: update from `origin/<branch>` (fast-forward) + build + restart |
| `bsctl fix-perms`  | Fix file permissions                 |
| `bsctl backup`     | Snapshot the SQLite file from `BOOKSTORAGE_DATABASE` in `.env` (uses `sqlite3 .backup` when available, else `cp`), optional retention via `BOOKSTORAGE_BACKUP_RETENTION_DAYS` (default 14), output under `BOOKSTORAGE_BACKUP_DIR` (default `/var/lib/bookstorage/backups`) |

**Scheduled backups:** set `INSTALL_WITH_BACKUP_TIMER=1` when running [`deploy/install.sh`](../deploy/install.sh) to install and enable `bookstorage-backup.timer` (daily snapshot; adjust the timer unit if needed). Logs: `journalctl -u bookstorage-backup.service`.

**Non-interactive release:** set `BSCTL_UPDATE_TAG=v5.5.0` and run `sudo -E bsctl update` to skip the menu. The clone is forced to match the chosen tag or `origin/<branch>` (local changes to tracked files are discarded).

If you deploy from a GitHub Actions artifact instead of cloning, extract the archive, copy `bookstorage`, `bsctl`, and `deploy/bookstorage.service` to the right paths, then use `bsctl install` / `bsctl update` as usual. See [Development — Deployment workflow](development.md#deployment-workflow).

---

## Configuration

### Environment variables

Copy the example file and edit it (never commit secrets):

```bash
cp .env.example .env
```

Use the same path on a server (e.g. `/opt/bookstorage/.env`). The stock `deploy/bookstorage.service` includes `EnvironmentFile=-/opt/bookstorage/.env` so variables from `.env` are applied (optional file: the leading `-` ignores a missing path).

Example `.env` contents:

```env
# Server
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000

# Database
BOOKSTORAGE_DATABASE=/opt/bookstorage/database.db

# Security (use a long random key in production)
BOOKSTORAGE_SECRET_KEY=your-very-long-secret-key

# Super administrator
BOOKSTORAGE_SUPERADMIN_USERNAME=admin
BOOKSTORAGE_SUPERADMIN_PASSWORD=SecurePassword123!
```

| Variable                    | Description            | Default                 |
|-----------------------------|------------------------|-------------------------|
| `BOOKSTORAGE_HOST`         | Listen address         | `127.0.0.1`             |
| `BOOKSTORAGE_PORT`         | Port                   | `5000`                  |
| `BOOKSTORAGE_DATABASE`     | SQLite database path   | `database.db`           |
| `BOOKSTORAGE_SECRET_KEY`   | Session secret key (min. 32 bytes if `BOOKSTORAGE_ENV=production`) | `dev-secret-change-me`  |
| `BOOKSTORAGE_ENV`          | `development` or `production` (production forbids default secret) | `development` |
| `BOOKSTORAGE_ENABLE_HSTS`  | Set to `true` or `1` to send `Strict-Transport-Security` (use only behind HTTPS) | (off) |
| `BOOKSTORAGE_METRICS_TOKEN` | If set, secures `GET /metrics` with `Authorization: Bearer …` or `?token=…` for Prometheus. If empty, only loopback clients may scrape `/metrics`. | (empty) |
| `BOOKSTORAGE_PROMETHEUS_QUERY_URL` | Base URL for Prometheus’s HTTP API (**Admin → Monitoring** embedded summary). Default `http://127.0.0.1:9091`. **Loopback hosts only** (`127.0.0.1`, `localhost`, `::1`). | (default) |

### Session lifetime

Sessions use a **sliding TTL of 2 hours** and an **absolute TTL of 24 hours**. If a user is inactive for more than 2 hours, they must log in again. Regardless of activity, every session expires 24 hours after creation.

### Legal notice

To customize the legal page (`/legal`), copy the example config:

```bash
cp config/site.json.example config/site.json
```

Then edit `config/site.json` with your information:

```json
{
  "site_name": "BookStorage",
  "site_url": "https://your-domain.com",
  "legal": {
    "owner_name": "Your Name",
    "owner_email": "contact@example.com",
    "owner_address": "Your Address",
    "hosting_provider": "Hosting Provider Name",
    "hosting_address": "Hosting Address",
    "data_retention": "Data retention policy...",
    "data_usage": "How data is used...",
    "custom_sections": []
  }
}
```

---

## Prometheus metrics (optional)

BookStorage exposes **`GET /metrics`** in Prometheus text format (counters and histograms prefixed with `bookstorage_http_*`).

- **No `BOOKSTORAGE_METRICS_TOKEN`:** scrapers must connect from **loopback** (`127.0.0.1` / `::1`) to read `/metrics`. This suits a Prometheus instance on the same host.
- **With `BOOKSTORAGE_METRICS_TOKEN`:** send `Authorization: Bearer <token>` or `GET /metrics?token=<token>` (documented for operators; prefer Bearer in Prometheus `bearer_token` / `bearer_token_file`).

**Automatic install (Linux installer):** `INSTALL_WITH_PROMETHEUS=1 sudo -E ./deploy/install.sh` runs `deploy/setup-bookstorage-prometheus.sh` via `bash`, which installs the distro `prometheus` package, writes `/etc/bookstorage/prometheus-bs.yml` and `/etc/bookstorage/bookstorage-metrics.token`, and enables **`bookstorage-prometheus`** (listens on `127.0.0.1:9091`, separate TSDB under `/var/lib/prometheus-bookstorage`). Then:

```bash
sudo systemctl restart bookstorage   # pick up BOOKSTORAGE_METRICS_TOKEN from .env if it was just added
sudo systemctl status bookstorage-prometheus
```

The setup script falls back to the **official Prometheus tarball** from GitHub when no distro package exists (set `PROMETHEUS_VERSION`, e.g. `2.55.2`, to override the default bundled in the script). Requires `curl` or `wget` and outbound HTTPS.

**Manual setup** (if your distribution has no `prometheus` package, the script failed, or you use containers):

1. Set the same secret in BookStorage and Prometheus, e.g. add `BOOKSTORAGE_METRICS_TOKEN=<long-random>` to `/opt/bookstorage/.env` and `systemctl restart bookstorage`.
2. Create `/etc/bookstorage/bookstorage-metrics.token` containing **only** that token (one line), `chmod 640`, owned by `root:prometheus` so the `prometheus` user can read it.
3. Minimal `scrape_configs` entry:

```yaml
scrape_configs:
  - job_name: bookstorage
    metrics_path: /metrics
    scheme: http
    bearer_token_file: /etc/bookstorage/bookstorage-metrics.token
    static_configs:
      - targets: ['127.0.0.1:5000']   # match BOOKSTORAGE_PORT
```

4. Point Grafana at your Prometheus server as usual.

The **Admin → Monitoring** page shows a short **embedded summary** (scrape health, request counter, 5‑minute rate) by querying the Prometheus HTTP API on the server (`BOOKSTORAGE_PROMETHEUS_QUERY_URL`, default `http://127.0.0.1:9091`), plus the local `/metrics` scrape URL and token hints. It auto-refreshes in the browser.

---

## Using the app

### Keyboard shortcuts (desktop)

On the dashboard (desktop web view only — not available on the mobile PWA):

| Key   | Action              |
|-------|---------------------|
| `N`   | Add new work        |
| `/`   | Focus search bar    |
| `S`   | Go to Statistics    |
| `P`   | Go to Profile       |
| `?`   | Show help           |
| `Esc` | Close/Unfocus       |

### Mobile PWA

The mobile view provides a **simplified experience** focused on everyday tracking. Available features: dashboard (search, filter, sort), add/edit works, and quick chapter +/- buttons. Pages like Statistics, Profile, Tools, Users, Admin, Export/Import and Legal are **only accessible from the desktop web view** and redirect to the dashboard on mobile.

The mobile app **auto-refreshes** when brought back to the foreground (e.g. after switching from the desktop web version), so changes sync automatically.

### Export / import (desktop)

**Export:** Profile → download your library as CSV, or **Download JSON** for a versioned backup (`export_version` field) suitable for re-import.

**Import:** Profile → upload a CSV or JSON export. CSV uses semicolon separator; optional columns `CatalogID`, `IsAdult`, `ImagePath` may follow `Notes`. Choose whether existing titles are **skipped** or **updated**.

> **Note:** Export and import are only accessible from the desktop web view (Profile page).

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Status values**: En cours, Terminé, En pause, Abandonné, À lire  
**Type values**: Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

---

## Troubleshooting

### `BOOKSTORAGE_SECRET_KEY must be at least 32 bytes when BOOKSTORAGE_ENV=production`

The unit file loads `/opt/bookstorage/.env` via `EnvironmentFile`. In **production**, `BOOKSTORAGE_SECRET_KEY` must be a **non-default** string of **at least 32 characters**.

Fix:

```bash
# Generate a new key (example)
openssl rand -base64 48
```

Put the result in `/opt/bookstorage/.env` as `BOOKSTORAGE_SECRET_KEY=...` (no quotes unless your process manager requires them; avoid trailing spaces). Then:

```bash
chmod 600 /opt/bookstorage/.env
systemctl restart bookstorage
```

Do **not** shorten the key to fit a note in a wiki; use a password manager or `openssl` as above.

### `sudo: bsctl: command not found`

`bsctl` is installed under `/usr/local/bin`. Some `sudo` configurations use a **restricted `PATH`** (`secure_path` in `/etc/sudoers`) that omits `/usr/local/bin`, so `sudo bsctl …` fails even though `bsctl` works in an interactive root shell.

**Fix:** call the full path, for example:

```bash
sudo /usr/local/bin/bsctl install
sudo /usr/local/bin/bsctl update main
```

If you are **already logged in as root**, run `bsctl install` **without** `sudo` (your shell already has the correct `PATH`).

### "readonly database" error

```bash
bsctl fix-perms
bsctl restart
```

### Port already in use

```bash
# See which process uses the port
sudo lsof -i :5000

# Change port in .env
BOOKSTORAGE_PORT=5001
```

### View detailed logs

```bash
bsctl logs
```

---

[Documentation index](README.md) · [Development](development.md)
