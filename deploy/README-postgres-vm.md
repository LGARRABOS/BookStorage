# PostgreSQL on a dedicated VM

The script [`setup-postgres-vm.sh`](setup-postgres-vm.sh) provisions an empty database role and database for BookStorage on the PostgreSQL host.

For general production install (systemd, `bsctl`, firewall), see [Self-hosting](../docs/self-hosting.md). Project overview in French: [README.fr.md](../README.fr.md).

---

## Table of contents

- [Network model](#network-model)
- [Usage](#usage)
- [Optional environment variables](#optional-environment-variables)
- [Troubleshooting: slow or stuck apt](#troubleshooting-slow-or-stuck-apt)
- [PostgreSQL not responding (socket)](#postgresql-not-responding-socket)
- [After you run the script](#after-you-run-the-script)

---

## Network model

BookStorage assumes **PostgreSQL runs on another machine reachable from the app VM over a private network** (LAN, VLAN, site-to-site VPN, …) and **port 5432 is not exposed to the public Internet** — the usual security posture.

Practical implications:

- `BOOKSTORAGE_POSTGRES_URL` should use a **private IP** (e.g. `192.168.x.x`) or a **hostname the app VM can resolve** (`/etc/hosts`, internal corporate DNS, …).
- A hostname known only on the Postgres VM (e.g. `BookStorageDB`) **will not** resolve via public DNS (`lookup … no such host`). Fix with `/etc/hosts` or put the IP in the URL.
- On the **Postgres** host you still need **`listen_addresses`** and **`pg_hba.conf`** that allow the **app VM subnet** (see below); the firewall should only permit that internal traffic.

---

## Usage

From the cloned repo root (`BookStorage/`):

```bash
chmod +x deploy/setup-postgres-vm.sh
# Optional: install Debian/Ubuntu packages (sudo required)
sudo ./deploy/setup-postgres-vm.sh --install-packages
# If apt hangs on "Waiting for headers" (often IPv6 or routing to archive.ubuntu.com):
sudo ./deploy/setup-postgres-vm.sh --install-packages --apt-ipv4
# If you cannot tell slow vs stuck: very verbose apt HTTP log
sudo ./deploy/setup-postgres-vm.sh --install-packages --apt-ipv4 --apt-debug-http
# Or when PostgreSQL is already installed (no apt):
./deploy/setup-postgres-vm.sh
```

Avoid `sudo deploy/...` without `./`: depending on the current directory, the shell may not resolve the path as intended.

Running with **`sudo ./deploy/...`** is supported: SQL runs via **`cd /tmp` then `sudo -H -u postgres psql`** (libpq changes to the process working directory; user `postgres` often cannot traverse another user’s home — this avoids the old *could not change directory* warning).

---

## Optional environment variables

| Variable | Purpose |
|----------|---------|
| `BS_PG_DB` | Database name (default: `bookstorage`) |
| `BS_PG_USER` | Role name (default: `bookstorage`) |
| `BS_PG_HOST` | Host in the printed URL; if unset, the script uses a detected **LAN IPv4** when possible so the app VM does not depend on internal DNS |
| `BS_PG_PORT` | Port (default: `5432`) |
| `BS_PG_SSLMODE` | `sslmode` for `lib/pq`: `disable`, `require`, `verify-ca`, `verify-full` (default: `disable` for typical LAN) |
| `BS_PG_APT_WATCHDOG_SECS` | Seconds between `[watchdog …]` lines while apt runs (default: `25`) |
| `BS_PG_PSQL_CWD` | Directory to `cd` into before `sudo -u postgres psql` (default: `/tmp`) |

While apt runs, the script prints a **`[watchdog +…s]`** line to stderr every N seconds: if it keeps appearing, the process **is not frozen** (slow network or mirror wait). **`--apt-debug-http`** logs each HTTP request (verbose but shows whether anything is moving).

---

## Troubleshooting: slow or stuck apt

- **Symptom**: `0% [Waiting for headers]` on `archive.ubuntu.com` or `security.ubuntu.com` — sometimes **`apt-get update` succeeds** then **`apt-get install` stays at 0%** (package downloads are not the same connections as the index fetch).
- **Updated script**: the repo uses conservative apt options (no HTTP pipelining, few parallel connections, longer timeouts). Run `git pull` and retry.
- **Try**: **`--apt-ipv4`** if you suspect IPv6 or routing issues.
- **Slow but OK**: at ~50–60 kB/s, ~40–45 MB of packages can take **10–20 minutes** without being stuck; if the percentage advances in bursts, let it finish.
- **`git pull` refuses to merge**: `git stash push -m wip` (or discard local changes to `deploy/setup-postgres-vm.sh`), or re-clone the repo.
- **Manual**: `sudo apt-get update` then `sudo apt-get install -y postgresql postgresql-contrib` with the same network policy your environment uses (mirror, proxy, `-o Acquire::http::Pipeline-Depth=0`, …).
- **Workaround**: install PostgreSQL with your usual method on the VM, then **`./deploy/setup-postgres-vm.sh`** without `--install-packages`.

---

## PostgreSQL not responding (socket)

The script uses the local Unix socket. If the server **is not running**, the socket may be missing.

### Ubuntu / Debian: `postgresql.service` is `active (exited)`

That is **expected**: `postgresql.service` is a meta-unit (`ExecStart=/bin/true`); it does **not** start the PostgreSQL engine.

List clusters and their state:

```bash
pg_lsclusters
```

Start the **cluster** (adjust `14` / `main` to match your output):

```bash
sudo systemctl start postgresql@14-main
sudo systemctl status postgresql@14-main
sudo systemctl enable postgresql@14-main   # at boot
```

Then re-run `./deploy/setup-postgres-vm.sh` (the script also tries to start clusters listed as `down` in `pg_lsclusters`).

### Cluster fails to start (`could not bind IPv4`, `could not create any listen sockets`)

`systemctl` truncates log lines; read the PostgreSQL journal for the exact error:

```bash
sudo tail -100 /var/log/postgresql/postgresql-14-main.log
# or
sudo journalctl -u postgresql@14-main -n 80 --no-pager
```

Common causes:

1. **Port 5432 already in use** (another Postgres, Docker, another service):
   ```bash
   sudo ss -lntp | grep 5432
   ```
   Stop the conflicting process or change `port` in `/etc/postgresql/14/main/postgresql.conf`.

2. **`listen_addresses` binds an IP this VM does not have** (*Cannot assign requested address*). Edit `/etc/postgresql/14/main/postgresql.conf` and use at least for a quick validation:
   - `listen_addresses = 'localhost'` or `'*'`
   Then: `sudo systemctl restart postgresql@14-main`.

3. **Disk space** on `/var`: `df -h /var/lib/postgresql`.

---

## After you run the script

1. Copy the line `BOOKSTORAGE_POSTGRES_URL=...` (or the printed fields) into `.env` on the **app VM**, or enter them in **Admin → PostgreSQL** (superadmin, SQLite → PostgreSQL migration). Ensure the URL is **complete**; for `sslmode`, the Go driver (`lib/pq`) accepts **`disable`**, **`require`**, **`verify-ca`**, **`verify-full`** (not `prefer` — the script and the app normalize or default to `disable` on LAN).

2. **Connections from another host**: on Ubuntu/Debian PostgreSQL often listens **only on `127.0.0.1`**. Then:
   - In **`postgresql.conf`** (often `/etc/postgresql/14/main/postgresql.conf`): `listen_addresses = '*'` or this VM’s LAN IP.
   - In **`pg_hba.conf`** (same directory): a rule for the app VM. With **`sslmode=disable`** (common on LAN), PostgreSQL reports *no encryption*: prefer a **`hostnossl`** line (TCP **without** TLS), for example  
     `hostnossl  all  all  192.168.1.0/24  scram-sha-256`  
     or, more restrictive, only the app:  
     `hostnossl  bookstorage  bookstorage  192.168.1.116/32  scram-sha-256`  
     (adapt IP and subnet). **`host`** lines often work too; place your rule **above** any broad `reject` / `all all all reject` lines.
   - **`sudo systemctl reload postgresql@14-main`** (or `restart`) after edits.
   - Open the **firewall** for `5432/tcp` **only** from the app VM’s IP.

3. If the app returns **`connect_failed`**: put the **IP** in the URL if the Postgres VM hostname is not resolvable on the app VM (`dial tcp: lookup …`). After updating BookStorage, the admin connectivity test also includes a **`detail`** field with the PostgreSQL/driver message.

4. Application table schema is created on first BookStorage start (`EnsureSchema`); this script does not create the app schema separately.

To print the URL with a specific host or IP (instead of the auto-detected value), on the Postgres VM run:  
`sudo env BS_PG_HOST=192.168.1.117 ./deploy/setup-postgres-vm.sh` (adjust the IP or hostname).
