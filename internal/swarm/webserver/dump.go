// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

// The state dump endpoint: a plain text report of everything the tracker knows
// about one bot (the character sheet, the inventory and equipment, the
// surrounding objects with their combat state, the walk plan, the zone, the
// chat and a deep event window). The Dump state button of the HUD copies it
// into the clipboard, so a live problem report arrives with the debugging
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

// dumpEventLimit bounds the dump event window: six times the newest 100 the
// snapshot streams for the UI log tail, because the story of a stuck session
// lives minutes back.
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

// BuildStateDump assembles the plain text debug report in reading order: the
// identity and the phase, the character, its surroundings, the inventory, the
// movement plan and the long event log last.
func BuildStateDump(bot *state.Bot) string {
    snap := bot.Snapshot()
    events := bot.NewestEvents(dumpEventLimit)
    b := &strings.Builder{}

    writeDumpHeader(b, snap)
    writeDumpCharacter(b, snap)
    writeDumpBuffs(b, snap)
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

// writeDumpHeader writes the title, the build identity and the session summary;
// the build line pins the exact code state the report came from.
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

// writeDumpBuffs lists the active effects with their remaining time, so a
// report answers what was up and for how long without a browser. The applied
// age derives from the full duration (Total - Left; the recast detection of
// SetBuffs keeps it honest).
func writeDumpBuffs(b *strings.Builder, snap state.Snapshot) {
    fmt.Fprintf(b, "buffs (%d):\n", len(snap.Buffs))
    for i := range snap.Buffs {
        buff := &snap.Buffs[i]
        fmt.Fprintf(b,
            "  %s (%d) lvl %d: %d/%ds left, applied ~%s ago\n",
            buff.Name, buff.SkillID, buff.Level, buff.Left,
            buff.Total, (time.Duration(buff.Total-buff.Left) *
                time.Second).Round(time.Minute))
    }
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

// dumpSlotNames maps the item body part mask to the paperdoll slot name (masks
// outside the table fall back to the hex form), mirroring the Mobius BodyPart
// enum: 0x4000 the two hand weapon, 0x8000 the one-piece armor, and the pair
// families carry the OR of their two slots (0x6 the earrings, 0x30 the rings).
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
    // The report prints the occupied slots only, so the empty families name
    // themselves right below the equipment: a missing legs line was invisible.
    if empty := dumpEmptySlots(snap.Inventory); len(empty) > 0 {
        fmt.Fprintf(b, "empty slots: %s\n", strings.Join(empty, ", "))
    }
    fmt.Fprintf(b, "bag (%d):\n", len(bag))
    for _, line := range bag {
        fmt.Fprintln(b, line)
    }
    fmt.Fprintln(b)
}

// dumpEmptySlots lists the paperdoll families no equipped item covers, the
// unfilled pair halves included (one earring or ring equipped names the other
// half empty). The two hand weapon blocks the left hand and the one-piece armor
// the legs, so those stay unlisted while the blocker is worn; underwear and
// hair never fill and stay out.
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

// writeDumpObjects writes the known objects closest first, with the combat
// state and the targets (a mob holding the character is the aggro).
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

// writeDumpWalkPlan writes the active walk plan: the whole segment from the
// planning origin to the final destination, every passed waypoint with its
// arrival moment and segment duration, and the aimed waypoint with its walking
// time, so a stuck or drifting walk reads at a glance. A gone plan (arrived,
// timed out, phase left) prints its last snapshot instead: a stuck segment
// needs the whole planned walk.
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

// writeWalkPlanSection prints one plan under the header prefix: the waypoint
// count with the mesh contract word (direct segments name nothing), the
// follower cursor, the origin, the walk zero point the timing suffixes read
// against, the waypoints and the destination. A passed waypoint carries its
// moment on the walk timeline (t+) and its segment duration; the aimed one
// carries the time spent on it, the stuck number. The last plan reads the same
// suffixes against the moment it ended.
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
        fmt.Fprintf(b, "%s%d waypoints, mesh, aiming at wp %d):\n",
            headerPrefix, len(path), target)
        // The full search contract rides its own line: the mesh word alone
        // rebuilds a lookalike, the approach radius and the bans rebuild the
        // very search.
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

// walkPassedSuffix renders ", t+12.4s, segment 5.2s" for a passed waypoint: the
// arrival moment on the walk timeline and the segment duration that ended
// there; an unobserved arrival keeps the plain line.
func walkPassedSuffix(start time.Time, wpAt []time.Time, i int) string {
    if start.IsZero() || i >= len(wpAt) || wpAt[i].IsZero() {
        return ""
    }
    segmentStart := start
    if i > 0 && !wpAt[i-1].IsZero() {
        segmentStart = wpAt[i-1]
    }

    return fmt.Sprintf(", t+%s, segment %s",
        walkDur(wpAt[i].Sub(start)), walkDur(wpAt[i].Sub(segmentStart)))
}

// walkTargetSuffix renders " (walking 45.2s)" for the aimed waypoint: the time
// spent on the segment since the previous arrival (or the start), measured up
// to now or to the moment the ended plan was last seen alive.
func walkTargetSuffix(
    start time.Time, at time.Time, wpAt []time.Time, i int,
) string {
    if start.IsZero() {
        return ""
    }
    segmentStart := start
    if i > 0 && i-1 < len(wpAt) && !wpAt[i-1].IsZero() {
        segmentStart = wpAt[i-1]
    }
    end := time.Now()
    if !at.IsZero() && at.Before(end) {
        end = at
    }
    if !end.After(segmentStart) {
        return ""
    }

    return fmt.Sprintf(" (walking %s)", walkDur(end.Sub(segmentStart)))
}

// walkDur keeps the tenth of a second under a minute and rounds the longer
// ones.
func walkDur(d time.Duration) string {
    if d < time.Minute {
        return fmt.Sprintf("%.1fs", d.Seconds())
    }

    return d.Round(time.Second).String()
}

func writeDumpCombat(b *strings.Builder, snap state.Snapshot) {
    fmt.Fprintf(b, "recent combat (%d):\n", len(snap.CombatEvents))
    for i := range snap.CombatEvents {
        e := &snap.CombatEvents[i]
        fmt.Fprintf(b, "  %s: attacker %d -> target %d, %.0f damage\n",
            e.Kind, e.AttackerID, e.TargetID, e.Amount)
    }
    fmt.Fprintln(b)
}

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

func writeDumpEvents(b *strings.Builder, events []state.Event) {
    fmt.Fprintf(b, "events (%d):\n", len(events))
    for i := range events {
        e := &events[i]
        fmt.Fprintf(b, "  %s %s\n",
            e.Time.Format("15:04:05"), e.Message)
    }
}
