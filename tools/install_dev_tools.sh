# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT

#!/usr/bin/env bash
#
# Installs the Go developer tools the swarm workflow expects: go-task
# (Taskfile.yml runner), golangci-lint v2 (the strict gate) and gci
# (import grouping), then builds the repository formatter
# gofmt-spaces from cmd/gofmt-spaces (the spaces-only gofmt the
# repository style mandates, see AGENTS.md - the stock gofumpt
# would re-tab the tree and is no longer part of the toolchain).
# The fast deploy (tools/swarm_fast_deploy.sh) installs the Go
# toolchain itself; this script fills the developer-side gap so a
# fresh agent can run `task check:all`, `task lint`, `task fmt` and
# `task lint:new` without guessing which binaries are missing.
#
# Idempotent: every tool is skipped when its binary already exists.
# The install path is $GOPATH/bin (default ~/go/bin); add it to PATH
# or invoke through the `task` wrapper the deploy script sets up.
#
# Usage:
#   tools/install_dev_tools.sh           install the tools
#   tools/install_dev_tools.sh check     verify presence, exit 1 if any
#                                        missing (use in CI / preflight)
#   tools/install_dev_tools.sh hook      install the pre-push gate as
#                                        .git/hooks/pre-push (opt-in)
#
# Environment overrides:
#   GO_BIN      path to the go binary (default: auto-detect on PATH,
#               then /home/z/opt/go-root/usr/lib/go-1.24/bin/go)
#   GOPATH      Go workspace (default: ~/go)
#   TOOLS       space-separated tool list override (default:
#               "task golangci-lint gci gofmt-spaces govulncheck")

set -euo pipefail

MODE="${1:-install}"

# Locate go: explicit override, then PATH, then the fast-deploy layout.
GO_BIN="${GO_BIN:-}"
if [ -z "$GO_BIN" ]; then
    if command -v go >/dev/null 2>&1; then
        GO_BIN="$(command -v go)"
    elif [ -x /home/z/opt/go-root/usr/lib/go-1.24/bin/go ]; then
        GO_BIN=/home/z/opt/go-root/usr/lib/go-1.24/bin/go
    else
        echo "Error: go toolchain not found. Run tools/swarm_fast_deploy.sh first." >&2
        exit 1
    fi
fi

GOPATH_VAL="${GOPATH:-$HOME/go}"
TOOLS_LIST="${TOOLS:-task golangci-lint gci gofmt-spaces govulncheck}"
BIN_DIR="$GOPATH_VAL/bin"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$BIN_DIR"

# Pinned versions: pin major versions so an upstream gofumpt that
# demands Go 1.26 cannot quietly switch the toolchain. Bump them
# deliberately and update the comment in AGENTS.md Tech stack together.
# gofmt-spaces carries no pin: it is built from this repository
# (cmd/gofmt-spaces) and travels with the committed source.
declare -A PIN=(
    [task]="github.com/go-task/task/v3/cmd/task@v3.53.1"
    [golangci-lint]="github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"
    [gci]="github.com/daixiang0/gci@v0.13.5"
    [govulncheck]="golang.org/x/vuln/cmd/govulncheck@v1.8.0"
)

check_tool() {
    local name="$1"
    if [ -x "$BIN_DIR/$name" ]; then
        return 0
    fi
    if command -v "$name" >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

if [ "$MODE" = "check" ]; then
    missing=0
    for name in $TOOLS_LIST; do
        if check_tool "$name"; then
            echo "ok: $name"
        else
            echo "missing: $name"
            missing=1
        fi
    done
    exit "$missing"
fi

# Hook mode: install the pre-push gate as a git hook (opt-in, this
# clone only - the hook file is not tracked). The hook runs
# tools/prepush.sh (build, vet, lint --new, whitespace, the tests of
# the touched packages) so a red push cannot reach the shared branch
# unnoticed.
if [ "$MODE" = "hook" ]; then
    HOOK_DIR="$REPO_ROOT/.git/hooks"
    mkdir -p "$HOOK_DIR"
    cat > "$HOOK_DIR/pre-push" <<HOOK
#!/usr/bin/env bash
# Installed by tools/install_dev_tools.sh hook - the fast pre-push
# gate (tools/prepush.sh). Remove the file to opt out.
exec bash "$REPO_ROOT/tools/prepush.sh"
HOOK
    chmod +x "$HOOK_DIR/pre-push"
    echo "hook: $HOOK_DIR/pre-push installed (runs tools/prepush.sh)"

    exit 0
fi

# Install mode: install each missing tool with its pinned version.
installed=0
skipped=0
for name in $TOOLS_LIST; do
    if check_tool "$name"; then
        echo "skip: $name already installed"
        skipped=$((skipped + 1))
        continue
    fi
    if [ "$name" = "gofmt-spaces" ]; then
        echo "install: gofmt-spaces (built from ./cmd/gofmt-spaces)"
        GOFLAGS="-trimpath" "$GO_BIN" build -o "$BIN_DIR/gofmt-spaces" \
            "$REPO_ROOT/cmd/gofmt-spaces"
        installed=$((installed + 1))
        continue
    fi
    pkg="${PIN[$name]:-}"
    if [ -z "$pkg" ]; then
        echo "Error: no pinned version for $name" >&2
        exit 1
    fi
    echo "install: $name ($pkg)"
    GOFLAGS="-trimpath" "$GO_BIN" install "$pkg"
    installed=$((installed + 1))
done

echo "Installed $installed, skipped $skipped."
echo "Ensure $BIN_DIR is on PATH (the fast deploy adds /home/z/opt/go-root/usr/lib/go-1.24/bin, add \$GOPATH/bin too)."
