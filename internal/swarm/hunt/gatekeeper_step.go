// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "errors"
    "fmt"
    "time"
)

// gatekeeperDialogWait bounds the wait for the server NpcHTMLMessage reply to a
// bypass command: the server answers within a few hundred milliseconds, so five
// seconds covers a slow tick or a brief network stall. Past it the step fails
// instead of hanging the loop. The var (not a const) is a test seam: the unit
// tests shorten it to keep the timeout case under a second.
var gatekeeperDialogWait = 5 * time.Second

// gatekeeperPollPeriod paces the html arrival wait until the npc object id
// changes or the wait lapses.
const gatekeeperPollPeriod = 250 * time.Millisecond

// DriveGatekeeperTeleport drives one gatekeeper segment from within the
// interaction distance: it sends the showTeleports bypass, waits for the
// teleport list html, finds the destination button whose label contains
// destLabel and sends the teleport bypass; the connection layer answers the
// arrival TeleportToLocation with Appearing. The blocking call is bounded by
// gatekeeperDialogWait per dialog reply, and the tick machine does not call
// it yet. The listName is "NORMAL" for the standard list.
func (l *Loop) DriveGatekeeperTeleport(
    npcObjID int32, listName, destLabel string,
) error {
    if err := l.openTeleportDialog(npcObjID); err != nil {
        return err
    }
    html, err := l.awaitDialog(npcObjID)
    if err != nil {
        return err
    }
    buttons := ParseGatekeeperHTML(html)
    teleport := FindTeleportButton(buttons, listName, destLabel)
    if teleport == nil {
        return fmt.Errorf(
            "gatekeeper: no teleport button for %q in list %q",
            destLabel, listName)
    }
    command := fmt.Sprintf("npc_%d_teleport %s %d",
        npcObjID, teleport.ListName, teleport.LocID)
    l.logf("gatekeeper: teleporting to %s", teleport.Label)

    return l.game.SendBypass(command)
}

// openTeleportDialog sends the showTeleports bypass to the teleporter.
// The server answers with a NpcHTMLMessage carrying the teleport list
// (the teleports.htm template with the %locations% replaced).
func (l *Loop) openTeleportDialog(npcObjID int32) error {
    command := fmt.Sprintf("npc_%d_showTeleports", npcObjID)

    return l.game.SendBypass(command)
}

// awaitDialog waits for the server NpcHTMLMessage reply from the teleporter
// npc. The step reads LastHTMLDialog until the npc object id matches (the
// server replaces the dialog with the teleport list) or the wait lapses. A zero
// npcObjID on the first read means no dialog arrived yet - the wait continues.
func (l *Loop) awaitDialog(npcObjID int32) (string, error) {
    deadline := l.now().Add(gatekeeperDialogWait)
    for {
        id, html := l.game.LastHTMLDialog()
        if id == npcObjID && html != "" {
            return html, nil
        }
        if l.now().After(deadline) {
            return "", errors.New(
                "gatekeeper: the teleport list dialog never arrived")
        }
        pace(gatekeeperPollPeriod)
    }
}
