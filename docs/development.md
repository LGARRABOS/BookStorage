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
| `make test` | Run unit tests with coverage profile (`coverage.out`) |
| `make test-race` | Run race detector suite |
| `make lint` | Enforce `gofmt -l .` + run `golangci-lint` |
| `make ci-local` | Local CI parity (`lint`, `test`, `test-race`) |
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

### API and import notes

- `GET /metrics` exposes Prometheus text metrics (`bookstorage_http_*`). If `BOOKSTORAGE_METRICS_TOKEN` is unset, only loopback clients may scrape it; otherwise use `Authorization: Bearer …` or `?token=…`. See [Self-hosting — Prometheus metrics](self-hosting.md#prometheus-metrics-optional).
- `GET /api/works` supports pagination (`page`, `limit`), filters (`status`, `reading_type`, `search`), and sorting (`sort`).
- The response now includes both `data` and `meta` (`total`, `total_pages`, `has_next`, `has_prev`).
- Import accepts standard BookStorage exports plus common external formats: **MyAnimeList** (CSV) and **AniList** (JSON/CSV).
- Current search implementation uses SQL `LIKE` patterns for broad matching; if library size grows significantly, an FTS5-backed path can be introduced while keeping the API contract stable.

### HTTP hardening

- Authenticated mutating requests (POST/PATCH/DELETE/PUT) are protected with an origin check (`Origin`/`Referer`) to reduce CSRF risk.
- Lightweight rate limiting is applied on sensitive endpoints (authentication and write-heavy routes).
- The HTTP server is started with explicit timeouts (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`) in `cmd/bookstorage/main.go` for better resilience under load and slowloris-like traffic.

---

## Continuous integration & deployment

### CI (GitHub Actions)

On **push** (all branches) and on **pull requests** to `main`, `.github/workflows/ci.yml` runs:

| Job | What it does |
|-----|----------------|
| **Lint** | `gofmt -l .` must be empty; `golangci-lint` |
| **Unit tests** | `go test ./... -coverprofile=coverage.out` (coverage uploaded as artifact) |
| **Race tests** | `go test -race ./...` |
| **Build Linux binary** | `go build -o bookstorage ./cmd/bookstorage` (uploaded as CI artifact) |
| **HTTP smoke tests** | Download built CI binary artifact, start app, wait for `/`, then curl `/`, `/login`, `/register` |

Execution order is staged: **Lint** first, then **Unit tests** and **Race tests** in parallel, then **Build Linux binary**, and finally **HTTP smoke tests**.

CI also enables workflow concurrency (`cancel-in-progress`) to automatically cancel outdated runs on the same branch.

All jobs in that chain must pass before merging.

### Security testing in CI

The same trigger (push / PR) also runs **security-oriented jobs** in parallel with the core pipeline. These jobs use `continue-on-error` so they **never block a merge** (warn-only / observability mode).

| Job | Tool | What it checks |
|-----|------|----------------|
| **SAST** | `gosec` | Static analysis of Go source for common security issues |
| **Dependency vulnerabilities** | `govulncheck` | Known CVEs in Go modules |
| **Secrets scan** | `gitleaks` | Leaked credentials, API keys, tokens in the git history |
| **DAST smoke** | `scripts/ci/security_smoke.sh` | Live checks against the running app: security headers, API auth (401), wrong methods (405), CSRF origin blocking (403), auth rate limiting (429), admin route protection |

Each job uploads a report artifact (JSON or TXT) for review.

#### Hardening roadmap

The security jobs are designed for a progressive enforcement strategy:

- **Phase 1 (current):** Observability only -- all jobs are `continue-on-error: true`. Review artifacts after each run to assess baseline noise.
- **Phase 2:** Remove `continue-on-error` on `gosec` and `govulncheck` so that High/Critical findings block PRs. Optionally add severity thresholds (`gosec -severity=high`, `govulncheck` exit code).
- **Phase 3:** Tighten to Medium+ once the baseline is clean; add `gitleaks` to required checks.

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

`APP_VERSION` in [`Makefile`](../Makefile) and [`scripts/bsctl`](../scripts/bsctl) should stay in sync for release builds (`-X main.Version=...`). Maintainers: use an annotated SemVer tag `vX.Y.Z`, push it, publish the GitHub release, and keep the injected version aligned with that tag.

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

Optional **French synopsis translation**: when `BOOKSTORAGE_TRANSLATE_URL` points to a LibreTranslate-compatible service (base URL, `POST /translate`), `GET /api/recommendations/media` translates the description to French for users with the French UI (`lang` cookie), caches results in SQLite (`translation_cache`), and returns `description_translated: true`. Leave unset to keep English-only text.

---

## Contributing

1. Fork and branch from `main` (or open a PR from a branch with a clear name).
2. Run **tests**, **gofmt**, and **golangci-lint** locally; CI must be green.
3. Avoid committing `.env` or secrets; use `.env.example` and `config/site.json.example` as templates.
4. For user-facing behavior changes, update the relevant doc under `docs/` (and `docs/fr/` if you maintain French).

---

[Documentation index](README.md) · [Self-hosting](self-hosting.md)
