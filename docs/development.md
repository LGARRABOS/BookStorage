# Development

Local development, tooling, CI, and codebase layout. For running BookStorage as a service on Linux (systemd, updates, `.env` in production), see [Self-hosting](self-hosting.md).

---

## Table of contents

- [Quick start](#quick-start)
- [Local environment](#local-environment)
- [Makefile](#makefile)
- [Tests and linting locally](#tests-and-linting-locally)
- [Continuous integration & deployment](#continuous-integration--deployment)
- [bsctl for development](#bsctl-for-development)
- [Tab completion (bash)](#tab-completion-bash)
- [Project structure and architecture](#project-structure-and-architecture)
- [AniList recommendations](#anilist-recommendations)
- [Contributing](#contributing)

---

## Quick start

### Prerequisites

- **Go 1.22+**
- **GCC** (for SQLite compilation with CGO)

### Run the app

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Server listens on **http://127.0.0.1:5000** by default.

Alternatives: `make run`, `bsctl run`, or run the `cmd/bookstorage` package from your IDE.

---

## Local environment

For development, copy the environment template and edit as needed:

```bash
cp .env.example .env
```

Use defaults for local work; for a full variable reference (production paths, secrets, legal `site.json`), see [Self-hosting — Configuration](self-hosting.md#configuration).

---

## Makefile

`make` targets mirror some `bsctl` commands. Prefer **`bsctl help`** for the canonical English CLI (see [`scripts/bsctl`](../scripts/bsctl)).

| Target | Purpose |
|--------|---------|
| `make build` | Debug build: `go build -o bookstorage ./cmd/bookstorage` |
| `make build-prod` | Optimized binary with `-ldflags` and `APP_VERSION` from the Makefile |
| `make run` | `go run ./cmd/bookstorage` |
| `make clean` | Remove the `bookstorage` binary in the repo root |
| `make help` | Print short help (messages may be in French) |

Production-oriented targets (`install`, `uninstall`, `update`, `fix-perms`, `status`, `logs`) require root or a configured host and are described from an operator perspective in [Self-hosting](self-hosting.md).

---

## Tests and linting locally

Align with [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) before opening a PR:

```bash
go mod download
go test ./... -coverprofile=coverage.out
go test -race ./...
```

Formatting:

```bash
gofmt -w .
# or: gofmt -l .   # list files that need formatting
```

Linting (install [golangci-lint](https://golangci-lint.run/) if needed):

```bash
golangci-lint run
```

CI also runs `gofmt` in strict mode (fails if any non-formatted file is listed) and a **smoke-http** job that starts `go run ./cmd/bookstorage` and curls `/`, `/login`, `/register`.

---

## Continuous integration & deployment

### CI (GitHub Actions)

On **push** (all branches) and on **pull requests** to `main`, `.github/workflows/ci.yml` runs:

| Job | What it does |
|-----|----------------|
| **Lint** | `gofmt -l .` must be empty; `golangci-lint` |
| **Unit tests** | `go test ./... -coverprofile=coverage.out` (coverage uploaded as artifact) |
| **Race tests** | `go test -race ./...` |
| **HTTP smoke tests** | Start app with env vars, wait for `/`, then curl `/`, `/login`, `/register` |

All jobs must pass before merging.

<a id="deployment-workflow"></a>
### Deployment workflow

The workflow `.github/workflows/deploy.yml` builds a **Linux amd64** binary with CGO, packages `bookstorage`, `bsctl`, and `deploy/bookstorage.service`, and uploads a `bookstorage-linux-amd64` `.tar.gz` artifact (manual **Run workflow**).

Using that artifact on a server (install paths, `bsctl install` / `bsctl update`) is covered in [Self-hosting](self-hosting.md).

---

<a id="bsctl-for-development"></a>
## bsctl for development

`bsctl` (BookStorage Control) is the same script as in production; for **service** (`start`, `stop`, `update`, …) and **install** commands, see [Self-hosting — bsctl](self-hosting.md#bsctl--service-and-updates).

| Command | Description |
|---------|-------------|
| `bsctl build` | Compile the application |
| `bsctl build-prod` | Optimized binary with `-ldflags` and `APP_VERSION` from `scripts/bsctl` |
| `bsctl run` | Start dev server (`go run ./cmd/bookstorage`) |
| `bsctl clean` | Remove compiled binary files |

Run `bsctl help` for the full command list.

If the first argument is not a recognized subcommand, `bsctl` prints the full help (exit code `1`).

### Version string in builds

`APP_VERSION` in [`Makefile`](../Makefile) and [`scripts/bsctl`](../scripts/bsctl) should stay in sync for release builds (`-X main.Version=...`). See the release workflow in the [push-release-agent](../.cursor/skills/push-release-agent/SKILL.md) skill if you maintain releases.

---

## Tab completion (bash)

From a **development clone**:

```bash
source scripts/bsctl.completion.bash
```

After `sudo bsctl install` or `./deploy/install.sh`, completion may live in `/etc/bash_completion.d/bsctl` — `source` that file in a new shell. Then type `bsctl` and press Tab; after `bsctl update`, Tab can suggest `main`, tags, and branch names when the cwd is a repo clone.

---

## Project structure and architecture

```
BookStorage/
├── cmd/bookstorage/     # Entry point (flags, HTTP server bootstrap)
│   └── main.go
├── internal/
│   ├── server/          # HTTP handlers, HTML/API routes, Web Push, import/export
│   ├── config/          # Env loading, site/legal JSON config
│   ├── database/        # SQLite access, schema migrations
│   ├── catalog/         # External catalog integrations (AniList, MangaDex)
│   └── i18n/            # French/English strings
├── scripts/
│   ├── bsctl                    # Management CLI
│   └── bsctl.completion.bash    # Bash completion
├── Makefile
├── .env.example
├── config/site.json.example
├── deploy/
│   ├── install.sh
│   └── bookstorage.service
├── templates/           # Templates (.gohtml)
└── static/              # CSS, avatars, images, PWA, PWA assets
```

<a id="anilist-recommendations"></a>
### AniList recommendations

The dashboard **For you** section and `GET /api/recommendations` call AniList’s public GraphQL API (`internal/catalog`) to blend **browse** results (genres/tags inferred from the user’s library) with **recommendation** edges from highly rated linked titles. Only works whose `catalog` row has `source = 'anilist'` participate in the taste profile; add items via catalog search so they get an AniList link. Respect AniList rate limits in production (the server batches fetches per request).

---

## Contributing

1. Fork and branch from `main` (or open a PR from a branch with a clear name).
2. Run **tests**, **gofmt**, and **golangci-lint** locally; CI must be green.
3. Avoid committing `.env` or secrets; use `.env.example` and `config/site.json.example` as templates.
4. For user-facing behavior changes, update the relevant doc under `docs/` (and `docs/fr/` if you maintain French).

---

[Documentation index](README.md) · [Self-hosting](self-hosting.md)
