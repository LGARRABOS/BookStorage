# Development

Local development, CI/CD, and repository layout. For installing BookStorage on a server, see [Self-hosting](self-hosting.md).

---

## Table of contents

- [Quick start](#quick-start)
- [Continuous integration & deployment](#continuous-integration--deployment)
- [bsctl CLI](#bsctl-cli)
- [Project structure](#project-structure)

---

## Quick start

### Prerequisites

- **Go 1.22+**
- **GCC** (for SQLite compilation with CGO)

### Run in development

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Server listens on **http://127.0.0.1:5000** by default.

You can also use `make run`, `bsctl run`, or open the project in your editor and run the `cmd/bookstorage` package.

---

## Continuous integration & deployment

### CI (GitHub Actions)

On every **push** and on **pull requests** to `main`, the workflow `.github/workflows/ci.yml` runs:

- `lint`: formatting (`gofmt`) and `golangci-lint`
- `tests`: unit tests with coverage (`go test ./... -coverprofile=coverage.out`, artifact upload)
- `race-tests`: `go test -race ./...`
- `smoke-http`: starts the app and checks key routes (`/`, `/login`, `/register`)

All jobs must pass before merging.

### Deployment workflow

The workflow `.github/workflows/deploy.yml` is a base for shipping binaries:

- Manual trigger: **Run workflow** in the Actions tab (`workflow_dispatch`)
- Builds **Linux amd64** with CGO
- Packages `bookstorage`, `bsctl`, `deploy/bookstorage.service`
- Uploads a `bookstorage-linux-amd64` `.tar.gz` artifact

On a server: download the artifact, extract, copy files into place, then use `bsctl install` / `bsctl update` as described in [Self-hosting](self-hosting.md).

---

## bsctl CLI

`bsctl` (BookStorage Control) manages builds, the dev server, and production installs. Run `bsctl help` for the full English help text.

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
| `bsctl update`     | Interactive release: **1** / **2** = last two **major** tags `vX.0.0`, **3** = type any tag; or `BSCTL_UPDATE_TAG=vX.Y.Z` for non-interactive + build + restart |
| `bsctl update main` | Update from `origin/main` (fast-forward) + build + restart |
| `bsctl update <branch>` | Advanced: update from `origin/<branch>` (fast-forward) + build + restart |
| `bsctl fix-perms`  | Fix file permissions                 |

**Non-interactive release:** set `BSCTL_UPDATE_TAG=v4.0.1` and run `sudo -E bsctl update` to skip the menu. The clone is forced to match the chosen tag or `origin/<branch>` (local changes to tracked files are discarded).

### Unknown commands

If the first argument is not a recognized subcommand, `bsctl` prints the full help (exit code `1`).

### Tab completion (bash)

Programmable completion works in **bash**. After `sudo bsctl install` or `./deploy/install.sh`, completion is installed to `/etc/bash_completion.d/bsctl`. Open a new terminal, or:

```bash
source /etc/bash_completion.d/bsctl
```

From a development clone:

```bash
source scripts/bsctl.completion.bash
```

Then type `bsctl` and press Tab. After `bsctl update`, Tab can suggest **`main`**, recent **tags**, and **branch** names when your current directory is a clone of the repo.

---

## Project structure

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
│   ├── bsctl                    # Management CLI
│   └── bsctl.completion.bash    # Bash tab completion (source or install)
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

[Documentation index](README.md) · [Self-hosting](self-hosting.md)
