#!/bin/bash
# ============================================================================
# bsctl library (sourced by scripts/bsctl)
# ============================================================================

# ============================================================================
# Utility functions
# ============================================================================

print_header() {
    printf "\n"
    printf "${BOLD}╔════════════════════════════════════════╗${NC}\n"
    printf "${BOLD}║  📚 BookStorage Control v${APP_VERSION}         ║${NC}\n"
    printf "${BOLD}╚════════════════════════════════════════╝${NC}\n"
    printf "\n"
}

print_success() {
    printf "${GREEN}✓ $1${NC}\n"
}

print_error() {
    printf "${RED}✗ $1${NC}\n"
}

print_info() {
    printf "${BLUE}▶ $1${NC}\n"
}

print_step() {
    printf "${BLUE}[$1]${NC} $2\n"
}

# Copy shell text stripping CR bytes (CRLF checkout); does not alter the five ASCII chars of $'\r' in source.
bsctl_install_script_strip_cr() {
    local src="$1" dst="$2" chmod_mode="$3"
    [[ -f "$src" ]] || return 1
    tr -d '\r' <"$src" >"${dst}.bsctl~$$" || return 1
    chmod "$chmod_mode" "${dst}.bsctl~$$" || return 1
    mv -f "${dst}.bsctl~$$" "$dst"
}

# Validates a git branch name (alphanumeric, . _ / - only; no ..)
# Copy bash completion: /etc/bash_completion.d (legacy) and
# /usr/share/bash-completion/completions (Debian/Ubuntu bash-completion package).
install_bsctl_bash_completion() {
    local repo="$1"
    [[ -n "$repo" && -f "${repo}/scripts/bsctl.completion.bash" ]] || return 0
    local src="${repo}/scripts/bsctl.completion.bash"
    local did=0
    if [[ -d /etc/bash_completion.d ]]; then
        bsctl_install_script_strip_cr "$src" /etc/bash_completion.d/bsctl 644
        did=1
    fi
    if [[ -d /usr/share/bash-completion/completions ]]; then
        bsctl_install_script_strip_cr "$src" /usr/share/bash-completion/completions/bsctl 644
        did=1
    fi
    if [[ "$did" -eq 1 ]]; then
        print_success "Bash completion refreshed (new login shell, or: hash -r)"
    fi
}

validate_git_branch_name() {
    local b="$1"
    if [[ -z "$b" ]]; then
        return 1
    fi
    if [[ "$b" == -* ]]; then
        return 1
    fi
    if [[ "$b" == *..* ]]; then
        return 1
    fi
    if [[ ! "$b" =~ ^[a-zA-Z0-9._/-]+$ ]]; then
        return 1
    fi
    return 0
}

# Trim spaces; add leading v if version looks like 1.2.3
normalize_tag() {
    local t="$1"
    t="${t#"${t%%[![:space:]]*}"}"
    t="${t%"${t##*[![:space:]]}"}"
    if [[ -z "$t" ]]; then
        return 1
    fi
    if [[ "$t" =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        t="v${t}"
    fi
    printf '%s' "$t"
}

latest_major_release_tag() {
    git tag -l 'v*' 2>/dev/null | grep -E '^v[0-9]+\.0\.0$' | sort -V | tail -n 1
}

latest_non_major_release_tag() {
    git tag -l 'v*' 2>/dev/null | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | grep -Ev '^v[0-9]+\.0\.0$' | sort -V | tail -n 1
}

# Interactive choice: writes selected tag into the variable whose name is passed as $1 (nameref, bash 4.3+).
# Do not use tag=$(prompt_release_choice): subshell breaks TTY / captures menu on stdout.
prompt_release_choice() {
    local -n __out_ref="$1"

    if [[ ! -t 0 ]] && [[ ! -r /dev/tty ]]; then
        print_error "No interactive terminal for the menu."
        printf "Set ${BOLD}BSCTL_UPDATE_TAG=vX.Y.Z${NC} or run ${BOLD}bsctl update main${NC}.\n"
        exit 1
    fi

    git fetch -q origin
    git fetch -q origin --tags 2>/dev/null || git fetch -q --tags origin 2>/dev/null || true
    printf "\n"

    local major_tag patch_tag
    major_tag=$(latest_major_release_tag)
    patch_tag=$(latest_non_major_release_tag)

    printf "\n${BOLD}── Deploy version ──${NC}\n"
    printf "Recent tags:\n\n"
    if [[ -n "$major_tag" ]]; then
        printf "  ${BOLD}1.${NC}  %s  ${YELLOW}(latest major vX.0.0)${NC}\n" "$major_tag"
    else
        printf "  ${BOLD}1.${NC}  ${YELLOW}— (no major tags found)${NC}\n"
    fi
    if [[ -n "$patch_tag" ]]; then
        printf "  ${BOLD}2.${NC}  %s  ${YELLOW}(latest non-major vX.Y.Z)${NC}\n" "$patch_tag"
    else
        printf "  ${BOLD}2.${NC}  ${YELLOW}— (no non-major tags found)${NC}\n"
    fi
    printf "  ${BOLD}Tag:${NC} type a tag directly (e.g. v5.1.0)\n"
    printf "\n"

    local choice normalized
    while true; do
        printf '\n'
        printf $'\033[1m→\033[0m Enter \033[1m1\033[0m (latest major), \033[1m2\033[0m (latest non-major), type a tag (vX.Y.Z), or \033[1mEnter\033[0m to cancel: '
        if [[ -r /dev/tty ]]; then
            read -r choice < /dev/tty
        else
            read -r choice
        fi || true
        choice=$(printf '%s' "$choice" | tr '[:upper:]' '[:lower:]')
        if [[ -z "$choice" ]]; then
            print_error "Cancelled."
            exit 1
        fi
        if [[ "$choice" == "1" ]]; then
            if [[ -n "$major_tag" ]]; then
                __out_ref="$major_tag"
                return 0
            fi
            print_error "No major tag available for choice 1."
            continue
        fi
        if [[ "$choice" == "2" ]]; then
            if [[ -n "$patch_tag" ]]; then
                __out_ref="$patch_tag"
                return 0
            fi
            print_error "No non-major tag available for choice 2."
            continue
        fi

        if ! normalized=$(normalize_tag "$choice"); then
            print_error "Empty tag."
            continue
        fi
        __out_ref="$normalized"
        return 0
    done
}

# Shared tail: build, install, restart (steps 4–8)
cmd_update_finish() {
    local build_version="${1:-}"
    local repo
    repo="$(bsctl_repo_dir)" || { print_error "Repository not found."; printf "Expected at ${BOLD}${INSTALL_DIR:-/opt/bookstorage}${NC}\n"; exit 1; }

    print_step "4/8" "Compiling..."
    if [[ -z "$build_version" && -f "${repo}/scripts/bsctl" ]]; then
        build_version="$(sed -n 's/^APP_VERSION="\([^"]\+\)".*/\1/p' "${repo}/scripts/bsctl" | head -n 1 || true)"
    fi
    if [[ -n "$build_version" ]]; then
        cmd_build_prod "$build_version"
    else
        cmd_build_prod
    fi
    printf "\n"

    print_step "5/8" "Installing binary..."
    cp "${repo}/${APP_NAME}" ${BIN_DIR}/
    print_success "Binary installed to ${BIN_DIR}/"
    printf "\n"

    print_step "6/8" "Updating bsctl script..."
    bsctl_install_script_strip_cr "${repo}/scripts/bsctl" "${BIN_DIR}/bsctl" 755
    bsctl_install_script_strip_cr "${repo}/scripts/bsctl.lib.sh" "${BIN_DIR}/bsctl.lib.sh" 644
    print_success "bsctl updated"
    install_bsctl_bash_completion "$repo"
    printf "\n"

    print_step "7/8" "Fixing permissions..."
    cmd_fix_perms
    printf "\n"

    print_step "8/8" "Restarting service..."
    if ! systemctl restart ${APP_NAME}; then
        printf "\n"
        print_error "systemctl restart failed."
        printf "${YELLOW}Last journal lines:${NC}\n"
        journalctl -u ${APP_NAME} -n 20 --no-pager 2>/dev/null || true
        printf "\n${BLUE}More detail:${NC} sudo journalctl -u ${APP_NAME} -xe\n"
        printf "${BLUE}Service status:${NC} sudo systemctl status ${APP_NAME}\n"
        exit 1
    fi

    sleep 0.5
    printf "\n"

    if systemctl is-active --quiet ${APP_NAME}; then
        print_success "Service restarted and running."
        printf "\n"
        printf "${GREEN}╔════════════════════════════════════════╗${NC}\n"
        printf "${GREEN}║         UPDATE COMPLETE ✓              ║${NC}\n"
        printf "${GREEN}╚════════════════════════════════════════╝${NC}\n"
        printf "\n"
    else
        print_error "Service is not active after restart."
        printf "${YELLOW}Last journal lines:${NC}\n"
        journalctl -u ${APP_NAME} -n 25 --no-pager 2>/dev/null || true
        printf "\n${BLUE}To diagnose:${NC}\n"
        printf "  sudo journalctl -u ${APP_NAME} -xe\n"
        printf "  sudo systemctl status ${APP_NAME}\n"
        exit 1
    fi
}

cmd_update_at_tag() {
    local tag="$1"
    if [[ -z "$tag" ]]; then
        print_error "No tag selected."
        exit 1
    fi

    # If the requested version is already installed, avoid rebuild/restart.
    # We still print a clear message for the user.
    local target_version="${tag#v}"
    local installed_tag=""
    local installed_out=""
    local bookstorage_bin=""
    if [[ -x "${BIN_DIR}/${APP_NAME}" ]]; then
        bookstorage_bin="${BIN_DIR}/${APP_NAME}"
    elif command -v "${APP_NAME}" >/dev/null 2>&1; then
        bookstorage_bin="$(command -v "${APP_NAME}")"
    fi
    if [[ -n "$bookstorage_bin" ]]; then
        installed_out="$("$bookstorage_bin" -v 2>/dev/null || true)"
        # Parse "BookStorage vX.Y.Z" robustly (avoid relying on grep -o).
        installed_tag="$(printf '%s\n' "$installed_out" | sed -n 's/.*\(v[0-9]\+\.[0-9]\+\.[0-9]\+\).*/\1/p' | head -n 1 || true)"
        # Fallback: if parsing fails but the raw output contains the target version, accept it.
        if [[ -z "$installed_tag" && -n "$target_version" ]]; then
            if printf '%s' "$installed_out" | grep -Fq "v${target_version}"; then
                installed_tag="v${target_version}"
            elif printf '%s' "$installed_out" | grep -Fq "${target_version}"; then
                installed_tag="v${target_version}"
            fi
        fi
        if [[ -n "$installed_tag" && "$installed_tag" == "$tag" ]]; then
            print_header
            print_info "Requested version: ${BOLD}${tag}${NC}"
            print_info "Installed version:  ${BOLD}${installed_tag}${NC}"
            print_success "No update needed — skipping build/install/restart."
            printf "\n"
            printf "${GREEN}╔════════════════════════════════════════╗${NC}\n"
            printf "${GREEN}║          ALREADY UP TO DATE ✓          ║${NC}\n"
            printf "${GREEN}╚════════════════════════════════════════╝${NC}\n"
            printf "\n"
            return 0
        fi
        if [[ -z "$installed_tag" && -n "$installed_out" ]]; then
            print_info "Installed version detected but could not parse it: ${installed_out}"
        fi
    fi

    bsctl_require_repo

    print_step "1/8" "Fetching from origin..."
    bsctl_git fetch -q origin
    bsctl_git fetch -q origin --tags 2>/dev/null || bsctl_git fetch -q --tags origin 2>/dev/null || true
    printf "\n"

    if ! bsctl_git rev-parse "$tag^{}" >/dev/null 2>&1; then
        print_error "Unknown tag: ${tag}"
        printf "Fetch remote tags: ${BOLD}git fetch origin --tags${NC}\n"
        exit 1
    fi

    print_step "2/8" "Checking out release ${tag}..."
    bsctl_git -c advice.detachedHead=false checkout -f -q "$tag"
    printf "\n"

    print_step "3/8" "Aligning on release ${tag}..."
    bsctl_git reset --hard -q "$tag"
    print_success "Now at ${tag}."
    printf "\n"

    cmd_update_finish "$target_version"
}

cmd_update_branch() {
    local branch="$1"
    if ! validate_git_branch_name "$branch"; then
        print_error "Invalid branch name: ${branch:-empty}"
        printf "Use only letters, digits, and ${BOLD}.${NC}, ${BOLD}_${NC}, ${BOLD}/${NC}, ${BOLD}-${NC} (no ${BOLD}..${NC}).\n"
        exit 1
    fi

    print_header
    print_info "Target branch: ${BOLD}${branch}${NC} (origin/${branch})"
    printf "\n"

    bsctl_require_repo

    print_step "1/8" "Fetching from origin..."
    bsctl_git fetch -q origin "$branch"
    printf "\n"

    # If we're already on the exact same commit as origin/<branch> and the worktree is clean,
    # avoid rebuild/install/restart.
    local head_ref remote_ref cur_branch
    head_ref="$(bsctl_git rev-parse HEAD 2>/dev/null || true)"
    remote_ref="$(bsctl_git rev-parse "origin/${branch}" 2>/dev/null || true)"
    cur_branch="$(bsctl_git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
    if [[ -n "${head_ref}" && -n "${remote_ref}" && "${head_ref}" == "${remote_ref}" ]] \
        && [[ "${cur_branch}" == "${branch}" ]] \
        && bsctl_git diff --quiet 2>/dev/null \
        && bsctl_git diff --cached --quiet 2>/dev/null; then
        print_success "Already up to date: ${BOLD}origin/${branch}${NC}"
        print_success "No update needed — skipping build/install/restart."
        printf "\n"
        printf "${GREEN}╔════════════════════════════════════════╗${NC}\n"
        printf "${GREEN}║          ALREADY UP TO DATE ✓          ║${NC}\n"
        printf "${GREEN}╚════════════════════════════════════════╝${NC}\n"
        printf "\n"
        return 0
    fi

    print_step "2/8" "Checking out branch ${branch}..."
    bsctl_git checkout -f -q "$branch"
    printf "\n"

    print_step "3/8" "Aligning on origin/${branch}..."
    if ! bsctl_git reset --hard -q "origin/${branch}"; then
        print_error "Could not align with origin/${branch}."
        exit 1
    fi
    print_success "Branch matches origin/${branch}."
    printf "\n"

    cmd_update_finish
}

# ============================================================================
# Commands
# ============================================================================

cmd_help() {
    printf "\n"
    printf "${BOLD}📚 BookStorage v${APP_VERSION} - Personal reading tracker${NC}\n"
    printf "\n"
    printf "${BOLD}USAGE${NC}\n"
    printf "    bsctl ${BLUE}<command>${NC}\n"
    printf "\n"
    printf "${BOLD}DEVELOPMENT${NC}\n"
    printf "    ${GREEN}build${NC}        Compile the application\n"
    printf "    ${GREEN}build-prod${NC}   Compile for production (optimized)\n"
    printf "    ${GREEN}run${NC}          Start the development server\n"
    printf "    ${GREEN}clean${NC}        Remove compiled files\n"
    printf "\n"
    printf "${BOLD}PRODUCTION${NC} (requires root)\n"
    printf "    ${GREEN}install${NC}      Install the systemd service\n"
    printf "    ${GREEN}uninstall${NC}    Uninstall the service\n"
    printf "    ${GREEN}update${NC}         Interactive release menu or ${GREEN}BSCTL_UPDATE_TAG${NC}\n"
    printf "    ${GREEN}update main${NC}    Update from origin/main\n"
    printf "    ${GREEN}update <branch>${NC}  Update from origin/<branch>\n"
    printf "    ${GREEN}fix-perms${NC}    Fix file permissions\n"
    printf "\n"
    printf "${BOLD}SERVICE${NC}\n"
    printf "    ${GREEN}start${NC}        Start the service\n"
    printf "    ${GREEN}stop${NC}         Stop the service\n"
    printf "    ${GREEN}restart${NC}      Restart the service\n"
    printf "    ${GREEN}status${NC}       Show service status\n"
    printf "    ${GREEN}logs${NC}         Show logs in real-time\n"
    printf "    ${GREEN}backup${NC}       Snapshot SQLite (reads ${BLUE}BOOKSTORAGE_DATABASE${NC} from ${BOOKSTORAGE_ENV_FILE:-/opt/bookstorage/.env})\n"
    printf "\n"
    printf "${BOLD}EXAMPLES${NC}\n"
    printf "    ${BLUE}bsctl run${NC}              Local development\n"
    printf "    ${BLUE}sudo /usr/local/bin/bsctl install${NC}   Install in production (full path if ${BOLD}sudo${NC} strips /usr/local/bin from PATH)\n"
    printf "    ${BLUE}bsctl install${NC}            Same when your shell is already ${BOLD}root${NC} (no sudo needed)\n"
    printf "    ${BLUE}sudo /usr/local/bin/bsctl update${NC}    Release tag menu (use full path if needed)\n"
    printf "    ${BLUE}BSCTL_UPDATE_TAG=v4.0.1 sudo -E /usr/local/bin/bsctl update${NC}  Non-interactive tag\n"
    printf "    ${BLUE}sudo /usr/local/bin/bsctl update main${NC}   Sync to latest origin/main\n"
    printf "    ${BLUE}sudo /usr/local/bin/bsctl update my-branch${NC}  From origin/my-branch\n"
    printf "    ${BLUE}bsctl logs${NC}             View logs\n"
    printf "\n"
    printf "${BOLD}CONFIGURATION${NC}\n"
    printf "    Environment variables in ${BLUE}.env${NC}:\n"
    printf "    - BOOKSTORAGE_HOST         (default: 127.0.0.1)\n"
    printf "    - BOOKSTORAGE_PORT         (default: 5000)\n"
    printf "    - BOOKSTORAGE_DATABASE     (default: database.db)\n"
    printf "    - BOOKSTORAGE_SECRET_KEY   (default: dev-secret-change-me)\n"
    printf "    - BOOKSTORAGE_METRICS_TOKEN (optional) secures GET /metrics for Prometheus scrapers\n"
    printf "    - ${GREEN}BSCTL_UPDATE_TAG${NC}         (optional) e.g. v4.0.1 — non-interactive ${GREEN}bsctl update${NC}\n"
    printf "    Prometheus sidecar (not run by ${GREEN}bsctl update${NC}): ${BLUE}INSTALL_APP_DIR=/opt/bookstorage bash /opt/bookstorage/deploy/setup-bookstorage-prometheus.sh${NC}\n"
    printf "\n"
    printf "${BOLD}TAB COMPLETION (bash)${NC}\n"
    printf "    ${BLUE}source scripts/bsctl.completion.bash${NC}   (from the repo root)\n"
    printf "    After ${GREEN}install${NC}/${GREEN}update${NC}: new login shell, or ${BLUE}source /etc/bash_completion.d/bsctl${NC}, or ${BLUE}hash -r${NC}\n"
    printf "    (Debian/Ubuntu: file also under ${BLUE}/usr/share/bash-completion/completions/bsctl${NC} when that dir exists.)\n"
    printf "\n"
}

#
# Repo helpers (make bsctl usable from anywhere)
#
bsctl_repo_dir() {
    # Prefer current directory if it's a git checkout
    if command -v git >/dev/null 2>&1; then
        local top
        top="$(git rev-parse --show-toplevel 2>/dev/null || true)"
        if [[ -n "$top" && -d "$top/.git" ]]; then
            printf '%s' "$top"
            return 0
        fi
    fi
    # Default install dir
    if [[ -n "${INSTALL_DIR:-}" && -d "${INSTALL_DIR}/.git" ]]; then
        printf '%s' "${INSTALL_DIR}"
        return 0
    fi
    # Legacy fallback
    if [[ -d "/opt/bookstorage/.git" ]]; then
        printf '%s' "/opt/bookstorage"
        return 0
    fi
    return 1
}

bsctl_require_repo() {
    if ! bsctl_repo_dir >/dev/null 2>&1; then
        print_error "Repository not found."
        printf "Expected a git checkout at ${BOLD}${INSTALL_DIR:-/opt/bookstorage}${NC} (or run from inside the repo).\n"
        exit 1
    fi
}

bsctl_git() {
    local repo
    repo="$(bsctl_repo_dir)" || return 1
    git -C "$repo" "$@"
}

bsctl_in_repo() {
    local repo
    repo="$(bsctl_repo_dir)" || return 1
    ( cd "$repo" && "$@" )
}

cmd_version() {
    printf "BookStorage v${APP_VERSION}\n"
}

cmd_build() {
    print_info "Compiling..."
    bsctl_require_repo
    bsctl_in_repo go build -o ${APP_NAME} ./cmd/bookstorage
    print_success "Build complete: ./${APP_NAME}"
}

cmd_build_prod() {
    local build_version="${1:-$APP_VERSION}"
    print_info "Compiling for production..."
    bsctl_require_repo
    bsctl_in_repo env CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=${build_version}" -o ${APP_NAME} ./cmd/bookstorage
    print_success "Optimized binary: ./${APP_NAME}"
}

cmd_run() {
    print_info "Starting development server..."
    bsctl_require_repo
    bsctl_in_repo go run ./cmd/bookstorage
}

cmd_clean() {
    print_info "Cleaning up..."
    bsctl_require_repo
    bsctl_in_repo rm -f ${APP_NAME}
    print_success "Cleanup complete"
}

cmd_install() {
    local repo
    repo="$(bsctl_repo_dir)" || { print_error "Repository not found."; printf "Expected at ${BOLD}${INSTALL_DIR:-/opt/bookstorage}${NC}\n"; exit 1; }

    print_info "Installing ${APP_NAME} service..."
    cmd_build_prod
    cp "${repo}/${APP_NAME}" ${BIN_DIR}/
    bsctl_install_script_strip_cr "${repo}/scripts/bsctl" "${BIN_DIR}/bsctl" 755
    bsctl_install_script_strip_cr "${repo}/scripts/bsctl.lib.sh" "${BIN_DIR}/bsctl.lib.sh" 644
    cp "${repo}/deploy/bookstorage.service" /etc/systemd/system/
    if [[ -f "${repo}/deploy/bookstorage-backup.service" ]]; then
        cp "${repo}/deploy/bookstorage-backup.service" /etc/systemd/system/
        cp "${repo}/deploy/bookstorage-backup.timer" /etc/systemd/system/
    fi
    install_bsctl_bash_completion "$repo"
    systemctl daemon-reload
    systemctl enable ${APP_NAME}
    printf "\n"
    print_success "Service installed successfully"
    printf "\n"
    printf "Available commands:\n"
    printf "  ${BOLD}bsctl start${NC}   - Start\n"
    printf "  ${BOLD}bsctl stop${NC}    - Stop\n"
    printf "  ${BOLD}bsctl status${NC}  - Status\n"
}

cmd_uninstall() {
    print_info "Uninstalling ${APP_NAME} service..."
    systemctl stop ${APP_NAME} 2>/dev/null || true
    systemctl disable ${APP_NAME} 2>/dev/null || true
    rm -f /etc/systemd/system/bookstorage.service
    rm -f ${BIN_DIR}/${APP_NAME}
    rm -f ${BIN_DIR}/bsctl
    rm -f ${BIN_DIR}/bsctl.lib.sh
    rm -f /etc/bash_completion.d/bsctl 2>/dev/null || true
    rm -f /usr/share/bash-completion/completions/bsctl 2>/dev/null || true
    systemctl daemon-reload
    print_success "Service uninstalled"
}

cmd_update() {
    local arg="${1:-}"
    if [[ -n "$arg" ]]; then
        cmd_update_branch "$arg"
        return
    fi

    local tag=""
    print_header
    if [[ -n "${BSCTL_UPDATE_TAG:-}" ]]; then
        if ! tag=$(normalize_tag "${BSCTL_UPDATE_TAG}"); then
            print_error "Invalid BSCTL_UPDATE_TAG."
            exit 1
        fi
    else
        bsctl_require_repo
        # Must run in the current shell (not a subshell), otherwise the nameref
        # inside prompt_release_choice can't set our local variable.
        local repo
        repo="$(bsctl_repo_dir)" || { print_error "Repository not found."; exit 1; }
        pushd "$repo" >/dev/null || { print_error "Could not enter repo: $repo"; exit 1; }
        prompt_release_choice tag
        popd >/dev/null || true
    fi

    print_info "Target release: ${BOLD}${tag}${NC}"
    printf "\n"

    cmd_update_at_tag "$tag"
}

cmd_fix_perms() {
    print_info "Fixing permissions..."
    chown ${APP_USER}:${APP_GROUP} . 2>/dev/null || true
    chmod 755 . 2>/dev/null || true
    chown ${APP_USER}:${APP_GROUP} database.db 2>/dev/null || true
    chmod 664 database.db 2>/dev/null || true
    chown -R ${APP_USER}:${APP_GROUP} static/avatars/ 2>/dev/null || true
    chown -R ${APP_USER}:${APP_GROUP} static/images/ 2>/dev/null || true
    chown -R ${APP_USER}:${APP_GROUP} templates/ 2>/dev/null || true
    print_success "Permissions fixed."
}

cmd_start() {
    print_info "Starting service..."
    systemctl start ${APP_NAME}
    print_success "Service started"
    cmd_status
}

cmd_stop() {
    print_info "Stopping service..."
    systemctl stop ${APP_NAME}
    print_success "Service stopped"
}

cmd_restart() {
    print_info "Restarting service..."
    systemctl restart ${APP_NAME}
    print_success "Service restarted"
    cmd_status
}

cmd_status() {
    systemctl status ${APP_NAME} --no-pager -l 2>/dev/null || printf "${YELLOW}Service not installed${NC}\n"
}

cmd_logs() {
    print_info "Real-time logs (Ctrl+C to quit)..."
    journalctl -u ${APP_NAME} -f
}

cmd_backup() {
    print_info "SQLite backup..."
    local env_file="${BOOKSTORAGE_ENV_FILE:-/opt/bookstorage/.env}"
    local backup_root="${BOOKSTORAGE_BACKUP_DIR:-/var/lib/bookstorage/backups}"
    local retention_days="${BOOKSTORAGE_BACKUP_RETENTION_DAYS:-14}"
    if [[ ! -f "$env_file" ]]; then
        print_error "Missing env file: $env_file (set BOOKSTORAGE_ENV_FILE)"
        exit 1
    fi
    local db_line db_path
    db_line="$(grep -E '^[[:space:]]*BOOKSTORAGE_DATABASE=' "$env_file" | tail -n1 || true)"
    db_path="${db_line#*=}"
    db_path="${db_path%\"}"
    db_path="${db_path#\"}"
    db_path="${db_path%\'}"
    db_path="${db_path#\'}"
    db_path="$(echo -n "$db_path" | tr -d '\r')"
    if [[ -z "$db_path" || ! -f "$db_path" ]]; then
        print_error "BOOKSTORAGE_DATABASE not set or file missing: '${db_path}'"
        exit 1
    fi
    mkdir -p "$backup_root"
    local stamp dest
    stamp="$(date -u +%Y%m%dT%H%M%SZ)"
    dest="${backup_root}/bookstorage-${stamp}.sqlite"
    if command -v sqlite3 >/dev/null 2>&1; then
        sqlite3 "$db_path" ".backup '$dest'"
    else
        cp -a -- "$db_path" "$dest"
    fi
    chmod 600 "$dest" 2>/dev/null || true
    print_success "Backup: $dest"
    find "$backup_root" -maxdepth 1 -type f -name 'bookstorage-*.sqlite' -mtime "+${retention_days}" -delete 2>/dev/null || true
    print_success "Pruned backups older than ${retention_days} days (if find supports -mtime)."
}

