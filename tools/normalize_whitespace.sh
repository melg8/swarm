# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT

#!/usr/bin/env bash
#
# Normalizes the whitespace of every tracked text file that still
# contains a tab: the repository uses spaces only (see the "Code
# conventions" section of AGENTS.md). Go files are NOT touched here -
# they belong to gofmt-spaces (`task fmt` runs both). The split:
#
#   go.mod, *.md   every tab becomes four spaces (neither can carry a
#                  meaningful tab inside a string)
#   everything else only the leading tab run of a line is widened to
#                  four spaces per tab, so a tab inside a shell string
#                  literal or an xml attribute value stays intact
#
# Idempotent: a file without tabs is not opened at all. Untracked
# files are not considered (only `git grep` is used).
#
# Usage: tools/normalize_whitespace.sh          (no arguments)

set -euo pipefail

cd "$(dirname "$0")/.."

# Tracked text files that still carry a tab (git grep -I skips
# binaries; the exit code 1 of "no match" must not abort).
tabbed="$(git grep -IlP '\t' -- . 2>/dev/null || true)"
if [ -z "$tabbed" ]; then
    echo "Whitespace already normalized: no tabs in the tracked tree"
    exit 0
fi

while IFS= read -r file; do
    case "$file" in
        *.go)
            # Owned by gofmt-spaces; running sed here would fight the
            # literal protection of the Go formatter.
            ;;
        go.mod|*.md)
            # No string literals: widen every tab of the file.
            sed -i 's/\t/    /g' "$file"
            echo "spaces: $file"
            ;;
        *)
            # Scripts and data keep interior tabs (string literals);
            # only the leading whitespace run is widened, spaces of
            # the run preserved. The label loop repeats until no tab
            # of the run is left, so a mixed "space tab space tab"
            # prefix widens every tab to four spaces too.
            sed -i -E ':a; s/^( *)\t/\1    /; ta' "$file"
            echo "spaces (leading): $file"
            ;;
    esac
done <<< "$tabbed"
