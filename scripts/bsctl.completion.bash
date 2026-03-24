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
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local cmds='help -h --help version -v --version build build-prod run clean install uninstall update fix-perms start stop restart status logs'

    # First argument: subcommand
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
        return
    fi

    # Second argument: optional branch name for "update" (git, current directory)
    if [[ ${COMP_CWORD} -eq 2 ]] && [[ "${COMP_WORDS[1]}" == "update" ]] && command -v git >/dev/null 2>&1; then
        if git rev-parse --git-dir >/dev/null 2>&1; then
            local branches
            branches=$(git branch -a 2>/dev/null | sed 's/^[* ]*//;s|^remotes/origin/||' | sort -u | grep -v '^HEAD' || true)
            if [[ -n "${branches}" ]]; then
                COMPREPLY=( $(compgen -W "${branches}" -- "${cur}") )
            fi
        fi
    fi
}

complete -o bashdefault -o default -F _bsctl_completion bsctl
