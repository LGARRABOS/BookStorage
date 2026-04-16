#!/usr/bin/env bash
# Provisions a dedicated PostgreSQL role and database for BookStorage on this host.
# Run on the database VM (Debian/Ubuntu) as a user who can sudo to postgres, or as postgres.
#
# Environment (optional):
#   BS_PG_DB       database name (default: bookstorage)
#   BS_PG_USER     role name (default: bookstorage)
#   BS_PG_HOST     hostname shown in the connection summary (default: hostname -f or hostname)
#   BS_PG_PORT     port shown in summary (default: 5432)
#   BS_PG_SSLMODE  sslmode query param in URL (default: prefer)
#
# Flags:
#   --install-packages   run apt-get update && apt-get install -y postgresql postgresql-contrib (requires sudo)
#   --apt-ipv4           pass -o Acquire::ForceIPv4=true to apt (when IPv6 or some routes hang on "Waiting for headers")
#
# Does NOT open PostgreSQL to the world: adjust listen_addresses and pg_hba.conf yourself
# to allow only your application VM / subnet.

set -euo pipefail

INSTALL_PKGS=0
APT_IPV4=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--install-packages) INSTALL_PKGS=1; shift ;;
		--apt-ipv4) APT_IPV4=1; shift ;;
		-h|--help)
			sed -n '1,45p' "$0"
			exit 0
			;;
		*) echo "unknown option: $1" >&2; exit 1 ;;
	esac
done

# apt: avoid indefinite "Waiting for headers" on bad mirrors / IPv6; allow override via BS_PG_APT_EXTRA_OPTS.
apt_base_args() {
	local -a a=(
		-y
		-o Acquire::http::Timeout=120
		-o Acquire::https::Timeout=120
		-o Acquire::ftp::Timeout=120
		-o Acquire::Retries=3
	)
	if [[ "${APT_IPV4}" -eq 1 ]]; then
		a+=(-o Acquire::ForceIPv4=true)
	fi
	printf '%s\n' "${a[@]}"
}

if [[ "${INSTALL_PKGS}" -eq 1 ]]; then
	if ! command -v apt-get >/dev/null 2>&1; then
		echo "apt-get not found; install PostgreSQL with your distro tools, then re-run without --install-packages." >&2
		exit 1
	fi
	echo "==> apt-get update (timeouts 120s, retries 3; use --apt-ipv4 if this hangs)..." >&2
	mapfile -t _APT_ARGS < <(apt_base_args)
	sudo apt-get "${_APT_ARGS[@]}" update
	echo "==> apt-get install postgresql ..." >&2
	sudo apt-get "${_APT_ARGS[@]}" install postgresql postgresql-contrib
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

if [[ "$(id -u)" -eq 0 ]]; then
	run_psql() { psql -v ON_ERROR_STOP=1 "$@"; }
elif sudo -n true 2>/dev/null; then
	run_psql() { sudo -u postgres psql -v ON_ERROR_STOP=1 "$@"; }
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
echo "Sécurité: restreignez pg_hba.conf à l'IP ou au sous-réseau de la VM applicative ;"
echo "          utilisez sslmode=require ou verify-full si le trafic traverse un réseau non dédié."
