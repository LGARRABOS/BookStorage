# 📚 BookStorage

**BookStorage** is a personal reading tracker web application. Track your novels, manga, webtoons, light novels and more.

_🇫🇷 [Version française](./README.fr.md)_

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## 📑 Table of Contents

- [Features](#-features)
- [Quick Start](#-quick-start)
- [Production Installation (Linux)](#-production-installation-linux)
- [Continuous Integration & Deployment](#-continuous-integration--deployment)
- [bsctl CLI](#-bsctl-cli)
- [Configuration](#-configuration)
- [Keyboard Shortcuts](#-keyboard-shortcuts)
- [Export / Import](#-exportimport)
- [Project Structure](#-project-structure)
- [Troubleshooting](#-troubleshooting)
- [License](#-license)

---

## ✨ Features

- 📖 **Multi-format**: Novels, manga, manhwa, webtoons, light novels...
- ⭐ **Ratings & notes**: Rate your works from 1 to 5 stars with personal notes
- 📊 **Statistics**: Visualize your reading habits
- 👥 **Community**: Explore other readers' public libraries
- 🌓 **Dark mode**: Light or dark interface based on your preferences
- 🔐 **Privacy**: Public or private profile, you choose
- 🌍 **Multilingual**: French and English interface
- 📱 **PWA**: Install as a mobile app on iOS/Android
- 📦 **Export/Import**: Backup and restore your library via CSV
- ⌨️ **Keyboard shortcuts**: Navigate quickly (N, /, S, P, ?)

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.22+**
- **GCC** (for SQLite compilation with CGO)

### Run in development

```bash
# Clone the project
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage

# Start the server
go run .
```

Server starts on **http://127.0.0.1:5000**

---

## 📦 Production Installation (Linux)

### Automatic installation

```bash
# Clone and install (as root)
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

The script automatically installs:
- Compiled application
- `bsctl` CLI to manage the service
- systemd service
- Firewall configuration

### Start the service

```bash
bsctl start
```

---

## ✅ Continuous Integration & Deployment

### CI (GitHub Actions)

On every **push** and on **pull requests** to `main`, the workflow `.github/workflows/ci.yml` runs several jobs:

- `lint`: formatting check (`gofmt`) and advanced linting with `golangci-lint`
- `tests`: unit tests with coverage (`go test ./... -coverprofile=coverage.out`, uploaded as an artifact)
- `race-tests`: tests with race detector (`go test -race ./...`)
- `smoke-http`: starts the application and performs basic HTTP checks on key routes (e.g. `/`, `/login`, `/register`)

All jobs must pass for the PR to be **green** and safely mergeable.

### Deployment workflow

The workflow `.github/workflows/deploy.yml` provides a base for deployment:

- Manual trigger via **“Run workflow”** in the GitHub Actions tab (`workflow_dispatch`)
- Build of a **Linux amd64** binary with CGO enabled
- Packaging of:
  - `bookstorage` binary
  - `bsctl` CLI
  - `deploy/bookstorage.service`
- Upload of a `bookstorage-linux-amd64` artifact (`.tar.gz`)

You can download this artifact on your server and:

1. Extract it
2. Copy `bookstorage`, `bsctl` and `bookstorage.service` to the appropriate locations
3. Use `bsctl install` / `bsctl update` (optional branch: `bsctl update my-branch`) to manage the service

---

## 🛠️ bsctl CLI

`bsctl` (BookStorage Control) is the CLI to manage BookStorage.

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

### Development commands

| Command           | Description             |
|-------------------|-------------------------|
| `bsctl build`     | Compile the application |
| `bsctl build-prod`| Compile for production  |
| `bsctl run`       | Start dev server        |
| `bsctl clean`     | Remove compiled files   |

### Production / maintenance commands

| Command            | Description                          |
|--------------------|--------------------------------------|
| `bsctl install`    | Install systemd service              |
| `bsctl uninstall`  | Uninstall service                    |
| `bsctl update [branch]` | Update from `origin/main` or `origin/<branch>` (fast-forward) + build + restart |
| `bsctl fix-perms`  | Fix file permissions                 |

---

## ⚙️ Configuration

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
| `BOOKSTORAGE_VAPID_PUBLIC` | VAPID public key (Web Push) | (optional)           |
| `BOOKSTORAGE_VAPID_PRIVATE`| VAPID private key (Web Push) | (optional)           |

Generate VAPID keys: `./bookstorage -gen-vapid`

### Legal Notice / Mentions légales

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

## ⌨️ Keyboard Shortcuts

On the dashboard, use these keyboard shortcuts for quick navigation:

| Key   | Action              |
|-------|---------------------|
| `N`   | Add new work        |
| `/`   | Focus search bar    |
| `S`   | Go to Statistics    |
| `P`   | Go to Profile       |
| `?`   | Show help           |
| `Esc` | Close/Unfocus       |

---

## 📦 Export/Import

### Export

Go to **Profile** → Download your library as a CSV file, or use **Download JSON** for a versioned backup (`export_version` field) suitable for re-import.

### Import

Go to **Profile** → Upload a CSV or JSON export. CSV uses semicolon separator; optional columns `CatalogID`, `IsAdult`, `ImagePath` may be present after `Notes`. Choose whether existing titles are **skipped** or **updated**.

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Status values**: En cours, Terminé, En pause, Abandonné, À lire  
**Type values**: Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

---

## 📁 Project Structure

```
BookStorage/
├── cmd/bookstorage/     # Entry point
│   └── main.go
├── internal/            # Internal packages
│   ├── server/         # HTTP handlers, API, Push
│   ├── config/         # Configuration handling
│   ├── database/       # SQLite database
│   ├── catalog/        # AniList, MangaDex
│   └── i18n/           # Internationalization
│
├── scripts/
│   └── bsctl           # Management CLI
├── Makefile            # Make commands
│
├── .env.example        # Environment template (copy to .env)
├── config/
│   └── site.json.example  # Legal config template
├── go.mod / go.sum      # Go dependencies
│
├── deploy/
│   ├── install.sh       # Installation script
│   └── bookstorage.service
│
├── templates/           # HTML templates (.gohtml)
└── static/
    ├── css/             # Stylesheets
    ├── avatars/         # User avatars
    ├── images/          # App images
    ├── icons/           # Favicon & icons
    └── pwa/             # PWA files
        ├── manifest.json
        └── sw.js
```

---

## 🐛 Troubleshooting

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

## 📝 License

MIT License
