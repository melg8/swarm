#!/usr/bin/env bash
# ============================================================================
# proxy_e2e.sh — E2E проверка MITM-прокси для клиента C1 на живом стеке.
#
# Поднимает (проверяет) стек Mobius C1, затем запускает
# SWARM_PROXY_E2E=1 go test: бот входит в мир, фейковый C1-клиент
# проходит весь путь через прокси (логин с произвольными кредами,
# список из одного персонажа, вход в мир через реплей, live-релей,
# движение персонажа командой клиента) и процесс корректно завершается.
#
# Успех печатает PROXY_E2E_OK.
# ============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

# Стек должен быть поднят (idемпотентный деплой, ~90 с с нуля).
export PATH="${HOME}/opt/go-root/usr/lib/go-1.24/bin:${PATH}"
export JDK_DIR="${HOME}/opt/jdk25-root/usr/lib/jvm/java-25-openjdk-amd64"
export MARIADB_DIR="${HOME}/opt/mariadb"

if ! ss -ltn 2>/dev/null | grep -qE ':(2106|7777) '; then
    echo ">>> стек не поднят, запускаю tools/mobius_start.sh"
    tools/mobius_start.sh
fi

echo ">>> запуск proxy E2E (живой стек)"
if SWARM_PROXY_E2E=1 go test ./internal/swarm/proxy/ \
    -run TestProxyE2E -v -count=1 -timeout 5m; then
    echo "PROXY_E2E_OK"
else
    echo "PROXY_E2E_FAILED"
    exit 1
fi
