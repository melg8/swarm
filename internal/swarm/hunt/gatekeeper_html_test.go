// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseGatekeeperHTMLInitialDialog pins the first dialog page: the
// gatekeeper html carries one showTeleports button whose command the
// client sends as "npc_<objId>_showTeleports" (the "bypass " prefix
// is stripped client-side).
func TestParseGatekeeperHTMLInitialDialog(t *testing.T) {
	html := "<html><body>Gatekeeper Mirabel:<br>" +
		"<a action=\"bypass npc_30146_showTeleports\">Teleport</a><br>" +
		"<a action=\"bypass Script\">Quest</a>" +
		"</body></html>"
	buttons := ParseGatekeeperHTML(html)
	require.Len(t, buttons, 2)
	show := buttons[0]
	require.Equal(t, "npc_30146_showTeleports", show.Command)
	require.Equal(t, "Teleport", show.Label)
	require.Equal(t, bypassKindShowTeleports, show.Kind)
	require.Equal(t, int32(30146), show.NpcObjID)
	require.Equal(t, "NORMAL", show.ListName,
		"the omitted list name defaults to NORMAL")
	require.Equal(t, -1, show.LocID)
	// The non-bypass-npc "Quest" button is not a teleport action; it
	// still parses (the parser records every bypass command).
	quest := buttons[1]
	require.Equal(t, "Script", quest.Command)
}

// TestParseGatekeeperHTMLTeleportList pins the teleport list page: the
// %locations% replacement carries one button per destination, each
// with the "bypass -h " prefix and the "npc_<id>_teleport <list> <loc>"
// command shape.
func TestParseGatekeeperHTMLTeleportList(t *testing.T) {
	html := "<html><body>Region where teleporting is possible<br><br>" +
		"<a action=\"bypass -h npc_30146_teleport NORMAL 0\" msg=\"811;The Town of Gludio\">The Town of Gludio - 9200 Adena</a><br1>" +
		"<a action=\"bypass -h npc_30146_teleport NORMAL 1\" msg=\"811;Dwarven Village\">Dwarven Village - 23000 Adena</a><br1>" +
		"<br></body></html>"
	buttons := ParseGatekeeperHTML(html)
	require.Len(t, buttons, 2)
	first := buttons[0]
	require.Equal(t, "npc_30146_teleport NORMAL 0", first.Command)
	require.Equal(t, "The Town of Gludio - 9200 Adena", first.Label)
	require.Equal(t, bypassKindTeleport, first.Kind)
	require.Equal(t, int32(30146), first.NpcObjID)
	require.Equal(t, "NORMAL", first.ListName)
	require.Equal(t, 0, first.LocID)
	second := buttons[1]
	require.Equal(t, 1, second.LocID)
}

// TestParseGatekeeperHTMLShowTeleportsWithList pins the explicit list
// name: a showTeleports button may carry the list name
// ("npc_<id>_showTeleports NOBLES_TOKEN").
func TestParseGatekeeperHTMLShowTeleportsWithList(t *testing.T) {
	html := "<a action=\"bypass npc_30146_showTeleports NOBLES_TOKEN\">Noblesse</a>"
	buttons := ParseGatekeeperHTML(html)
	require.Len(t, buttons, 1)
	require.Equal(t, bypassKindShowTeleports, buttons[0].Kind)
	require.Equal(t, "NOBLES_TOKEN", buttons[0].ListName)
}

// TestFindTeleportButtonByLabel pins the destination lookup: the hunt
// loop finds the button whose label contains the wanted destination
// name (the "The Town of Gludio" leg of Mirabel -> Gludio).
func TestFindTeleportButtonByLabel(t *testing.T) {
	buttons := []BypassButton{
		{
			Kind: bypassKindTeleport, ListName: "NORMAL", LocID: 0,
			Label: "The Town of Gludio - 9200 Adena",
		},
		{
			Kind: bypassKindTeleport, ListName: "NORMAL", LocID: 1,
			Label: "Dwarven Village - 23000 Adena",
		},
	}
	gludio := FindTeleportButton(buttons, "NORMAL", "The Town of Gludio")
	require.NotNil(t, gludio)
	require.Equal(t, 0, gludio.LocID)
	missing := FindTeleportButton(buttons, "NORMAL", "Giran")
	require.Nil(t, missing)
}

// TestFindShowTeleportsButton pins the initial-dialog lookup: the first
// showTeleports button is the entry point of the gatekeeper flow.
func TestFindShowTeleportsButton(t *testing.T) {
	buttons := []BypassButton{
		{Kind: "Script", Label: "Quest"},
		{
			Kind: bypassKindShowTeleports, Label: "Teleport",
			NpcObjID: 30146, ListName: "NORMAL",
		},
	}
	show := FindShowTeleportsButton(buttons)
	require.NotNil(t, show)
	require.Equal(t, int32(30146), show.NpcObjID)
}

// TestParseGatekeeperHTMLNoButtons pins the empty case: an html with no
// bypass buttons returns an empty slice (the gatekeeper dialog never
// arrived, or the html was a non-teleport page).
func TestParseGatekeeperHTMLNoButtons(t *testing.T) {
	buttons := ParseGatekeeperHTML("<html><body>Nothing here</body></html>")
	require.Empty(t, buttons)
}

// TestParseGatekeeperHTMLMalformedTags pins the robustness: a tag
// without a close, a missing action attribute or a non-bypass action
// are skipped without panicking.
func TestParseGatekeeperHTMLMalformedTags(t *testing.T) {
	html := "<a href=\"other\">Link</a>" +
		"<a action=\"bypass -h npc_30146_teleport NORMAL 0\">Gludio</a>" +
		"<a action=\"bypass"
	buttons := ParseGatekeeperHTML(html)
	require.Len(t, buttons, 1, "only the well-formed bypass parses")
	require.Equal(t, bypassKindTeleport, buttons[0].Kind)
}
