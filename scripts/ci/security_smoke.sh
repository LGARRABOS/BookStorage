#!/usr/bin/env bash
# =============================================================================
# BookStorage - Security DAST smoke tests
# =============================================================================
# Lightweight checks run against a live instance in CI.
# Exit code 0 = all checks passed; non-zero = at least one failure.
#
# Usage:
#   BASE_URL=http://127.0.0.1:5000 ./scripts/ci/security_smoke.sh
# =============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:5000}"
PASS=0
FAIL=0
REPORT=""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log_pass() {
  PASS=$((PASS + 1))
  REPORT+="  PASS  $1\n"
  echo "  PASS  $1"
}

log_fail() {
  FAIL=$((FAIL + 1))
  REPORT+="  FAIL  $1\n"
  echo "  FAIL  $1"
}

check_header() {
  local url="$1" header="$2" label="$3"
  if curl -sI "$url" | grep -qi "^${header}:"; then
    log_pass "$label"
  else
    log_fail "$label"
  fi
}

# ---------------------------------------------------------------------------
# 1) Security headers on public pages
# ---------------------------------------------------------------------------

echo ""
echo "=== Security headers ==="

for page in "/" "/login" "/register"; do
  check_header "${BASE_URL}${page}" "X-Content-Type-Options" "X-Content-Type-Options on ${page}"
  check_header "${BASE_URL}${page}" "X-Frame-Options"        "X-Frame-Options on ${page}"
  check_header "${BASE_URL}${page}" "Referrer-Policy"        "Referrer-Policy on ${page}"
  check_header "${BASE_URL}${page}" "Content-Security-Policy" "Content-Security-Policy on ${page}"
  check_header "${BASE_URL}${page}" "Permissions-Policy"     "Permissions-Policy on ${page}"
done

# ---------------------------------------------------------------------------
# 2) Prometheus /metrics (loopback scrape, no token in dev)
# ---------------------------------------------------------------------------

echo ""
echo "=== /metrics (Prometheus) ==="

status=$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}/metrics")
if [ "$status" = "200" ]; then
  log_pass "GET /metrics => 200"
else
  log_fail "GET /metrics => ${status} (expected 200 on loopback dev)"
fi

# ---------------------------------------------------------------------------
# 3) API access control without session (expect 401)
# ---------------------------------------------------------------------------

echo ""
echo "=== API auth (no session => 401) ==="

for endpoint in "/api/works" "/api/stats" "/api/recommendations"; do
  status=$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}${endpoint}")
  if [ "$status" = "401" ]; then
    log_pass "GET ${endpoint} => 401 (no session)"
  else
    log_fail "GET ${endpoint} => ${status} (expected 401)"
  fi
done

# ---------------------------------------------------------------------------
# 4) Wrong HTTP method (expect 405)
# ---------------------------------------------------------------------------

echo ""
echo "=== Method not allowed ==="

# DELETE on /login should not be accepted
status=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${BASE_URL}/login")
if [ "$status" = "405" ]; then
  log_pass "DELETE /login => 405"
else
  log_fail "DELETE /login => ${status} (expected 405)"
fi

# PUT on /register
status=$(curl -s -o /dev/null -w '%{http_code}' -X PUT "${BASE_URL}/register")
if [ "$status" = "405" ]; then
  log_pass "PUT /register => 405"
else
  log_fail "PUT /register => ${status} (expected 405)"
fi

# ---------------------------------------------------------------------------
# 5) CSRF protection (mutating request with foreign origin)
# ---------------------------------------------------------------------------

echo ""
echo "=== CSRF origin check ==="

# POST /api/works with a fake session cookie and a foreign Origin header
# should be blocked with 403 (csrf_blocked).
status=$(curl -s -o /dev/null -w '%{http_code}' \
  -X POST "${BASE_URL}/api/works" \
  -H "Cookie: session=fake-session-value" \
  -H "Origin: https://evil.example.com" \
  -H "Content-Type: application/json" \
  -d '{}')
if [ "$status" = "403" ]; then
  log_pass "POST /api/works with foreign Origin => 403 (CSRF blocked)"
else
  log_fail "POST /api/works with foreign Origin => ${status} (expected 403)"
fi

# Same for a non-API mutating route
status=$(curl -s -o /dev/null -w '%{http_code}' \
  -X POST "${BASE_URL}/login" \
  -H "Cookie: session=fake-session-value" \
  -H "Origin: https://evil.example.com" \
  -d "username=test&password=test")
if [ "$status" = "403" ]; then
  log_pass "POST /login with foreign Origin => 403 (CSRF blocked)"
else
  log_fail "POST /login with foreign Origin => ${status} (expected 403)"
fi

# ---------------------------------------------------------------------------
# 6) Rate limiting on auth endpoints
# ---------------------------------------------------------------------------

echo ""
echo "=== Rate limiting (auth) ==="

# The auth bucket has capacity 8, refill 0.5/s.
# Send 12 rapid POST /login requests; at least one should get 429.
GOT_429=0
for i in $(seq 1 12); do
  status=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${BASE_URL}/login" \
    -H "Origin: ${BASE_URL}" \
    -d "username=ci-ratelimit-probe&password=wrong")
  if [ "$status" = "429" ]; then
    GOT_429=1
    break
  fi
done

if [ "$GOT_429" = "1" ]; then
  log_pass "POST /login rate limit triggered (429)"
else
  log_fail "POST /login rate limit NOT triggered after 12 requests"
fi

# ---------------------------------------------------------------------------
# 7) Admin routes without auth (expect redirect to login)
# ---------------------------------------------------------------------------

echo ""
echo "=== Admin access without auth ==="

status=$(curl -s -o /dev/null -w '%{http_code}' -L -o /dev/null "${BASE_URL}/admin/accounts")
# Follow redirects; final page should be /login (200) or initial 302 to /login
status_no_follow=$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}/admin/accounts")
if [ "$status_no_follow" = "302" ] || [ "$status_no_follow" = "303" ] || [ "$status_no_follow" = "401" ] || [ "$status_no_follow" = "403" ]; then
  log_pass "GET /admin/accounts without session => ${status_no_follow}"
else
  log_fail "GET /admin/accounts without session => ${status_no_follow} (expected 302/303/401/403)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
echo "==========================================="
echo "  Security smoke results: ${PASS} passed, ${FAIL} failed"
echo "==========================================="
echo ""

# Write machine-readable report for CI artifact
REPORT_FILE="${REPORT_FILE:-security-smoke-report.txt}"
{
  echo "Security DAST Smoke Report"
  echo "========================="
  echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo "Base URL: ${BASE_URL}"
  echo ""
  echo -e "${REPORT}"
  echo ""
  echo "Total: ${PASS} passed, ${FAIL} failed"
} > "${REPORT_FILE}"

if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
