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
	fmt.Fprintf(b, "packets: %d, state version %d\n\n",
		snap.Packets, snap.Version)
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
// name; masks outside the table fall back to the hex form.
var dumpSlotNames = map[int32]string{
	0x01:    "underwear",
	0x04:    "rear ear",
	0x08:    "lear ear",
	0x10:    "necklace",
	0x20:    "rfinger",
	0x40:    "lfinger",
	0x80:    "rhand",
	0x100:   "lhand",
	0x200:   "gloves",
	0x400:   "chest",
	0x4000:  "legs",
	0x8000:  "feet",
	0x40000: "lrhand",
	0x80000: "hair",
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
	fmt.Fprintf(b, "bag (%d):\n", len(bag))
	for _, line := range bag {
		fmt.Fprintln(b, line)
	}
	fmt.Fprintln(b)
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

// writeDumpWalkPlan writes the active walk plan of the hunt loop.
func writeDumpWalkPlan(b *strings.Builder, snap state.Snapshot) {
	fmt.Fprintf(b, "walk plan (%d waypoints):\n", len(snap.WalkPath))
	for i := range snap.WalkPath {
		wp := &snap.WalkPath[i]
		fmt.Fprintf(b, "  wp %d: %d %d %d\n", i, wp.X, wp.Y, wp.Z)
	}
	fmt.Fprintln(b)
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
