// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

// The state dump endpoint of the web UI: a plain text report of
// everything the tracker knows about one bot - the character sheet,
// the inventory and equipment, the world objects around with their
// combat state, the active walk plan, the hunting zone, the recent
// chat and a deep window of the rolling event log (the hunt loop
// decisions mirror into it, see the logger wiring of cmd/swarm). The
// Dump state button of the HUD copies the report into the clipboard,
// so a live session problem report comes with the full debugging
// material attached instead of a screenshot.

import (
    "fmt"
    "math"
    "net/http"
    "sort"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/version"
)

// dumpEventLimit bounds the event window of the dump: the snapshot
// streams the newest 100 for the UI log tail, the dump reads six
// times deeper - the story of a stuck session lives minutes back.
const dumpEventLimit = 600

// dumpChatLimit bounds the chat window of the dump.
const dumpChatLimit = 40

// handleBotDump serves the state dump of one bot as plain text.
func (s *Server) handleBotDump(w http.ResponseWriter, r *http.Request) {
    bot, ok := s.lookupBot(w, r)
    if !ok {
        return
    }
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    report := BuildStateDump(bot)
    if _, err := w.Write([]byte(report)); err != nil {
        s.logger.Printf("Error writing dump response: %v", err)
    }
}

// BuildStateDump assembles the plain text debug report of the bot.
// The layout mirrors the order a debugging session reads it in: the
// identity and the phase first, then the character where it stands,
// then what surrounds it, then the inventory, then the movement plan
// and at the end the long event log.
func BuildStateDump(bot *state.Bot) string {
    snap := bot.Snapshot()
    events := bot.NewestEvents(dumpEventLimit)
    b := &strings.Builder{}

    writeDumpHeader(b, snap)
    writeDumpCharacter(b, snap)
    writeDumpAttackers(b, snap)
    writeDumpZone(b, snap)
    writeDumpInventory(b, snap)
    writeDumpObjects(b, snap)
    writeDumpWalkPlan(b, snap)
    writeDumpCombat(b, snap)
    writeDumpChat(b, snap)
    writeDumpEvents(b, events)

    return b.String()
}

// writeDumpHeader writes the report title, the build identity and
// the session summary. The build line pins the exact code state the
// report came from - a live problem report never leaves room for
// guessing which commit produced it.
func writeDumpHeader(b *strings.Builder, snap state.Snapshot) {
    fmt.Fprintf(b, "swarm state dump\n")
    fmt.Fprintf(b, "build: %s\n", version.Identity())
    fmt.Fprintf(b, "bot: %s (status %s, phase %s)\n",
        snap.ID, snap.Status, snap.Phase)
    fmt.Fprintf(b, "dumped: %s\n", time.Now().Format(time.RFC3339))
    fmt.Fprintf(b, "session: started %s, uptime %s\n",
        snap.StartedAt.Format(time.RFC3339),
        time.Since(snap.StartedAt).Round(time.Second))
    fmt.Fprintf(b, "packets: %d, state version %d\n",
        snap.Packets, snap.Version)
    if !snap.ActionFailedAt.IsZero() {
        fmt.Fprintf(b, "last action failed: %s ago (the server "+
            "refused a request, see the refusal lines of the hunt "+
            "events)\n",
            time.Since(snap.ActionFailedAt).Round(time.Second))
    }
    fmt.Fprintf(b, "\n")
}

// writeDumpCharacter writes the full character sheet of the dump.
func writeDumpCharacter(b *strings.Builder, snap state.Snapshot) {
    c := snap.Character
    fmt.Fprintf(b, "character:\n")
    fmt.Fprintf(b, "  name: %s (object %d)\n", c.Name, c.ObjectID)
    fmt.Fprintf(b, "  class: %d, race: %d, level %d, exp %d (%.2f%%), sp %d\n",
        c.ClassID, c.Race, c.Level, c.Exp, c.ExpPercent, c.Sp)
    fmt.Fprintf(b, "  position: x %d, y %d, z %d, heading %d\n",
        c.X, c.Y, c.Z, c.Heading)
    fmt.Fprintf(b,
        "  vitals: HP %.0f/%.0f, MP %.0f/%.0f, sitting %v, in combat %v\n",
        c.CurHP, c.MaxHP, c.CurMP, c.MaxMP, c.Sitting, c.InCombat)
    if c.Moving {
        fmt.Fprintf(b, "  moving: yes -> x %d, y %d, z %d, speed %.0f\n",
            c.DestX, c.DestY, c.DestZ, c.Speed)
    } else {
        fmt.Fprintf(b, "  moving: no (speed %.0f)\n", c.Speed)
    }
    if c.TargetID != 0 {
        fmt.Fprintf(b, "  target: %d\n", c.TargetID)
    }
    fmt.Fprintf(b, "  stats: STR %d DEX %d CON %d INT %d WIT %d MEN %d\n",
        c.STR, c.DEX, c.CON, c.INT, c.WIT, c.MEN)
    fmt.Fprintf(b, "  load: %d/%d, slots %d/%d, adena %d\n",
        c.CurrentLoad, c.MaxLoad, c.InventorySlots, c.InventoryMax,
        c.Adena)
    fmt.Fprintln(b)
}

// writeDumpAttackers lists the living attackable npcs that hold the
// character as their target - the aggro load of the moment.
func writeDumpAttackers(b *strings.Builder, snap state.Snapshot) {
    lines := make([]string, 0, len(snap.Objects))
    for i := range snap.Objects {
        o := &snap.Objects[i]
        if o.Kind != state.KindNPC || !o.Attackable || o.Dead ||
            o.TargetID != snap.Character.ObjectID {
            continue
        }
        lines = append(lines, fmt.Sprintf(
            "  %d %s (level %d, hp %.0f/%.0f) at %d %d %d",
            o.ObjectID, o.Name, o.Level, o.CurHP, o.MaxHP,
            o.X, o.Y, o.Z))
    }
    fmt.Fprintf(b, "attackers (%d):\n", len(lines))
    for _, line := range lines {
        fmt.Fprintln(b, line)
    }
    fmt.Fprintln(b)
}

// writeDumpZone writes the hunting zone of the session.
func writeDumpZone(b *strings.Builder, snap state.Snapshot) {
    if snap.HuntingZone == nil {
        fmt.Fprintf(b, "hunting zone: none\n\n")

        return
    }
    z := snap.HuntingZone
    fmt.Fprintf(b, "hunting zone: center %d %d, half %d (square %dx%d)\n",
        z.CX, z.CY, z.Half, z.Half*2, z.Half*2)
    c := snap.Character
    inside := z.Contains(c.X, c.Y)
    fmt.Fprintf(b, "  character inside: %v\n\n", inside)
}

// dumpSlotNames maps the item body part mask to the paperdoll slot
// name; masks outside the table fall back to the hex form. The masks
// mirror the Mobius BodyPart enum (entity/item/enums/BodyPart.java):
// 0x02 the right ear, 0x04 the left ear, 0x08 the neck, 0x10 the
// right finger, 0x20 the left finger, 0x40 the head, 0x800 the legs,
// 0x1000 the feet, 0x2000 the back, 0x4000 the two hand weapon and
// 0x8000 the one-piece armor; the pair families carry the OR of their
// two slots (0x6 the earrings, 0x30 the rings). The old table shifted
// the jewel and armor labels (a Cloth Cap printed as lfinger, a
// Necklace of Magic as lear ear), which read like a corrupted
// paperdoll in the field reports while the equipment was fine.
var dumpSlotNames = map[int32]string{
    0x01:    "underwear",
    0x02:    "rear ear",
    0x04:    "lear ear",
    0x06:    "earring",
    0x08:    "necklace",
    0x10:    "rfinger",
    0x20:    "lfinger",
    0x30:    "ring",
    0x40:    "head",
    0x80:    "rhand",
    0x100:   "lhand",
    0x200:   "gloves",
    0x400:   "chest",
    0x800:   "legs",
    0x1000:  "feet",
    0x2000:  "back",
    0x4000:  "lrhand",
    0x8000:  "full armor",
    0x10000: "hair",
}

// dumpSlotName renders the paperdoll slot of the item body part mask.
func dumpSlotName(bodyPart int32) string {
    if name, ok := dumpSlotNames[bodyPart]; ok {
        return name
    }

    return fmt.Sprintf("part 0x%x", bodyPart)
}

// writeDumpInventory writes the equipment and the bag of the dump.
func writeDumpInventory(b *strings.Builder, snap state.Snapshot) {
    var equipped, bag []string
    for i := range snap.Inventory {
        item := &snap.Inventory[i]
        name := item.Name
        if name == "" {
            name = fmt.Sprintf("item %d", item.ItemID)
        }
        line := fmt.Sprintf("  object %d: %s (item %d) x%d%s",
            item.ObjectID, name, item.ItemID, item.Count,
            enchSuffix(item.Enchant))
        if item.Equipped {
            equipped = append(equipped,
                line+" ["+dumpSlotName(item.BodyPart)+"]")
        } else {
            bag = append(bag, line)
        }
    }
    fmt.Fprintf(b, "equipment (%d):\n", len(equipped))
    for _, line := range equipped {
        fmt.Fprintln(b, line)
    }
    // The paperdoll holes an equipment section hides: the report only
    // prints the occupied slots, so a character farming without its
    // legs armor (the 2026-09-12 04:58 pantsless dump) read as a fine
    // outfit - the missing legs line was invisible. The empty families
    // now name themselves right below the equipment.
    if empty := dumpEmptySlots(snap.Inventory); len(empty) > 0 {
        fmt.Fprintf(b, "empty slots: %s\n", strings.Join(empty, ", "))
    }
    fmt.Fprintf(b, "bag (%d):\n", len(bag))
    for _, line := range bag {
        fmt.Fprintln(b, line)
    }
    fmt.Fprintln(b)
}

// dumpEmptySlots lists the paperdoll slot families no equipped item
// covers: the armor and hand slots, the necklace and the unfilled pair
// halves (one earring or ring equipped names the other half empty).
// The two hand weapon blocks the left hand and the one-piece armor the
// legs, so those stay unlisted while the blocker is worn. The
// underwear and hair slots stay out: no gear of the catalogs ever
// fills them and they would read as permanent holes.
func dumpEmptySlots(items []state.InventoryItemSnapshot) []string {
    covered := make(map[int32]int, len(items))
    twoHand, onePiece := false, false
    for i := range items {
        item := &items[i]
        if !item.Equipped {
            continue
        }
        covered[item.BodyPart]++
        switch item.BodyPart {
        case 0x4000:
            twoHand = true
        case 0x8000:
            onePiece = true
        }
    }
    families := []struct {
        mask int32
        name string
    }{
        {0x40, "head"},
        {0x80, "rhand"},
        {0x100, "lhand"},
        {0x200, "gloves"},
        {0x400, "chest"},
        {0x800, "legs"},
        {0x1000, "feet"},
        {0x2000, "back"},
        {0x08, "necklace"},
    }
    empty := make([]string, 0, len(families)+2)
    for _, family := range families {
        if covered[family.mask] > 0 {
            continue
        }
        if twoHand && (family.mask == 0x80 || family.mask == 0x100) {
            continue
        }
        if onePiece && (family.mask == 0x400 || family.mask == 0x800) {
            continue
        }
        empty = append(empty, family.name)
    }
    if covered[0x6] == 1 {
        empty = append(empty, "earring half")
    }
    if covered[0x30] == 1 {
        empty = append(empty, "ring half")
    }

    return empty
}

// enchSuffix renders the enchant level of a dump item line.
func enchSuffix(enchant int16) string {
    if enchant <= 0 {
        return ""
    }

    return fmt.Sprintf(" +%d", enchant)
}

// writeDumpObjects writes the known world objects sorted by their
// distance to the character - the closest first, the combat state and
// the targets included (a mob holding the character is the aggro).
func writeDumpObjects(b *strings.Builder, snap state.Snapshot) {
    c := snap.Character
    type ranked struct {
        line  string
        distX float64
        distY float64
    }
    ranked2 := make([]ranked, 0, len(snap.Objects))
    for i := range snap.Objects {
        o := &snap.Objects[i]
        dx := float64(o.X - c.X)
        dy := float64(o.Y - c.Y)
        name := o.Name
        if o.Title != "" {
            name += " [" + o.Title + "]"
        }
        if name == "" {
            name = fmt.Sprintf("%s %d", o.Kind, o.ObjectID)
        }
        line := fmt.Sprintf(
            "  %d %-7s %s (level %d, hp %.0f/%.0f) at %d %d %d, dist %.0f",
            o.ObjectID, o.Kind, name, o.Level, o.CurHP, o.MaxHP,
            o.X, o.Y, o.Z, math.Hypot(dx, dy))
        var flags []string
        if o.Attackable {
            flags = append(flags, "attackable")
        }
        if o.Dead {
            flags = append(flags, "dead")
        }
        if o.InCombat {
            flags = append(flags, "combat")
        }
        if o.Moving {
            flags = append(flags, "moving")
        }
        if o.TargetID != 0 {
            flags = append(flags, fmt.Sprintf("target %d", o.TargetID))
        }
        if o.Aggressive {
            flags = append(flags, "aggressive")
        }
        if len(flags) > 0 {
            line += ", " + strings.Join(flags, ", ")
        }
        ranked2 = append(ranked2, ranked{line: line, distX: dx, distY: dy})
    }
    sort.SliceStable(ranked2, func(i, j int) bool {
        return math.Hypot(ranked2[i].distX, ranked2[i].distY) <
            math.Hypot(ranked2[j].distX, ranked2[j].distY)
    })
    fmt.Fprintf(b, "objects (%d):\n", len(ranked2))
    for _, entry := range ranked2 {
        fmt.Fprintln(b, entry.line)
    }
    fmt.Fprintln(b)
}

// writeDumpWalkPlan writes the active walk plan of the hunt loop: the
// whole leg from the planning origin (where we wanted to go from) to
// the final destination (where we want to arrive), with every passed
// waypoint marked, the per waypoint timing (when the follower reached
// it and how long the leg took - a point the bot dawdles on shows its
// cost right on the line) and the waypoint the follower currently
// aims at emphasized with its walking time - a stuck or drifting walk
// reads at a glance. When the live plan is already gone (the walk
// arrived, timed out or the loop left the phase), the most recent
// plan prints instead - the report of a stuck leg needs the whole
// planned walk even when the walk is over (the owner pathfind test
// round: the double click plans, the bot walks, the dump names the
// waypoint it stuck on).
func writeDumpWalkPlan(b *strings.Builder, snap state.Snapshot) {
    if snap.WalkPath == nil {
        if snap.LastWalkPath == nil {
            fmt.Fprintf(b, "walk plan: none\n\n")

            return
        }
        writeWalkPlanSection(b, "last walk plan (",
            snap.LastWalkPath, snap.LastWalkOrigin,
            snap.LastWalkIndex, snap.LastWalkDest,
            snap.LastWalkSearch,
            snap.LastWalkStart, snap.LastWalkAt, snap.LastWalkWpAt)

        return
    }
    writeWalkPlanSection(b, "walk plan (", snap.WalkPath,
        snap.WalkOrigin, snap.WalkIndex, snap.WalkDest,
        snap.WalkSearch,
        snap.WalkStart, snap.WalkAt, snap.WalkWpAt)
}

// writeWalkPlanSection prints one walk plan section under the given
// header prefix: the waypoint count with the search word of the mesh
// contract the plan answers (dry walls the water, swim prices it -
// the repro word the pathfind link replays the search with; no word
// for the direct legs no mesh search produced), the follower cursor,
// the planning origin, the walk zero point (the started line the
// timing suffixes read against), every waypoint with the aimed one
// emphasized and the final destination. The timing suffixes print
// from the observed arrival times (the walkWpAt record): a passed
// waypoint carries its moment on the walk timeline (t+) and the leg
// duration from the previous waypoint (or the start), the aimed one
// carries the time the walk already spends on it - the stuck leg
// number. The last walk plan reads the same suffixes against the
// moment the plan ended (the finished walk keeps the leg durations,
// an unfinished one shows how long the follower sat on the waypoint
// it never confirmed).
func writeWalkPlanSection(
    b *strings.Builder, headerPrefix string,
    path []state.WalkPoint, origin *state.WalkPoint, index int,
    dest *state.WalkPoint, search *state.WalkSearch,
    start time.Time, at time.Time,
    wpAt []time.Time,
) {
    target := index
    if target < 0 || target >= len(path) {
        target = len(path) - 1
    }
    if search != nil {
        filterWord := "swim"
        if search.Dry {
            filterWord = "dry"
        }
        fmt.Fprintf(b, "%s%d waypoints, %s, aiming at wp %d):\n",
            headerPrefix, len(path), filterWord, target)
        // The full search contract rides its own line: the filter
        // word alone rebuilds a lookalike, the approach radius and
        // the ban circles rebuild the very search (the paste a dump
        // into the HUD flow restores the plan through the parser).
        fmt.Fprintf(b, "  search approach %.0f", search.Approach)
        if len(search.Avoid) > 0 {
            circles := make([]string, len(search.Avoid))
            for i := range search.Avoid {
                circles[i] = fmt.Sprintf("%.0f %.0f %.0f",
                    search.Avoid[i].X,
                    search.Avoid[i].Y,
                    search.Avoid[i].R)
            }
            fmt.Fprintf(b, " avoid %s", strings.Join(circles, ","))
        }
        fmt.Fprint(b, "\n")
    } else {
        fmt.Fprintf(b, "%s%d waypoints, aiming at wp %d):\n",
            headerPrefix, len(path), target)
    }
    if origin != nil {
        fmt.Fprintf(b, "  from %d %d %d\n",
            origin.X, origin.Y, origin.Z)
    }
    if !start.IsZero() {
        fmt.Fprintf(b, "  started %s", start.Format("15:04:05"))
        if !at.IsZero() && at.After(start) {
            fmt.Fprintf(b, ", last seen %s, %s on the walk",
                at.Format("15:04:05"), walkDur(at.Sub(start)))
        }
        fmt.Fprint(b, "\n")
    }
    for i := range path {
        wp := &path[i]
        switch {
        case i == target:
            fmt.Fprintf(b, "  wp %d: %d %d %d  <-- TARGET%s\n",
                i, wp.X, wp.Y, wp.Z, walkTargetSuffix(start, at, wpAt, i))
        case i < target:
            fmt.Fprintf(b, "  wp %d: %d %d %d (passed%s)\n",
                i, wp.X, wp.Y, wp.Z, walkPassedSuffix(start, wpAt, i))
        default:
            fmt.Fprintf(b, "  wp %d: %d %d %d\n", i, wp.X, wp.Y, wp.Z)
        }
    }
    if dest != nil {
        fmt.Fprintf(b, "  dest %d %d %d\n",
            dest.X, dest.Y, dest.Z)
    }
    fmt.Fprintln(b)
}

// walkPassedSuffix renders the timing suffix of a passed waypoint
// line: ", t+12.4s, leg 5.2s" - the moment the follower reached the
// waypoint on the walk timeline and the duration of the leg that
// ended there. An empty string keeps the plain line when the walk
// carries no timing view (an arrival was never observed - the zero
// entries of the record, or a plan older than the timing tracking).
func walkPassedSuffix(start time.Time, wpAt []time.Time, i int) string {
    if start.IsZero() || i >= len(wpAt) || wpAt[i].IsZero() {
        return ""
    }
    legStart := start
    if i > 0 && !wpAt[i-1].IsZero() {
        legStart = wpAt[i-1]
    }

    return fmt.Sprintf(", t+%s, leg %s",
        walkDur(wpAt[i].Sub(start)), walkDur(wpAt[i].Sub(legStart)))
}

// walkTargetSuffix renders the timing suffix of the aimed waypoint
// line: " (walking 45.2s)" - the time the walk already spends on
// the leg that has not confirmed its arrival yet, the stuck number
// of a dawdling point. The leg opened at the previous waypoint's
// arrival (or the walk start); the live plan measures up to now,
// the ended one up to the moment the plan was last seen alive.
func walkTargetSuffix(
    start time.Time, at time.Time, wpAt []time.Time, i int,
) string {
    if start.IsZero() {
        return ""
    }
    legStart := start
    if i > 0 && i-1 < len(wpAt) && !wpAt[i-1].IsZero() {
        legStart = wpAt[i-1]
    }
    end := time.Now()
    if !at.IsZero() && at.Before(end) {
        end = at
    }
    if !end.After(legStart) {
        return ""
    }

    return fmt.Sprintf(" (walking %s)", walkDur(end.Sub(legStart)))
}

// walkDur renders one walk timing: sub minute durations keep the
// tenth of a second (the per waypoint cost lives there), the longer
// ones round to the second (the minute shape reads faster).
func walkDur(d time.Duration) string {
    if d < time.Minute {
        return fmt.Sprintf("%.1fs", d.Seconds())
    }

    return d.Round(time.Second).String()
}

// writeDumpCombat writes the recent combat beats.
func writeDumpCombat(b *strings.Builder, snap state.Snapshot) {
    fmt.Fprintf(b, "recent combat (%d):\n", len(snap.CombatEvents))
    for i := range snap.CombatEvents {
        e := &snap.CombatEvents[i]
        fmt.Fprintf(b, "  %s: attacker %d -> target %d, %.0f damage\n",
            e.Kind, e.AttackerID, e.TargetID, e.Amount)
    }
    fmt.Fprintln(b)
}

// writeDumpChat writes the recent chat lines.
func writeDumpChat(b *strings.Builder, snap state.Snapshot) {
    start := 0
    if len(snap.Chat) > dumpChatLimit {
        start = len(snap.Chat) - dumpChatLimit
    }
    chat := snap.Chat[start:]
    fmt.Fprintf(b, "chat (%d):\n", len(chat))
    for i := range chat {
        line := &chat[i]
        fmt.Fprintf(b, "  %s %s: %s\n",
            line.Time.Format("15:04:05"), line.Kind, line.Text)
    }
    fmt.Fprintln(b)
}

// writeDumpEvents writes the deep event window - the game events, the
// web commands and the mirrored hunt loop decisions in chronological
// order.
func writeDumpEvents(b *strings.Builder, events []state.Event) {
    fmt.Fprintf(b, "events (%d):\n", len(events))
    for i := range events {
        e := &events[i]
        fmt.Fprintf(b, "  %s %s\n",
            e.Time.Format("15:04:05"), e.Message)
    }
}
