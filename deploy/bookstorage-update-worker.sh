#!/bin/bash
set -euo pipefail

REQ="/var/lib/bookstorage/update/request.json"
STATUS="/var/lib/bookstorage/update/status.json"

ts() { date +%s; }

write_status() {
  local running="$1"
  local ok="$2"
  local mode="$3"
  local tag="$4"
  local message="$5"
  local command="$6"
  local output="$7"

  mkdir -p "$(dirname "$STATUS")"
  cat > "${STATUS}.tmp" <<EOF
{
  "running": ${running},
  "last": {
    "ok": ${ok},
    "mode": "$(printf '%s' "$mode" | sed 's/"/\\"/g')",
    "tag": "$(printf '%s' "$tag" | sed 's/"/\\"/g')",
    "message": "$(printf '%s' "$message" | sed 's/"/\\"/g')",
    "started_at_unix": $(ts),
    "command": "$(printf '%s' "$command" | sed 's/"/\\"/g')",
    "output": "$(printf '%s' "$output" | sed 's/"/\\"/g')"
  }
}
EOF
  mv "${STATUS}.tmp" "$STATUS"
}

if [[ ! -f "$REQ" ]]; then
  exit 0
fi

mode="$(sed -n 's/.*"mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$REQ" | head -n1 || true)"
tag="$(sed -n 's/.*"tag"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$REQ" | head -n1 || true)"

if [[ -z "$tag" ]]; then
  write_status false false "${mode:-unknown}" "" "invalid_request" "" "missing tag in request.json"
  rm -f "$REQ" || true
  exit 0
fi

cmd="BSCTL_UPDATE_TAG=${tag} /usr/local/bin/bsctl update"
write_status true false "${mode:-unknown}" "$tag" "running" "$cmd" ""

set +e
out="$(BSCTL_UPDATE_TAG="${tag}" /usr/local/bin/bsctl update 2>&1)"
code=$?
set -e

if [[ $code -eq 0 ]]; then
  write_status false true "${mode:-unknown}" "$tag" "done" "$cmd" "$out"
else
  write_status false false "${mode:-unknown}" "$tag" "failed" "$cmd" "$out"
fi

rm -f "$REQ" || true

