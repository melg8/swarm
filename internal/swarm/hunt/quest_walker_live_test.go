// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The live dialog walker verification of T-014: a fresh elven
// fighter enters the deployed Mobius C1 stack by the elven village
// trainer Ellenia (the position lands there through a DB injection
// between the character creation and the world entry - the same
// channel the acceptance scenarios use) and drives the dialog
// walker through a real conversation: the two-click talk entry,
// the trainer page, the "Quest" bypass link and the answer page.
// The suite is opt-in (the live stack, a real session):
//
//	SWARM_LIVE_DIALOG=1 go test ./internal/swarm/hunt/ \
//	    -run TestLiveDialogWalkerRoundTrip -v -timeout 5m
//
// The route link defaults to "Quest" (the trainer pages of the
// elven village carry the bare `bypass Script` link - the quest
// choose window or the no-quest message answers with an html page,
// see ScriptLink.showQuestWindow); SWARM_LIVE_DIALOG_LINK overrides
// it for ad-hoc experiments with other links of the page.
const (
	liveDialogAccount  = "dialogw1"
	liveDialogPassword = "test"
	liveLoginAddress   = "127.0.0.1:2106"
	// The DB injection channel of the acceptance scenarios: the
	// passwordless root account over TCP (the game server uses the
	// same channel).
	liveDBHost = "127.0.0.1"
	liveDBPort = "3306"
	liveDBName = "l2jmobiusc1"
	liveDBUser = "root"
	// The mariadb CLI of the sandbox deployment (the client library
	// path rides LD_LIBRARY_PATH - the sandbox binary quirk).
	liveMysqlBin  = "/home/z/opt/mariadb/bin/mariadb"
	liveMysqlLibs = "/home/z/opt/mariadb/lib:" +
		"/home/z/opt/mariadb/lib/x86_64-linux-gnu"
	// The injection point: the approach ring south of Ellenia (the
	// elven village trainer at 45725, 52105, -2792 - the npcdata
	// teacher position), 150 units off her cell (the roof rule: the
	// character never spawns on the interior cell itself).
	liveInjectX = 45725
	liveInjectY = 51955
	liveInjectZ = -2792
	// The default route link of the trainer page conversation.
	liveDefaultLink = "Quest"

	liveWalkTimeout    = 90 * time.Second
	liveOnlineTimeout  = 30 * time.Second
	liveNpcFindTimeout = 30 * time.Second
)

// liveTeacherTemplateIDs are the elven village trainer npcs of the
// elven fighter (the npcdata teachers Ellenia and Cobendell; the
// tracker template ids carry the display id plus the 1000000
// offset). Both trainer pages carry the SkillList and Quest links.
var liveTeacherTemplateIDs = []int32{
	7155 + npcDisplayOffset, // Ellenia
	7156 + npcDisplayOffset, // Cobendell
}

// TestLiveDialogWalkerRoundTrip verifies the dialog walker against
// the live stack: the talk entry, the trainer page parse, the
// tracker feed, the "Quest" bypass round trip and the answer page.
func TestLiveDialogWalkerRoundTrip(t *testing.T) {
	if os.Getenv("SWARM_LIVE_DIALOG") != "1" {
		t.Skip("set SWARM_LIVE_DIALOG=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the live dialog walker test")
	}
	tracker := state.NewBot(liveDialogAccount)
	logger := log.New(os.Stderr, "[dialog-live] ", log.LstdFlags)
	game, cancel := liveDialogSession(t, tracker, logger)
	defer cancel()

	// The trainer npc of the injected position: the world store
	// fills from the spawn packets of the world entry.
	teacher := liveAwaitTeacher(t, tracker)
	t.Logf("teacher found: %s (object %d) at (%d, %d, %d)",
		teacher.Name, teacher.ObjectID, teacher.X, teacher.Y, teacher.Z)

	// The close approach: walk the npc approach ring (the offset
	// click, not the npc cell - the roof rule).
	liveApproachNpc(t, game, tracker, teacher)

	loop := NewLoop(game, tracker)
	loop.SetLogger(logger)

	linkWanted := os.Getenv("SWARM_LIVE_DIALOG_LINK")
	if linkWanted == "" {
		linkWanted = liveDefaultLink
	}
	route := []DialogStep{{LinkText: linkWanted}}
	err := loop.DriveDialog(teacher.ObjectID, route)
	require.NoError(t, err, "the %q link round trip failed", linkWanted)
	require.Equal(t, teacher.ObjectID, tracker.DialogOrigin(),
		"the answer page is the open dialog of the tracker")
	t.Logf("round trip ok: the %q link walk completed, the tracker "+
		"holds the answer page with %d links",
		linkWanted, len(tracker.DialogLinks()))
	for i, link := range tracker.DialogLinks() {
		t.Logf("answer link %d: %q -> %q", i+1, link.Text, link.Command)
	}
}

// liveDialogSession performs the session bootstrap of the fleet
// recipe with the position injection: login, handshake, the elven
// fighter character (created when missing), the DB move of the
// character to the trainer approach ring, the world entry and the
// receive loop. The returned cancel stops the session.
func liveDialogSession(
	t *testing.T, tracker *state.Bot, logger *log.Logger,
) (*connection.GameClient, context.CancelFunc) {
	t.Helper()
	loginConn, err := net.DialTimeout("tcp", liveLoginAddress, 10*time.Second)
	require.NoError(t, err, "the login dial failed")
	auth, err := connection.Authenticate(
		loginConn, liveDialogAccount, liveDialogPassword)
	require.NoError(t, err, "the login authentication failed")

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2],
		auth.ServerIP[3], auth.ServerPort)
	gameConn, err := net.DialTimeout("tcp", gameAddress, 10*time.Second)
	require.NoError(t, err, "the game dial failed")
	game, err := connection.NewGameClient(gameConn)
	require.NoError(t, err, "the game handshake failed")
	game.SetTracker(tracker)
	game.SetLogger(logger)

	charList, err := game.Authenticate(connection.GameSessionParams{
		Account:    auth.Account,
		LoginOkID1: auth.LoginOkID1,
		LoginOkID2: auth.LoginOkID2,
		PlayOkID1:  auth.PlayOkID1,
		PlayOkID2:  auth.PlayOkID2,
	})
	require.NoError(t, err, "the game authentication failed")
	charList, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      liveDialogAccount,
		Race:      1, // ELF
		Female:    0,
		ClassID:   18, // ELVEN_FIGHTER
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}, charList)
	require.NoError(t, err, "the character creation failed")
	liveInjectPosition(t)
	slot, _, found := charList.FindCharacterByName(liveDialogAccount)
	require.True(t, found, "the character is missing")
	require.NoError(t, game.EnterWorld(int32(slot)),
		"the world entry failed")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		runErr := game.Run(ctx, liveDialogAccount)
		if runErr != nil && ctx.Err() == nil {
			logger.Printf("session ended: %v", runErr)
		}
	}()
	deadline := time.Now().Add(liveOnlineTimeout)
	for tracker.Status() != state.StatusOnline {
		if time.Now().After(deadline) {
			t.Fatal("the character never entered the world")
		}
		time.Sleep(100 * time.Millisecond)
	}

	return game, cancel
}

// liveInjectPosition moves the offline character row to the trainer
// approach ring through the DB (the game server loads the position
// from the row at the world entry).
func liveInjectPosition(t *testing.T) {
	t.Helper()
	sql := fmt.Sprintf(
		"UPDATE characters SET x=%d, y=%d, z=%d, online=0 "+
			"WHERE char_name='%s'",
		liveInjectX, liveInjectY, liveInjectZ, liveDialogAccount)
	cmd := exec.Command(liveMysqlBin,
		"-h", liveDBHost, "-P", liveDBPort, "-u", liveDBUser,
		liveDBName, "-e", sql)
	cmd.Env = append(os.Environ(),
		"LD_LIBRARY_PATH="+liveMysqlLibs)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"the position injection failed: %s", string(out))
}

// liveAwaitTeacher waits for a trainer npc to appear in the world
// store of the tracker (the spawn packets of the world entry).
func liveAwaitTeacher(t *testing.T, tracker *state.Bot) state.AttackTarget {
	t.Helper()
	deadline := time.Now().Add(liveNpcFindTimeout)
	for {
		teacher, ok := tracker.NearestNpcByTemplates(
			liveTeacherTemplateIDs, 6000)
		if ok {
			return teacher
		}
		if time.Now().After(deadline) {
			liveDumpNamedNpcs(t, tracker)
			t.Fatal("no trainer npc appeared around the injection point")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// liveDumpNamedNpcs lists every named npc of the world store (the
// diagnostic of a failed teacher search: what the spawn actually
// delivered around the character).
func liveDumpNamedNpcs(t *testing.T, tracker *state.Bot) {
	t.Helper()
	snap := tracker.Snapshot()
	selfX, selfY, _, _ := tracker.SelfPosition()
	for i := range snap.Objects {
		obj := snap.Objects[i]
		if obj.Name == "" || obj.TemplateID <= npcDisplayOffset {
			continue
		}
		dx := float64(obj.X - selfX)
		dy := float64(obj.Y - selfY)
		t.Logf("npc %s (template %d, object %d) at (%d, %d, %d), "+
			"dist %.0f", obj.Name, obj.TemplateID-npcDisplayOffset,
			obj.ObjectID, obj.X, obj.Y, obj.Z, math.Hypot(dx, dy))
	}
}

// liveApproachNpc walks the character to the interaction distance
// of the npc: the approach ring clicks (the offset point toward the
// walker) until the 3D distance fits the server interaction gate.
func liveApproachNpc(
	t *testing.T, game *connection.GameClient,
	tracker *state.Bot, teacher state.AttackTarget,
) {
	t.Helper()
	deadline := time.Now().Add(liveWalkTimeout)
	lastMove := time.Time{}
	for {
		selfX, selfY, selfZ, ok := tracker.SelfPosition()
		require.True(t, ok, "the self position vanished")
		dx := float64(teacher.X - selfX)
		dy := float64(teacher.Y - selfY)
		dz := float64(teacher.Z - selfZ)
		if math.Sqrt(dx*dx+dy*dy+dz*dz) <= npcInteractionDist-20 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the teacher approach never closed: dist %.0f",
				math.Sqrt(dx*dx+dy*dy+dz*dz))
		}
		if time.Since(lastMove) >= walkRequestPeriod {
			lastMove = time.Now()
			ax, ay, az := npcApproachPoint(
				teacher.X, teacher.Y, teacher.Z, selfX, selfY)
			require.NoError(t, game.WalkTo(ax, ay, az))
		}
		time.Sleep(200 * time.Millisecond)
	}
}
