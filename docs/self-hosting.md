# Self-hosting BookStorage

Run BookStorage on your own Linux machine. For local development and CI, see [Development](development.md).

---

## Table of contents

- [Production installation (Linux)](#production-installation-linux)
- [bsctl — service and updates](#bsctl--service-and-updates)
- [Configuration](#configuration)
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
- systemd service
- Firewall configuration

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

**Non-interactive release:** set `BSCTL_UPDATE_TAG=v4.1.0` and run `sudo -E bsctl update` to skip the menu. The clone is forced to match the chosen tag or `origin/<branch>` (local changes to tracked files are discarded).

If you deploy from a GitHub Actions artifact instead of cloning, extract the archive, copy `bookstorage`, `bsctl`, and `deploy/bookstorage.service` to the right paths, then use `bsctl install` / `bsctl update` as usual. See [Development — Deployment workflow](development.md#deployment-workflow).

---

## Configuration

### Environment variables

Copy the example file and edit it (never commit secrets):

```bash
cp .env.example .env
```

Use the same path on a server (e.g. `/opt/bookstorage/.env`). With **systemd**, load it with `EnvironmentFile=/opt/bookstorage/.env` in your unit file so variables are applied to the process.

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

## Using the app

### Keyboard shortcuts

On the dashboard:

| Key   | Action              |
|-------|---------------------|
| `N`   | Add new work        |
| `/`   | Focus search bar    |
| `S`   | Go to Statistics    |
| `P`   | Go to Profile       |
| `?`   | Show help           |
| `Esc` | Close/Unfocus       |

### Export / import

**Export:** Profile → download your library as CSV, or **Download JSON** for a versioned backup (`export_version` field) suitable for re-import.

**Import:** Profile → upload a CSV or JSON export. CSV uses semicolon separator; optional columns `CatalogID`, `IsAdult`, `ImagePath` may follow `Notes`. Choose whether existing titles are **skipped** or **updated**.

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Status values**: En cours, Terminé, En pause, Abandonné, À lire  
**Type values**: Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

---

## Troubleshooting

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
