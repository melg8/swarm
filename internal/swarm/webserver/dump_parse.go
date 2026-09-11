// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The dump parser: the inverse of BuildStateDump (dump.go). It reads
// the plain text dump back into the state.Snapshot view structs so a
// dump captured from a long-lived user server (tools/dump_state.sh)
// reconstructs the exact world a misbehaving bot saw. See the
// dump-state-repro skill for the reproduction workflow.
//
// The parser is line-oriented and tolerant: it skips blank lines and
// section headers, reads the indented key:value lines it knows, and
// leaves unknown fields at their zero value. The goal is enough
// state to reproduce a behavior (the character position, the world
// objects around it, the target, the walk plan), not a byte-exact
// round trip (the dump carries presentation fields the view structs
// do not store).
package webserver

import (
	"strconv"
	"strings"

	"github.com/melg8/swarm/internal/swarm/state"
)

// ParseDump reads the plain text dump of BuildStateDump into the
// state.Snapshot view structs. The returned snapshot carries the
// identity, the character, the attackers, the hunting zone, the
// equipment, the bag, the world objects, the walk plan and the recent
// combat/chat/events that the dump printed - the minimal repro set of
// the dump-state-repro skill.
//
// The parser is tolerant: unknown lines and missing sections are
// skipped, the fields the dump does not carry stay at their zero
// value. A parse error of a known field returns the error so the
// caller can decide whether to trust the partial snapshot.
func ParseDump(dump string) (state.Snapshot, error) {
	p := &dumpParser{ //nolint:exhaustruct_v5 // pos/snap start zero
		lines: strings.Split(dump, "\n"),
	}

	return p.parse()
}

// ApplyDump replays a parsed snapshot into a fresh state.Bot through
// the public Apply API, so the storage invariants (the SoA split of
// the world store, the object id index map, the version counter) hold
// exactly like a live session built them. Use this when a dump-driven
// repro test needs the real storage layout (the hunt tick scans the
// dense hot array, the target search walks the index map); use
// ParseDump alone when the test only asserts on the view fields.
//
// The replay covers the character, the world objects, the hunting
// zone, the walk plan and the rolling events. The inventory, the
// combat events and the chat are view-only projections of the live
// state (they carry presentation fields the Apply API does not take),
// so they stay on the snapshot and do not round-trip through the bot.
// A test that needs them reads them off the returned snapshot.
func ApplyDump(bot *state.Bot, snap state.Snapshot) {
	applyDumpCharacter(bot, snap.Character)
	applyDumpObjects(bot, snap.Objects)
	if snap.HuntingZone != nil {
		bot.SetHuntingZone(snap.HuntingZone.CX, snap.HuntingZone.CY,
			snap.HuntingZone.Half)
	}
	if len(snap.WalkPath) > 0 {
		bot.SetWalkPlan(state.WalkPlan{
			Origin: snap.WalkOrigin,
			Points: snap.WalkPath,
			Index:  snap.WalkIndex,
			Dest:   snap.WalkDest,
		})
	}
	for _, ev := range snap.Events {
		bot.RecordEvent(ev.Message)
	}
}

// applyDumpCharacter replays the character view into the bot. The
// SetCharacter call seeds the identity and the position; the
// ApplyUserInfo call fills the level, the stats, the max HP/MP and
// the load; the ApplyStatusUpdate calls set the current HP/MP. The
// target id and the sit/stand state ride along through ApplyUserInfo
// (the dump does not carry them as separate fields, the hunt loop
// derives them from the packets).
func applyDumpCharacter(bot *state.Bot, c state.CharacterSnapshot) {
	bot.SetCharacter(c.Name, c.ObjectID, c.ClassID, c.X, c.Y, c.Z,
		c.CurHP, c.CurMP)
	//nolint:exhaustruct_v5 // the dump carries a subset of UserInfo
	bot.ApplyUserInfo(state.UserInfo{
		Name:        c.Name,
		Level:       c.Level,
		Race:        c.Race,
		ClassID:     c.ClassID,
		X:           c.X,
		Y:           c.Y,
		Z:           c.Z,
		STR:         c.STR,
		DEX:         c.DEX,
		CON:         c.CON,
		INT:         c.INT,
		WIT:         c.WIT,
		MEN:         c.MEN,
		Exp:         c.Exp,
		Sp:          c.Sp,
		MaxHP:       int32(c.MaxHP),
		CurHP:       int32(c.CurHP),
		MaxMP:       int32(c.MaxMP),
		CurMP:       int32(c.CurMP),
		CurrentLoad: c.CurrentLoad,
		MaxLoad:     c.MaxLoad,
		RunSpeed:    int32(c.Speed),
	})
}

// applyDumpObjects replays the world object views into the bot. Each
// object becomes an ApplyNpcInfo (for npcs) or ApplyItemInfo (for
// items) call, followed by an ApplyStatusUpdate that sets the current
// HP/MP (the ApplyNpcInfo call only seeds the max from the template).
// The dead flag drops the object to 0 HP; the attackable flag stays
// on the hot record.
func applyDumpObjects(bot *state.Bot, objects []state.ObjectSnapshot) {
	for i := range objects {
		o := &objects[i]
		switch o.Kind {
		case state.KindNPC:
			applyDumpNpc(bot, o)
		case state.KindItem:
			applyDumpItem(bot, o)
		default:
			// Players and unknown kinds: the dump carries no
			// Apply API path for them, skip.
		}
	}
}

// applyDumpNpc replays one npc object view into the bot.
//
//nolint:exhaustruct_v5 // the dump carries a subset of NpcInfo fields
func applyDumpNpc(bot *state.Bot, o *state.ObjectSnapshot) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   o.ObjectID,
		Name:       o.Name,
		Title:      o.Title,
		Attackable: o.Attackable,
		X:          o.X,
		Y:          o.Y,
		Z:          o.Z,
	})
	bot.ApplyStatusUpdate(o.ObjectID, []state.Attribute{
		{ID: state.AttrCurHP, Value: int32(o.CurHP)},
		{ID: state.AttrMaxHP, Value: int32(o.MaxHP)},
		{ID: state.AttrCurMP, Value: int32(o.CurMP)},
		{ID: state.AttrMaxMP, Value: int32(o.MaxMP)},
	})
}

// applyDumpItem replays one item object view into the bot.
func applyDumpItem(bot *state.Bot, o *state.ObjectSnapshot) {
	//nolint:exhaustruct_v5 // the dump carries a subset of ItemInfo
	bot.ApplyItemInfo(state.ItemInfo{
		ObjectID:   o.ObjectID,
		TemplateID: o.TemplateID,
		Count:      o.Count,
		X:          o.X,
		Y:          o.Y,
		Z:          o.Z,
	})
}

// dumpParser walks the dump line by line, tracking the current
// section and dispatching the indented key:value lines to the
// matching field reader. The section header switches the dispatch
// table; blank lines separate sections but do not end them (a
// section runs until the next header).
type dumpParser struct {
	lines []string
	pos   int
	snap  state.Snapshot
}

// parse walks the dump and returns the assembled snapshot.
func (p *dumpParser) parse() (state.Snapshot, error) {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		p.pos++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "swarm state dump") ||
			strings.HasPrefix(trimmed, "build:") ||
			strings.HasPrefix(trimmed, "dumped:") ||
			strings.HasPrefix(trimmed, "session:") {
			continue
		}
		if strings.HasPrefix(trimmed, "bot:") {
			p.parseBotLine(trimmed)

			continue
		}
		if strings.HasPrefix(trimmed, "packets:") {
			continue
		}
		// Section headers end with a colon and a count or none.
		if err := p.dispatchSection(trimmed); err != nil {
			return p.snap, err
		}
	}

	return p.snap, nil
}

// dispatchSection reads the section header and the indented body that
// follows, calling the matching section reader.
func (p *dumpParser) dispatchSection(header string) error {
	switch {
	case strings.HasPrefix(header, "character:"):
		return p.parseCharacter()
	case strings.HasPrefix(header, "attackers"):
		return p.parseAttackers()
	case strings.HasPrefix(header, "hunting zone:"):
		return p.parseHuntingZone(header)
	case strings.HasPrefix(header, "equipment"):
		return p.parseEquipment()
	case strings.HasPrefix(header, "bag"):
		return p.parseBag()
	case strings.HasPrefix(header, "objects"):
		return p.parseObjects()
	case strings.HasPrefix(header, "walk plan"):
		return p.parseWalkPlan(header)
	case strings.HasPrefix(header, "recent combat"):
		return p.parseCombat()
	case strings.HasPrefix(header, "chat"):
		return p.parseChat()
	case strings.HasPrefix(header, "events"):
		return p.parseEvents()
	}
	// Unknown section: skip its indented body.
	p.skipIndented()

	return nil
}

// parseBotLine reads "bot: <id> (status <s>, phase <p>)".
func (p *dumpParser) parseBotLine(line string) {
	// "bot: test1 (status online, phase engage)"
	rest := strings.TrimPrefix(line, "bot:")
	rest = strings.TrimSpace(rest)
	paren := strings.LastIndex(rest, " (status ")
	if paren < 0 {
		return
	}
	p.snap.ID = strings.TrimSpace(rest[:paren])
	tail := rest[paren+len(" (status "):]
	// tail: "online, phase engage)"
	tail = strings.TrimSuffix(tail, ")")
	parts := strings.SplitN(tail, ", phase ", 2)
	if len(parts) > 0 {
		p.snap.Status = state.Status(strings.TrimSpace(parts[0]))
	}
	if len(parts) > 1 {
		p.snap.Phase = strings.TrimSpace(parts[1])
	}
}

// parseCharacter reads the "character:" section.
func (p *dumpParser) parseCharacter() error {
	c := &p.snap.Character
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		field := strings.TrimSpace(line)
		if err := parseCharField(c, field); err != nil {
			return err
		}
	}

	return nil
}

// parseCharField reads one indented character field.
func parseCharField(c *state.CharacterSnapshot, field string) error {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	switch key {
	case "name":
		// "name: Test (object 123)"
		c.Name = firstToken(val)
		if objID := takeAfter(val, "(object "); objID != "" {
			c.ObjectID = parseInt32(takeBefore(objID, ")"))
		}
	case "class":
		// "class: 18, race: 1, level 1, exp 0 (0.00%), sp 0"
		scanClassLine(c, val)
	case "position":
		// "position: x 7811, y 40993, z -3452, heading 0"
		scanPositionLine(c, val)
	case "vitals":
		scanVitalsLine(c, val)
	case "moving":
		scanMovingLine(c, val)
	case "target":
		c.TargetID = parseInt32(val)
	case "stats":
		scanStatsLine(c, val)
	case "load":
		scanLoadLine(c, val)
	}

	return nil
}

// firstToken returns the first whitespace-delimited token of s.
func firstToken(s string) string {
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i]
		}
	}

	return s
}

// parseInt32 parses a decimal int32, returning 0 on error.
func parseInt32(s string) int32 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 32)

	return int32(n)
}

// parseFloat64 parses a float, returning 0 on error.
func parseFloat64(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)

	return f
}

// scanClassLine reads "class: <classId>, race: <race>, level <lvl>,
// exp <exp> (<pct>%), sp <sp>".
func scanClassLine(c *state.CharacterSnapshot, val string) {
	// "18, race: 1, level 1, exp 0 (0.00%), sp 0"
	c.ClassID = parseInt32(takeBefore(val, ","))
	rest := afterToken(val, ", race:")
	c.Race = parseInt32(takeBefore(rest, ","))
	rest = afterToken(rest, ", level")
	c.Level = parseInt32(takeBefore(rest, ","))
	rest = afterToken(rest, ", exp")
	c.Exp = parseInt32(takeBefore(rest, " ("))
	c.Sp = parseInt32(afterToken(rest, "sp"))
}

// scanPositionLine reads "x <x>, y <y>, z <z>, heading <h>".
func scanPositionLine(c *state.CharacterSnapshot, val string) {
	// val: "x 45000, y 50000, z -3500, heading 0"
	c.X = parseInt32(takeBefore(takeAfter(val, "x "), ","))
	c.Y = parseInt32(takeBefore(takeAfter(val, "y "), ","))
	c.Z = parseInt32(takeBefore(takeAfter(val, "z "), ","))
	c.Heading = parseInt32(takeAfter(val, "heading "))
}

// scanVitalsLine reads "HP <cur>/<max>, MP <cur>/<max>, sitting <bool>,
// in combat <bool>".
func scanVitalsLine(c *state.CharacterSnapshot, val string) {
	// val: "HP 80/113, MP 30/39, sitting false, in combat false"
	hp := takeAfter(val, "HP ")
	c.CurHP = parseFloat64(takeBefore(hp, "/"))
	maxHP := takeAfter(hp, "/")
	c.MaxHP = parseFloat64(takeBefore(maxHP, ","))
	mp := takeAfter(val, "MP ")
	c.CurMP = parseFloat64(takeBefore(mp, "/"))
	c.MaxMP = parseFloat64(takeBefore(takeAfter(mp, "/"), ","))
	c.Sitting = strings.Contains(val, "sitting true")
	c.InCombat = strings.Contains(val, "in combat true")
}

// scanMovingLine reads "yes -> x <dx>, y <dy>, z <dz>, speed <s>" or
// "no (speed <s>)".
func scanMovingLine(c *state.CharacterSnapshot, val string) {
	if strings.HasPrefix(val, "yes") {
		c.Moving = true
		c.DestX = parseInt32(takeBefore(takeAfter(val, "x "), ","))
		c.DestY = parseInt32(takeBefore(takeAfter(val, "y "), ","))
		c.DestZ = parseInt32(takeBefore(takeAfter(val, "z "), ","))
		c.Speed = parseFloat64(takeAfter(val, "speed "))

		return
	}
	c.Moving = false
	c.Speed = parseFloat64(takeBefore(takeAfter(val, "speed "), ")"))
}

// scanStatsLine reads "STR <v> DEX <v> CON <v> INT <v> WIT <v> MEN <v>".
func scanStatsLine(c *state.CharacterSnapshot, val string) {
	// val: "STR 40 DEX 30 CON 43 INT 21 WIT 20 MEN 25"
	c.STR = parseInt32(takeBefore(takeAfter(val, "STR "), " "))
	c.DEX = parseInt32(takeBefore(takeAfter(val, "DEX "), " "))
	c.CON = parseInt32(takeBefore(takeAfter(val, "CON "), " "))
	c.INT = parseInt32(takeBefore(takeAfter(val, "INT "), " "))
	c.WIT = parseInt32(takeBefore(takeAfter(val, "WIT "), " "))
	c.MEN = parseInt32(takeAfter(val, "MEN "))
}

// scanLoadLine reads "<cur>/<max>, slots <cur>/<max>, adena <v>".
func scanLoadLine(c *state.CharacterSnapshot, val string) {
	c.CurrentLoad = parseInt32(takeBefore(val, "/"))
	c.MaxLoad = parseInt32(takeAfter(val, "/"))
	c.InventorySlots = int(parseInt32(takeAfter(val, "slots ")))
	slots := takeAfter(val, "slots ")
	c.InventoryMax = int(parseInt32(takeAfter(slots, "/")))
	c.Adena = parseInt32(takeAfter(val, "adena "))
}

// parseAttackers reads the "attackers (N):" section. The attackers
// are a subset of the world objects (the living attackable npcs that
// hold the character as their target); they are already captured by
// parseObjects, so this section is a no-op for the snapshot.
func (p *dumpParser) parseAttackers() error {
	p.skipIndented()

	return nil
}

// parseHuntingZone reads the "hunting zone:" header line.
func (p *dumpParser) parseHuntingZone(header string) error {
	// "hunting zone: center 46500 50000, half 800 (square 1600x1600)"
	// or "hunting zone: none"
	if strings.Contains(header, "none") {
		return nil
	}
	z := &state.Zone{} //nolint:exhaustruct_v5 // CX/CY/Half set below
	rest := takeAfter(header, "center ")
	z.CX = parseInt32(takeBefore(rest, " "))
	rest = afterToken(rest, " ")
	z.CY = parseInt32(takeBefore(rest, ","))
	z.Half = parseInt32(takeBefore(takeAfter(header, "half "), " "))
	p.snap.HuntingZone = z
	p.skipIndented()

	return nil
}

// parseEquipment reads the "equipment (N):" section.
func (p *dumpParser) parseEquipment() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		item, ok := parseItemLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		item.Equipped = true
		p.snap.Inventory = append(p.snap.Inventory, item)
	}

	return nil
}

// parseBag reads the "bag (N):" section.
func (p *dumpParser) parseBag() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		item, ok := parseItemLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		p.snap.Inventory = append(p.snap.Inventory, item)
	}

	return nil
}

// parseItemLine reads "object <id>: <name> (item <itemId>) x<count>
// +<enchant> [<slot>]".
func parseItemLine(line string) (state.InventoryItemSnapshot, bool) {
	// "object 123: Wooden Helmet (item 42) x1 [head]"
	var item state.InventoryItemSnapshot
	if !strings.HasPrefix(line, "object ") {
		return item, false
	}
	rest := takeAfter(line, "object ")
	item.ObjectID = parseInt32(takeBefore(rest, ":"))
	rest = takeAfter(rest, ": ")
	// name is everything up to " (item "
	nameEnd := strings.Index(rest, " (item ")
	if nameEnd < 0 {
		return item, false
	}
	item.Name = strings.TrimSpace(rest[:nameEnd])
	rest = rest[nameEnd+len(" (item "):]
	item.ItemID = parseInt32(takeBefore(rest, ")"))
	rest = takeAfter(rest, ")")
	if strings.HasPrefix(rest, " x") {
		item.Count = parseInt32(takeAfter(rest, "x"))
	}
	if i := strings.Index(rest, " +"); i >= 0 {
		item.Enchant = int16(parseInt32(rest[i+2:]))
	}
	if i := strings.Index(rest, " ["); i >= 0 {
		slot := rest[i+2:]
		slot = strings.TrimSuffix(slot, "]")
		item.BodyPart = dumpSlotMask(slot)
	}

	return item, true
}

// dumpSlotMask is the inverse of dumpSlotName: the paperdoll slot
// name back to the body part mask.
func dumpSlotMask(slot string) int32 {
	for mask, name := range dumpSlotNames {
		if name == slot {
			return mask
		}
	}

	return 0
}

// parseObjects reads the "objects (N):" section.
func (p *dumpParser) parseObjects() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		obj, ok := parseObjectLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		p.snap.Objects = append(p.snap.Objects, obj)
	}

	return nil
}

// parseObjectLine reads "<id> <kind> <name> [title] (level <lvl>,
// hp <cur>/<max>) at <x> <y> <z>, dist <d>[, <flags>]".
func parseObjectLine(line string) (state.ObjectSnapshot, bool) {
	var obj state.ObjectSnapshot
	// "<id> <kind> <name ...> (level <lvl>, hp <cur>/<max>) at
	// <x> <y> <z>, dist <d>"
	idEnd := strings.Index(line, " ")
	if idEnd < 0 {
		return obj, false
	}
	obj.ObjectID = parseInt32(line[:idEnd])
	rest := line[idEnd+1:]
	kindEnd := strings.Index(rest, " ")
	if kindEnd < 0 {
		return obj, false
	}
	obj.Kind = state.ObjectKind(strings.TrimSpace(rest[:kindEnd]))
	rest = rest[kindEnd+1:]
	// name is everything up to " (level "
	nameEnd := strings.Index(rest, " (level ")
	if nameEnd < 0 {
		return obj, false
	}
	nameField := rest[:nameEnd]
	if i := strings.Index(nameField, " ["); i >= 0 {
		obj.Name = strings.TrimSpace(nameField[:i])
		obj.Title = strings.TrimSuffix(nameField[i+2:], "]")
	} else {
		obj.Name = strings.TrimSpace(nameField)
	}
	rest = rest[nameEnd+len(" (level "):]
	obj.Level = parseInt32(takeBefore(rest, ","))
	rest = takeAfter(rest, "hp ")
	obj.CurHP = parseFloat64(takeBefore(rest, "/"))
	obj.MaxHP = parseFloat64(takeBefore(takeAfter(rest, "/"), ")"))
	rest = takeAfter(rest, ") at ")
	obj.X = parseInt32(takeBefore(rest, " "))
	rest = afterToken(rest, " ")
	obj.Y = parseInt32(takeBefore(rest, " "))
	rest = afterToken(rest, " ")
	obj.Z = parseInt32(takeBefore(rest, ","))
	// flags after ", "
	if i := strings.Index(rest, ", "); i >= 0 {
		flags := rest[i+2:]
		obj.Attackable = strings.Contains(flags, "attackable")
		obj.Dead = strings.Contains(flags, "dead")
		obj.InCombat = strings.Contains(flags, "combat")
		obj.Moving = strings.Contains(flags, "moving")
		obj.Aggressive = strings.Contains(flags, "aggressive")
		if targetStr := takeAfter(flags, "target "); targetStr != "" {
			obj.TargetID = parseInt32(takeBefore(targetStr, ","))
		}
	}

	return obj, true
}

// parseWalkPlan reads the "walk plan (N waypoints, aiming at wp M):"
// section.
func (p *dumpParser) parseWalkPlan(header string) error {
	if strings.Contains(header, "none") {
		return nil
	}
	count := parseInt32(takeAfter(header, "waypoints"))
	aimingRest := takeAfter(header, "aiming at wp ")
	aiming := parseInt32(takeBefore(aimingRest, ")"))
	p.snap.WalkIndex = int(aiming)
	points := make([]state.WalkPoint, 0, count)
	var origin *state.WalkPoint
	var dest *state.WalkPoint
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			break
		}
		p.pos++
		field := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(field, "from "):
			coords := takeAfter(field, "from ")
			wp := parseWalkCoords(coords)
			origin = &wp
		case strings.HasPrefix(field, "dest "):
			coords := takeAfter(field, "dest ")
			wp := parseWalkCoords(coords)
			dest = &wp
		case strings.HasPrefix(field, "wp "):
			// "wp <i>: <x> <y> <z>" or "... (passed)" or
			// "... <-- TARGET"
			rest := takeAfter(field, "wp ")
			idxStr := takeBefore(rest, ":")
			_ = parseInt32(idxStr) // index implicit in slice order
			coords := takeAfter(rest, ": ")
			wp := parseWalkCoords(coords)
			points = append(points, wp)
		}
	}
	p.snap.WalkPath = points
	p.snap.WalkOrigin = origin
	p.snap.WalkDest = dest

	return nil
}

// parseWalkCoords reads "<x> <y> <z>" into a WalkPoint.
func parseWalkCoords(s string) state.WalkPoint {
	var wp state.WalkPoint
	wp.X = parseInt32(takeBefore(s, " "))
	s = afterToken(s, " ")
	wp.Y = parseInt32(takeBefore(s, " "))
	s = afterToken(s, " ")
	wp.Z = parseInt32(takeBefore(s, " "))

	return wp
}

// parseCombat reads the "recent combat (N):" section.
func (p *dumpParser) parseCombat() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		field := strings.TrimSpace(line)
		ev, ok := parseCombatLine(field)
		if !ok {
			continue
		}
		p.snap.CombatEvents = append(p.snap.CombatEvents, ev)
	}

	return nil
}

// parseCombatLine reads "<kind>: attacker <aid> -> target <tid>,
// <amount> damage".
func parseCombatLine(line string) (state.CombatEventView, bool) {
	var ev state.CombatEventView
	colon := strings.Index(line, ":")
	if colon < 0 {
		return ev, false
	}
	ev.Kind = strings.TrimSpace(line[:colon])
	rest := line[colon+1:]
	ev.AttackerID = parseInt32(takeAfter(rest, "attacker "))
	rest = takeAfter(rest, "-> target ")
	ev.TargetID = parseInt32(takeBefore(rest, ","))
	ev.Amount = parseFloat64(takeAfter(rest, ", "))

	return ev, true
}

// parseChat reads the "chat (N):" section.
func (p *dumpParser) parseChat() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		field := strings.TrimSpace(line)
		// "<time> <kind>: <text>"
		space := strings.Index(field, " ")
		if space < 0 {
			continue
		}
		rest := field[space+1:]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			continue
		}
		//nolint:exhaustruct_v5 // Time not in the dump format
		chat := state.ChatEvent{
			Kind: strings.TrimSpace(rest[:colon]),
			Text: strings.TrimSpace(rest[colon+1:]),
		}
		p.snap.Chat = append(p.snap.Chat, chat)
	}

	return nil
}

// parseEvents reads the "events (N):" section.
func (p *dumpParser) parseEvents() error {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if !strings.HasPrefix(line, "  ") {
			return nil
		}
		p.pos++
		field := strings.TrimSpace(line)
		// "<time> <message>"
		space := strings.Index(field, " ")
		if space < 0 {
			continue
		}
		//nolint:exhaustruct_v5 // Time not in the dump format
		ev := state.Event{
			Message: strings.TrimSpace(field[space+1:]),
		}
		p.snap.Events = append(p.snap.Events, ev)
	}

	return nil
}

// skipIndented advances past the indented body of a section.
func (p *dumpParser) skipIndented() {
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if line == "" || strings.HasPrefix(line, "  ") {
			p.pos++

			continue
		}

		return
	}
}

// takeBefore returns the substring of s up to the first occurrence of
// sep (or s if sep is absent).
func takeBefore(s, sep string) string {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i]
	}

	return s
}

// takeAfter returns the substring of s after the first occurrence of
// needle (or "" if absent).
func takeAfter(s, needle string) string {
	if i := strings.Index(s, needle); i >= 0 {
		return s[i+len(needle):]
	}

	return ""
}

// afterToken returns the substring after the first run of non-space
// characters following a separator. It is a helper for the scan
// helpers above that read "key: value rest" lines.
func afterToken(s, sep string) string {
	rest := takeAfter(s, sep)
	rest = strings.TrimLeft(rest, " ")

	return rest
}
