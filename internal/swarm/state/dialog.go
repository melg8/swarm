// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "strings"

// dialogLinkCap bounds the stored links of one dialog page: the
// parser itself caps at 128, the stored copy keeps the same bound
// so a pathological page cannot grow the tracker state.
const dialogLinkCap = 128

// DialogLinkView is one link of the open dialog page: the bypass
// command the link carries (the exact string RequestBypassToServer
// must send back) and the visible text the dialog walker matches
// pages by.
type DialogLinkView struct {
	Command string
	Text    string
}

// DialogPageView is one server dialog page (an NpcHtmlMessage the
// connection layer parsed and extracted): the npc the dialog
// belongs to (the origin object id of every bypass it carries - the
// 250 unit interaction check rides on it), the item id of item
// bound dialogs (0 for the npc scope) and the page links.
type DialogPageView struct {
	NpcObjID int32
	ItemID   int32
	Links    []DialogLinkView
}

// ApplyDialog replaces the open dialog page: the NPC_HTML scope of
// the server holds only the last page (every arrival clears the
// scope cache before caching the new links, see
// HtmlUtil.buildHtmlActionCache), so the tracker stores exactly one
// page and replaces it whole.
func (b *Bot) ApplyDialog(page DialogPageView) {
	b.mu.Lock()
	defer b.mu.Unlock()
	links := make([]DialogLinkView, 0, min(len(page.Links), dialogLinkCap))
	links = append(links, page.Links[:min(len(page.Links), dialogLinkCap)]...)
	b.dialog = openDialog{
		npcObjID: page.NpcObjID,
		itemID:   page.ItemID,
		links:    links,
	}
	b.touch()
}

// DialogLinks returns the links of the open dialog page (nil when
// no dialog is open).
func (b *Bot) DialogLinks() []DialogLinkView {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.dialog.links == nil {
		return nil
	}
	links := make([]DialogLinkView, len(b.dialog.links))
	copy(links, b.dialog.links)

	return links
}

// DialogOrigin returns the npc object id of the open dialog page
// (0 when no dialog is open).
func (b *Bot) DialogOrigin() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.dialog.npcObjID
}

// IsDialogCommand mirrors the server side validation of
// Player.validateHtmlAction: a command passes when the open page
// carries it exactly, or when the page carries it as a variable
// parameter link (the command prefix up to the '$' marker matches).
// A command the open page never offered must not be sent - the
// server drops it silently (the anti-injection gate of
// docs/quest_protocol.md).
func (b *Bot) IsDialogCommand(command string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for i := range b.dialog.links {
		cached := b.dialog.links[i].Command
		if cached == command {
			return true
		}
		if strings.HasSuffix(cached, "$") &&
			strings.HasPrefix(command, dialogPrefix(cached)) {
			return true
		}
	}

	return false
}

// openDialog is the stored form of the open dialog page.
type openDialog struct {
	npcObjID int32
	itemID   int32
	links    []DialogLinkView
}

// dialogPrefix returns the validation prefix of a variable
// parameter link: the '$' marker stripped and the remainder
// trimmed - the form the server compares commands against (see
// Player.validateHtmlAction).
func dialogPrefix(cached string) string {
	return strings.TrimSpace(strings.TrimSuffix(cached, "$"))
}

// ClearDialog drops the open dialog page (the session reset path:
// a relogin starts with no dialog open).
func (b *Bot) ClearDialog() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dialog = openDialog{} //nolint:exhaustruct_v5 // the zero page clears
	b.touch()
}
