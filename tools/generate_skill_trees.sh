#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# Regenerate the skill tree dictionary of the bot from the Mobius C1
# skill stats and the class skill trees. Two Mobius data sources feed
# the dictionary:
#
#   skill stats:   dist/game/data/stats/skills/*.xml
#                  (skill id, name, icon, operateType, effect stats,
#                  level descriptions - the effect stats drive the
#                  warrior priority category: pAtk/physical damage ->
#                  attack power, pDef/sDef -> defense, everything
#                  else -> other; the descriptions are the XML
#                  comments of the skill blocks, the classic client
#                  tooltips carried by the server data - "Level N:"
#                  comments cover one level each and collapse into
#                  runs, a single unlabeled comment covers the whole
#                  skill, the enchant route comments "(+N Cost)" and
#                  levels beyond the declared level count never
#                  enter the dictionary; the cast block - target
#                  type, cast range, reuse delay, hit time, the
#                  per level mana cost, the magic flag, the abnormal
#                  time of the timed buffs and the weapon kinds of
#                  the <using> conditions - feeds the combat skill
#                  casting of the hunt loop)
#   skill trees:   dist/game/data/stats/players/skillTrees/
#                  {StartingClass,1stClass,2ndClass,3rdClass}/*.xml
#                  (classId, parentClassId, per skill: id, level,
#                  getLevel, levelUpSp, autoGet, the required item
#                  id of the skill books - the <item> children of
#                  the lesson entries)
#
# The script merges every class tree with its parent chain (the C1
# trees list only the skills of their own class window, the parent
# entries complete them) and resolves the icon of every skill against
# the icon pack of this repository (data/icons): the classic client
# names skill icons icon.skillNNNN which maps to skillNNNN.png, the
# few table-driven icons resolve through their first table entry and
# the rest fall back to skill<id zero padded to 4> when the pack
# carries it.
#
# Usage: tools/generate_skill_trees.sh [mobius_c1_root]
# Output: internal/swarm/npcdata/skill_trees.go

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWARM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MOBIUS_C1="${1:-$(dirname "${SWARM_ROOT}")/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar}"
OUT="${SWARM_ROOT}/internal/swarm/npcdata/skill_trees.go"
ICONS="${SWARM_ROOT}/data/icons"

SKILL_STATS="${MOBIUS_C1}/dist/game/data/stats/skills"
SKILL_TREES="${MOBIUS_C1}/dist/game/data/stats/players/skillTrees"
[ -d "${SKILL_STATS}" ] || {
    echo "Error mobius skill stats not found at ${SKILL_STATS}"
    echo "Usage: $0 [mobius_c1_root]"
    exit 1
}
[ -d "${SKILL_TREES}" ] || {
    echo "Error mobius skill trees not found at ${SKILL_TREES}"
    echo "Usage: $0 [mobius_c1_root]"
    exit 1
}
[ -d "${ICONS}" ] || {
    echo "Error icon pack not found at ${ICONS}"
    exit 1
}

mkdir -p "$(dirname "${OUT}")"

python3 - "${SKILL_STATS}" "${SKILL_TREES}" "${ICONS}" "${OUT}" << 'PYEOF'
import glob
import os
import re
import sys

skill_stats, skill_trees_dir, icons, out = sys.argv[1:5]

# ---------------------------------------------------------------- skill info
# One <skill id="N" name="..."> block: the icon, the operate type,
# the effect stats/effect names and the level descriptions drive the
# display (the tooltip text) and the warrior priority category (0
# attack power, 1 defense, 2 other).
skill_infos = {}
skill_descs = {}
skill_casts = {}
for path in sorted(glob.glob(os.path.join(skill_stats, '*.xml'))):
    text = open(path, encoding='utf-8').read()
    for m in re.finditer(
            r'<skill id="(\d+)"[^>]*?name="([^"]*)"[^>]*>(.*?)</skill>',
            text, re.S):
        skill_id, name, body = int(m.group(1)), m.group(2), m.group(3)
        tag = m.group(0)[:m.start(3) - m.start(0)]
        lv = re.search(r'levels="(\d+)"', tag)
        max_level = int(lv.group(1)) if lv else 1 << 30
        im = re.search(r'<icon>([^<]+)</icon>', body)
        op = re.search(r'<operateType>([^<]+)</operateType>', body)
        # The #icons table entries (level dependent icons): the first
        # entry answers for the whole skill - the display never needs
        # a per level icon.
        icon = ''
        if im:
            icon = im.group(1)
            if icon.startswith('#'):
                tm = re.search(
                    r'<table name="#icons">([^<]+)</table>', body)
                icon = tm.group(1).split()[0] if tm else ''
        icons_found = re.findall(
            r'stat="(pAtk|pDef|sDef|rShld)"', body)
        effects = set(re.findall(
            r'<effect name="([A-Za-z]+)"', body))
        # Warrior priority: physical weapon attack power first, then
        # defense. Attack power skills carry a pAtk stat effect
        # (masteries, auras) or are physical strikes (PhysicalDamage,
        # FatalBlow). Defense skills carry pDef/sDef (masteries,
        # auras) or the shield block rate rShld (Shield Mastery).
        if 'pAtk' in icons_found or \
                effects & {'PhysicalDamage', 'FatalBlow'}:
            category = 0
        elif {'pDef', 'sDef', 'rShld'} & set(icons_found):
            category = 1
        else:
            category = 2
        skill_infos[skill_id] = {
            'name': name,
            'icon': icon,
            'operate': op.group(1) if op else '',
            'category': category,
        }

        # ------------------------------------------------------------- cast
        # The cast block of the skill stats feeds the combat casting:
        # the target type picks the cast target (ONE attacks the
        # selected target, SELF buffs always land), the cast range
        # and the reuse delay pace the requests, the per level mana
        # cost gates the casting (the mage out of mana rest), the
        # magic flag marks the spells of the mystics, the abnormal
        # time bounds the timed buffs and the weapon kinds of the
        # <using> conditions match the skill to the weapon in hand
        # (Power Strike wants a sword or blunt, Power Shot a bow).
        # The #mpConsume table entries resolve per level here; a
        # static value repeats for every level of the skill.
        tg = re.search(r'<targetType>([^<]+)</targetType>', body)
        cr = re.search(r'<castRange>(\d+)</castRange>', body)
        rd = re.search(r'<reuseDelay>(\d+)</reuseDelay>', body)
        ht = re.search(r'<hitTime>(\d+)</hitTime>', body)
        mag = re.search(r'<isMagic>\s*1', body)
        abt = re.search(r'<abnormalTime>(\d+)</abnormalTime>', body)
        mp = re.search(r'<mpConsume>([^<]+)</mpConsume>', body)
        costs = []
        if mp:
            ref = mp.group(1).strip()
            if ref.startswith('#'):
                tab = re.search(
                    r'<table name="%s">([^<]+)</table>' % re.escape(ref),
                    body)
                if tab:
                    costs = [int(x) for x in tab.group(1).split()]
            else:
                costs = [int(ref)] * max(1, min(max_level, 100))
        kinds = re.findall(r'<using kind="([^"]+)"', body)
        weapons = []
        if kinds:
            weapons = sorted({k.strip() for k in
                              kinds[0].split(',') if k.strip()})
        skill_casts[skill_id] = {
            'operate': op.group(1) if op else '',
            'target': tg.group(1) if tg else '',
            'mp': costs,
            'range': int(cr.group(1)) if cr else 0,
            'reuse': int(rd.group(1)) if rd else 0,
            'hit': int(ht.group(1)) if ht else 0,
            'magic': bool(mag),
            'buff': int(abt.group(1)) if abt else 0,
            'weapons': weapons,
        }

        # -------------------------------------------------------- descriptions
        # The XML comments of the block are the classic client tooltip
        # texts. "Level N: text" comments describe one level each (the
        # enchant route comments "Level N (+k Cost)" and the levels
        # beyond the declared level count never count); a single
        # unlabeled comment describes the whole skill (the masteries,
        # the NPC effects) and only leads when no level comment came
        # before it. Consecutive levels with the same text collapse
        # into one run, so the masteries cost one entry instead of
        # their level count.
        runs = []
        for cm in re.finditer(r'<!--\s*(.*?)\s*-->', body):
            comment = cm.group(1)
            dm = re.match(r'Level (\d+)(\s*\([^)]*\))?:\s*(.*)',
                          comment)
            if dm:
                if dm.group(2):
                    continue
                level = int(dm.group(1))
                if level < 1 or level > max_level:
                    continue
                desc = dm.group(3).strip()
            elif not runs:
                level = 1
                desc = comment.strip()
            else:
                continue
            if not desc or desc.startswith(('FIXME:', 'TODO:')):
                continue
            if runs and runs[-1][1] == desc:
                continue
            runs.append((level, desc))
        if runs:
            skill_descs[skill_id] = runs

# ---------------------------------------------------------------- class trees
# One skillTree block per class: the entries plus the parentClassId
# chain that completes the tree (a 1st class tree only lists its own
# window, the starting class entries come from the parent). The
# lesson entries carry the skill book requirement as <item> children
# (only the level that needs the book lists it - Defence Aura level
# 1 wants the spellbook, levels 2+ do not).
class_entries = {}
class_parent = {}
for group in ('StartingClass', '1stClass', '2ndClass', '3rdClass'):
    group_dir = os.path.join(skill_trees_dir, group)
    if not os.path.isdir(group_dir):
        continue
    for path in sorted(glob.glob(os.path.join(group_dir, '*.xml'))):
        text = open(path, encoding='utf-8').read()
        for tm in re.finditer(
                r'<skillTree type="classSkillTree"[^>]*?classId="(\d+)"'
                r'(?:[^>]*?parentClassId="(\d+)")?[^>]*>(.*?)</skillTree>',
                text, re.S):
            class_id = int(tm.group(1))
            parent = tm.group(2)
            body = tm.group(3)
            if parent is not None:
                class_parent[class_id] = int(parent)
            entries = class_entries.setdefault(class_id, [])
            for sm in re.finditer(
                    r'<skill skillName="[^"]*" skillId="(\d+)"'
                    r' skillLevel="(\d+)" getLevel="(\d+)"[^>]*?'
                    r'(?:/>|>)', body):
                attrs = sm.group(0)
                skill_id, level = int(sm.group(1)), int(sm.group(2))
                get_level = int(sm.group(3))
                sp = re.search(r'levelUpSp="(\d+)"', attrs)
                auto = 'autoGet="true"' in attrs
                book = 0
                if not attrs.endswith('/>'):
                    close = body.find('</skill>', sm.end())
                    inner = body[sm.end():close if close > 0 else sm.end()]
                    bm = re.search(r'<item id="(\d+)"', inner)
                    if bm:
                        book = int(bm.group(1))
                entries.append((skill_id, level, get_level,
                                int(sp.group(1)) if sp else 0, auto,
                                book))

# The complete tree of a class: its own entries plus the parent chain,
# deduped by (skillId, skillLevel) with the lowest getLevel winning
# (the parent entry unlocks the same skill level earlier).
def complete_tree(class_id, seen=None):
    seen = seen or set()
    if class_id in seen or class_id not in class_entries:
        return []
    seen.add(class_id)
    merged = {}
    for chain_id in ([class_id] + walk_parents(class_parent.get(class_id))):
        for skill_id, level, get_level, sp, auto, book in class_entries[chain_id]:
            key = (skill_id, level)
            if key not in merged or get_level < merged[key][2]:
                merged[key] = (skill_id, level, get_level, sp, auto, book)
    return merged.values()

def walk_parents(parent):
    ids = []
    while parent is not None and parent in class_entries:
        ids.append(parent)
        parent = class_parent.get(parent)
    return ids

complete_trees = {}
for class_id in class_entries:
    entries = sorted(complete_tree(class_id),
                     key=lambda e: (e[2], e[0], e[1]))
    complete_trees[class_id] = entries

# ---------------------------------------------------------------- icons
# The icon pack carries the classic client skill icons as skillNNNN
# files: resolve every tree icon against the pack and fall back to the
# zero padded skill id when the pack has no such file (the icon name
# and the id naming agree for the skillNNNN family).
pack_files = set()
for name in os.listdir(icons):
    if name.endswith('.png'):
        pack_files.add(name[:-len('.png')])

def resolve_icon(skill_id, icon_name):
    for candidate in (icon_name, 'skill%04d' % skill_id):
        if not candidate:
            continue
        if candidate.startswith('icon.'):
            candidate = candidate[len('icon.'):]
        if candidate in pack_files:
            return candidate
    return ''

# ---------------------------------------------------------------- emit
def go_str(value):
    return '"%s"' % value.replace('\\', '\\\\').replace('"', '\\"')

lines = []
lines.append('// SPDX-FileCopyrightText: 2026 Melg Eight '
             '<public.melg8@gmail.com>')
lines.append('//')
lines.append('// SPDX-License-Identifier: MIT')
lines.append('')
lines.append('// Code generated by tools/generate_skill_trees.sh. '
             'DO NOT EDIT.')
lines.append('')
lines.append('package npcdata')
lines.append('')
lines.append('// skillInfos maps the skill id to the static display data of')
lines.append('// the skill: the name, the icon file of the pack, the')
lines.append('// operate type (P passive, A* active) and the warrior')
lines.append('// priority category (0 attack power, 1 defense, 2 other -')
lines.append('// see SkillCategoryAttack in skills.go).')
lines.append('var skillInfos = map[int32]SkillInfo{')
for skill_id in sorted(skill_infos):
    info = skill_infos[skill_id]
    icon = resolve_icon(skill_id, info['icon'])
    passive = 'true' if info['operate'] == 'P' else 'false'
    lines.append('\t%d: {Name: %s, Icon: %s, Passive: %s, Category: %d},'
                 % (skill_id, go_str(info['name']), go_str(icon),
                    passive, info['category']))
lines.append('}')
lines.append('')
lines.append('// skillDescs maps the skill id to the level description runs')
lines.append('// of the skill: the Mobius C1 skill stats XML comments - the')
lines.append('// classic client tooltip texts the server data carries. Every')
lines.append('// run covers the levels from its Level up to the next run (the')
lines.append('// consecutive levels with the same text collapse into one run,')
lines.append('// so the masteries cost one entry). SkillDescription in skills.go')
lines.append('// walks the runs and answers with the text of the exact level.')
lines.append('var skillDescs = map[int32][]SkillDesc{')
for skill_id in sorted(skill_descs):
    runs = skill_descs[skill_id]
    lines.append('\t%d: {' % skill_id)
    for level, desc in runs:
        lines.append('\t\t{Level: %d, Text: %s},' % (level, go_str(desc)))
    lines.append('\t},')
lines.append('}')
lines.append('')
lines.append('// skillTrees maps the class id to the complete skill tree of')
lines.append('// the class (its own entries plus the parent chain): every')
lines.append('// learnable (skillId, level) pair with the character level')
lines.append('// it unlocks at, the SP cost, the autoGet flag and the skill')
lines.append('// book item the lesson consumes (0 - no book). The entries are')
lines.append('// sorted by (getLevel, skillId, level).')
lines.append('var skillTrees = map[int32][]SkillLearn{')
for class_id in sorted(complete_trees):
    entries = complete_trees[class_id]
    if not entries:
        continue
    lines.append('\t%d: {' % class_id)
    for skill_id, level, get_level, sp, auto, book in entries:
        lines.append('\t\t{SkillID: %d, Level: %d, GetLevel: %d, '
                     'SpCost: %d, AutoGet: %s, BookItem: %d},'
                     % (skill_id, level, get_level, sp,
                        'true' if auto else 'false', book))
    lines.append('\t},')
lines.append('}')
lines.append('')
lines.append('// skillCasts maps the skill id to the cast data of the skill:')
lines.append('// the operate type (A1 active damage, A2 timed buff, P passive),')
lines.append('// the target type (ONE attacks the selected target, SELF lands')
lines.append('// on the caster), the per level mana cost, the cast range, the')
lines.append('// reuse delay and the hit time in milliseconds, the magic flag')
lines.append('// of the spells, the abnormal time of the timed buffs in')
lines.append('// seconds and the weapon kinds the <using> condition demands')
lines.append('// (empty - any weapon). See SkillCast in skills.go.')
lines.append('var skillCasts = map[int32]SkillCast{')
for skill_id in sorted(skill_casts):
    cast = skill_casts[skill_id]
    if cast['operate'] == 'P':
        continue
    mp = cast['mp']
    mp_list = ', '.join(str(x) for x in mp)
    weapons = ', '.join(go_str(k) for k in cast['weapons'])
    lines.append('\t%d: {Operate: %s, Target: %s, MPCost: []int32{%s}, '
                 'CastRange: %d, ReuseDelay: %d, HitTime: %d, '
                 'Magic: %s, BuffTime: %d, Weapons: []string{%s}},'
                 % (skill_id, go_str(cast['operate']),
                    go_str(cast['target']), mp_list, cast['range'],
                    cast['reuse'], cast['hit'],
                    'true' if cast['magic'] else 'false', cast['buff'],
                    weapons))
lines.append('}')
lines.append('')

with open(out, 'w', encoding='utf-8') as fh:
    fh.write('\n'.join(lines))

tree_classes = len(complete_trees)
tree_entries = sum(len(v) for v in complete_trees.values())
book_entries = sum(1 for e in
                   [entry for v in complete_trees.values() for entry in v]
                   if e[5])
cast_skills = sum(1 for c in skill_casts.values()
                  if c['operate'] != 'P')
icon_hits = sum(1 for e in
                [entry for v in complete_trees.values() for entry in v]
                if resolve_icon(e[0], skill_infos.get(e[0], {})
                                .get('icon', '')))
desc_skills = len(skill_descs)
desc_runs = sum(len(v) for v in skill_descs.values())
desc_bytes = sum(len(t) for v in skill_descs.values() for _, t in v)
tree_desc_hits = sum(1 for sid in {e[0] for v in
                     complete_trees.values() for e in v}
                     if skill_descs.get(sid))
print('skill stats: %d skills, class trees: %d classes, %d entries,'
      % (len(skill_infos), tree_classes, tree_entries))
print('tree entries with a resolved icon: %d' % icon_hits)
print('lessons requiring a book: %d, active skills with cast data: %d'
      % (book_entries, cast_skills))
print('descriptions: %d skills, %d runs, %d chars; tree skills with'
      ' a description: %d' % (desc_skills, desc_runs, desc_bytes,
                              tree_desc_hits))
print('written: %s' % out)
PYEOF

# The emitted map literals need the tab alignment of the repository
# style - gofmt fixes the spacing when the toolchain is around.
if command -v gofmt >/dev/null 2>&1; then
    gofmt -w "${OUT}"
fi
