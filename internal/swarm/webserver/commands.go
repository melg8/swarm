// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/melg8/swarm/internal/swarm/state"
)

// commandRequest mirrors state.Command: the manual command posted by
// the web UI (map clicks, equipment widget drags and double clicks).
type commandRequest struct {
	Kind     string `json:"kind"`
	ObjectID int32  `json:"objectId"`
	Count    int32  `json:"count"`
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	Z        int32  `json:"z"`
}

// validCommand reports whether the request carries the fields its kind
// needs: a move needs coordinates, the object commands need an object
// id and a drop needs a positive count.
func validCommand(cmd commandRequest) bool {
	switch cmd.Kind {
	case state.CommandMove:
		return cmd.X != 0 || cmd.Y != 0 || cmd.Z != 0
	case state.CommandAttack, state.CommandPickup, state.CommandUseItem:
		return cmd.ObjectID != 0
	case state.CommandDrop, state.CommandDestroy:
		return cmd.ObjectID != 0 && cmd.Count >= 1
	default:
		return false
	}
}

// describeCommand renders the queued command for the event log.
func describeCommand(cmd commandRequest) string {
	switch cmd.Kind {
	case state.CommandMove:
		return "user command: walk to " + strconv.Itoa(int(cmd.X)) + " " +
			strconv.Itoa(int(cmd.Y)) + " " + strconv.Itoa(int(cmd.Z))
	case state.CommandAttack:
		return "user command: attack object " +
			strconv.Itoa(int(cmd.ObjectID))
	case state.CommandPickup:
		return "user command: pick up item " +
			strconv.Itoa(int(cmd.ObjectID))
	case state.CommandUseItem:
		return "user command: use item " +
			strconv.Itoa(int(cmd.ObjectID))
	case state.CommandDrop:
		return "user command: drop " + strconv.Itoa(int(cmd.Count)) +
			" of item " + strconv.Itoa(int(cmd.ObjectID))
	case state.CommandDestroy:
		return "user command: destroy " + strconv.Itoa(int(cmd.Count)) +
			" of item " + strconv.Itoa(int(cmd.ObjectID))
	default:
		return "user command: " + cmd.Kind
	}
}

// handleBotCommand queues one manual command for the hunt loop of the
// bot. The loop consumes it on its next tick (a quarter second), so
// the answer is an accepted receipt, not a completion.
func (s *Server) handleBotCommand(w http.ResponseWriter, r *http.Request) {
	bot, ok := s.lookupBot(w, r)
	if !ok {
		return
	}
	var request commandRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)

		return
	}
	if !validCommand(request) {
		http.Error(w, "invalid command", http.StatusBadRequest)

		return
	}
	bot.PushCommand(state.Command{
		Kind:     request.Kind,
		ObjectID: request.ObjectID,
		Count:    request.Count,
		X:        request.X,
		Y:        request.Y,
		Z:        request.Z,
	})
	bot.RecordEvent(describeCommand(request))
	w.WriteHeader(http.StatusAccepted)
}
