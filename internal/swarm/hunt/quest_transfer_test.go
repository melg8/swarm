// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"context"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The transfer, rest, route walk and dress-up tests of the quest trip
// engine. The scripted dialog stub of the walker tests serves the
// pages; the wrappers below move the server state the engine pins.

// bellaFirstPage is the first page a gatekeeper talk opens: the
// showTeleports button (the html action cache needs the page open
// before the bypass validates).
const bellaFirstPage = "<html><body>Bella:<br>" +
	"I can teleport you anywhere you wish.<br>" +
	"<a action=\"bypass npc_11_showTeleports\">Teleport</a><br>" +
	"<a action=\"bypass Script\">Quest</a>" +
	"</body></html>"

// bellaListPage is the teleport list page: the NORMAL destinations
// of the Gludio gatekeeper (the live xml labels).
const bellaListPage = "<html><body>Region where teleporting is possible<br><br>" +
	"<a action=\"bypass -h npc_11_teleport NORMAL 0\" " +
	"msg=\"811;The Village of Gludin\">" +
	"The Village of Gludin - 7300 Adena</a><br1>" +
	"<a action=\"bypass -h npc_11_teleport NORMAL 1\" " +
	"msg=\"811;Elven Village\">Elven Village - 9200 Adena</a><br1>" +
	"<br></body></html>"

// transferGame serves the gatekeeper flow: the ClickObject talk opens
// the first page, the showTeleports bypass opens the list, the
// teleport bypass moves the character to the arrival square.
type transferGame struct {
	*fakeGame
	bot     *state.Bot
	stage   int
	noMove  bool
	arriveX int32
	arriveY int32
}

func (g *transferGame) ClickObject(objectID int32) error {
	if err := g.fakeGame.ClickObject(objectID); err != nil {
		return err
	}
	g.htmlNPC = objectID
	g.htmlBody = bellaFirstPage

	return nil
}

func (g *transferGame) SendBypass(command string) error {
	if err := g.fakeGame.SendBypass(command); err != nil {
		return err
	}
	switch g.stage {
	case 0:
		if command == "npc_11_showTeleports" {
			g.htmlBody = bellaListPage
			g.stage = 1
		}
	case 1:
		if command == "npc_11_teleport NORMAL 0" {
			g.stage = 2
			if !g.noMove {
				g.bot.ApplyUserInfo(state.UserInfo{
					Name: "test1", Level: 20,
					X: g.arriveX, Y: g.arriveY, Z: -3043,
				})
			}
		}
	}

	return nil
}

// TestDriveQuestTransferFlow pins the full gatekeeper hop: the walk
// to the teleporter, the talk that opens the first page, the
// showTeleports bypass, the destination button lookup and the
// teleport arrival at the destination square.
func TestDriveQuestTransferFlow(t *testing.T) {
	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 11, TemplateID: 1007256, Attackable: false,
		X: 45050, Y: 50050, Z: -3500, Name: "Bella",
	})
	game := &transferGame{
		fakeGame: &fakeGame{}, bot: bot,
		arriveX: -80826, arriveY: 149775,
	}
	loop := NewLoop(game, bot)

	transfer := QuestTransfer{
		Gatekeeper: QuestNpc{
			TemplateID: 7256, Name: "Bella",
			X: 45050, Y: 50050, Z: -3500,
		},
		DestLabel: "The Village of Gludin",
		ArriveX:   -80826, ArriveY: 149775, ArriveZ: -3043,
	}
	err := loop.driveQuestTransfer(context.Background(), transfer)
	require.NoError(t, err)
	require.Equal(t, []string{
		"npc_11_showTeleports",
		"npc_11_teleport NORMAL 0",
	}, game.bypasses,
		"the hop sends the showTeleports then the teleport bypass")
	require.Len(t, game.clicks, 1,
		"the talk opens the first page exactly once")
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	require.InDelta(t, -80826, selfX, 1,
		"the character stands at the Gludin arrival square")
	require.InDelta(t, 149775, selfY, 1)
}

// TestDriveQuestTransferMissingDestination pins the lookup guard: a
// destination the teleport list does not carry fails the hop with
// the reason naming it. The label wait shrinks to the test stub
// period: the list page of the fake gatekeeper is static, so the
// production 20 s wait can only lapse - its expiry path is what the
// assertion pins (the sibling stuck arrival test shrinks it too).
func TestDriveQuestTransferMissingDestination(t *testing.T) {
	original := questTransferArriveWait
	questTransferArriveWait = 200 * time.Millisecond
	t.Cleanup(func() { questTransferArriveWait = original })

	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 11, TemplateID: 1007256, Attackable: false,
		X: 45050, Y: 50050, Z: -3500, Name: "Bella",
	})
	game := &transferGame{fakeGame: &fakeGame{}, bot: bot}
	loop := NewLoop(game, bot)

	transfer := QuestTransfer{
		Gatekeeper: QuestNpc{
			TemplateID: 7256, Name: "Bella",
			X: 45050, Y: 50050, Z: -3500,
		},
		DestLabel: "Giran Castle Town",
		ArriveX:   -80826, ArriveY: 149775, ArriveZ: -3043,
	}
	err := loop.driveQuestTransfer(context.Background(), transfer)
	require.ErrorContains(t, err, "Giran Castle Town")
}

// TestDriveQuestTransferStuckArrival pins the arrival guard: a
// teleport that never lands fails the hop (the stuck page flow left
// the character in Gludio).
func TestDriveQuestTransferStuckArrival(t *testing.T) {
	original := questTransferArriveWait
	questTransferArriveWait = 200 * time.Millisecond
	t.Cleanup(func() { questTransferArriveWait = original })

	bot := newTestBot()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 11, TemplateID: 1007256, Attackable: false,
		X: 45050, Y: 50050, Z: -3500, Name: "Bella",
	})
	// The teleport fires but never moves the character (the stuck
	// arrival of a lost TeleportToLocation).
	game := &transferGame{fakeGame: &fakeGame{}, bot: bot, noMove: true}
	loop := NewLoop(game, bot)

	transfer := QuestTransfer{
		Gatekeeper: QuestNpc{
			TemplateID: 7256, Name: "Bella",
			X: 45050, Y: 50050, Z: -3500,
		},
		DestLabel: "The Village of Gludin",
		ArriveX:   -80826, ArriveY: 149775, ArriveZ: -3043,
	}
	err := loop.driveQuestTransfer(context.Background(), transfer)
	require.ErrorContains(t, err, "never landed")
}

// TestRestBetweenFightsSitsAndStands pins the rest: a tired character
// sits down, regenerates to the stand threshold and stands up before
// the next engage.
func TestRestBetweenFightsSitsAndStands(t *testing.T) {
	bot := newTestBot()
	game := &healingGame{fakeGame: &fakeGame{}, bot: bot}
	loop := NewLoop(game, bot)

	// The character is tired: 30 of 100 HP.
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 30},
	})
	require.NoError(t, loop.restBetweenFights(context.Background()))
	require.Equal(t, 2, game.sits,
		"the rest sits down once and stands up once")
	require.GreaterOrEqual(t, bot.SelfHealthPercent(), questRestStandHP,
		"the rest stands up at the regeneration threshold")
}

// healingGame heals the character when the sit toggle lands (the
// server regeneration the sitting state grants: the HP flips with
// the first toggle and stays for the stand).
type healingGame struct {
	*fakeGame
	bot *state.Bot
}

func (h *healingGame) ActionSitStand() error {
	if err := h.fakeGame.ActionSitStand(); err != nil {
		return err
	}
	// The server confirms the toggle through the ChangeWaitType
	// broadcast: the sit drives the regeneration, the stand frees
	// the walk.
	sitting := h.sits%2 == 1
	h.bot.ApplyWaitType(state.WaitType{
		ObjectID: 100, Sitting: sitting,
	})
	if sitting {
		// The sit toggle: the regeneration covers the rest.
		h.bot.ApplyStatusUpdate(100, []state.Attribute{
			{ID: state.AttrCurHP, Value: 90},
		})
	}

	return nil
}

// TestRestBetweenFightsHealthySkips pins the healthy short circuit:
// a character above the sit threshold does not toggle at all.
func TestRestBetweenFightsHealthySkips(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	// newTestBot stands at 90 of 100 HP: above the threshold.
	require.NoError(t, loop.restBetweenFights(context.Background()))
	require.Equal(t, 0, game.sits,
		"a healthy character does not sit down")
}

// movingGame walks the character to the click target at once (the
// instant arrival the route tests need).
type movingGame struct {
	*fakeGame
	bot *state.Bot
}

func (m *movingGame) WalkTo(x int32, y int32, z int32) error {
	if err := m.fakeGame.WalkTo(x, y, z); err != nil {
		return err
	}
	m.bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 20, X: x, Y: y, Z: z,
	})

	return nil
}

// TestWalkQuestRoutePlannedSegments pins the segment walk: a goal
// beyond one segment length plans through the geodata navigator and
// follows the waypoints to the arrival radius.
func TestWalkQuestRoutePlannedSegments(t *testing.T) {
	bot := newTestBot()
	game := &movingGame{fakeGame: &fakeGame{}, bot: bot}
	loop := NewLoop(game, bot)
	// The straight route of the fake navigator: the destination is
	// far, the segment target stays inside it.
	loop.SetNavigator(&fakeNavigator{found: true})

	// The goal sits 20 000 units east: three segments of 8 000.
	err := loop.walkToQuestPoint(65000, 50000, -3500, 30*time.Second)
	require.NoError(t, err)
	selfX, selfY, _, ok := bot.SelfPosition()
	require.True(t, ok)
	require.InDelta(t, 65000, selfX, questArriveRadius,
		"the route walk must reach the goal")
	require.InDelta(t, 50000, selfY, questArriveRadius)
	require.NotEmpty(t, game.walks,
		"the walk sent its move requests")
}

// TestWalkQuestRouteDirectFallback pins the no-navigator walk: the
// legacy direct click still closes the walk when no geodata
// navigator is wired (the acceptance scenarios of the pre-quest
// shape).
func TestWalkQuestRouteDirectFallback(t *testing.T) {
	bot := newTestBot()
	game := &movingGame{fakeGame: &fakeGame{}, bot: bot}
	loop := NewLoop(game, bot)

	err := loop.walkToQuestPoint(45600, 50600, -3500, 30*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, game.walks,
		"the legacy walk sent its move request")
}

// dressingGame serves the equip confirmations: the use request
// equips the item the gear planner picked (the server flip).
type dressingGame struct {
	*fakeGame
	bot     *state.Bot
	itemIDs map[int32]int32
}

func (d *dressingGame) UseItem(objectID int32) error {
	if err := d.fakeGame.UseItem(objectID); err != nil {
		return err
	}
	itemID, ok := d.itemIDs[objectID]
	if !ok {
		return nil
	}
	items := d.bot.InventoryItems()
	for i := range items {
		if items[i].ObjectID == objectID {
			d.bot.ApplyInventoryUpdate([]state.InventoryItem{{
				ObjectID: objectID, ItemID: itemID, Count: 1,
				Equipped: true, Change: 2,
			}})

			break
		}
	}

	return nil
}

// TestEquipBaggedGear pins the dress-up: the injected bag holds the
// paperdoll pieces, the loop equips them one by one until the gear
// simulation reports no further upgrade.
func TestEquipBaggedGear(t *testing.T) {
	bot := newTestBot()
	itemIDs := map[int32]int32{
		551: 68, // Falchion
		552: 24, // Bone Breastplate
		553: 31, // Bone Gaiters
	}
	var bag []state.InventoryItem
	for objectID, itemID := range itemIDs {
		bag = append(bag, state.InventoryItem{
			ObjectID: objectID, ItemID: itemID, Count: 1,
		})
	}
	bot.ApplyItemList(bag)

	game := &dressingGame{
		fakeGame: &fakeGame{}, bot: bot, itemIDs: itemIDs,
	}
	loop := NewLoop(game, bot)

	require.NoError(t, loop.EquipBaggedGear())
	require.Len(t, game.uses, len(itemIDs),
		"every wearable piece must equip")
	dressed := 0
	for _, item := range bot.InventoryItems() {
		if item.Equipped {
			dressed++
		}
	}
	require.Equal(t, len(itemIDs), dressed,
		"the paperdoll must wear every piece")
}

// TestQuestSegmentTarget pins the segment geometry: the goal inside
// the segment length stays the goal, a farther goal cuts the first
// questSegmentLen along the straight line.
func TestQuestSegmentTarget(t *testing.T) {
	nearX, nearY := questSegmentTarget(45000, 50000, 45600, 50600)
	require.Equal(t, int32(45600), nearX)
	require.Equal(t, int32(50600), nearY)

	farX, farY := questSegmentTarget(45000, 50000, 85000, 50000)
	require.InDelta(t, 53000, farX, 1,
		"the segment target sits one segment east")
	require.Equal(t, int32(50000), farY)
}

// TestKnightChainTransferData pins the gatekeeper legs of the knight
// chain: the Bella hop out of Gludio, the Richlin hop back.
func TestKnightChainTransferData(t *testing.T) {
	chain := ElvenKnightChain()
	require.Len(t, chain.Stages, 6)

	cond3, ok := QuestStageByCond(chain, 3)
	require.True(t, ok)
	require.NotNil(t, cond3.Transfer,
		"the Kluto talk stage rides the Gludin transfer")
	require.Equal(t, "The Village of Gludin", cond3.Transfer.DestLabel)
	require.Equal(t, int32(7256), cond3.Transfer.Gatekeeper.TemplateID)

	cond6, ok := QuestStageByCond(chain, 6)
	require.True(t, ok)
	require.NotNil(t, cond6.Transfer,
		"the closing Sorius talk rides the Gludio transfer")
	require.Equal(t, "The Town of Gludio", cond6.Transfer.DestLabel)
	require.Equal(t, int32(7320), cond6.Transfer.Gatekeeper.TemplateID)

	// The walk-only stages carry no transfer.
	cond1, ok := QuestStageByCond(chain, 1)
	require.True(t, ok)
	require.Nil(t, cond1.Transfer)
	cond4, ok := QuestStageByCond(chain, 4)
	require.True(t, ok)
	require.Nil(t, cond4.Transfer)
}

// TestWalkQuestRouteReplansStuckSegment pins the stuck recovery: a
// walled waypoint chain that never moves the character re-plans
// through the navigator instead of hanging until the timeout.
func TestWalkQuestRouteReplansStuckSegment(t *testing.T) {
	original := questStuckWait
	questStuckWait = 100 * time.Millisecond
	t.Cleanup(func() { questStuckWait = original })

	bot := newTestBot()
	// The plain fake game: the walk requests never move the
	// character (the walled corridor of the live world).
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true})

	err := loop.walkToQuestPoint(65000, 50000, -3500, 600*time.Millisecond)
	require.Error(t, err,
		"a walk that never moves must fail at the deadline")
	require.Contains(t, err.Error(), "did not arrive")
	require.NotEmpty(t, game.walks,
		"the walk kept sending its move requests")
}

// TestQuestTransferData pins the arrival squares against the live
// teleporter xml values (the Gludin and the Gludio squares).
func TestQuestTransferData(t *testing.T) {
	out := TransferGludioToGludin()
	require.Equal(t, "Bella", out.Gatekeeper.Name)
	require.Equal(t, "The Village of Gludin", out.DestLabel)
	require.Equal(t, int32(-80826), out.ArriveX)
	require.Equal(t, int32(149775), out.ArriveY)

	back := TransferGludinToGludio()
	require.Equal(t, "Richlin", back.Gatekeeper.Name)
	require.Equal(t, "The Town of Gludio", back.DestLabel)
	require.Equal(t, int32(-12694), back.ArriveX)
	require.Equal(t, int32(122776), back.ArriveY)
}
