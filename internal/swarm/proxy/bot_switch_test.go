// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
	"unicode/utf16"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// tightenAutoSelectTimings shortens the auto select delay of the restart
// dance for one test and restores it afterwards.
func tightenAutoSelectTimings(t *testing.T, delay time.Duration) {
	t.Helper()
	holdKnobsMu.Lock()
	original := botSwitchAutoSelectDelay
	botSwitchAutoSelectDelay = delay
	holdKnobsMu.Unlock()
	t.Cleanup(func() {
		holdKnobsMu.Lock()
		defer holdKnobsMu.Unlock()
		botSwitchAutoSelectDelay = original
	})
}

// registerFakeBotWithID publishes a bot session with a custom id (so
// multiple bots can coexist in one server) and the given recorded
// history. Mirrors registerFakeBot but takes the id as a parameter.
func registerFakeBotWithID(
	t *testing.T, server *Server, id, name string, objectID int32,
) (*Recorder, *state.Bot) {
	t.Helper()
	tracker := state.NewBot(id)
	tracker.SetCharacter(name, objectID, 18, 45000, 50000, -3500, 90, 40)
	tracker.ApplyUserInfo(state.UserInfo{
		Name:    name,
		Level:   7,
		Race:    1,
		ClassID: 18,
		X:       45000,
		Y:       50000,
		Z:       -3500,
		CurHP:   90,
		MaxHP:   120,
		CurMP:   40,
		MaxMP:   50,
		Sp:      5,
		Exp:     1000,
	})
	tracker.SetOnline(name)

	sender := newFakeRawSender()
	recorder := server.RegisterSession(id, sender, tracker)
	t.Cleanup(func() { server.UnregisterSession(id, recorder) })

	return recorder, tracker
}

// buildCharSelectedWithID builds a CharSelected packet carrying the
// given object id (so two bots differ in their self id and the replay
// can be told apart).
func buildCharSelectedWithID(name string, objectID int32) []byte {
	data := []byte{0x21}
	data = appendUTF16(data, name)
	data = binary.LittleEndian.AppendUint32(data, uint32(objectID))
	data = appendUTF16(data, "")
	data = binary.LittleEndian.AppendUint32(data, 42)
	for range 5 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}
	data = binary.LittleEndian.AppendUint32(data, 1)
	data = binary.LittleEndian.AppendUint32(data, 45000)
	data = binary.LittleEndian.AppendUint32(data, 50000)
	data = binary.LittleEndian.AppendUint32(data, 0xFFFFF268)
	data = binary.LittleEndian.AppendUint64(data, 0)
	data = binary.LittleEndian.AppendUint64(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 7)
	for range 10 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}

	return data
}

// buildUserInfoWithID builds a UserInfo packet carrying the given
// object id and name (the self state the proxy live-patches).
func buildUserInfoWithID(name string, objectID int32) []byte {
	data := []byte{0x04}
	data = binary.LittleEndian.AppendUint32(data, 45000)
	data = binary.LittleEndian.AppendUint32(data, 50000)
	data = binary.LittleEndian.AppendUint32(data, 0xFFFFF268)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, uint32(objectID))
	data = appendUTF16(data, name)
	for range 15 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}

	return data
}

// readDance reads the restart dance the proxy plays for a connected
// client on a bot switch: the RestartResponse (0x74), the char list
// (0x1F) of the newly selected bot and the auto selected CharSelected
// (0x21) of its live state. It returns the parsed char list and the
// char selected answer.
func readDance(t *testing.T, client *fakeGameClient) (
	*fromgameserver.CharSelectInfoPacket, []byte,
) {
	t.Helper()

	restart := client.readPacket()
	require.Equal(t, byte(0x74), restart[0], "the restart response opcode")
	require.Equal(t, []byte{0x01, 0x00, 0x00, 0x00}, restart[1:5],
		"the restart response result field (true)")

	listPayload := client.readPacket()
	require.Equal(t, byte(0x1F), listPayload[0], "the char list opcode")
	list := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(list, listPayload))

	selected := client.readPacket()
	require.Equal(t, byte(0x21), selected[0], "the auto char selected opcode")

	return list, selected
}

// charSelectedField reads the int32 field at the given offset of a
// CharSelected packet (the tail starts after the two strings, the
// object id and the seven header ints - see patchCharSelectedLive).
func charSelectedField(t *testing.T, payload []byte, offset int) int32 {
	t.Helper()
	offset += skipUtf16String(payload, 1)     // name
	offset += 4                               // object id
	offset = skipUtf16String(payload, offset) // title
	offset += charSelectedHeaderInts * 4

	require.LessOrEqual(t, offset+4, len(payload))

	return int32(binary.LittleEndian.Uint32(payload[offset:]))
}

// TestProxySwitchesConnectedClientToSelectedBot verifies the cross-bot
// switch through the official C1 restart flow: a client connected and
// watching bot A receives the RestartResponse + the char list of bot B
// + the auto selected CharSelected when the WebUI selects B, and after
// its EnterWorld the replay and the live feed of bot B stream. The
// client builds its whole world view from the enter world packets of
// bot B: the position, the appearance, the race, the class and the
// items arrive exactly as a fresh login would deliver them, because
// the restart is the one mid-session identity change the C1 client
// implements on its own.
func TestProxySwitchesConnectedClientToSelectedBot(t *testing.T) {
	tightenAutoSelectTimings(t, 150*time.Millisecond)
	server := startTestServer(t)

	// Bot A: the initially selected bot.
	recorderA, trackerA := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	trackerA.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 5001, Attackable: true,
		X: 45200, Y: 50200, Z: -3500, Name: "KeltirA",
	})
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))
	recorderA.Record(buildTestWorldPacket(0x22, 5001))

	// Bot B: the target of the switch. Its live place differs from the
	// login-time coordinates of the recorded packet, so the live patch
	// of the dance is observable.
	recorderB, trackerB := registerFakeBotWithID(
		t, server, "botB", "CharB", 2002)
	trackerB.ApplyPlacement(state.Placement{
		ObjectID: 2002, X: 46000, Y: 51000, Z: -3500, Heading: 320,
	})
	trackerB.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 6002, Attackable: true,
		X: 46000, Y: 51000, Z: -3500, Name: "GoblinB",
	})
	recorderB.Record(buildCharSelectedWithID("CharB", 2002))
	recorderB.Record(buildUserInfoWithID("CharB", 2002))
	recorderB.Record(buildTestWorldPacket(0x22, 6002))

	// The client connects and selects bot A (the first registered).
	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)

	// The replay of bot A: the UserInfo (0x04) and the npc spawn (0x22).
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")
	npcA := client.readPacket()
	require.Equal(t, byte(0x22), npcA[0], "the npc spawn of bot A")

	// The WebUI switches the selection to bot B: the dance fires.
	server.SelectBot("botB")

	// The client receives the restart dance of bot B: the restart
	// answer, the char list carrying CharB and the auto selected answer
	// live-patched to the place the bot actually stands.
	list, selected := readDance(t, client)
	require.Len(t, list.Characters, 1, "the dance offers exactly one character")
	require.Equal(t, "CharB", list.Characters[0].Name,
		"the offered character is the one of bot B")
	require.EqualValues(t, 2002, list.Characters[0].ObjectID,
		"the offered object id is the self id of bot B")
	require.EqualValues(t, 46000, charSelectedField(t, selected, 0),
		"the live patched x of the answer")
	require.EqualValues(t, 51000, charSelectedField(t, selected, 4),
		"the live patched y of the answer")
	require.EqualValues(t, 46000, charSelectedField(t, selected, 0),
		"the live patched x is where bot B stands")

	// The client loads the character and enters the world; the replay
	// of bot B follows: its UserInfo and its npc spawn.
	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot B")
	npcB := client.readPacket()
	require.Equal(t, byte(0x22), npcB[0],
		"the npc spawn of bot B after the switch")
	require.EqualValues(t, 6002, int32(binary.LittleEndian.Uint32(npcB[1:5])))

	// The live feed of bot B flows to the switched client.
	recorderB.Record(buildTestWorldPacket(0x59, 6002))
	require.Equal(t, byte(0x59), client.readPacket()[0], "the live feed of bot B")

	// The client connection stays open.
	requireConnOpen(t, client, 250*time.Millisecond)
}

// TestProxySelectBotSameIdDoesNotSwitch verifies that selecting the
// already-selected bot does not trigger a dance (no spurious restart).
func TestProxySelectBotSameIdDoesNotSwitch(t *testing.T) {
	server := startTestServer(t)
	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)

	// Read the replay.
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// Re-selecting the same bot must not fire a dance.
	server.SelectBot("botA")

	// No restart packet should arrive: the client stays on the live
	// feed of bot A. A short wait confirms nothing is sent.
	requireNoPacket(t, client, 200*time.Millisecond)
}

// TestProxySwitchToOfflineBotStaysOnCurrent verifies that selecting a
// bot that is not yet online does NOT dance the client: it stays on
// the current bot's live feed until the target comes online.
func TestProxySwitchToOfflineBotStaysOnCurrent(t *testing.T) {
	server := startTestServer(t)
	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// Select a bot that is not registered.
	server.SelectBot("nonexistent")

	// No dance: the client stays on bot A's live feed.
	requireNoPacket(t, client, 200*time.Millisecond)
}

// requireNoPacket asserts that no packet arrives within the given
// duration: the relay stays idle (no spurious dance, no live feed).
func requireNoPacket(t *testing.T, client *fakeGameClient, d time.Duration) {
	t.Helper()
	requireConnOpen(t, client, d)
}

// TestProxySwitchToOfflineTargetCompletesWhenOnline pins the pending
// switch: a WebUI selection of a bot that was registered but not
// online yet (still connecting) keeps the client on the current live
// feed, and the switch completes by itself the moment the target
// enters the world - without the user re-clicking anything (a second
// SelectBot of the same id is a no-op that never refires).
func TestProxySwitchToOfflineTargetCompletesWhenOnline(t *testing.T) {
	tightenAutoSelectTimings(t, 150*time.Millisecond)
	server := startTestServer(t)

	// Bot A: the initially selected, online bot.
	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	// Bot B: registered with a recorded history, but the tracker never
	// reached the world yet (the connecting state of a session login).
	trackerB := state.NewBot("botB")
	trackerB.SetCharacter("CharB", 2002, 18, 46000, 51000, -3500, 90, 40)
	trackerB.ApplyUserInfo(state.UserInfo{
		Name:    "CharB",
		Level:   7,
		Race:    1,
		ClassID: 18,
		X:       46000,
		Y:       51000,
		Z:       -3500,
		CurHP:   90,
		MaxHP:   120,
		CurMP:   40,
		MaxMP:   50,
		Sp:      5,
		Exp:     1000,
	})
	senderB := newFakeRawSender()
	recorderB := server.RegisterSession("botB", senderB, trackerB)
	t.Cleanup(func() { server.UnregisterSession("botB", recorderB) })
	recorderB.Record(buildCharSelectedWithID("CharB", 2002))
	recorderB.Record(buildUserInfoWithID("CharB", 2002))
	recorderB.Record(buildTestWorldPacket(0x22, 6002))

	// The client connects and watches bot A.
	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// The selection moves to the still connecting bot B: no dance
	// happens (the client stays on bot A's live feed), the pending
	// switch arms silently.
	server.SelectBot("botB")
	requireNoPacket(t, client, 300*time.Millisecond)

	// Bot B enters the world: the pending switch completes on its own
	// with the restart dance of bot B.
	trackerB.SetOnline("CharB")

	list, _ := readDance(t, client)
	require.Equal(t, "CharB", list.Characters[0].Name,
		"the pending switch offers bot B once it is online")

	// The re-entry streams the world of bot B.
	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot B")
	npcB := client.readPacket()
	require.Equal(t, byte(0x22), npcB[0], "the npc spawn of bot B")
	require.EqualValues(t, 6002, int32(binary.LittleEndian.Uint32(npcB[1:5])))

	// The client connection stays open.
	requireConnOpen(t, client, 250*time.Millisecond)
}

// TestProxySwitchServesUserClickWhenAutoSelectLags pins the manual
// fallback of the dance: the auto select timer has not fired yet (a
// long delay), and the user's own double click of the offered
// character is answered with the CharSelected of the target bot. The
// client then enters the world exactly like after the auto select.
func TestProxySwitchServesUserClickWhenAutoSelectLags(t *testing.T) {
	tightenAutoSelectTimings(t, time.Minute)
	server := startTestServer(t)

	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	recorderB, _ := registerFakeBotWithID(
		t, server, "botB", "CharB", 2002)
	recorderB.Record(buildCharSelectedWithID("CharB", 2002))
	recorderB.Record(buildUserInfoWithID("CharB", 2002))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// The switch offers the dance but the auto select lags.
	server.SelectBot("botB")
	restart := client.readPacket()
	require.Equal(t, byte(0x74), restart[0], "the restart response opcode")
	listPayload := client.readPacket()
	require.Equal(t, byte(0x1F), listPayload[0], "the char list opcode")

	// The user double clicks the offered character: the answer carries
	// the CharSelected of bot B.
	client.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selected := client.readPacket()
	require.Equal(t, byte(0x21), selected[0], "the char selected answer")
	name, err := parseCharSelectedName(selected)
	require.NoError(t, err)
	require.Equal(t, "CharB", name, "the answered character is bot B's")

	// The world entry streams bot B.
	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot B")

	// The auto select never fires afterwards (the click cancelled it).
	requireNoPacket(t, client, 200*time.Millisecond)

	requireConnOpen(t, client, 250*time.Millisecond)
}

// TestProxySwitchDuringDanceFollowsNewestSelection pins the parked
// relay: a WebUI selection change that fires while the client sits on
// the char select screen re-offers the char list of the newest target,
// and the character the client enters is the newest selection (the
// serve path re-resolves it), not the one the dance started with.
func TestProxySwitchDuringDanceFollowsNewestSelection(t *testing.T) {
	tightenAutoSelectTimings(t, time.Minute)
	server := startTestServer(t)

	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	recorderB, _ := registerFakeBotWithID(
		t, server, "botB", "CharB", 2002)
	recorderB.Record(buildCharSelectedWithID("CharB", 2002))
	recorderB.Record(buildUserInfoWithID("CharB", 2002))

	recorderC, _ := registerFakeBotWithID(
		t, server, "botC", "CharC", 3003)
	recorderC.Record(buildCharSelectedWithID("CharC", 3003))
	recorderC.Record(buildUserInfoWithID("CharC", 3003))
	recorderC.Record(buildTestWorldPacket(0x22, 7003))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// The dance onto bot B starts (the auto select is parked far away):
	// the restart answer and the char list of CharB.
	server.SelectBot("botB")
	require.Equal(t, byte(0x74), client.readPacket()[0],
		"the restart response of the dance onto bot B")
	listPayload := client.readPacket()
	require.Equal(t, byte(0x1F), listPayload[0], "the dance char list")
	list := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(list, listPayload))
	require.Equal(t, "CharB", list.Characters[0].Name,
		"the dance offers the character of bot B")

	// While the client sits on the char select screen, the WebUI moves
	// the selection to bot C: the screen is re-offered onto CharC.
	server.SelectBot("botC")
	reOffered := client.readPacket()
	require.Equal(t, byte(0x1F), reOffered[0], "the re-offered char list")
	reOfferedList := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(reOfferedList, reOffered))
	require.Equal(t, "CharC", reOfferedList.Characters[0].Name,
		"the re-offered character is the newest selection")

	// The user's click is answered with the newest selection as well
	// (the serve path re-resolves the selection, not the offered list).
	client.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selected := client.readPacket()
	require.Equal(t, byte(0x21), selected[0], "the char selected answer")
	name, err := parseCharSelectedName(selected)
	require.NoError(t, err)
	require.Equal(t, "CharC", name, "the answered character is CharC")

	// The world entry streams bot C.
	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot C")
	npcC := client.readPacket()
	require.Equal(t, byte(0x22), npcC[0], "the npc spawn of bot C")
	require.EqualValues(t, 7003, int32(binary.LittleEndian.Uint32(npcC[1:5])))

	requireConnOpen(t, client, 250*time.Millisecond)
}

// parseCharSelectedName reads the name of a CharSelected packet (the
// first null terminated utf16 string behind the opcode).
func parseCharSelectedName(payload []byte) (string, error) {
	if len(payload) < 3 || payload[0] != 0x21 {
		return "", errors.New("not a char selected packet")
	}

	end := -1
	for i := 1; i+1 < len(payload); i += 2 {
		if payload[i] == 0 && payload[i+1] == 0 {
			end = i

			break
		}
	}
	if end < 0 {
		return "", errors.New("the char selected name is unterminated")
	}

	units := make([]uint16, 0, end/2)
	for i := 1; i < end; i += 2 {
		units = append(units, uint16(payload[i])|uint16(payload[i+1])<<8)
	}

	return string(utf16.Decode(units)), nil
}
