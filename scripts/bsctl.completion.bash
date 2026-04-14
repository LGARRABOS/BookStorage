#!/usr/bin/env bash
# Bash completion for bsctl (BookStorage Control)
#
# Installed to:
#   /etc/bash_completion.d/bsctl              (legacy, if dir exists)
#   /usr/share/bash-completion/completions/bsctl  (Debian/Ubuntu bash-completion)
#
# Usage (development):
#   source scripts/bsctl.completion.bash
#
# Requires programmable completion (bash-completion package recommended on Debian/Ubuntu).

_bsctl_completion() {
    local cur subcmd
    # Avoid falling back to directory listing when COMPREPLY is empty.
    compopt +o filenames +o dirnames 2>/dev/null || true

    cur="${COMP_WORDS[COMP_CWORD]//$'\r'/}"
    subcmd="${COMP_WORDS[1]//$'\r'/}"

    local cmds='help -h --help version -v --version build build-prod run clean install uninstall update fix-perms backup start stop restart status logs'

    # Completing the command name itself (rare); never offer filesystem paths.
    if [[ ${COMP_CWORD} -eq 0 ]]; then
        COMPREPLY=()
        return 0
    fi

    # First argument after bsctl: subcommands
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
        return 0
    fi

    case "${subcmd}" in
        update)
            # Arguments for "update": branch (e.g. main) or release tag (vX.Y.Z).
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
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "main" -- "${cur}") )
                return 0
            fi
            ;;
        help|-h|--help|version|-v|--version|build|build-prod|run|clean|install|uninstall|fix-perms|backup|start|stop|restart|status|logs)
            ;;
        *)
            ;;
    esac

    COMPREPLY=()
    return 0
}

complete -o nospace -o nofile -F _bsctl_completion bsctl
