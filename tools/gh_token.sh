#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# gh_token.sh runs the GitHub CLI with the OAuth token git already
# stores, so an agent closes and comments on pull requests without a
# second credential on disk.
#
# Why not `gh auth login`: gh keeps its token in plain text under
# ~/.config/gh/hosts.yml, while the same token already lives in the
# credential helper git pushes with (the Windows Credential Manager,
# DPAPI encrypted, on the owner host). This wrapper reads it once per
# invocation through `git credential fill`, hands it to gh as
# GH_TOKEN and never prints or persists it. The token therefore has
# exactly one home and expires or revokes with the git credential.
#
# The wrapper needs a git credential that can reach the repository
# (any successful fetch or push stores one) and a gh binary on PATH
# (tools/install_dev_tools.sh does not install gh; the deployment doc
# records the release zip drop into ~/go/bin).
#
# Usage:
#   bash tools/gh_token.sh auth status
#   bash tools/gh_token.sh pr list --state open
#   bash tools/gh_token.sh pr close 61 --comment "landed as ..."
#
# Environment:
#   GH_CREDENTIAL_HOST  the credential host, default github.com;
#   GH_BIN              the gh binary, default `gh`.

set -euo pipefail

host="${GH_CREDENTIAL_HOST:-github.com}"
gh_bin="${GH_BIN:-gh}"

if ! command -v "${gh_bin}" >/dev/null 2>&1; then
    echo "gh_token: ${gh_bin} is not on PATH" >&2
    exit 1
fi

token="$(
    printf 'protocol=https\nhost=%s\n\n' "${host}" |
        GCM_INTERACTIVE=never git credential fill 2>/dev/null |
        sed -n 's/^password=//p'
)"

if [ -z "${token}" ]; then
    echo "gh_token: no stored credential for ${host}" >&2
    echo "gh_token: run a git fetch so the helper stores one" >&2
    exit 1
fi

GH_TOKEN="${token}" GH_HOST="${host}" exec "${gh_bin}" "$@"
