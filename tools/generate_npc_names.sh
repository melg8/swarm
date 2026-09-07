#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Regenerate the npc and item name dictionaries of the bot from the
# Mobius C1 data files. The game server sends NpcInfo packets with an
# empty name for most npcs (the classic client resolves names from
# NPCName-e.dat by the display template id) and DropItem packets carry
# only the item display id. This script reproduces both mappings and
# additionally extracts the npc level, aggression, clan and social data
# used by the web interface to color mobs by threat and by the hunt
# loop to avoid pulling social mobs (the clan help mechanics of the
# Mobius AttackableAI):
#
#   npc:   template id (packet) - 1000000 -> display id
#          display id -> internal id via CT0_to_C4_ids.txt
#          internal id -> name, level, aggroRange, isAggressive,
#          clanHelpRange, clan list via data/stats/npcs/*.xml
#   item:  display id -> name via data/stats/items/*.xml
#
# Usage: tools/generate_npc_names.sh [path/to/L2J_Mobius_C1_HarbingersOfWar]
# Output: internal/swarm/npcdata/names.go

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWARM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MOBIUS_C1="${1:-$(dirname "${SWARM_ROOT}")/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar}"
OUT="${SCRIPT_DIR}/../internal/swarm/npcdata/names.go"

STATS="${MOBIUS_C1}/dist/game/data/stats"
[ -f "${STATS}/npcs/CT0_to_C4_ids.txt" ] || {
    echo "Error mobius npc stats not found at ${STATS}"
    echo "Usage: $0 [path/to/L2J_Mobius_C1_HarbingersOfWar]"
    exit 1
}

mkdir -p "$(dirname "${OUT}")"

python3 - "${STATS}" "${OUT}" << 'PYEOF'
import glob
import re
import sys

stats, out = sys.argv[1], sys.argv[2]

# Internal npc id -> client display id (from CT0_to_C4_ids.txt).
internal_to_display = {}
with open(f"{stats}/npcs/CT0_to_C4_ids.txt", encoding="utf-8") as handle:
    for line in handle:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        internal, display = line.split(";")
        internal_to_display[int(internal)] = int(display)

# Internal npc id -> name, level and aggression (from the npc stats
# xml files). Attribute order inside the npc and ai elements varies, so
# attributes are parsed by dictionary.
npc_open_pattern = re.compile(r'<npc\s+([^>]*?)>')
npc_ai_pattern = re.compile(r'<ai\s+([^>]*?)(/?)>')
clan_pattern = re.compile(r'<clan>([^<]+)</clan>')
attr_pattern = re.compile(r'(\w+)="([^"]*)"')
npc_source = {}
npc_levels = {}
npc_aggro = {}
npc_aggressive = {}
npc_clan_help = {}
npc_clans = {}
for path in glob.glob(f"{stats}/npcs/*.xml"):
    with open(path, encoding="utf-8") as handle:
        content = handle.read()
    matches = list(npc_open_pattern.finditer(content))
    for index, match in enumerate(matches):
        attrs = dict(attr_pattern.findall(match.group(1)))
        npc_id = int(attrs.get("id", 0))
        name = attrs.get("name", "")
        if name:
            npc_source[npc_id] = name
        if "level" in attrs:
            npc_levels[npc_id] = int(attrs["level"])
        block_end = matches[index + 1].start() if index + 1 < len(matches) else len(content)
        ai = npc_ai_pattern.search(content, match.end(), block_end)
        if ai:
            ai_attrs = dict(attr_pattern.findall(ai.group(1)))
            npc_aggro[npc_id] = int(ai_attrs.get("aggroRange", 0))
            # The Mobius NpcTemplate defaults isAggressive to true: the
            # mobs that omit the attribute (the Kaboo Orc Fighter for
            # example) attack players on sight.
            npc_aggressive[npc_id] = ai_attrs.get("isAggressive", "true") == "true"
            if "clanHelpRange" in ai_attrs:
                npc_clan_help[npc_id] = int(ai_attrs["clanHelpRange"])
            # The clan list lives inside the ai element: the attacked
            # mob calls every clan mate within its clanHelpRange to
            # help. A self closing ai tag carries no clans.
            ai_end = block_end
            if not ai.group(2):
                close = content.find("</ai>", ai.end())
                if close != -1 and close <= block_end:
                    ai_end = close
            clans = sorted(set(clan_pattern.findall(content[ai.end():ai_end])))
            if clans:
                npc_clans[npc_id] = " ".join(clans)
        else:
            npc_aggro.setdefault(npc_id, 0)
            npc_aggressive.setdefault(npc_id, False)
            npc_clan_help.setdefault(npc_id, 0)

# Display id -> name/level/aggro/clan, the lookup the bot needs.
npc_names = {}
display_levels = {}
display_aggro = {}
display_aggressive = {}
display_clan_help = {}
display_clans = {}
for internal, name in npc_source.items():
    display = internal_to_display.get(internal, internal)
    npc_names[display] = name
    display_levels[display] = npc_levels.get(internal, 0)
    display_aggro[display] = npc_aggro.get(internal, 0)
    display_aggressive[display] = npc_aggressive.get(internal, False)
    display_clan_help[display] = npc_clan_help.get(internal, 0)
    display_clans[display] = npc_clans.get(internal, "")

# Item display id -> name (from the item stats xml files).
item_pattern = re.compile(r'<item id="(\d+)"[^>]*?name="([^"]*)"')
item_names = {}
for path in glob.glob(f"{stats}/items/*.xml"):
    with open(path, encoding="utf-8") as handle:
        for match in item_pattern.finditer(handle.read()):
            item_id, name = int(match.group(1)), match.group(2)
            if name:
                item_names[item_id] = name

def render_string_map(values):
    if not values:
        return "\t{}\n"
    lines = []
    for key in sorted(values):
        name = values[key].replace("\\", "\\\\").replace('"', '\\"')
        lines.append(f'\t{key}: "{name}",\n')
    return "".join(lines)

def render_map(values):
    if not values:
        return "\t{}\n"
    lines = []
    for key in sorted(values):
        name = values[key].replace("\\", "\\\\").replace('"', '\\"')
        lines.append(f'\t{key}: "{name}",\n')
    return "".join(lines)

def render_int_map(values):
    if not values:
        return "\t{}\n"
    return "".join(f"\t{key}: {values[key]},\n" for key in sorted(values))

def render_bool_map(values):
    if not values:
        return "\t{}\n"
    lines = []
    for key in sorted(values):
        value = "true" if values[key] else "false"
        lines.append(f"\t{key}: {value},\n")
    return "".join(lines)

content = (
    "// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>\n"
    "//\n"
    "// SPDX-License-Identifier: MIT\n"
    "\n"
    "// Code generated by tools/generate_npc_names.sh from the Mobius C1\n"
    "// data files. DO NOT EDIT.\n"
    "package npcdata\n"
    "\n"
    "// npcNames maps the NpcInfo display template id (packet template id\n"
    "// minus 1000000) to the npc name.\n"
    f"var npcNames = map[int32]string{{\n{render_map(npc_names)}}}\n"
    "\n"
    "// npcLevels maps the NpcInfo display template id to the npc level.\n"
    f"var npcLevels = map[int32]int32{{\n{render_int_map(display_levels)}}}\n"
    "\n"
    "// npcAggroRanges maps the NpcInfo display template id to the ai\n"
    "// aggroRange of the npc (0 for passive npcs).\n"
    f"var npcAggroRanges = map[int32]int32{{\n{render_int_map(display_aggro)}}}\n"
    "\n"
    "// npcAggressives maps the NpcInfo display template id to the ai\n"
    "// isAggressive flag of the npc.\n"
    f"var npcAggressives = map[int32]bool{{\n{render_bool_map(display_aggressive)}}}\n"
    "\n"
    "// npcClanHelpRanges maps the NpcInfo display template id to the ai\n"
    "// clanHelpRange of the npc: the distance within the attacked npc\n"
    "// calls its clan mates to help (0 for loners).\n"
    f"var npcClanHelpRanges = map[int32]int32{{\n{render_int_map(display_clan_help)}}}\n"
    "\n"
    "// npcClans maps the NpcInfo display template id to the space\n"
    "// separated clan names of the npc (the ALL clan matches every\n"
    "// clan in the assist check of the server).\n"
    f"var npcClans = map[int32]string{{\n{render_string_map(display_clans)}}}\n"
    "\n"
    "// itemNames maps the DropItem display id to the item name.\n"
    f"var itemNames = map[int32]string{{\n{render_map(item_names)}}}\n"
)
with open(out, "w", encoding="utf-8") as handle:
    handle.write(content)
print(
    f"wrote {len(npc_names)} npc names, "
    f"{len(display_levels)} levels, {len(display_aggressive)} aggression flags, "
    f"{len(display_clans)} clan lists and "
    f"{len(item_names)} item names to {out}"
)
PYEOF

# The generated maps must be gofmt clean (aligned columns).
if command -v gofmt > /dev/null 2>&1; then
    gofmt -w "${OUT}"
fi
