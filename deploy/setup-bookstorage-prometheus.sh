#!/usr/bin/env bash
# =============================================================================
# Optional Prometheus sidecar for BookStorage (systemd: bookstorage-prometheus)
# 1) Tries distro package (apt / dnf / yum).
# 2) If no package: downloads official release from GitHub (override with PROMETHEUS_VERSION).
#
# Usage (as root):
#   INSTALL_APP_DIR=/opt/bookstorage bash /opt/bookstorage/deploy/setup-bookstorage-prometheus.sh
# =============================================================================

set -euo pipefail

APP_DIR="${INSTALL_APP_DIR:-/opt/bookstorage}"
# Pin a known-good release; override e.g. PROMETHEUS_VERSION=2.55.2
PROMETHEUS_VERSION="${PROMETHEUS_VERSION:-2.55.1}"

if [ "${EUID:-0}" -ne 0 ]; then
	echo "This script must be run as root (sudo)." >&2
	exit 1
fi

if [ ! -d "$APP_DIR" ]; then
	echo "APP_DIR not found: $APP_DIR" >&2
	exit 1
fi

install_prometheus_pkg() {
	if command -v apt-get >/dev/null 2>&1; then
		DEBIAN_FRONTEND=noninteractive apt-get update -qq || return 1
		DEBIAN_FRONTEND=noninteractive apt-get install -y prometheus || return 1
		return 0
	fi
	if command -v dnf >/dev/null 2>&1; then
		if dnf install -y prometheus 2>/dev/null; then return 0; fi
		if dnf install -y golang-github-prometheus-prometheus 2>/dev/null; then return 0; fi
		return 1
	fi
	if command -v yum >/dev/null 2>&1; then
		yum install -y prometheus 2>/dev/null || return 1
		return 0
	fi
	return 1
}

linux_arch_suffix() {
	case "$(uname -m)" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	armv7l) echo armv7 ;;
	*)
		echo ""
		;;
	esac
}

install_prometheus_from_github() {
	local arch suffix url tmpdir tball extract_dir
	suffix="$(linux_arch_suffix)"
	if [ -z "$suffix" ]; then
		echo "Unsupported machine $(uname -m) for Prometheus upstream tarball." >&2
		return 1
	fi
	arch="linux-${suffix}"
	url="https://github.com/prometheus/prometheus/releases/download/v${PROMETHEUS_VERSION}/prometheus-${PROMETHEUS_VERSION}.${arch}.tar.gz"
	tmpdir="$(mktemp -d)"
	trap 'rm -rf "${tmpdir}"' EXIT
	tball="${tmpdir}/prometheus.tgz"

	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$tball"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$tball" "$url"
	else
		echo "Need curl or wget to download Prometheus." >&2
		return 1
	fi

	tar -xzf "$tball" -C "$tmpdir"
	extract_dir="${tmpdir}/prometheus-${PROMETHEUS_VERSION}.${arch}"
	if [ ! -x "${extract_dir}/prometheus" ]; then
		echo "Extracted archive missing prometheus binary at ${extract_dir}/prometheus" >&2
		return 1
	fi

	install -m 0755 "${extract_dir}/prometheus" /usr/local/bin/prometheus
	if [ -x "${extract_dir}/promtool" ]; then
		install -m 0755 "${extract_dir}/promtool" /usr/local/bin/promtool
	fi
	rm -rf "${tmpdir}"
	trap - EXIT

	echo "Installed Prometheus v${PROMETHEUS_VERSION} to /usr/local/bin/prometheus (upstream tarball)."
	return 0
}

ensure_prometheus_user() {
	if id prometheus >/dev/null 2>&1; then
		return 0
	fi
	if useradd -r -s /sbin/nologin -d /var/lib/prometheus-bookstorage -c "Prometheus TSDB" prometheus 2>/dev/null; then
		return 0
	fi
	echo "Could not create system user 'prometheus'." >&2
	return 1
}

resolve_prometheus_binary() {
	local b
	b="$(command -v prometheus || true)"
	if [ -n "$b" ]; then
		printf '%s' "$b"
		return 0
	fi
	if [ -x /usr/local/bin/prometheus ]; then
		printf '%s' "/usr/local/bin/prometheus"
		return 0
	fi
	if [ -x /usr/bin/prometheus ]; then
		printf '%s' "/usr/bin/prometheus"
		return 0
	fi
	return 1
}

if install_prometheus_pkg; then
	echo "Prometheus installed from distribution package."
else
	echo "No distro prometheus package found; trying official binary (v${PROMETHEUS_VERSION})..."
	if ! install_prometheus_from_github; then
		echo "Could not install Prometheus automatically." >&2
		echo "See docs/self-hosting.md (Prometheus metrics) for a manual setup." >&2
		exit 1
	fi
fi

if ! ensure_prometheus_user; then
	exit 1
fi

PROM_BIN="$(resolve_prometheus_binary)" || {
	echo "prometheus binary not found after install." >&2
	exit 1
}

install -d -m 0755 /etc/bookstorage
install -d -m 0755 /var/lib/prometheus-bookstorage
chown prometheus:prometheus /var/lib/prometheus-bookstorage

PORT=5000
ENV_FILE="$APP_DIR/.env"
if [ -f "$ENV_FILE" ]; then
	line="$(grep -E '^BOOKSTORAGE_PORT=' "$ENV_FILE" | tail -n 1 || true)"
	if [ -n "${line:-}" ]; then
		PORT="${line#BOOKSTORAGE_PORT=}"
		PORT="$(printf '%s' "$PORT" | tr -d '\r' | tr -d '"' | tr -d "'")"
	fi
fi

METRICS_TOKEN=""
if [ -f "$ENV_FILE" ]; then
	line="$(grep -E '^BOOKSTORAGE_METRICS_TOKEN=' "$ENV_FILE" | tail -n 1 || true)"
	if [ -n "${line:-}" ]; then
		METRICS_TOKEN="${line#BOOKSTORAGE_METRICS_TOKEN=}"
		METRICS_TOKEN="$(printf '%s' "$METRICS_TOKEN" | tr -d '\r' | tr -d '"' | tr -d "'")"
	fi
fi

if [ -z "$METRICS_TOKEN" ]; then
	if command -v openssl >/dev/null 2>&1; then
		METRICS_TOKEN="$(openssl rand -hex 24)"
	else
		METRICS_TOKEN="$(tr -dc 'a-zA-Z0-9' </dev/urandom | head -c 48 || true)"
	fi
	if [ -z "$METRICS_TOKEN" ]; then
		echo "Failed to generate BOOKSTORAGE_METRICS_TOKEN." >&2
		exit 1
	fi
	{
		echo ""
		echo "# Added by setup-bookstorage-prometheus.sh"
		echo "BOOKSTORAGE_METRICS_TOKEN=$METRICS_TOKEN"
	} >>"$ENV_FILE"
	chmod 0600 "$ENV_FILE" 2>/dev/null || true
fi

printf '%s\n' "$METRICS_TOKEN" >/etc/bookstorage/bookstorage-metrics.token
chown root:prometheus /etc/bookstorage/bookstorage-metrics.token
chmod 0640 /etc/bookstorage/bookstorage-metrics.token

cat >/etc/bookstorage/prometheus-bs.yml <<EOF
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: bookstorage
    metrics_path: /metrics
    scheme: http
    bearer_token_file: /etc/bookstorage/bookstorage-metrics.token
    static_configs:
      - targets: ['127.0.0.1:${PORT}']
EOF
chmod 0644 /etc/bookstorage/prometheus-bs.yml

cat >/etc/systemd/system/bookstorage-prometheus.service <<UNIT
[Unit]
Description=Prometheus (BookStorage metrics only)
After=network-online.target bookstorage.service
Wants=network-online.target

[Service]
Type=simple
User=prometheus
Group=prometheus
ExecStart=${PROM_BIN} --config.file=/etc/bookstorage/prometheus-bs.yml --storage.tsdb.path=/var/lib/prometheus-bookstorage --web.listen-address=127.0.0.1:9091
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable bookstorage-prometheus.service >/dev/null 2>&1 || true
systemctl restart bookstorage-prometheus.service || systemctl start bookstorage-prometheus.service

echo ""
echo "bookstorage-prometheus configured."
echo "  - Prometheus UI (local): http://127.0.0.1:9091"
echo "  - Scrape target: 127.0.0.1:${PORT}/metrics (bearer token file for Prometheus user)"
echo "Restart BookStorage to pick up BOOKSTORAGE_METRICS_TOKEN if it was just added:"
echo "  systemctl restart bookstorage"
