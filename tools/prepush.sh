#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# prepush.sh is the fast pre-push gate: build, vet, lint,
# the whitespace check and the tests of the packages the unpushed
# diff touches. It protects the shared feature branch from a red
# push - the rebase-before-push rule protects the history, this
# protects the next session's budget (a broken push costs every
# parallel agent a full verify cycle to even notice).
#
# The full gate stays `task check:all` (or `task verify`); this
# script trades completeness for the 20-40 s a push decision needs:
# lint runs with --new (the existing debt is the lint task's job),
# tests run for the touched packages only. Install it as a git hook
# with `tools/install_dev_tools.sh hook` or run it by hand through
# `task prepush`.
#
# The diff base is @{u}...HEAD (the unpushed commits after a
# rebase); without an upstream it falls back to the last commit.

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel)"
cd "${REPO_DIR}"

step() {
    echo "prepush: $1"
}

fail() {
    echo "Error prepush: $1" >&2

    exit 1
}

step "build"
go build ./...

step "vet"
go vet ./...

step "lint (the existing debt stays with task lint)"
golangci-lint run --new

step "whitespace (gofmt-spaces + no tabs)"
out="$(go run ./cmd/gofmt-spaces -l . .agents/skills)"
if [ -n "$out" ]; then
    echo "$out" >&2
    fail "unformatted go files (run task fmt)"
fi
if git grep -IlP '\t' -- .; then
    fail "tracked text files still contain tabs (run task fmt)"
fi

step "tests of the touched packages"
base="@{u}...HEAD"
if ! git rev-parse --verify --quiet "@{u}" > /dev/null 2>&1; then
    base="HEAD~1"
    step "no upstream - diffing the last commit only"
fi
files="$(git diff --name-only "$base" | grep '\.go$' || true)"
if [ -z "$files" ]; then
    echo "prepush: no go files in the unpushed diff - test step skipped"

    exit 0
fi
packages="$(printf '%s\n' "$files" | xargs -n1 dirname | sort -u | \
    sed 's|^|./|' | tr '\n' ' ')"
echo "prepush: go test $packages"
go test -count=1 $packages

echo "prepush: OK"
