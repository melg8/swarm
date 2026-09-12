// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"fmt"
	"strconv"
	"strings"
)

// The bypass command kinds the gatekeeper flow drives. The server
// NpcHtmlMessage carries <a action="bypass ..."> or
// <a action="bypass -h ..."> buttons; the client strips the prefix and
// sends the rest as a RequestBypassToServer command (see
// docs/protocol_description.md and the H-002 verification).
const (
	bypassKindShowTeleports = "showTeleports"
	bypassKindTeleport      = "teleport"
	bypassKindChat          = "chat"
)

// BypassButton is one parsed html bypass of a gatekeeper dialog: the
// command the client sends, the display label and, for the teleport
// buttons, the destination coordinates the list carried.
type BypassButton struct {
	Command string
	Label   string
	Kind    string
	// NpcObjID is the npc object id of the teleporter the button
	// addresses (the npc_<objId> prefix of the command).
	NpcObjID int32
	// ListName is the teleport list name ("NORMAL" for the default
	// list); empty for the non-teleport buttons.
	ListName string
	// LocID is the destination index inside the teleport list; -1 for
	// the non-teleport buttons.
	LocID int
}

// ParseGatekeeperHTML extracts the bypass buttons from a gatekeeper
// NpcHTMLMessage body. The html is the raw string the server sent
// (the teleports.htm template with the %locations% replaced, or the
// initial npc dialog). The parser finds every
// <a action="bypass [(-h )]<command>"> tag, strips the bypass prefix
// and classifies the command:
//
//   - "npc_<objId>_showTeleports [<list>]" opens the teleport list.
//   - "npc_<objId>_teleport <list> <locId>" teleports to the
//     destination.
//   - "npc_<objId>_chat <val>" opens the next dialog page.
//
// The %objectId% placeholder the html template carries is already
// replaced by the server with the real npc object id before the
// NpcHTMLMessage is sent, so the bot never sees it.
func ParseGatekeeperHTML(html string) []BypassButton {
	var buttons []BypassButton
	cursor := 0
	for {
		open := strings.Index(html[cursor:], "<a ")
		if open < 0 {
			break
		}
		start := cursor + open
		closeTag := strings.Index(html[start:], ">")
		if closeTag < 0 {
			break
		}
		tag := html[start : start+closeTag]
		labelStart := start + closeTag + 1
		labelEnd := strings.Index(html[labelStart:], "</a>")
		if labelEnd < 0 {
			break
		}
		label := strings.TrimSpace(
			html[labelStart : labelStart+labelEnd])
		cursor = labelStart + labelEnd + len("</a>")
		command, ok := extractBypassCommand(tag)
		if !ok {
			continue
		}
		button := classifyBypass(command, label)
		buttons = append(buttons, button)
	}

	return buttons
}

// extractBypassCommand pulls the bypass command out of an
// <a action="bypass [(-h )<command>"> tag. The client strips the
// "bypass -h " or "bypass " prefix before sending, so the returned
// command is the raw string the bot sends (for example
// "npc_30146_teleport NORMAL 0"). Returns ok=false when the tag carries
// no bypass action.
func extractBypassCommand(tag string) (string, bool) {
	action := extractAttribute(tag, "action")
	if action == "" {
		return "", false
	}
	if strings.HasPrefix(action, "bypass -h ") {
		return strings.TrimSpace(action[len("bypass -h "):]), true
	}
	if strings.HasPrefix(action, "bypass ") {
		return strings.TrimSpace(action[len("bypass "):]), true
	}

	return "", false
}

// extractAttribute returns the value of the given attribute of an html
// tag, or an empty string when the attribute is absent. The lookup is
// case-insensitive (the server html mixes "action" and the parser must
// not miss it on a casing drift).
func extractAttribute(tag, name string) string {
	lower := strings.ToLower(tag)
	key := strings.ToLower(name) + "=\""
	start := strings.Index(lower, key)
	if start < 0 {
		return ""
	}
	valueStart := start + len(key)
	end := strings.Index(tag[valueStart:], "\"")
	if end < 0 {
		return ""
	}

	return tag[valueStart : valueStart+end]
}

// classifyBypass turns a raw bypass command into a BypassButton. The
// npc_<objId>_<action> shape routes to the teleporter; the action word
// classifies the kind. A teleport command carries the list name and the
// destination index; a showTeleports command carries the list name
// (defaulting to "NORMAL" when the button omits it).
func classifyBypass(command, label string) BypassButton {
	button := BypassButton{
		Command:  command,
		Label:    label,
		Kind:     "",
		NpcObjID: 0,
		ListName: "",
		LocID:    -1,
	}
	if !strings.HasPrefix(command, "npc_") {
		return button
	}
	rest := command[len("npc_"):]
	underscore := strings.Index(rest, "_")
	if underscore <= 0 {
		return button
	}
	objID, err := strconv.Atoi(rest[:underscore])
	if err != nil || objID < 0 || objID > maxNpcObjID {
		return button
	}
	//nolint:gosec // G109: objID is bounds-checked to maxNpcObjID above.
	button.NpcObjID = int32(objID)
	action := rest[underscore+1:]
	fields := strings.Fields(action)
	if len(fields) == 0 {
		return button
	}
	switch fields[0] {
	case bypassKindShowTeleports:
		button.Kind = bypassKindShowTeleports
		if len(fields) >= 2 {
			button.ListName = fields[1]
		} else {
			button.ListName = defaultTeleportList
		}
	case bypassKindTeleport:
		button.Kind = bypassKindTeleport
		if len(fields) >= 2 {
			button.ListName = fields[1]
		}
		if len(fields) >= 3 {
			if locID, err := strconv.Atoi(fields[2]); err == nil {
				button.LocID = locID
			}
		}
	case bypassKindChat:
		button.Kind = bypassKindChat
	default:
		button.Kind = fields[0]
	}

	return button
}

// defaultTeleportList is the list name the server uses when the
// showTeleports button omits it (TeleportType.NORMAL, see
// Teleporter.onBypassFeedback).
const defaultTeleportList = "NORMAL"

// maxNpcObjID bounds the npc object id parse: the server ids are
// positive int32 values, so anything above this is a parse error or
// an overflow attempt (the gosec G109 guard).
const maxNpcObjID = 2_000_000_000

// FindTeleportButton returns the teleport button of the given list
// whose label or LocID matches the wanted destination, or nil when no
// button matches. The hunt loop calls this after ParseGatekeeperHTML to
// pick the destination of the next leg.
func FindTeleportButton(
	buttons []BypassButton, listName string, wantLabel string,
) *BypassButton {
	for i := range buttons {
		button := &buttons[i]
		if button.Kind != bypassKindTeleport {
			continue
		}
		if button.ListName != listName {
			continue
		}
		if wantLabel == "" ||
			strings.Contains(button.Label, wantLabel) {
			return button
		}
	}

	return nil
}

// FindShowTeleportsButton returns the first showTeleports button of the
// parsed html (the gatekeeper's initial dialog carries one). Returns
// nil when the html has no showTeleports button.
func FindShowTeleportsButton(
	buttons []BypassButton,
) *BypassButton {
	for i := range buttons {
		if buttons[i].Kind == bypassKindShowTeleports {
			return &buttons[i]
		}
	}

	return nil
}

// String renders the button for the log (the hunt loop logs the chosen
// destination).
func (b BypassButton) String() string {
	if b.Kind == bypassKindTeleport {
		return fmt.Sprintf("%s (npc %d, list %s, loc %d)",
			b.Label, b.NpcObjID, b.ListName, b.LocID)
	}

	return fmt.Sprintf("%s (command %q)", b.Label, b.Command)
}
