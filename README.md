# BookStorage

**BookStorage** is a personal reading tracker web application. Track your novels, manga, webtoons, light novels and more.

_🇫🇷 [Version française](./README.fr.md)_

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## About BookStorage

BookStorage is a **self-hosted** web app for readers who want a single place to **catalogue** what they read—novels, manga, webtoons, light novels, and similar formats—and **follow progress** over time (planned, reading, completed, dropped). You can attach **ratings and notes**, explore **statistics** on your habits, and use **community libraries** to discover what others have made public, without giving up control of your data.

The interface is built for **everyday use**: responsive layout, **dark mode**, **English and French**, optional **PWA** install, and **keyboard shortcuts** for power users. Everything runs on **your** machine or server: a **SQLite** database stores your library locally, with **export/import** when you need backups or a fresh install.

---

## Features

- Multi-format library (novels, manga, webtoons, light novels…)
- Ratings, notes, statistics, public community libraries
- Dark mode, multilingual UI (EN/FR), PWA
- Export/import (CSV, JSON), keyboard shortcuts

---

## Documentation

| | |
|---|---|
| **Self-host** (Linux server, systemd, configuration, daily use) | [docs/self-hosting.md](docs/self-hosting.md) |
| **Development** (local dev, CI/CD, `bsctl` reference, repo layout) | [docs/development.md](docs/development.md) |

Full index: [docs/README.md](docs/README.md)

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

Details: [Self-hosting guide](docs/self-hosting.md).

---

## License

MIT License
