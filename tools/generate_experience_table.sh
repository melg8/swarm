#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Regenerate the C1 experience table of the bot from the Mobius data
# file. The table drives the experience progress bar of the web
# interface: the character snapshot reports the percentage of the
# experience gathered toward the next level.
#
# Usage: tools/generate_experience_table.sh [path/to/L2J_Mobius_C1_HarbingersOfWar]
# Output: internal/swarm/state/experience.go

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWARM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MOBIUS_C1="${1:-$(dirname "${SWARM_ROOT}")/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar}"
OUT="${SWARM_ROOT}/internal/swarm/state/experience.go"

XML="${MOBIUS_C1}/dist/game/data/stats/players/experience.xml"
if [ ! -f "${XML}" ]; then
    # A sparse checkout may not carry dist/: fall back to the git object.
    if git -C "${MOBIUS_C1}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        git -C "${MOBIUS_C1}" show \
            "HEAD:L2J_Mobius_C1_HarbingersOfWar/dist/game/data/stats/players/experience.xml" \
            > /tmp/swarm_experience.xml
        XML=/tmp/swarm_experience.xml
    else
        echo "Error mobius experience data not found at ${XML}"
        echo "Usage: $0 [path/to/L2J_Mobius_C1_HarbingersOfWar]"
        exit 1
    fi
fi

python3 - "${XML}" "${OUT}" << 'PYEOF'
import pathlib
import re
import sys

xml = pathlib.Path(sys.argv[1]).read_text()
out = pathlib.Path(sys.argv[2])

vals = []
for line in xml.splitlines():
    m = re.search(r'level="(\d+)" tolevel="(\d+)"', line)
    if m:
        vals.append(int(m.group(2)))
if len(vals) != 81:
    raise SystemExit(f"expected 81 levels, got {len(vals)}")

rows = []
for i in range(0, len(vals), 4):
    rows.append("\t" + "\t".join(f"{v:>12}," for v in vals[i:i + 4]))
table = "\n".join(rows)

out.write_text(f'''// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// experienceTable is the cumulative experience needed to reach each
// level of the L2J Mobius C1 data pack
// (dist/game/data/stats/players/experience.xml, table maxLevel 78).
// experienceTable[N-1] is the total experience a character has exactly
// when reaching level N, so the progress toward the next level is
// (exp - experienceTable[level-1]) / (experienceTable[level] -
// experienceTable[level-1]). Values above level 78 exceed the int32
// protocol field and wrap, the percentage is clamped in that case.
// Regenerate with tools/generate_experience_table.sh.
var experienceTable = [...]int64{{
{table}
}}

// maxExperienceLevel is the highest level the table knows about.
const maxExperienceLevel = {len(vals)}

// ExpPercent returns the experience progress of a character toward the
// next level as a percentage (0..100) based on the C1 experience table.
// Unknown levels and wrapped values clamp to the nearest bound.
func ExpPercent(level int32, exp int64) float64 {{
\tif level < 1 {{
\t\treturn 0
\t}}
\tif level > maxExperienceLevel {{
\t\treturn 100
\t}}
\tbase := experienceTable[level-1]
\tnext := experienceTable[level]
\tif next <= base || exp <= base {{
\t\treturn 0
\t}}
\tif exp >= next {{
\t\treturn 100
\t}}

\treturn float64(exp-base) / float64(next-base) * 100
}}
''')
print(f"{out}: {len(vals)} levels")
PYEOF
