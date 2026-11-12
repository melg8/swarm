#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Regenerate the npc and item name dictionaries of the bot from the
# Mobius C1 data files. The game server sends NpcInfo packets with an
# empty name for most npcs (the classic client resolves names from
# NPCName-e.dat by the display template id) and DropItem packets carry
# only the item display id. This script reproduces both mappings and
# additionally extracts the npc level, effective aggression, clan and
# social data used by the web interface to color mobs by threat and by
# the hunt loop to avoid pulling social mobs (the clan help mechanics
# of the Mobius AttackableAI):
#
#   npc:   template id (packet) - 1000000 -> display id
#          display id -> internal id via CT0_to_C4_ids.txt
#          internal id -> name, level, effective aggroRange, effective
#          on-sight aggression, clanHelpRange, clan list via
#          data/stats/npcs/*.xml
#   item:  display id -> name via data/stats/items/*.xml
#
# The effective values mirror the Mobius template constructor and the
# AttackableAI on-sight check: the xml aggroRange is capped at the
# deployed MaxAggroRange (NPC.ini) for the Monster type only, and only
# the monster class templates (Monster, RaidBoss, GrandBoss) with a
# true isAggressive flag and a positive effective range attack a
# neutral player on sight.
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
import os
import re
import sys
import xml.etree.ElementTree as ET

stats, out = sys.argv[1], sys.argv[2]


def int_attr(attrs, key, default=0):
    value = attrs.get(key)
    if value is None or value == "":
        return default

    return int(float(value))


# The template constructor caps the xml aggroRange at the deployed
# MaxAggroRange only for the Monster type (NPC.ini ships 450 against
# the L2J default 1500).
max_aggro_range = 450
ini_path = os.path.normpath(
    os.path.join(stats, "..", "..", "config", "NPC.ini"))
try:
    with open(ini_path, encoding="utf-8") as handle:
        for line in handle:
            match = re.match(r"\s*MaxAggroRange\s*=\s*(\d+)", line)
            if match:
                max_aggro_range = int(match.group(1))
                break
except OSError:
    pass

# The npc type values of the pack that instantiate a Monster: the only
# class whose AttackableAI attacks a neutral player on sight. Guards
# and friendly mobs answer only players with karma, folk never attack.
monster_types = {"monster", "raidboss", "grandboss"}


def effective_aggro_range(npc_type, raw_range):
    if npc_type.lower() == "monster" and raw_range > max_aggro_range:
        return max_aggro_range

    return raw_range


def on_sight(npc_type, raw_range, aggressive):
    return (npc_type.lower() in monster_types and aggressive
            and effective_aggro_range(npc_type, raw_range) > 0)


# Internal npc id -> client display id (from CT0_to_C4_ids.txt).
internal_to_display = {}
with open(f"{stats}/npcs/CT0_to_C4_ids.txt", encoding="utf-8") as handle:
    for line in handle:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        internal, display = line.split(";")
        internal_to_display[int(internal)] = int(display)

# Internal npc id -> NpcInfo wire template id (display id + 1000000,
# the encoding the Mobius AbstractNpcInfo writes on the wire). The
# hunt zone registries carry the internal xml ids of the spawn data,
# so the zone mob priorities resolve through this map.
internal_to_wire = {
    internal: display + 1000000
    for internal, display in internal_to_display.items()
}

# Internal npc id -> name, level, effective aggro range, effective
# on-sight aggression, clan help and clan list. ElementTree visits
# only the top level npc elements, so the nested npc references of
# the minion lists never shadow the parent template.
npc_source = {}
npc_levels = {}
npc_aggro = {}
npc_aggressive = {}
npc_clan_help = {}
npc_clans = {}
for path in sorted(glob.glob(f"{stats}/npcs/*.xml")):
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError as err:
        raise SystemExit(f"Error parsing {path}: {err}")
    for npc in root.findall("npc"):
        attrs = npc.attrib
        npc_id = int_attr(attrs, "id")
        name = attrs.get("name", "")
        npc_type = attrs.get("type", "Folk")
        if name:
            npc_source[npc_id] = name
        if "level" in attrs:
            npc_levels[npc_id] = int_attr(attrs, "level")
        ai = npc.find("ai")
        raw_range = int_attr(ai.attrib, "aggroRange") if ai is not None else 0
        # The Mobius NpcTemplate defaults isAggressive to true: the
        # mobs that omit the attribute (the Kaboo Orc Fighter for
        # example) attack players on sight.
        aggressive = (ai is None
                      or ai.get("isAggressive", "true").lower() != "false")
        npc_aggro[npc_id] = effective_aggro_range(npc_type, raw_range)
        npc_aggressive[npc_id] = on_sight(npc_type, raw_range, aggressive)
        if ai is not None:
            npc_clan_help[npc_id] = int_attr(ai.attrib, "clanHelpRange")
            # The clan list lives inside the ai element: the attacked
            # mob calls every clan mate within its clanHelpRange to
            # help. A self closing ai tag carries no clans.
            clans = sorted({
                clan.text.strip()
                for clan in ai.findall(".//clan")
                if clan.text and clan.text.strip()
            })
            if clans:
                npc_clans[npc_id] = " ".join(clans)
        else:
            npc_clan_help[npc_id] = 0

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
    "// Code generated by tools/generate_npc_names.sh. DO NOT EDIT.\n"
    "//\n"
    "package npcdata\n"
    "\n"
    "// npcNames maps the NpcInfo display template id (packet template id\n"
    "// minus 1000000) to the npc name.\n"
    f"var npcNames = map[int32]string{{\n{render_map(npc_names)}}}\n"
    "\n"
    "// npcLevels maps the NpcInfo display template id to the npc level.\n"
    f"var npcLevels = map[int32]int32{{\n{render_int_map(display_levels)}}}\n"
    "\n"
    "// npcAggroRanges maps the NpcInfo display template id to the\n"
    "// effective ai aggroRange the server builds into the template: the\n"
    "// xml value capped at MaxAggroRange for the Monster type only, the\n"
    "// raw xml value for every other type. The on-sight check of\n"
    "// AttackableAI runs against this number.\n"
    f"var npcAggroRanges = map[int32]int32{{\n{render_int_map(display_aggro)}}}\n"
    "\n"
    "// npcAggressives maps the NpcInfo display template id to the\n"
    "// effective on-sight aggression for a neutral player: a monster\n"
    "// class template (Monster, RaidBoss, GrandBoss) whose ai isAggressive\n"
    "// flag is true and whose effective aggro range is positive. Guards\n"
    "// and friendly mobs attack only players with karma, folk never.\n"
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
    "// npcInternalWireIDs maps the Mobius internal (CT0 xml) npc id\n"
    "// onto the NpcInfo wire template id of the same npc (the C4\n"
    "// display id + 1000000, the translation the Mobius NpcIdConverter\n"
    "// applies on the wire). The hunt zone registries carry the xml\n"
    "// ids of the spawn data, the packets carry the wire ids.\n"
    f"var npcInternalWireIDs = map[int32]int32{{\n{render_int_map(internal_to_wire)}}}\n"
    "\n"
    "// itemNames maps the DropItem display id to the item name.\n"
    f"var itemNames = map[int32]string{{\n{render_map(item_names)}}}\n"
)
with open(out, "w", encoding="utf-8") as handle:
    handle.write(content)
print(
    f"wrote {len(npc_names)} npc names, "
    f"{len(display_levels)} levels, {len(display_aggressive)} aggression flags, "
    f"{len(display_clans)} clan lists, {len(internal_to_wire)} internal wire ids and "
    f"{len(item_names)} item names to {out} "
    f"(MaxAggroRange {max_aggro_range})"
)
PYEOF

# The generated maps must be formatted with the repository formatter of
# record (spaces, never tabs).
if command -v go > /dev/null 2>&1; then
    (cd "${SWARM_ROOT}" && go run ./cmd/gofmt-spaces -w "${OUT}")
fi
