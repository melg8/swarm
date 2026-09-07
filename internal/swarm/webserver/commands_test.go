// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// postCommand posts one manual command to the bot endpoint.
func postCommand(
	t *testing.T, server *Server, botID string, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost, "/api/bots/"+botID+"/commands", bytes.NewBufferString(body))
	server.httpServer.Handler.ServeHTTP(recorder, request)

	return recorder
}

// TestBotCommandQueued pins the happy path: a valid command lands in
// the queue of the bot and the event log records the user intent.
func TestBotCommandQueued(t *testing.T) {
	server, bot := newTestServer(t)

	recorder := postCommand(t, server, "test1",
		`{"kind":"move","x":46112,"y":41500,"z":-3056}`)
	require.Equal(t, http.StatusAccepted, recorder.Code)

	select {
	case cmd := <-bot.Commands():
		require.Equal(t, state.CommandMove, cmd.Kind)
		require.Equal(t, int32(46112), cmd.X)
		require.Equal(t, int32(41500), cmd.Y)
		require.Equal(t, int32(-3056), cmd.Z)
	default:
		t.Fatal("the command must be queued")
	}
	events := bot.Snapshot().Events
	found := false
	for _, event := range events {
		if event.Message == "user command: walk to 46112 41500 -3056" {
			found = true
		}
	}
	require.True(t, found, "the event log must record the command")
}

// TestBotCommandValidation pins the request validation: unknown kinds,
// missing object ids and non positive drop counts are rejected without
// queueing anything.
func TestBotCommandValidation(t *testing.T) {
	server, bot := newTestServer(t)

	for _, body := range []string{
		`{"kind":"dance"}`,
		`{"kind":"attack"}`,
		`{"kind":"pickup","objectId":0}`,
		`{"kind":"useItem","objectId":0}`,
		`{"kind":"drop","objectId":5,"count":0}`,
		`{"kind":"move","x":0,"y":0,"z":0}`,
		`not json`,
	} {
		recorder := postCommand(t, server, "test1", body)
		require.Equal(t, http.StatusBadRequest, recorder.Code,
			"body %q must be rejected", body)
	}

	select {
	case <-bot.Commands():
		t.Fatal("no command may be queued from invalid bodies")
	default:
	}
}

// TestBotCommandUnknownBot pins the 404 of unknown bot ids.
func TestBotCommandUnknownBot(t *testing.T) {
	server, _ := newTestServer(t)

	recorder := postCommand(t, server, "nobody",
		`{"kind":"move","x":1,"y":2,"z":3}`)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

// TestBotCommandKinds pins every accepted kind round trips through the
// queue with its fields.
func TestBotCommandKinds(t *testing.T) {
	server, bot := newTestServer(t)
	//nolint:exhaustruct // fields under test are the only ones set
	commands := []state.Command{
		{Kind: state.CommandAttack, ObjectID: 7},
		{Kind: state.CommandPickup, ObjectID: 9},
		{Kind: state.CommandUseItem, ObjectID: 555},
		{Kind: state.CommandDrop, ObjectID: 555, Count: 40},
	}

	for _, want := range commands {
		body, err := json.Marshal(want)
		require.NoError(t, err)
		recorder := postCommand(t, server, "test1", string(body))
		require.Equal(t, http.StatusAccepted, recorder.Code,
			"command %s must be accepted", want.Kind)
		select {
		case got := <-bot.Commands():
			require.Equal(t, want, got)
		default:
			t.Fatalf("command %s must be queued", want.Kind)
		}
	}
}
