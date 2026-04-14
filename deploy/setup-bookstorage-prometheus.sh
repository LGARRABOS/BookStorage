#!/usr/bin/env bash
# =============================================================================
# Optional Prometheus sidecar for BookStorage (systemd: bookstorage-prometheus)
# Installs distro "prometheus" package when possible, writes scrape config with
# bearer_token_file, enables a dedicated TSDB under /var/lib/prometheus-bookstorage.
#
# Usage (as root), typically invoked by install.sh when INSTALL_WITH_PROMETHEUS=1:
#   INSTALL_APP_DIR=/opt/bookstorage ./deploy/setup-bookstorage-prometheus.sh
# =============================================================================

set -euo pipefail

APP_DIR="${INSTALL_APP_DIR:-/opt/bookstorage}"

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
		DEBIAN_FRONTEND=noninteractive apt-get update -qq
		DEBIAN_FRONTEND=noninteractive apt-get install -y prometheus
		return 0
	fi
	if command -v dnf >/dev/null 2>&1; then
		dnf install -y prometheus 2>/dev/null || dnf install -y golang-github-prometheus-prometheus 2>/dev/null || return 1
		return 0
	fi
	if command -v yum >/dev/null 2>&1; then
		yum install -y prometheus 2>/dev/null || return 1
		return 0
	fi
	return 1
}

if ! install_prometheus_pkg; then
	echo "Could not install a Prometheus package automatically on this distribution." >&2
	echo "See docs/self-hosting.md (Prometheus metrics) for a manual setup." >&2
	exit 1
fi

PROM_BIN="$(command -v prometheus || true)"
if [ -z "$PROM_BIN" ] && [ -x /usr/bin/prometheus ]; then
	PROM_BIN=/usr/bin/prometheus
fi
if [ -z "$PROM_BIN" ]; then
	echo "prometheus binary not found in PATH after package install." >&2
	exit 1
fi

if ! id prometheus >/dev/null 2>&1; then
	echo "System user 'prometheus' is missing after package install." >&2
	exit 1
fi

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
