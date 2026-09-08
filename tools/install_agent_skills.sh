# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT

#!/usr/bin/env bash
#
# Installs and updates the vendored golang agent skills of this
# repository (.agents/skills/golang-*), sourced from the upstream
# samber/cc-skills-golang collection (MIT). The vendored copy travels
# with the repository, so a fresh environment gets the skills through
# git clone alone; this script exists to (re)install them and to pull
# newer upstream versions.
#
# Usage:
#   tools/install_agent_skills.sh           install the pinned commit
#                                           (idempotent, always safe)
#   tools/install_agent_skills.sh latest    install upstream HEAD
#   tools/install_agent_skills.sh check     verify the vendored copy
#                                           against the pinned commit,
#                                           exit 1 on drift
#
# Environment overrides: SKILLS_REPO, SKILLS_COMMIT, SKILLS_DIR.
# The project's own playbooks (go-verify-loop, webui-harness,
# packet-recipe, mobius-stack) are hand-maintained and are never
# touched: only the golang-* directories are managed here.

set -euo pipefail

REPO_URL="${SKILLS_REPO:-https://github.com/samber/cc-skills-golang}"
PINNED_COMMIT="${SKILLS_COMMIT:-19a0626ae8565d27a7b7bdf59d8d99d94d7e284c}"
SKILLS_DIR="${SKILLS_DIR:-.agents/skills}"
MODE="${1:-install}"

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Cloning $REPO_URL..."
git clone --quiet "$REPO_URL" "$tmp/repo"

ref="$PINNED_COMMIT"
if [ "$MODE" = "latest" ]; then
	git -C "$tmp/repo" fetch --quiet origin
	ref="$(git -C "$tmp/repo" rev-parse origin/HEAD)"
fi
git -C "$tmp/repo" checkout --quiet "$ref"

upstream="$tmp/repo/skills"
if [ ! -d "$upstream" ]; then
	echo "Error: upstream layout changed, no skills/ directory at $ref"
	exit 1
fi

if [ "$MODE" = "check" ]; then
	drift=0
	for dir in "$upstream"/golang-*/; do
		name="$(basename "$dir")"
		if ! diff -r --brief "$dir" "$SKILLS_DIR/$name" > /dev/null 2>&1; then
			echo "Drift: $name differs from the pinned commit"
			drift=1
		fi
	done
	for dir in "$SKILLS_DIR"/golang-*/; do
		name="$(basename "$dir")"
		if [ ! -d "$upstream/$name" ]; then
			echo "Drift: $name is vendored but absent upstream"
			drift=1
		fi
	done
	if [ "$drift" -eq 0 ]; then
		echo "Vendored golang skills match $PINNED_COMMIT"
	fi

	exit "$drift"
fi

installed=0
for dir in "$upstream"/golang-*/; do
	name="$(basename "$dir")"
	rm -rf "$SKILLS_DIR/$name"
	cp -r "$dir" "$SKILLS_DIR/$name"
	installed=$((installed + 1))
done

# The MIT license of the upstream collection travels with the vendored
# copy (kept as a plain .md file: skill discovery only treats
# directories with a SKILL.md as skills).
cp "$tmp/repo/LICENSE" "$SKILLS_DIR/golang-cc-skills-LICENSE.md"

echo "Installed $installed golang skills at commit $ref into $SKILLS_DIR"
