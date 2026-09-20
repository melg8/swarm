# webui modernization proposal (awaiting approval)

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

Proposals for the swarm web interface refresh: readability, a modern
look, ergonomics. The document was written for approval: every item
is numbered, the checklist sits at the end. Nothing lands in the
interface before the approval.

## 1. How the analysis was done

- Live screenshots of every mode at 1440×900 (the Mobius stack + the
  bot `-hunt`): bot light/dark, the open view dropdown, the Log tab,
  the shop flyout, the zone panel, the pathfind test (8081), the
  fight showcase v1 (8082), the fight gallery (8083). Screenshots:
  `/home/z/my-project/download/audit/`.
- A full pass of `style.css` (2169 lines), `index.html` (377),
  the UI logic of `app.js`/`map.js`/`main.js`.
- Geometry measurements (light, bot): header 1440×34, sidebar
  200×840, toolbar 1240×37, map 1240×803, HUD 252×253 (top left),
  gear 254×395 (top right), zone 254×30 (bottom right, collapsed),
  chat 416×150 (bottom left), footer 1440×26.
- An independent VLM review of the screenshots (senior UI/UX
  critique).

## 2. What already works (untouched)

- The compact 34px header, one 37px toolbar row, the 26px footer -
  the vertical budget of the map is already squeezed (the past
  rounds).
- The unified HP/MP/XP palette of the "classic L2 C1" - a deliberate
  style.
- The theme switches and persists in the localStorage.
- The live indicator with the glow, the bot-status pulse, the caret
  turns, the slide-in flyout of the shop queue - the sprouts of the
  "live" UI are already there.
- All the layout is vanilla CSS/JS without a build - it stays that
  way: every proposal lands without new dependencies.

## 3. The diagnosis

| Axis | The main problem |
| --- | --- |
| Readability | 91 of 103 `font-size` declarations are ≤ 12px, the minimum 8px; the log lines have no rhythm; the XP bar is almost invisible in light; in dark the shadows are off (`--shadow: none`) - the panels merge with the background |
| Modern look | The rigid 1px borders, the 4-6px radius, the "capsule" ALL-CAPS 10px micro text with the 2px letter spacing, the text glyphs (◐ ▾ ◂) instead of the icons, 5 transitions on the whole CSS, 10 saturated activity colors |
| Ergonomics | The panels do not move and do not collapse (zone/shop excepted), not a single :focus-visible (2 inputs excepted), no hotkeys, the compass N is covered by the zone panel, the inventory always shows 4 empty rows, no media queries, the log has no filter chips and no pause |

The floating panels with the shop flyout open cover 508px of the
1240px map width (41%): HUD 252 left + gear 254 and shop 240+14
right.

## 4. Phase A - readability

### A1. Raise the base typography scale

Problem: body 13px; the labels 8px (slot-label), 8.5px
(icon-badge, shop-buying-chip), 9px (shop-meta, shop-missing,
shop-key), 9.5px (drop-hint). On a 1440×900 desktop this is the
squint mode; the VLM review of both themes named it the micro-font
syndrome.

Proposal: body 13 → 14px; a hard 10px minimum for every readable
text (the badges and the counters - 10px, the micro captions -
10.5px); the numeric badges allow a 10px mono.

Effect: the interface reads from a distance, less of the cheap
engineering look.
Size: S. Risk: the fixed panel widths (HUD 252, gear 254, shop 240)
- recheck the line wrapping, a +6-10px width is possible. Files:
style.css (+ the spot edits of the repro checks).

### A2. The vertical rhythm and the line density

Problem: the log lines - padding 2.5px, the default line-height;
the HUD kv lines - 2px; the legend and the captions have no air.
VLM: a "wall of text", a log line is easy to lose with the eyes.

Proposal: body line-height 1.45; the log - 3.5-4px vertical padding
+ line-height 1.5; kv - 3px padding; the inner panel paddings grow a
bit (10/12 → 12/14px).

Effect: the log and the stats scan without losing a line.
Size: S. Risk: the list heights (chat 150px, shop 112px max-height)
- recount the visible capacity.

### A3. The panel micro headers

Problem: sidebar-title / shop-head / drop-head / gear-key - 10px
ALL-CAPS with the 2px letter spacing. The blurry "capsule" captions
are the visual marker of the 2000s utilities.

Proposal: 10.5-11px, letter-spacing 0.8-1px, the text-dim color, a
normal-case semibold for the long headers (Equipment, Hunting
zones, Shop queue), the CAPS stays only for the 1-2 word ones
(EQUIPMENT → Equipment).

Effect: a modern tone at the same density.
Size: S. Risk: none.

### A4. The XP bar: the readable light silver

Problem: the XP gradient (#f4f4f4 → #9096a0) on the white panel is
almost invisible - the VLM flagged both vitals panels as a contrast
failure.

Proposal: without touching the classic palette - a dark track
backdrop (bg-panel-2 darkened locally) + a 1px inner stroke of the
fill; in the dark theme the track is a bit lighter than the
background. Optional: a fine 25/50/75% notching.

Effect: the third vital reads as well as HP/MP.
Size: S. Risk: none.

### A5. The dark theme: the layers and the depth

Problem: `html[data-theme="dark"] { --shadow: none }` - all the
panels of one flat plane, they merge with the map background.

Proposal: bring back a soft shadow (0 1px 2px + 0 8px 24px
rgba(0,0,0,0.4)), the border a bit lighter (-#2a303c → #333a47),
bg-panel one step up (#171b22 → #1a1f27).

Effect: dark stops being a "flat mud", a panel "detaches" from the
map.
Size: S. Risk: none.

### A6. The log: the line scannability

Problem: the same density of every line; the green of the spawn
dominates and cheapens the semantics; the timestamps of one second
repeat by the dozens.

Proposal: zebra-striping (the odd lines bg-panel-2 50%); the
timestamp stays dim but the repeats of the same second go away (an
empty space instead of the repeats - smart timestamps); the green
is reserved for loot/level-up, the spawn recolors into the neutral
text.

Effect: the eye holds the line; the important events jump out.
Size: S-M (smart timestamps - a log.js edit).
Risk: the log render tests may expect the current classes.

### A7. The tabular figures

Problem: the numbers that change at runtime (footer, map-scale,
map-objects, the hud values) jump in width.

Proposal: `font-variant-numeric: tabular-nums` on every numeric
cell (vital-text is already mono - add it to kv b, foot-item,
map-scale, map-objects, shop-val).

Effect: the figures do not "breathe" on the once a second refresh.
Size: S. Risk: none.

## 5. Phase B - the modern look

### B1. The panel chrome

Problem: the rigid 1px borders, the 4-6px radius, the single 1px
shadow - "windows 98 chrome" per the VLM.

Proposal: the floating panels (HUD, gear, zone, chat, shop,
drop-dialog, view-menu-pop, item-tooltip) - radius 8-10px,
border-soft, a two layer shadow (0 1px 2px rgba(20,30,45,0.10),
0 10px 28px rgba(20,30,45,0.16)); the inline elements (the buttons,
the chips, the cells) radius 6px.

Effect: the cheapest and the most visible step toward the modern
look.
Size: S. Risk: none.

### B2. The optional "glass" for the map overlays

Problem: the opaque panels cover the map dead.

Proposal: for the floating panels over the canvas - a translucent
backdrop (an rgba of the panel color 0.82-0.88) +
`backdrop-filter: blur(10px)` under
`@supports (backdrop-filter: blur(2px))`, the degradation to the
current look. One CSS variable --glass controls it (off - the exact
current look), so one word rolls it back.

Effect: the map reads through the panels, a "modern MMO overlay".
Size: M. Risk: the canvas repaint under the blur may cost FPS on the
weak machines - hence switchable; verify in a live session.

### B3. The micro motion

Problem: 5 transitions on the whole CSS; the hover changes only the
color - the interface is "static".

Proposal: one token `--t-fast: 140ms ease`; the hover transitions
(bg/color/border) on .btn, .tab, .check, .zone-item, .shop-item,
.fight-colhead, .bot-item; the view-menu-pop appearance - fade+scale
0.98→1 (transform-origin: top left) 120ms; the shop flyout is
already animated - keep it. No bounce/parallax.

Effect: a "live" response without the risk.

Size: S-M. Risk: prefers-reduced-motion - respect it through a
media query.

### B4. The thin scrollbars globally

Problem: the custom thin scrollbars live only in shop/inv/fight -
the rest are the system ones.

Proposal: `scrollbar-width: thin` + the webkit rules on every list
(bot-list, zone-list, chat-list, log-list, pf-list), one thumb
var(--border) radius 3px.

Effect: the integrity. Size: S. Risk: none.

### B5. The active tab

Problem: the active tab differs only by the text and the backdrop.

Proposal: a 2px accent underline indicator with a 150ms animation
(the ::after pseudo element), the hover bg stays.

Effect: the classic modern navigation pattern.
Size: S. Risk: none.

### B6. The SVG icons instead of the text glyphs

Problem: the theme button "◐", the carets ▾/◂/▸, the compass "N" -
the text glyphs render differently across the systems, they smear
on the retina; the UI already has 2 SVGs (copy, trash) - a style
mix.

Proposal: one inline-SVG set with a 1.75px stroke: theme-toggle
(sun/moon), view-menu-caret, shop-tab-chev, zone-panel-chev, compass
N (an arrow with the N). The size 12-16px, currentColor.

Effect: the crispness on hidpi, one icon language.
Size: M (a small but fiddly index.html + CSS edit).
Risk: low.

### B7. The activity colors: fewer screaming ones

Problem: 10 saturated colors (kind-*) paint the text, the border and
the bot dot at once; the rainbow tires and looks cheap.

Proposal: the text - the neutral text/text-dim; the color stays with
the dot and the left bar (border-left), the tints muted by ~15%
saturation; the map status banner - the same principle (a neutral
text, a colored dot).

Effect: a calm professional tone, the semantics kept.
Size: S. Risk: the habit - but the colors remain on the dots.

### B8. The state chips

Problem: chip-combat/chip-rest/chip-level - framed, of different
sizes; shop-buying-chip 8.5px.

Proposal: one chip metric (10px, radius 4px, padding 1px 6px); the
critical states (combat) - a tinted backdrop (12% of the color)
instead of the pure frame; shop-buying - filled with the accent.

Effect: the statuses are visible by the peripheral vision.
Size: S. Risk: none.

## 6. Phase C - ergonomics

### C1. The hotkeys

Problem: no hotkeys (Esc/Enter in the dialogs excepted). The constant
operations demand the mouse: the tab switch, the follow, the view
layers.

Proposal: `M`/`L` - the Map/Log tabs (or 1/2), `F` - follow, `V` -
the view menu, `T` - the theme. Active only when the focus is not in
a text input (log-filter, drop-count). The hints: in the title of
the elements and a short footer line (for example, `M map · L log ·
F follow · V view · T theme` - dim mono, like the current
foot-item).

Effect: the operator speed; the "serious tool" feel lands.
Size: S-M. Risk: a conflict with the future inputs - keep the focus
exception.

### C2. The keyboard focus

Problem: `:focus` exists only on the 2 text inputs; the tabs, the
buttons, the checkboxes, the view menu items show no focus - the
keyboard navigation is impossible.

Proposal: a global `:focus-visible { outline: 2px solid
var(--accent); outline-offset: 2px; }` + a thin edit of the input
outline look (the border-accent already exists).

Effect: the accessibility, the "modern toolkit" level.
Size: S. Risk: none.

### C3. The compass N is covered by the zone panel

Problem: map-rose (bottom 10, right 12) and zone-panel (bottom 12,
right 12) sit in one corner - the panel covers the compass.

Proposal: move the compass to the map's left bottom corner (above
the chat? no - into the chat-box left bottom) - better the top-right
under the gear panel (top 10, right 278) or the left-top under the
HUD (top 284, left 12). The calmest option: right top, below the
gear.

Effect: the map orientation is always visible. Size: S. Risk: none.

### C4. The floating panels: the drag and the collapse

Problem: HUD, gear, chat are nailed to the corners; nothing
collapses (the collapse pattern exists only in zone). With the shop
flyout open 41% of the map width is covered.

Proposal: one "panel head" pattern: the panel header drags (pointer
events, clamped to the map-wrap bounds), a collapse button (the same
chevron icon as zone); the positions/the states in the localStorage
(like swarm.theme). HUD gets a thin header row (the bot name is
already there - drag by it), gear - an EQUIPMENT header, chat - a
narrow header strip.

Effect: the main ergonomic win - the operator decides what sits
where and what is hidden; the dense map frees up in seconds.
Size: M-L. Risk: the drag over the canvas must not fight the map pan
gestures (start on the header only); the localStorage keys are
versioned.

### C5. The chat: the collapse and the new badge

Problem: the chat 416×150 covers the map all the time; no collapse.

Proposal: a minimize button in the chat header (C4 gives the common
pattern) + an "N new" badge on the collapsed strip, reset on the
expand.

Effect: a freer map; the events are not lost.
Size: M (cheap together with C4). Risk: none.

### C6. The inventory: the adaptive height

Problem: inv-grid is always 153px (4 rows) - with an empty bag a
huge void sits in the gear panel (VLM: a "massive void").

Proposal: grid-auto-rows + max-height 153px, the actual height =
the rows of the item count (min 1 visible row, an "empty" caption at
0); the footer (adena/weight/trash) presses up.

Effect: the panel looks "sized to the data".
Size: S-M. Risk: the shop flyout height position recount -
verify.

### C7. The legend: a collapsible section

Problem: the 8 legend rows + the dense note always occupy the
sidebar bottom.

Proposal: the legend collapses (modeled after zone-panel): a LEGEND
header + chevron, collapsed by default, the note ("tick = look
direction…") - into the header tooltip.

Effect: -150px of the standing noise in the sidebar; the reference
on demand.
Size: S. Risk: the newcomers must find the legend - a chevron header
is visible enough.

### C8. The toolbar right edge: the telemetry as capsules

Problem: map-scale and map-objects - a dim mono text "1:0.12 ·
25 objects · 10,333 units" without structure; VLM: an
"afterthought".

Proposal: two-three compact capsules (mono 10.5px, a chip style with
border-soft): `1:0.12`, `25 obj`, `10.3k units` + a tooltip with the
breakdown; the numbers tabular-nums (A7).

Effect: the telemetry looks designed, not tacked on.
Size: S. Risk: the toolbar width - watch the wrap at 1280px.

### C9. The responsive behavior

Problem: no media queries at all; body overflow: hidden. Below
~1200px the floating panels and the sidebar start to fight.

Proposal: 2 points: <1280px - sidebar 200 → 176px, the panel fonts
-0.5px; <1024px - the sidebar hides into an icon rail (a 28px strip
with the bot dots, a hover expand) - the map gets the space. HUD/
gear stay in the corners. Test at 1024×768 and 1440×900.

Effect: the UI lives on the laptop screens and the half windows.
Size: M. Risk: medium - careful checks in a live session.

### C10. The empty states

Problem: with no bot selected the panels show the "—" rows; the log
is empty until the first event.

Proposal: explicit placeholder states: HUD - "select a bot"; the log
- "waiting for events…"; both - dim, centered in the zone, gone once
the data arrives.

Effect: the interface does not look "broken" at the start.
Size: S-M. Risk: none.

### C11. Follow → a switch toggle

Problem: follow - the most used switch, hidden as a 12px checkbox.

Proposal: a CSS-only switch (34×18px, knob 14px, transition 140ms)
on the same input; the "follow" label stays.

Effect: the frequent operation is more visible and the mouse lands
faster.
Size: S. Risk: low.

### C12. The item tooltip: the delay and the appearance

Problem: the tooltip flashes instantly as the mouse crosses the
inventory - it flickers.

Proposal: a 100ms delay + a 120ms fade-in; the disappearance without
a delay. The positioning stays untouched (the left-anchor is already
done).

Effect: a calm, "expensive" hover. Size: S. Risk: none.

## 7. The dev mode odds and ends

- G1. FIGHT FX VARIANTS: the description block is in Russian inside
  the English UI - unify the language (English proposed: "Every
  variant plays the same fight in one tick: the hero hits, takes a
  hit, lands a crit and takes a crit - four enemy positions
  vertically, the ideas horizontally."). Size: S.
- G2. Pathfind: raise the search stats (search time, nodes…) 11 →
  12px, the values bold; the geodata-path mono block - the
  word-break is already there. Size: S.

## 8. What we deliberately do NOT change

- The classic HP/MP/XP palette of L2 C1 - the deliberate style of
  the project (A4 fixes only the track visibility, not the colors).
- The compact vertical budget: the 34px header, the 37px toolbar,
  the 26px footer - not given away.
- The dense "panel tool" character and the informativeness: the
  telemetry does not hide into the accordions beyond C7/C4.
- The vanilla CSS/JS without a build and the dependencies - every
  proposal lands in the current stack.
- The canvas map render (the markers, the aggro zones, the tiles) -
  a separate topic, outside this document.
- The Russian fight description keeps its meaning through the
  translation (G1).

## 9. The landing order

| Wave | The composition | The felt result |
| --- | --- | --- |
| 1. The polish (≈1 day) | A1-A5, A7, B1, B4, B5, C2, C3 | The interface visually "gets younger" without a structure change: the font, the shadows, the borders, the focus |
| 2. The liveliness and the order (≈1-2 days) | A6, B3, B6-B8, C1, C7, C8, C10, C11, C12, G1, G2 | The motion, the icons, the hotkeys, the empty states, the calm status palette |
| 3. The operator freedom (≈2-3 days) | B2, C4, C5, C6, C9 | The draggable/collapsible panels, the glass, the adaptive layout |

Every wave is separate atomic commits (one item per commit, as the
branch convention goes), with the touched repro checks updated
(`tools/repro_*.js`), the live verification through the
agent-browser (the geometry, no console errors) and a VLM screenshot
review, like the past rounds.

## 10. The approval checklist

Mark the items to land (or write "all the waves 1-2, from 3 only
C4"):

| ID | The gist | Wave | Size |
| --- | --- | --- | --- |
| A1 | the base font 13→14px, the 10px minimum | 1 | S |
| A2 | the vertical rhythm, the line air | 1 | S |
| A3 | the micro headers without the heavy CAPS | 1 | S |
| A4 | the XP bar visible | 1 | S |
| A5 | the dark theme: the shadows and the depth | 1 | S |
| A6 | the log: zebra, smart timestamps, the color semantics | 2 | S-M |
| A7 | the tabular-nums figures | 1 | S |
| B1 | the panel chrome: radius, shadows | 1 | S |
| B2 | the glass overlays (opt-in) | 3 | M |
| B3 | the micro motion | 2 | S-M |
| B4 | the thin scrollbars everywhere | 1 | S |
| B5 | the active tab underline indicator | 1 | S |
| B6 | the SVG icons | 2 | M |
| B7 | the muted activity colors | 2 | S |
| B8 | the unified state chips | 2 | S |
| C1 | the hotkeys | 2 | S-M |
| C2 | :focus-visible | 1 | S |
| C3 | the compass out of the zone panel shadow | 1 | S |
| C4 | the panels: drag + collapse | 3 | M-L |
| C5 | the chat: minimize + the new badge | 3 | M |
| C6 | the inventory sized to the data | 3 | S-M |
| C7 | the legend collapses | 2 | S |
| C8 | the toolbar telemetry as capsules | 2 | S |
| C9 | responsive: 1280/1024 | 3 | M |
| C10 | the empty states | 2 | S-M |
| C11 | follow → switch | 2 | S |
| C12 | the tooltip: delay + fade | 2 | S |
| G1 | the fight description language | 2 | S |
| G2 | the pathfind stats larger | 2 | S |
