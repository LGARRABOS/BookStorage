#!/usr/bin/env bash
# Bash completion for bsctl (BookStorage Control)
#
# Usage (development):
#   source scripts/bsctl.completion.bash
#
# System-wide (after copying bsctl to /usr/local/bin):
#   sudo cp scripts/bsctl.completion.bash /etc/bash_completion.d/bsctl
#   # open a new terminal, or: source /etc/bash_completion.d/bsctl
#
# Requires an interactive bash with programmable completion (default on most Linux distros).

_bsctl_completion() {
    local cur subcmd
    cur="${COMP_WORDS[COMP_CWORD]}"
    subcmd="${COMP_WORDS[1]}"

    local cmds='help -h --help version -v --version build build-prod run clean install uninstall update fix-perms backup start stop restart status logs'

    # If the user is completing just "bsctl" (no space yet), do not fall back to file completion.
    if [[ ${COMP_CWORD} -eq 0 ]]; then
        COMPREPLY=()
        return 0
    fi

    # Subcommand
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
        return 0
    fi

    case "${subcmd}" in
        update)
            # Arguments for "update": branch (e.g. main) or release tag (vX.Y.Z).
            # Works from inside the repo, and falls back to /opt/bookstorage if present.
            if [[ ${COMP_CWORD} -eq 2 ]] && command -v git >/dev/null 2>&1; then
                local git_args=()
                if git rev-parse --git-dir >/dev/null 2>&1; then
                    git_args=()
                elif [[ -d /opt/bookstorage/.git ]]; then
                    git_args=(-C /opt/bookstorage)
                else
                    COMPREPLY=( $(compgen -W "main" -- "${cur}") )
                    return 0
                fi

                local branches tags words
                branches="$(git "${git_args[@]}" branch -a 2>/dev/null | sed 's/^[* ]*//;s|^remotes/origin/||' | grep -v '^HEAD' | sort -u || true)"
                tags="$(git "${git_args[@]}" tag -l 'v*' 2>/dev/null | sort -V | tail -n 40 || true)"

                words="main"
                [[ -n "${tags}" ]] && words="${words} ${tags}"
                [[ -n "${branches}" ]] && words="${words} ${branches}"

                COMPREPLY=( $(compgen -W "${words}" -- "${cur}") )
                return 0
            fi
            ;;
        help|-h|--help|version|-v|--version|build|build-prod|run|clean|install|uninstall|fix-perms|backup|start|stop|restart|status|logs)
            # These subcommands don't accept (or need) arguments; return empty completion
            # to avoid falling back to file/path completion.
            ;;
        *)
            # Unknown / not yet completed: return empty completion
            ;;
    esac

    COMPREPLY=()
    return 0
}

complete -o nospace -o nofile -F _bsctl_completion bsctl
