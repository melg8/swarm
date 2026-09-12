// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// soriusDialogPage returns the parsed form of the first quest page
// of Q00406 (the links of the real page, the Sorius npc origin).
func soriusDialogPage() DialogPageView {
	return DialogPageView{
		NpcObjID: 818056,
		ItemID:   0,
		Links: []DialogLinkView{
			{
				Command: "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
				Text:    "Say you want to be an Elven Knight",
			},
		},
	}
}

// rainsDialogPage returns the parsed form of the class master page
// (the -h stripped class change command, the npc origin of Rains).
func rainsDialogPage() DialogPageView {
	return DialogPageView{
		NpcObjID: 818088,
		ItemID:   0,
		Links: []DialogLinkView{
			{
				Command: "Script ElfHumanFighterChange1 30288-13.htm",
				Text:    "Description of an Elven Knight",
			},
			{
				Command: "Script ElfHumanFighterChange1 19",
				Text:    "Change profession to an Elven Knight",
			},
		},
	}
}

// TestApplyDialogPinsThePage pins the apply path: the page lands
// with its links and the origin npc.
func TestApplyDialogPinsThePage(t *testing.T) {
	bot := NewBot("test1")
	require.Nil(t, bot.DialogLinks())
	require.Equal(t, int32(0), bot.DialogOrigin())

	bot.ApplyDialog(soriusDialogPage())
	links := bot.DialogLinks()
	require.Len(t, links, 1)
	require.Equal(t, "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		links[0].Command)
	require.Equal(t, "Say you want to be an Elven Knight", links[0].Text)
	require.Equal(t, int32(818056), bot.DialogOrigin())
}

// TestApplyDialogReplacesThePage pins the NPC_HTML scope
// semantics: a later page replaces the earlier one - the links of
// the old page stop validating.
func TestApplyDialogReplacesThePage(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyDialog(soriusDialogPage())
	require.True(t, bot.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm"))

	bot.ApplyDialog(rainsDialogPage())
	links := bot.DialogLinks()
	require.Len(t, links, 2)
	require.Equal(t, int32(818088), bot.DialogOrigin())
	require.False(t, bot.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm"),
		"the old page is gone")
	require.True(t, bot.IsDialogCommand(
		"Script ElfHumanFighterChange1 19"))
}

// TestIsDialogCommandParameterPrefix pins the variable parameter
// rule of the server validation: a link ending with the '$' marker
// validates every command carrying its prefix.
func TestIsDialogCommandParameterPrefix(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyDialog(DialogPageView{
		NpcObjID: 818056,
		Links: []DialogLinkView{
			{Command: "npc_818056_BuyList 3006000 $", Text: "buy"},
		},
	})

	require.True(t, bot.IsDialogCommand("npc_818056_BuyList 3006000 5"))
	require.True(t, bot.IsDialogCommand(
		"npc_818056_BuyList 3006000 1000"))
	require.False(t, bot.IsDialogCommand("npc_818056_BuyList 3006001 5"),
		"the prefix must match")
	require.False(t, bot.IsDialogCommand("npc_818056_Sell"))
}

// TestDialogLinksCopyNotAlias pins the defensive copy: mutating the
// returned slice (or the applied page after the call) does not
// rewrite the stored dialog.
func TestDialogLinksCopyNotAlias(t *testing.T) {
	bot := NewBot("test1")
	page := soriusDialogPage()
	bot.ApplyDialog(page)
	page.Links[0].Command = "mutated after apply"

	links := bot.DialogLinks()
	require.Equal(t, "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		links[0].Command)
	links[0].Command = "mutated by the caller"
	require.Equal(t, "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		bot.DialogLinks()[0].Command)
}

// TestResetSessionClearsTheDialog pins the session reset: the
// relogin starts with no dialog open.
func TestResetSessionClearsTheDialog(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyDialog(soriusDialogPage())

	bot.ResetSession()
	require.Nil(t, bot.DialogLinks())
	require.Equal(t, int32(0), bot.DialogOrigin())
	require.False(t, bot.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm"))
}

// TestClearDialogDropsThePage pins the explicit clear.
func TestClearDialogDropsThePage(t *testing.T) {
	bot := NewBot("test1")
	bot.ApplyDialog(soriusDialogPage())

	bot.ClearDialog()
	require.Nil(t, bot.DialogLinks())
}
