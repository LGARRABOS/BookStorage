#!/usr/bin/env bash
# Provisions a dedicated PostgreSQL role and database for BookStorage on this host.
# Run on the database VM (Debian/Ubuntu) as a user who can sudo to postgres, or as postgres.
#
# Environment (optional):
#   BS_PG_DB                 database name (default: bookstorage)
#   BS_PG_USER               role name (default: bookstorage)
#   BS_PG_HOST               hostname shown in the connection summary (default: hostname -f or hostname)
#   BS_PG_PORT               port shown in summary (default: 5432)
#   BS_PG_SSLMODE            sslmode query param in URL (default: prefer)
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
			echo "[watchdog +${s}s] «${label}» : apt est toujours actif (réseau lent ou attente du miroir ; ce message indique que le processus n'est pas figé)." >&2
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
		echo "==> (pas de curl) sonde réseau ignorée ; installez curl pour un test avant apt." >&2
		return 0
	fi
	echo "==> Sonde réseau (20s max) : InRelease (${codename}) sur archive.ubuntu.com …" >&2
	if curl "${curl_opts[@]}" -w "    OK — durée %{time_total}s, %{size_download} octets, débit %{speed_download} o/s\n" "${url}" >&2; then
		return 0
	fi
	echo "    Échec ou timeout : apt risque de rester longtemps sur « Waiting for headers » ; vérifiez DNS / pare-feu / miroir." >&2
	return 0
}

if [[ "${INSTALL_PKGS}" -eq 1 ]]; then
	if ! command -v apt-get >/dev/null 2>&1; then
		echo "apt-get not found; install PostgreSQL with your distro tools, then re-run without --install-packages." >&2
		exit 1
	fi
	probe_inrelease
	echo "==> apt-get update (HTTP conservateur, timeouts 300s ; --apt-debug-http pour le détail)…" >&2
	mapfile -t _APT_ARGS < <(apt_base_args)
	apt_with_watchdog "apt-get update" sudo DEBIAN_FRONTEND=noninteractive apt-get "${_APT_ARGS[@]}" update
	echo "==> apt-get install postgresql … (~45 Mo ; à ~50 kB/s compter ~15–25 min si le débit est stable)" >&2
	apt_with_watchdog "apt-get install" sudo DEBIAN_FRONTEND=noninteractive apt-get "${_APT_ARGS[@]}" install postgresql postgresql-contrib
fi

BS_PG_DB="${BS_PG_DB:-bookstorage}"
BS_PG_USER="${BS_PG_USER:-bookstorage}"
BS_PG_PORT="${BS_PG_PORT:-5432}"
BS_PG_SSLMODE="${BS_PG_SSLMODE:-prefer}"
BS_PG_HOST="${BS_PG_HOST:-}"
if [[ -z "${BS_PG_HOST}" ]]; then
	BS_PG_HOST="$(hostname -f 2>/dev/null || hostname)"
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
echo "Champs séparés :"
echo "  Hôte:     ${BS_PG_HOST}"
echo "  Port:     ${BS_PG_PORT}"
echo "  Utilisateur: ${BS_PG_USER}"
echo "  Base:     ${BS_PG_DB}"
echo "  sslmode:  ${BS_PG_SSLMODE}"
echo "  Mot de passe (brut, à protéger): ${PASS}"
echo ""
echo "Connexion depuis une autre VM (ex. BookStorage sur le LAN) :"
echo "  - Par défaut PostgreSQL n'écoute que sur localhost. Éditer :"
echo "      /etc/postgresql/*/main/postgresql.conf  → listen_addresses = '*' (ou votre IP LAN)"
echo "      /etc/postgresql/*/main/pg_hba.conf      → ex. host all all 192.168.1.0/24 scram-sha-256"
echo "    puis : sudo systemctl restart postgresql"
echo "  - Pare-feu : autoriser le port ${BS_PG_PORT}/tcp depuis l'IP de la VM applicative."
echo "  - Dans l'URL, si « ${BS_PG_HOST} » n'est pas résolu par l'app, remplacez par l'IP (ex. 192.168.1.117)."
echo "    Exemple : sudo env BS_PG_HOST=192.168.1.117 ./deploy/setup-postgres-vm.sh"
echo ""
echo "Sécurité: restreignez pg_hba.conf à l'IP ou au sous-réseau de la VM applicative ;"
echo "          utilisez sslmode=require ou verify-full si le trafic traverse un réseau non dédié."
