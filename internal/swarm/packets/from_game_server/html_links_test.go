// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// soriusQuestPage is the first quest page of Q00406 as the server
// sends it (data/scripts/quests/Q00406_PathOfTheElvenKnight/
// 30327-01.htm): one bypass link into the quest dialog.
const soriusQuestPage = "<html><body>Master Sorius:<br>\n" +
	"Greetings, child in search of the training of the sword.<br>\n" +
	"<a action=\"bypass Script Q00406_PathOfTheElvenKnight " +
	"30327-05.htm\">Say you want to be an Elven Knight</a>\n" +
	"</body></html>"

// rainsClassPage is the elven knight page of the class master Rains
// (village_master/ElfHumanFighterChange1/30288-12.htm): the -h
// prefixed class change command plus the plain description and
// return links.
const rainsClassPage = "<html><body>Grand Master Rains:<br>\n" +
	"The Elven Knight chooses the way of the sword.<br>\n" +
	"<a action=\"bypass Script ElfHumanFighterChange1 30288-13.htm\">" +
	"Description of an Elven Knight</a><br>\n" +
	"<a action=\"bypass -h Script ElfHumanFighterChange1 19\">" +
	"Change profession to an Elven Knight</a><br>\n" +
	"<a action=\"bypass Script ElfHumanFighterChange1 30288-11.htm\">" +
	"Return</a>\n" +
	"</body></html>"

// TestParseHTMLLinksQuestPage pins the quest dialog extraction: the
// command is the exact string RequestBypassToServer sends back, the
// text is what the walker matches by.
func TestParseHTMLLinksQuestPage(t *testing.T) {
	links := ParseHTMLLinks(soriusQuestPage)
	require.Len(t, links, 1)
	require.Equal(t, "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		links[0].Command)
	require.Equal(t, "Say you want to be an Elven Knight",
		links[0].Text)
}

// TestParseHTMLLinksClassMasterPage pins the -h prefix strip of the
// class change command and the multi-link page order.
func TestParseHTMLLinksClassMasterPage(t *testing.T) {
	links := ParseHTMLLinks(rainsClassPage)
	require.Len(t, links, 3)

	require.Equal(t, "Script ElfHumanFighterChange1 30288-13.htm",
		links[0].Command)
	require.Equal(t, "Description of an Elven Knight", links[0].Text)

	require.Equal(t, "Script ElfHumanFighterChange1 19",
		links[1].Command)
	require.Equal(t, "Change profession to an Elven Knight",
		links[1].Text)

	require.Equal(t, "Script ElfHumanFighterChange1 30288-11.htm",
		links[2].Command)
	require.Equal(t, "Return", links[2].Text)
}

// TestParseHTMLLinksLowercasesTheMatch pins the case-insensitive
// attribute match of the server side scan (the lowercased indexOf
// of HtmlUtil.buildHtmlBypassCache) while the command itself keeps
// its original casing.
func TestParseHTMLLinksLowercasesTheMatch(t *testing.T) {
	page := `<a ACTION="BYPASS Script Q00406_PathOfTheElvenKnight ` +
		`30327-05.htm">accept</a>`
	links := ParseHTMLLinks(page)
	require.Len(t, links, 1)
	require.Equal(t, "Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		links[0].Command)
	require.Equal(t, "accept", links[0].Text)
}

// TestParseHTMLLinksParameterMarker keeps the variable parameter of
// a link intact: the server cache validates only the prefix up to
// the marker, but the walker needs the full command to know where
// to substitute.
func TestParseHTMLLinksParameterMarker(t *testing.T) {
	page := `<a action="bypass -h npc_818056_BuyList 3006000 $cnt">` +
		`buy some</a>`
	links := ParseHTMLLinks(page)
	require.Len(t, links, 1)
	require.Equal(t, "npc_818056_BuyList 3006000 $cnt", links[0].Command)
	require.Equal(t, "buy some", links[0].Text)
}

// TestParseHTMLLinksTrimsTheCommand pins the server side trim of
// the command region.
func TestParseHTMLLinksTrimsTheCommand(t *testing.T) {
	page := `<a action="bypass -h  Script ElfHumanFighterChange1 19">` +
		`change</a>`
	links := ParseHTMLLinks(page)
	require.Len(t, links, 1)
	require.Equal(t, "Script ElfHumanFighterChange1 19", links[0].Command)
}

// TestParseHTMLLinksIgnoresOtherActions pins that non bypass
// attributes (the item links, the links of the community board)
// never produce entries.
func TestParseHTMLLinksIgnoresOtherActions(t *testing.T) {
	page := `<html><body><a action="link common/skill">skills</a>` +
		`<a action="bypass Script Q00406_PathOfTheElvenKnight ` +
		`30327-05.htm">quest</a></body></html>`
	links := ParseHTMLLinks(page)
	require.Len(t, links, 1)
	require.Equal(t, "quest", links[0].Text)
}

// TestParseHTMLLinksEmptyAndBroken pins the nil results of the
// empty, the link-free and the unterminated pages.
func TestParseHTMLLinksEmptyAndBroken(t *testing.T) {
	require.Nil(t, ParseHTMLLinks(""))
	require.Nil(t, ParseHTMLLinks("<html><body>plain text</body></html>"))
	// An unterminated action attribute ends the scan; the earlier
	// links of the page survive.
	page := `<a action="bypass Script One 01.htm">first</a>` +
		`<a action="bypass Script Two`
	links := ParseHTMLLinks(page)
	require.Len(t, links, 1)
	require.Equal(t, "Script One 01.htm", links[0].Command)
}

// TestParseHTMLLinksBigPageStaysBounded pins the link cap: a
// pathological page cannot grow the result past the bound.
func TestParseHTMLLinksBigPageStaysBounded(t *testing.T) {
	page := strings.Repeat(
		`<a action="bypass Script Q 01.htm">x</a>`, htmlLinkCap+50)
	links := ParseHTMLLinks(page)
	require.Len(t, links, htmlLinkCap)
}
