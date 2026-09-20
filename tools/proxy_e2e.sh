#!/usr/bin/env bash
# ============================================================================
# proxy_e2e.sh - the E2E check of the C1 client MITM proxy on the
# live stack.
#
# Brings up (verifies) the Mobius C1 stack, then runs
# SWARM_PROXY_E2E=1 go test: the bot enters the world, the fake C1
# client walks the whole path through the proxy (the login with
# arbitrary credentials, the single character list, the world entry
# through the replay, the live relay, the character movement by the
# client command) and the process shuts down cleanly.
#
# Success prints PROXY_E2E_OK.
# ============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

# The stack must be up (the deploy is idempotent, ~90 s from scratch).
export PATH="${HOME}/opt/go-root/usr/lib/go-1.24/bin:${PATH}"
export JDK_DIR="${HOME}/opt/jdk25-root/usr/lib/jvm/java-25-openjdk-amd64"
export MARIADB_DIR="${HOME}/opt/mariadb"

if ! ss -ltn 2>/dev/null | grep -qE ':(2106|7777) '; then
    echo ">>> the stack is down, starting tools/mobius_start.sh"
    tools/mobius_start.sh
fi

echo ">>> running the proxy E2E (live stack)"
if SWARM_PROXY_E2E=1 go test ./internal/swarm/proxy/ \
    -run TestProxyE2E -v -count=1 -timeout 5m; then
    echo "PROXY_E2E_OK"
else
    echo "PROXY_E2E_FAILED"
    exit 1
fi
