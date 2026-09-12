// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"fmt"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
)

// applyNpcHTMLMessage parses the server html dialog packet and stores
// the last html under the html lock so the hunt loop (another
// goroutine) can read it through LastHTMLMessage. The gatekeeper flow
// drives the dialog: the bot sends a RequestBypassToServer command
// and reads the NpcHTMLMessage reply to find the next bypass button
// (see the H-002 verification and docs/protocol_description.md).
func (gc *GameClient) applyNpcHTMLMessage(payload []byte) {
	err := fromgameserver.ParseNpcHTMLMessage(&gc.npcHTML, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse npc html: %v", err)

		return
	}
	gc.htmlMu.Lock()
	gc.lastHTML = gc.npcHTML
	gc.htmlMu.Unlock()
	if gc.tracker != nil {
		gc.tracker.RecordEvent(fmt.Sprintf(
			"npc html from %d: %d bytes",
			gc.npcHTML.NpcObjID, len(gc.npcHTML.HTML)))
		gc.tracker.ApplyDialog(state.DialogPageView{
			NpcObjID: gc.npcHTML.NpcObjID,
			ItemID:   gc.npcHTML.ItemID,
			Links:    gc.dialogLinks(),
		})
	}
}

// dialogLinks extracts the bypass links of the current html through
// the shared ParseHTMLLinks parser (T-012) and maps them to the
// state.DialogLinkView the open dialog section of the tracker (T-013)
// stores. The gatekeeper step reads the raw html through LastHTMLDialog
// for the teleport-specific classification (the list name and the
// destination index); the tracker path serves the web UI and the M2
// quest dialog walker.
func (gc *GameClient) dialogLinks() []state.DialogLinkView {
	parsed := fromgameserver.ParseHTMLLinks(gc.npcHTML.HTML)
	if len(parsed) == 0 {
		return nil
	}
	links := make([]state.DialogLinkView, 0, len(parsed))
	for i := range parsed {
		links = append(links, state.DialogLinkView{
			Command: parsed[i].Command,
			Text:    parsed[i].Text,
		})
	}

	return links
}

// LastHTMLMessage returns a copy of the last NpcHTMLMessage the
// server sent (the gatekeeper dialog the bot reads the bypass buttons
// from). The zero value (a zero NpcObjID) means no html arrived yet.
// Thread-safe: the dispatch goroutine writes, the hunt loop reads.
func (gc *GameClient) LastHTMLMessage() fromgameserver.NpcHTMLMessage {
	gc.htmlMu.Lock()
	defer gc.htmlMu.Unlock()

	return gc.lastHTML
}

// LastHTMLDialog returns the npc object id and the html body of the
// last NpcHTMLMessage, matching the hunt.GameAPI interface. A zero
// npcObjID means no dialog arrived yet.
func (gc *GameClient) LastHTMLDialog() (npcObjID int32, html string) {
	msg := gc.LastHTMLMessage()

	return msg.NpcObjID, msg.HTML
}

// SendBypass sends a RequestBypassToServer command to the server
// (the gatekeeper teleport flow, the H-002 verification). The command
// is the raw bypass string the NpcHtmlMessage button carried, WITHOUT
// the "bypass -h " prefix the client strips (the server receives
// "npc_<objId>_teleport <list> <index>" or "npc_<objId>_Chat"
// directly). An empty command is refused (the server disconnects on
// an empty bypass).
func (gc *GameClient) SendBypass(command string) error {
	return gc.sendPacket(
		&togameserver.RequestBypassToServer{Command: command})
}
