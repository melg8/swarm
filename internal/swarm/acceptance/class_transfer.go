// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/melg8/swarm/internal/swarm/connection"
    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The class transfer scenario (T-016, the M2 acceptance vehicle): a
// DB-injected level 20 elven fighter at Gludio drives the Q00406
// chain of hunt/quest_chains.go end to end through the quest trip
// engine - the accept, the skeleton hunt of the Ruins of Agony, the
// Bella teleport to Gludin, the Ol Mahum hunt, the Kluto talks, the
// Richlin teleport back, the closing Sorius talk - then walks to the
// grand master Rains and drives the class change route. The pass
// line of M2: the tracker reports SelfClassID 19 (Elven Knight) and
// the character selection packet of a fresh login agrees.
const (
    classTransferScenarioID = "class-transfer"
    classTransferAccount    = "temp9"
    classTransferPassword   = "temp9"
    // classTransferTimeout bounds the whole run: the kill stages walk
    // 37 000 unit segments twice each, farm 20 pieces at a 50-70 percent
    // drop rate, two gatekeeper teleports bridge the towns and the
    // closing chain talks pace their bypasses - an hour and a half
    // holds the honest run with the re-plans inside.
    classTransferTimeout = 90 * time.Minute
    // classTransferLevel is the injected level: 20 passes the
    // Q00406 start gate (the 05 event checks level < 19) and the
    // ElfHumanFighterChange1 class change gate (level < 20).
    classTransferLevel = 20
    // classTransferClassID is the resulting first profession of the
    // chain under test: 19, the Elven Knight.
    classTransferClassID = 19
    // classTransferReloginWait is the pause between the session
    // teardown and the fresh login of the char list verification:
    // the server stores the character of a closed socket within a
    // moment, the bound covers the flush.
    classTransferReloginWait = 5 * time.Second
)

// The Gludio injection geometry: the character wakes on the
// approach ring south of Master Sorius (GludioNPCs.xml: -13440
// 122643 -3103), 150 units off his cell - inside the server
// interaction distance without standing on the trainer hall
// interior cell itself (the roof rule of the teacher approach).
const (
    soriusApproachX = -13440
    soriusApproachY = 122493
    soriusApproachZ = -3103
)

// questNpcTemplateOffset mirrors the NpcInfo template id space of
// the world tracker: the hunt chain data carries the npcdata
// display ids, the tracker stores display id + 1000000.
const questNpcTemplateOffset = 1000000

// The condition ids of the staged checks.
const (
    checkSoriusFound     = "sorius-found"
    checkQuestAccepted   = "quest-accepted"
    checkTopazCleared    = "topaz-cleared"
    checkLetterStage     = "letter-stage"
    checkKlutoMemo       = "kluto-memo"
    checkEmeraldsCleared = "emeralds-cleared"
    checkKlutoBox        = "kluto-box"
    checkBrooch          = "brooch"
    checkClassChanged    = "class-changed"
    checkCharListClass   = "char-list-class"
)

// questNpcFindWait bounds the wait for the quest npc to appear in
// the world store (the spawn packets of the world entry flood in
// within the first seconds).
const questNpcFindWait = 30 * time.Second

// classTransferItems is the injected starter kit of the kill stages:
// the Falchion sword, the Kite shield, the Bone armor set and the
// Anguish jewels - the dress the Dion shops of the 20-25 band sell
// to a character of this level (docs/shopping_strategy.md), with
// the healing potions of the transit fights (the C1 Lesser Healing
// Potion is a slow heal over time with a ten second reuse, so the
// stock carries enough rounds for the whole hunt).
var classTransferItems = []ResetItem{
    {ItemID: 68, Count: 1},    // Falchion
    {ItemID: 629, Count: 1},   // Kite Shield
    {ItemID: 24, Count: 1},    // Bone Breastplate
    {ItemID: 31, Count: 1},    // Bone Gaiters
    {ItemID: 45, Count: 1},    // Bone Helmet
    {ItemID: 49, Count: 1},    // Gloves
    {ItemID: 1123, Count: 1},  // Blue Buckskin Boots
    {ItemID: 876, Count: 2},   // Ring of Anguish
    {ItemID: 873, Count: 2},   // Earring of Aid
    {ItemID: 907, Count: 1},   // Necklace of Anguish
    {ItemID: 1060, Count: 30}, // Lesser Healing Potion
}

// classTransferReset builds the start state of the class transfer
// scenario: the level 20 elven fighter with the milestone wallet
// (the two gatekeeper teleports cost 14 600 adena of it) and the
// D-grade dress in the bag, standing on Sorius's approach ring.
func classTransferReset() characterReset {
    gain := elvenFighterVitals[classTransferLevel-1]

    return characterReset{
        Account: classTransferAccount,
        Char:    classTransferAccount,
        Level:   classTransferLevel,
        Exp: soakExperienceTable[classTransferLevel] -
            milestoneXPTail(classTransferLevel),
        SP:    milestoneSP,
        Adena: milestoneWallet,
        Items: classTransferItems,
        X:     soriusApproachX,
        Y:     soriusApproachY,
        Z:     soriusApproachZ,
        MaxHP: gain.HP,
        MaxMP: gain.MP,
        MaxCP: gain.CP,
    }
}

// classTransferChecks is the staged condition list of the scenario.
func classTransferChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: "entered the world at Gludio",
            Done: false, Detail: "",
        },
        {
            ID: checkSoriusFound, Label: "found Master Sorius in the world",
            Done: false, Detail: "",
        },
        {
            ID:    checkQuestAccepted,
            Label: "the quest journal reports Q00406 cond 1",
            Done:  false, Detail: "",
        },
        {
            ID: checkTopazCleared,
            Label: "the Ruins of Agony hunt filled the 20 topaz pieces " +
                "(cond 2)",
            Done: false, Detail: "",
        },
        {
            ID: checkLetterStage,
            Label: "Sorius' Letter stage passed (cond 3, the Bella " +
                "teleport to Gludin)",
            Done: false, Detail: "",
        },
        {
            ID: checkKlutoMemo, Label: "Kluto's Memo stage passed (cond 4)",
            Done: false, Detail: "",
        },
        {
            ID: checkEmeraldsCleared,
            Label: "the Ol Mahum hunt filled the 20 emerald pieces " +
                "(cond 5)",
            Done: false, Detail: "",
        },
        {
            ID: checkKlutoBox,
            Label: "Kluto's Box stage passed (cond 6, the Richlin " +
                "teleport back to Gludio)",
            Done: false, Detail: "",
        },
        {
            ID:    checkBrooch,
            Label: "the Elven Knight Brooch landed (the quest exit)",
            Done:  false, Detail: "",
        },
        {
            ID:    checkClassChanged,
            Label: "the tracker reports SelfClassID 19 (Elven Knight)",
            Done:  false, Detail: "",
        },
        {
            ID: checkCharListClass,
            Label: "the character selection packet of a fresh login " +
                "reports the Elven Knight class",
            Done: false, Detail: "",
        },
    }
}

// classTransferScenario runs the full M2 acceptance: the injection,
// the world entry on the Sorius approach ring, the dress-up from the
// bag, the whole Q00406 chain through the quest trip engine (the
// kill stages with the segment walks and the rests, the two
// gatekeeper town hops), the Rains class change route and the double
// class id verification (the live tracker and the character
// selection packet of a fresh login).
func classTransferScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(classTransferChecks())
    test.appendLog("acceptance: the class transfer run is the full " +
        "Q00406 chain plus the Rains class change")

    if err := m.ensureCharacter(classTransferAccount,
        classTransferPassword, classTransferAccount,
        test.appendLog); err != nil {
        writeClassTransferFailMetrics(test, err.Error())

        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(classTransferReset(), test); err != nil {
        writeClassTransferFailMetrics(test, err.Error())

        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    tracker := m.tracker(test)
    session, err := startClassTransferSession(ctx, m, tracker,
        test.appendLog)
    if err != nil {
        writeClassTransferFailMetrics(test, err.Error())

        return fmt.Errorf("start session: %w", err)
    }
    defer session.cancel()

    if err := waitOnline(ctx, tracker, test); err != nil {
        writeClassTransferFailMetrics(test, err.Error())
        session.cancel()
        <-session.done

        return fmt.Errorf("world entry: %w", err)
    }

    return classTransferDrive(ctx, m, test, tracker, session)
}

// classTransferDrive runs the full quest segment of the scenario: the
// dress-up, the chain drive of the quest trip engine (the journal
// cond ladder with the kill stages, the rests and the two town
// transfers), the Rains class change and the double verification,
// ending with the graceful teardown and the metrics row.
func classTransferDrive(
    ctx context.Context, m *Manager, test *Test, tracker *state.Bot,
    session *classTransferSession,
) error {
    chain := hunt.ElvenKnightChain()
    start := time.Now()

    // The manual hunt loop of the session (no autonomy - the elven
    // zone machinery would drag the character home) with the
    // geodata navigator of the manager behind the segment walks.
    loop := hunt.NewLoop(session.game, tracker)
    loop.SetAutonomy(false)
    loop.SetNavigator(hunt.NewNavigator(m.engine))
    loop.SetLogger(sessionLogger(tracker, test.appendLog))

    // The dress-up: the injected bag holds the D-grade set, the
    // kill stages stand on it.
    if err := loop.EquipBaggedGear(); err != nil {
        return classTransferAbort(test, tracker, session, start,
            "the dress-up: "+err.Error())
    }
    test.appendLog("acceptance: the character dressed from the bag")

    sorius, err := awaitQuestNpc(ctx, tracker, chain.Start,
        questNpcFindWait, test)
    if err != nil {
        return classTransferAbort(test, tracker, session, start,
            "find Sorius: "+err.Error())
    }
    test.updateCheck(checkSoriusFound, true, fmt.Sprintf(
        "object %d at (%d, %d, %d)", sorius.ObjectID, sorius.X,
        sorius.Y, sorius.Z))

    // The chain drive runs the accept, the kill stages, the talks
    // and the two gatekeeper transfers; the per-cond observer below
    // flips the stage checks as the journal advances.
    go observeClassTransferCond(ctx, tracker, test)

    if err := loop.DriveQuestChain(ctx, chain); err != nil {
        return classTransferAbort(test, tracker, session, start,
            "the chain drive: "+err.Error())
    }
    test.updateCheck(checkBrooch, true,
        "the journal dropped the quest, the brooch survived")
    test.appendLog("acceptance: the quest chain completed, the " +
        "brooch is in the bag")

    // The class change at the grand master Rains.
    change := hunt.ElvenKnightClassChange()
    test.appendLog(fmt.Sprintf(
        "acceptance: walking to Grand Master %s for the class change",
        change.Npc.Name))
    rains, err := awaitQuestNpc(ctx, tracker, change.Npc,
        questNpcFindWait, test)
    if err != nil {
        return classTransferAbort(test, tracker, session, start,
            "find Rains: "+err.Error())
    }
    if err := driveClassChange(loop, rains, change); err != nil {
        return classTransferAbort(test, tracker, session, start,
            "the class change: "+err.Error())
    }

    // The live gate: the UserInfo broadcast flips the tracker.
    if err := awaitSelfClassID(ctx, tracker, classTransferClassID,
        questCondWait*3, test); err != nil {
        return classTransferAbort(test, tracker, session, start,
            "the SelfClassID gate: "+err.Error())
    }
    test.updateCheck(checkClassChanged, true, fmt.Sprintf(
        "SelfClassID %d", classTransferClassID))
    test.appendLog("acceptance: the tracker reports the Elven " +
        "Knight class, closing the session for the relogin check")

    return classTransferReloginVerify(ctx, m, test, tracker, session,
        start)
}

// driveClassChange walks to the grand master and drives the change
// route of the dialog walker (the static villagemaster page entry,
// the three links of the change segment).
func driveClassChange(
    loop *hunt.Loop, rains state.AttackTarget, change hunt.ClassChange,
) error {
    if err := loop.WalkQuestStation(
        rains.X, rains.Y, rains.Z); err != nil {
        return fmt.Errorf("the walk to %s: %w", change.Npc.Name, err)
    }

    return loop.DriveDialog(rains.ObjectID, change.Route)
}

// observeClassTransferCond watches the quest journal of the tracker
// and flips the stage checks as the cond ladder advances (the
// observer runs until the scenario context dies).
func observeClassTransferCond(
    ctx context.Context, tracker *state.Bot, test *Test,
) {
    stages := map[int32]string{
        1: checkQuestAccepted,
        2: checkTopazCleared,
        3: checkLetterStage,
        4: checkKlutoMemo,
        5: checkEmeraldsCleared,
        6: checkKlutoBox,
    }
    for {
        if ctx.Err() != nil {
            return
        }
        cond, ok := tracker.QuestCond(406)
        if ok {
            if id, known := stages[cond]; known {
                test.updateCheck(id, true,
                    fmt.Sprintf("journal cond %d", cond))
            }
        }
        time.Sleep(time.Second)
    }
}

// classTransferReloginVerify closes the session gracefully, logs in
// again and reads the class id of the character selection packet:
// the persisted class change of the server (the M2 criterion).
func classTransferReloginVerify(
    _ context.Context, m *Manager, test *Test, tracker *state.Bot,
    session *classTransferSession, start time.Time,
) error {
    session.cancel()
    sessionErr := <-session.done
    if sessionErr != nil {
        writeClassTransferOutcomeMetrics(test, tracker, true,
            time.Since(start), "session end: "+sessionErr.Error())

        return fmt.Errorf("session end: %w", sessionErr)
    }
    test.appendLog("acceptance: the bot left the world gracefully, " +
        "verifying the persisted class through a fresh login")
    time.Sleep(classTransferReloginWait)

    classID, err := verifyCharListClassID(m)
    if err != nil {
        writeClassTransferOutcomeMetrics(test, tracker, true,
            time.Since(start), "the relogin check: "+err.Error())

        return fmt.Errorf("the relogin check: %w", err)
    }
    if classID != classTransferClassID {
        writeClassTransferOutcomeMetrics(test, tracker, true,
            time.Since(start), fmt.Sprintf(
                "the char list class id %d", classID))

        return fmt.Errorf(
            "the char list reports class %d, want %d", classID,
            classTransferClassID)
    }
    test.updateCheck(checkCharListClass, true, fmt.Sprintf(
        "BaseClassID %d", classID))
    test.appendLog("acceptance: the character selection packet " +
        "reports the Elven Knight class - the M2 pass line")

    return classTransferPass(test, tracker, start)
}

// verifyCharListClassID logs in fresh and returns the base class id
// the character selection packet carries for the scenario character.
func verifyCharListClassID(m *Manager) (int32, error) {
    auth, game, err := m.openGame(classTransferAccount,
        classTransferPassword, m.proxy)
    if err != nil {
        return 0, err
    }
    defer func() { _ = game.Close() }()

    charList, err := game.Authenticate(gameSessionParams(auth))
    if err != nil {
        return 0, fmt.Errorf("the fresh authentication: %w", err)
    }
    _, info, found := charList.FindCharacterByName(classTransferAccount)
    if !found {
        return 0, errors.New("the character is missing from the list")
    }

    return info.BaseClassID, nil
}

// classTransferAbort tears a failed drive down and writes the FAIL
// metrics row of the stage.
func classTransferAbort(
    test *Test, tracker *state.Bot, session *classTransferSession,
    start time.Time, reason string,
) error {
    writeClassTransferOutcomeMetrics(test, tracker, true,
        time.Since(start), reason)
    session.cancel()
    <-session.done

    return errors.New(reason)
}

// classTransferPass writes the PASS metrics row of the full run (the
// session itself is already down).
func classTransferPass(
    test *Test, tracker *state.Bot, start time.Time,
) error {
    test.appendLog("acceptance: the class transfer scenario PASSED")
    writeClassTransferOutcomeMetrics(test, tracker, false,
        time.Since(start), "")

    return nil
}

// questCondWait bounds the journal flip watch (the QuestList push
// follows the event immediately; the bound covers a slow tick).
const questCondWait = 10 * time.Second

// awaitSelfClassID waits until the tracker reports the wanted class
// id of the played character (the UserInfo broadcast of the change).
func awaitSelfClassID(
    ctx context.Context, tracker *state.Bot, want int32,
    wait time.Duration, test *Test,
) error {
    deadline := time.Now().Add(wait)
    for {
        if tracker.SelfClassID() == want {
            return nil
        }
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        if time.Now().After(deadline) {
            return fmt.Errorf(
                "the class id stayed %d, want %d", tracker.SelfClassID(),
                want)
        }
        test.appendLog(fmt.Sprintf(
            "acceptance: waiting for the class id flip to %d", want))
        select {
        case <-ctx.Done():
            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(time.Second):
        }
    }
}

// awaitQuestNpc waits for the quest npc to appear in the world
// store (the spawn packets of the world entry).
func awaitQuestNpc(
    ctx context.Context, tracker *state.Bot, npc hunt.QuestNpc,
    wait time.Duration, test *Test,
) (state.AttackTarget, error) {
    templates := []int32{npc.TemplateID + questNpcTemplateOffset}
    deadline := time.Now().Add(wait)
    for {
        found, ok := tracker.NearestNpcByTemplates(templates, 6000)
        if ok {
            return found, nil
        }
        if ctx.Err() != nil {
            return state.AttackTarget{}, fmt.Errorf(
                "cancelled: %w", ctx.Err())
        }
        if time.Now().After(deadline) {
            return state.AttackTarget{}, fmt.Errorf(
                "the npc %s (template %d) never appeared",
                npc.Name, npc.TemplateID)
        }
        test.appendLog(fmt.Sprintf(
            "acceptance: waiting for %s to appear in the world",
            npc.Name))
        select {
        case <-ctx.Done():
            return state.AttackTarget{}, fmt.Errorf(
                "cancelled: %w", ctx.Err())
        case <-time.After(200 * time.Millisecond):
        }
    }
}

// classTransferSession is the manual session of the scenario: the
// game client handle the dialog walker drives, the cancel that
// ends the receive loop and the done channel of its exit.
type classTransferSession struct {
    game   *connection.GameClient
    cancel context.CancelFunc
    done   chan error
}

// startClassTransferSession opens the game session of the scenario
// (the world entry under the scenario context) without the hunt
// autonomy: the scenario itself drives the dialog walker through
// the game handle.
func startClassTransferSession(
    ctx context.Context, m *Manager, tracker *state.Bot,
    logLine func(string),
) (*classTransferSession, error) {
    auth, game, err := m.openGame(classTransferAccount,
        classTransferPassword, m.proxy)
    if err != nil {
        return nil, err
    }
    game.SetTracker(tracker)

    charList, err := game.Authenticate(gameSessionParams(auth))
    if err != nil {
        _ = game.Close()

        return nil, fmt.Errorf("game authentication: %w", err)
    }
    slot, _, found := charList.FindCharacterByName(classTransferAccount)
    if !found {
        _ = game.Close()

        return nil, errors.New("the injected character is missing")
    }
    if err := game.EnterWorld(int32(slot)); err != nil {
        _ = game.Close()

        return nil, fmt.Errorf("enter world: %w", err)
    }
    logLine("acceptance: the class transfer session entered the world")

    sessionCtx, cancel := context.WithCancel(ctx)
    done := make(chan error, 1)
    go func() {
        done <- game.Run(sessionCtx, classTransferAccount)
    }()

    return &classTransferSession{game: game, cancel: cancel, done: done}, nil
}

// writeClassTransferOutcomeMetrics appends the staged metrics row
// of the scenario (the scenario id carries the stage context, the
// quest gate of the stage rides the fail reason).
func writeClassTransferOutcomeMetrics(
    test *Test, tracker *state.Bot, failed bool, elapsed time.Duration,
    reason string,
) {
    status := statusPass
    if failed {
        status = statusFail
    }
    writeSoakMetrics(test, soakMetrics{
        Date:        nowUTC(),
        Scenario:    classTransferScenarioID,
        DurationSec: int64(elapsed / time.Second),
        StartLevel:  classTransferLevel,
        EndLevel:    tracker.SelfLevel(),
        XpPerHour:   0,
        Deaths:      0,
        Adena:       soakAdena(tracker),
        StuckEvents: 0,
        Status:      status,
        FailReason:  reason,
    })
}

// writeClassTransferFailMetrics appends the fail row of a setup
// failure (the numeric fields zero).
func writeClassTransferFailMetrics(test *Test, reason string) {
    writeSoakMetrics(test, soakMetrics{
        Date:        nowUTC(),
        Scenario:    classTransferScenarioID,
        DurationSec: 0,
        StartLevel:  0,
        EndLevel:    0,
        XpPerHour:   0,
        Deaths:      0,
        Adena:       0,
        StuckEvents: 0,
        Status:      statusFail,
        FailReason:  "setup: " + reason,
    })
}
