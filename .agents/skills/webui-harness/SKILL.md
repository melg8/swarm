---
name: webui-harness
description: >-
  How to change the swarm web UI (plain HTML/CSS/JS in
  internal/swarm/webserver/web) safely: the four Node repro harnesses,
  the vm-sandbox constraints, the keyed rendering rules and the
  end-to-end path of a snapshot field. Use for any map, HUD, equipment
  widget, chat or style change, for adding snapshot fields, panels or
  toolbar toggles, and whenever a harness fails - even if the user
  only says "карта глючит" or "добавь панель".
---

# Web UI harness (swarm)

The web UI is plain HTML/CSS/JS without a build step (embedded via
go:embed). That is a project rule, not a constraint of convenience: no
framework, no bundler, no npm.

## The verification loop

Four harnesses load the REAL `web/app.js` / `web/map.js` into a Node vm
sandbox with a stub DOM and recording canvas. They exit 1 while their
bug is present, so they are regression gates:

```bash
node tools/repro_hud.js          # HUD, target panel, chat window
node tools/repro_gear.js         # equipment widget (79 checks)
node tools/repro_map_render.js   # markers, links, camera, draw order
node tools/repro_movement.js     # movement interpolation vs the
                                 # simulated Mobius server
```

Run every harness you could plausibly have affected plus `go test
./internal/swarm/webserver/`. A UI change without a harness check is
not verified.

## The sandbox constraints (why harnesses break)

- No top-level DOM access in `app.js`/`map.js`: every
  `document.querySelector`, `Image`, `localStorage` call at load time
  must stay inside an init function or behind a
  `typeof Image === "undefined"` guard - the vm sandbox has none of
  those and a top-level call throws before the harness can act.
- Top-level `const` in classic scripts is not a `window` property.
  Cross-file references (app.js -> map.js -> main.js, load order fixed
  in index.html) use the bare binding name only.
- New harness scenarios go into the existing `tools/repro_*.js` files,
  following their `check()` pattern; the stub DOM lives per harness -
  extract shared pieces only into `tools/` helpers, never into the
  shipped web code.

## Rendering rules (learned the hard way)

- **Gear widget is keyed-incremental.** One persistent cell record per
  paperdoll slot / bag item (`GearCells` registry in app.js); an
  `<img>` element must NEVER be recreated by a snapshot re-render (the
  browser re-decodes and the icons blink). Update count/enchant badges
  in place, move cells with appendChild. Replacing this with an
  innerHTML rebuild reintroduces the flicker bug - the GearCells
  comment documents the history.
- **Full re-render panels** (HUD, chat, footer) rebuild freely; the
  log panel is append-only with a watermark (`seenEvents`) - a changed
  filter must re-render deterministically.
- **Map draw order is deterministic**: dead units first, then north to
  south, then object id - the snapshot objects arrive in random Go map
  order and overlapping units flicker otherwise.
- Marker palette (`mapColors` in map.js) is theme-independent on
  purpose: it must read over the light map imagery in both themes.

## Adding a snapshot field end to end

The best-practice path (each step small):

1. Track it in `internal/swarm/state` (apply method + field, guarded
   by the tracker mutex), bump the version via the existing touch.
2. Copy it into `Snapshot` in `state/bot.go` with a JSON tag.
3. Render it in `web/app.js` (a `renderX` fan-out from
   `renderSnapshot`) or draw it in `web/map.js` (inside the transform,
   scaled by the zoom factor like every world element).
4. Add a harness check for the rendered result.
5. Document the field in AGENTS.md if it adds a visible feature.

Server names, chat lines and event texts reach the DOM as
`textContent`/`fillText` only - keep it that way (XSS discipline).
