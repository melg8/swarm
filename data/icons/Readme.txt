##############################################
ITEM ICON PACK OF THIS REPOSITORY
##############################################

3134 item icons of the classic Lineage 2 client naming scheme
(icon.weapon_..., icon.armor_..., icon.accessary_..., icon.etc_...),
32x32 PNG files, 13 MB total.

Source: data/l2icons of the xMlex/l2walker GitHub mirror, as recommended
by the project user. The pack is the Interlude era client icon set.

C1 compatibility was verified against the Mobius C1 server item stats
(dist/game/data/stats/items/*.xml), which carry the canonical icon name
of every C1 item:

- 4044 of 4222 C1 items resolve by the exact C1 icon name.
- The remaining 178 items (mostly the low grade starter armor of the
  20-40 id range: bone/bronze/leather gear, whose icons the later
  clients renamed to the generic armor_tXX pattern) resolve through
  the l2walker Interlude item database (data/db/db.sqlite of the
  l2walker checkout): the same item id carries the renamed icon of
  the same artwork family in the pack.
- Result: 4222/4222 = 100% of the C1 items resolve to an icon file.

The mapping item id -> icon file name is generated into
internal/swarm/npcdata/item_icons.go by tools/generate_item_icons.sh;
the web interface serves the PNGs from this directory at
/icons/<name>.png.
