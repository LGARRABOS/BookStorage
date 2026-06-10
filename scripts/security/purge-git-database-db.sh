#!/usr/bin/env bash
# =============================================================================
# BookStorage — purge database.db from Git history (operator manual action)
# =============================================================================
#
# PURPOSE
#   If database.db was ever committed, removing it from the working tree is not
#   enough: clones still contain the blobs in history. This script documents the
#   recommended git-filter-repo procedure. It does NOT run automatically.
#
# WARNING
#   - Rewrites Git history (all commit SHAs change).
#   - Requires force-push and coordination with every clone/fork.
#   - Rotate exposed credentials (password hashes, session secrets, etc.).
#   - Run only from a trusted machine with a fresh backup of the remote.
#
# PREREQUISITES
#   pip install git-filter-repo   # or distro package git-filter-repo
#   git clone --mirror <url> bookstorage-mirror.git && cd bookstorage-mirror.git
#
# USAGE (review each step before executing)
#   1. Read this file completely.
#   2. Export DRY_RUN=1 and run:  bash scripts/security/purge-git-database-db.sh
#      (prints commands only — default when DRY_RUN unset is also dry-run.)
#   3. When ready, run manually on a mirror checkout:
#
#      git filter-repo --path database.db --invert-paths --force
#
#   4. Verify history:
#      git log --all -- database.db    # should be empty
#      git rev-list --objects --all | grep database.db || echo "OK: no blob"
#
#   5. Force-push (after team agreement):
#      git push --force --all
#      git push --force --tags
#
#   6. Invalidate GitHub caches / ask collaborators to re-clone.
#
# ALTERNATIVE: BFG Repo-Cleaner
#   bfg --delete-files database.db
#   git reflog expire --expire=now --all && git gc --prune=now --aggressive
#
# This wrapper defaults to dry-run and refuses to mutate the repo unless
# PURGE_GIT_DATABASE_DB_EXECUTE=1 is set (still requires explicit git-filter-repo).
# =============================================================================

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
TARGET_PATH="database.db"

if [[ -z "${REPO_ROOT}" ]]; then
	echo "error: not inside a git repository" >&2
	exit 1
fi

cd "${REPO_ROOT}"

echo "=== BookStorage: purge ${TARGET_PATH} from Git history ==="
echo "Repo: ${REPO_ROOT}"
echo ""

if ! git log --all --oneline -- "${TARGET_PATH}" 2>/dev/null | head -1 | grep -q .; then
	echo "No commits touching ${TARGET_PATH} found in this clone (may already be purged)."
	exit 0
fi

echo "Commits touching ${TARGET_PATH} (sample):"
git log --all --oneline -- "${TARGET_PATH}" | head -10
echo ""

FILTER_CMD=(git filter-repo --path "${TARGET_PATH}" --invert-paths --force)

echo "Recommended command:"
printf '  %q' "${FILTER_CMD[@]}"
echo ""
echo ""

if [[ "${PURGE_GIT_DATABASE_DB_EXECUTE:-}" != "1" ]]; then
	echo "Dry-run only (history NOT modified)."
	echo "To run filter-repo yourself on a mirror, execute the command above."
	echo "This script will not invoke filter-repo unless PURGE_GIT_DATABASE_DB_EXECUTE=1."
	exit 0
fi

if ! command -v git-filter-repo >/dev/null 2>&1; then
	echo "error: git-filter-repo not installed (pip install git-filter-repo)" >&2
	exit 1
fi

echo "PURGE_GIT_DATABASE_DB_EXECUTE=1 — running git-filter-repo in 5 seconds (Ctrl+C to abort)…"
sleep 5
"${FILTER_CMD[@]}"
echo "Done. Verify with: git log --all -- ${TARGET_PATH}"
