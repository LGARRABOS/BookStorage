#!/usr/bin/env bash
# Provisions a dedicated PostgreSQL role and database for BookStorage on this host.
# Run on the database VM (Debian/Ubuntu) as a user who can sudo to postgres, or as postgres.
#
# Environment (optional):
#   BS_PG_DB                 database name (default: bookstorage)
#   BS_PG_USER               role name (default: bookstorage)
#   BS_PG_HOST               host in printed URL if set; if unset, primary LAN IPv4 when detectable, else hostname -f
#   BS_PG_PORT               port shown in summary (default: 5432)
#   BS_PG_SSLMODE            sslmode for lib/pq: disable, require, verify-ca, verify-full (default: disable for typical LAN)
#   BS_PG_APT_WATCHDOG_SECS  seconds between heartbeat lines while apt runs (default: 25)
#   BS_PG_PSQL_CWD           directory to cd into before sudo -u postgres psql (default: /tmp)
#
# Flags:
#   --install-packages   run apt-get update && apt-get install -y postgresql postgresql-contrib (requires sudo)
#   --apt-ipv4           pass -o Acquire::ForceIPv4=true to apt (when IPv6 or some routes hang on "Waiting for headers")
#   --apt-debug-http     very verbose apt HTTP log (each request; use if "0%" looks stuck)
#   --apt-no-watchdog    disable periodic "[watchdog] apt still running" lines (e.g. CI)
#
# By default apt is also run with conservative HTTP options (no pipelining, few parallel connections)
# to reduce "0% [Waiting for headers]" during package downloads on slow or strict networks.
#
# Does NOT open PostgreSQL to the world: adjust listen_addresses and pg_hba.conf yourself
# to allow only your application VM / subnet.

set -euo pipefail

INSTALL_PKGS=0
APT_IPV4=0
APT_DEBUG_HTTP=0
APT_NO_WATCHDOG=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--install-packages) INSTALL_PKGS=1; shift ;;
		--apt-ipv4) APT_IPV4=1; shift ;;
		--apt-debug-http) APT_DEBUG_HTTP=1; shift ;;
		--apt-no-watchdog) APT_NO_WATCHDOG=1; shift ;;
		-h|--help)
			sed -n '2,/^set -euo pipefail$/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*) echo "unknown option: $1" >&2; exit 1 ;;
	esac
done

# apt: reduce indefinite "Waiting for headers" (indexes + .deb downloads): timeouts, retries, no HTTP pipelining.
apt_base_args() {
	local -a a=(
		-y
		-o Acquire::http::Timeout=300
		-o Acquire::https::Timeout=300
		-o Acquire::ftp::Timeout=300
		-o Acquire::Retries=5
		-o Acquire::http::Pipeline-Depth=0
		-o Acquire::http::MaxConnections=2
	)
	if [[ "${APT_IPV4}" -eq 1 ]]; then
		a+=(-o Acquire::ForceIPv4=true)
	fi
	if [[ "${APT_DEBUG_HTTP}" -eq 1 ]]; then
		a+=(-o Debug::Acquire::http=true)
	fi
	printf '%s\n' "${a[@]}"
}

# Prints a line every N seconds while apt runs so "0% [Waiting for headers]" is not mistaken for a hang.
apt_with_watchdog() {
	local label="$1"
	shift
	if [[ "${APT_NO_WATCHDOG}" -eq 1 ]]; then
		"$@"
		return
	fi
	local step="${BS_PG_APT_WATCHDOG_SECS:-25}"
	(
		local s=0
		while sleep "${step}"; do
			s=$((s + step))
			echo "[watchdog +${s}s] \"${label}\": apt still running (slow network or mirror wait; this line means the process is not frozen)." >&2
		done
	) &
	local wd=$!
	local ec=0
	"$@" || ec=$?
	kill "${wd}" 2>/dev/null || true
	wait "${wd}" 2>/dev/null || true
	return "${ec}"
}

probe_inrelease() {
	local codename="jammy"
	if command -v lsb_release >/dev/null 2>&1; then
		codename="$(lsb_release -cs 2>/dev/null || true)"
	fi
	if [[ -z "${codename}" ]]; then
		codename="jammy"
	fi
	local url="http://archive.ubuntu.com/ubuntu/dists/${codename}/InRelease"
	local curl_opts=(-fSL --connect-timeout 10 --max-time 20 -o /dev/null)
	if [[ "${APT_IPV4}" -eq 1 ]]; then
		curl_opts+=(-4)
	fi
	if ! command -v curl >/dev/null 2>&1; then
		echo "==> (no curl) skipping network probe; install curl for a quick pre-apt check." >&2
		return 0
	fi
	echo "==> Network probe (20s max): InRelease (${codename}) from archive.ubuntu.com …" >&2
	if curl "${curl_opts[@]}" -w "    OK — %{time_total}s, %{size_download} bytes, %{speed_download} B/s\n" "${url}" >&2; then
		return 0
	fi
	echo "    Failed or timed out: apt may sit on \"Waiting for headers\"; check DNS / firewall / mirror." >&2
	return 0
}

if [[ "${INSTALL_PKGS}" -eq 1 ]]; then
	if ! command -v apt-get >/dev/null 2>&1; then
		echo "apt-get not found; install PostgreSQL with your distro tools, then re-run without --install-packages." >&2
		exit 1
	fi
	probe_inrelease
	echo "==> apt-get update (conservative HTTP, 300s timeouts; use --apt-debug-http for detail)…" >&2
	mapfile -t _APT_ARGS < <(apt_base_args)
	apt_with_watchdog "apt-get update" sudo DEBIAN_FRONTEND=noninteractive apt-get "${_APT_ARGS[@]}" update
	echo "==> apt-get install postgresql … (~45 MB; at ~50 kB/s allow ~15–25 min if throughput is steady)" >&2
	apt_with_watchdog "apt-get install" sudo DEBIAN_FRONTEND=noninteractive apt-get "${_APT_ARGS[@]}" install postgresql postgresql-contrib
fi

# Printed BOOKSTORAGE_POSTGRES_URL must use a host the *application* VM can resolve (public DNS does not know internal names).
bs_pg_default_host_for_url() {
	local src=""
	if command -v ip >/dev/null 2>&1; then
		src="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit } }' || true)"
		src="$(echo "${src}" | tr -d '[:space:]')"
	fi
	if [[ -z "${src}" ]] && command -v ip >/dev/null 2>&1; then
		src="$(ip -br -4 addr show scope global 2>/dev/null | awk '{ n = split($3, a, "/"); if (n) { print a[1]; exit } }' | head -1 || true)"
		src="$(echo "${src}" | tr -d '[:space:]')"
	fi
	if [[ -z "${src}" ]] && command -v hostname >/dev/null 2>&1; then
		src="$(hostname -I 2>/dev/null | awk '{ print $1; exit }' || true)"
		src="$(echo "${src}" | tr -d '[:space:]')"
	fi
	if [[ -n "${src}" ]]; then
		printf '%s' "${src}"
		return 0
	fi
	printf '%s' "$(hostname -f 2>/dev/null || hostname)"
}

BS_PG_DB="${BS_PG_DB:-bookstorage}"
BS_PG_USER="${BS_PG_USER:-bookstorage}"
BS_PG_PORT="${BS_PG_PORT:-5432}"
BS_PG_SSLMODE="${BS_PG_SSLMODE:-disable}"
BS_PG_HOST="${BS_PG_HOST:-}"
if [[ -z "${BS_PG_HOST}" ]]; then
	BS_PG_HOST="$(bs_pg_default_host_for_url)"
fi

if ! command -v psql >/dev/null 2>&1; then
	echo "psql not found. Install PostgreSQL client/server or use --install-packages." >&2
	exit 1
fi

# Peer auth maps the Unix user to a PG role: root has no "root" role by default, so never call bare psql as root.
# libpq still chdirs to the *process cwd* (often /home/foo/... after sudo); user postgres cannot traverse /home/foo.
# Run psql from a world-accessible dir; -H sets HOME for postgres. Override with BS_PG_PSQL_CWD if needed.
BS_PG_PSQL_CWD="${BS_PG_PSQL_CWD:-/tmp}"
if [[ "$(id -u)" -eq 0 ]] || sudo -n true 2>/dev/null; then
	run_psql() { ( cd "${BS_PG_PSQL_CWD}" && sudo -H -u postgres psql -v ON_ERROR_STOP=1 "$@" ); }
else
	run_psql() { psql -v ON_ERROR_STOP=1 "$@"; }
fi

if ! run_psql -d postgres -tAc "select 1" >/dev/null 2>&1; then
	echo "Cannot connect to local PostgreSQL (missing socket or cluster stopped)." >&2
	if command -v pg_lsclusters >/dev/null 2>&1; then
		echo "" >&2
		echo "=== pg_lsclusters (cluster status) ===" >&2
		pg_lsclusters >&2 || true
		echo "" >&2
		echo "On Ubuntu/Debian, postgresql.service is often \"active (exited)\": that is not the database engine." >&2
		echo "The real unit is postgresql@<version>-<cluster>, e.g. postgresql@14-main." >&2
		while read -r ver cluster port status _rest; do
			[[ "${ver}" =~ ^[0-9]+$ ]] || continue
			if [[ "${status}" != "online" ]]; then
				echo "  → sudo systemctl start postgresql@${ver}-${cluster}" >&2
			fi
		done < <(pg_lsclusters 2>/dev/null | tail -n +2)
		echo "" >&2
		echo "Trying to start clusters listed as down…" >&2
		while read -r ver cluster port status _rest; do
			[[ "${ver}" =~ ^[0-9]+$ ]] || continue
			if [[ "${status}" != "online" ]]; then
				sudo systemctl start "postgresql@${ver}-${cluster}" 2>/dev/null || true
			fi
		done < <(pg_lsclusters 2>/dev/null | tail -n +2)
		sleep 2
	else
		echo "Install postgresql-common for pg_lsclusters, or start the cluster manually, e.g.:" >&2
		echo "  sudo systemctl start postgresql@14-main" >&2
	fi
fi

if ! run_psql -d postgres -tAc "select 1" >/dev/null 2>&1; then
	echo "" >&2
	echo "Still unreachable. Check logs:" >&2
	echo "  sudo journalctl -u 'postgresql@*' -n 40 --no-pager" >&2
	exit 1
fi

PASS="$(openssl rand -base64 24 | tr -d '\n' | tr '/+' 'AB')"
PASS_ESC="${PASS//\'/\'\'}"

exists_role="$(run_psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${BS_PG_USER}'" | tr -d '[:space:]' || true)"
if [[ "${exists_role}" == "1" ]]; then
	run_psql -d postgres -c "ALTER USER \"${BS_PG_USER}\" WITH PASSWORD '${PASS_ESC}';"
else
	run_psql -d postgres -c "CREATE USER \"${BS_PG_USER}\" WITH PASSWORD '${PASS_ESC}';"
fi

exists_db="$(run_psql -tAc "SELECT 1 FROM pg_database WHERE datname='${BS_PG_DB}'" | tr -d '[:space:]' || true)"
if [[ "${exists_db}" != "1" ]]; then
	run_psql -d postgres -c "CREATE DATABASE \"${BS_PG_DB}\" OWNER \"${BS_PG_USER}\";"
fi

run_psql -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE \"${BS_PG_DB}\" TO \"${BS_PG_USER}\";"

run_psql -d "${BS_PG_DB}" -c "GRANT CREATE ON SCHEMA public TO \"${BS_PG_USER}\";"
run_psql -d "${BS_PG_DB}" -c "GRANT ALL ON SCHEMA public TO \"${BS_PG_USER}\";"

ENC_PASS="${PASS}"
if command -v python3 >/dev/null 2>&1; then
	ENC_PASS="$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "${PASS}")"
fi

URL="postgresql://${BS_PG_USER}:${ENC_PASS}@${BS_PG_HOST}:${BS_PG_PORT}/${BS_PG_DB}?sslmode=${BS_PG_SSLMODE}"

echo ""
echo "=== BookStorage PostgreSQL (copy to secrets / BookStorage admin migration) ==="
echo "BOOKSTORAGE_POSTGRES_URL=${URL}"
echo ""
echo "Separate fields:"
echo "  Host:     ${BS_PG_HOST}"
echo "  Port:     ${BS_PG_PORT}"
echo "  User:     ${BS_PG_USER}"
echo "  Database: ${BS_PG_DB}"
echo "  sslmode:  ${BS_PG_SSLMODE}"
echo "  Password (raw, keep secret): ${PASS}"
echo ""
echo "Remote access (e.g. BookStorage on the LAN):"
echo "  - By default PostgreSQL listens only on localhost. Edit:"
echo "      /etc/postgresql/*/main/postgresql.conf  → listen_addresses = '*' (or your LAN IP)"
echo "      /etc/postgresql/*/main/pg_hba.conf      → e.g. hostnossl all all 192.168.1.0/24 scram-sha-256 (if sslmode=disable)"
echo "    then: sudo systemctl reload postgresql@*-main   # or restart"
echo "  - Firewall: allow port ${BS_PG_PORT}/tcp from the app VM IP only."
echo "  - The URL host is a detected LAN IPv4 when possible (else hostname). To force a name or IP:"
echo "      sudo env BS_PG_HOST=BookStorageDB ./deploy/setup-postgres-vm.sh"
echo "      sudo env BS_PG_HOST=192.168.1.117 ./deploy/setup-postgres-vm.sh"
echo ""
echo "Security: restrict pg_hba.conf to the app VM IP or subnet;"
echo "          on the public Internet use sslmode=require or verify-full (lib/pq does not support \"prefer\")."
