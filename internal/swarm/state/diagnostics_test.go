// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
        "testing"
        "time"

        "github.com/stretchr/testify/require"
)

// TestAgeMs pins the flooring and clamping of the diagnostics ages.
func TestAgeMs(t *testing.T) {
        require.Equal(t, int64(0), AgeMs(0))
        require.Equal(t, int64(0), AgeMs(999*time.Millisecond))
        require.Equal(t, int64(1000), AgeMs(time.Second))
        require.Equal(t, int64(1000), AgeMs(1999*time.Millisecond))
        require.Equal(t, int64(90000), AgeMs(90*time.Second+700*time.Millisecond))
        require.Equal(t, int64(0), AgeMs(-time.Second))
}

// TestPacketRateWindow pins the per second counting, the window
// shift, the dilution guard of the filled divisor and the clock step
// restart.
func TestPacketRateWindow(t *testing.T) {
        var window packetRateWindow

        // A fresh window divides by the seconds it really saw.
        window.note(1000)
        window.note(1000)
        window.note(1000)
        require.InDelta(t, 3.0, window.rate(), 1e-9)

        // The next second shifts the counts back.
        window.note(1001)
        require.InDelta(t, 2.0, window.rate(), 1e-9)

        // A quiet stretch ages the counts out, the divisor stays.
        window.note(1010)
        require.InDelta(t, 0.2, window.rate(), 1e-9)

        // A gap past the width restarts the window.
        window.note(1030)
        window.note(1030)
        require.InDelta(t, 2.0, window.rate(), 1e-9)

        // A clock step backwards restarts the window too.
        window.note(1005)
        require.InDelta(t, 1.0, window.rate(), 1e-9)

        // An untouched window reports zero.
        var empty packetRateWindow
        require.InDelta(t, 0.0, empty.rate(), 1e-9)
}

// TestCountPacketFeedsRate verifies the packet rate of the
// diagnostics grows with the counted packets of one second.
func TestCountPacketFeedsRate(t *testing.T) {
        bot := NewBot("acc1")
        snapshot := bot.Snapshot()
        require.InDelta(t, 0.0,
                snapshot.Diagnostics.PacketsPerSecond, 1e-9)

        for range 5 {
                bot.CountPacket()
        }
        snapshot = bot.Snapshot()
        require.InDelta(t, 5.0, snapshot.Diagnostics.PacketsPerSecond, 1e-9)
        require.Equal(t, int64(5), snapshot.Packets)
}

// TestDiagnosticsPhaseAge pins the phase age: SetPhase stamps the
// transition, a backdated stamp shows the floored age, an unset
// phase reports zero.
func TestDiagnosticsPhaseAge(t *testing.T) {
        bot := NewBot("acc1")
        bot.SetPhase("engage")
        require.Equal(t, int64(0), bot.Snapshot().Diagnostics.PhaseForMs)

        bot.mu.Lock()
        phaseAge := 72*time.Second + 900*time.Millisecond
        bot.phaseAt = time.Now().Add(-phaseAge)
        bot.mu.Unlock()

        require.Equal(t, int64(72000),
                bot.Snapshot().Diagnostics.PhaseForMs)
}

// TestDiagnosticsUpdateAge pins the update age: a fresh bot falls
// back to the session start, a backdated update shows the floored
// age.
func TestDiagnosticsUpdateAge(t *testing.T) {
        bot := NewBot("acc1")
        // Nothing changed yet: the age falls back to the session
        // start, which is now.
        require.Equal(t, int64(0), bot.Snapshot().Diagnostics.UpdatedAgoMs)

        bot.mu.Lock()
        bot.updated = time.Now().Add(-(3*time.Second + 100*time.Millisecond))
        bot.mu.Unlock()

        require.Equal(t, int64(3000),
                bot.Snapshot().Diagnostics.UpdatedAgoMs)
}

// TestDiagnosticsLoginCooldown pins the remaining cooldown of the
// emergency logout: armed shows the floored remainder, lapsed and
// unarmed show zero.
func TestDiagnosticsLoginCooldown(t *testing.T) {
        bot := NewBot("acc1")
        require.Equal(t, int64(0),
                bot.Snapshot().Diagnostics.LoginCooldownMs)

        bot.mu.Lock()
        bot.loginCooldownUntil = time.Now().Add(90900 * time.Millisecond)
        bot.mu.Unlock()

        require.Equal(t, int64(90000),
                bot.Snapshot().Diagnostics.LoginCooldownMs)

        bot.mu.Lock()
        bot.loginCooldownUntil = time.Now().Add(-time.Second)
        bot.mu.Unlock()

        require.Equal(t, int64(0),
                bot.Snapshot().Diagnostics.LoginCooldownMs)
}

// TestDiagnosticsWalkFreshness pins the stale moving flag signature:
// a moving character without a fresh movement broadcast reports a
// stale walk with the broadcast age.
func TestDiagnosticsWalkFreshness(t *testing.T) {
        bot := NewBot("acc1")
        bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 100, 50)
        bot.ApplyMovement(Movement{
                ObjectID: 100,
                X:        45000, Y: 50000, Z: -3500,
                DestX: 46000, DestY: 51000, DestZ: -3500,
        })

        snapshot := bot.Snapshot()
        require.True(t, snapshot.Character.Moving)
        require.True(t, snapshot.Diagnostics.WalkFresh)
        require.Equal(t, int64(0), snapshot.Diagnostics.MoveAgoMs)

        bot.mu.Lock()
        bot.char.MoveAt = time.Now().Add(-(8*time.Second + 100*time.Millisecond))
        bot.mu.Unlock()

        snapshot = bot.Snapshot()
        // The flag still claims motion while the freshness gate
        // shows the stall: the lost stop packet signature.
        require.True(t, snapshot.Character.Moving)
        require.False(t, snapshot.Diagnostics.WalkFresh)
        require.Equal(t, int64(8000), snapshot.Diagnostics.MoveAgoMs)
}

// TestDiagnosticsCombatNuance pins the combat fields: a live fight
// shows the auto attack flag, the fighting target, a fresh activity
// age and the hit age of an incoming blow.
func TestDiagnosticsCombatNuance(t *testing.T) {
        bot := NewBot("acc1")
        bot.SetCharacter("test1", 268473919, 18, 45000, 50000, -3500, 100, 50)
        snapshot := bot.Snapshot()
        require.False(t, snapshot.Diagnostics.AutoAttacking)
        require.Equal(t, int32(0), snapshot.Diagnostics.FightingTargetID)
        require.Equal(t, int64(0), snapshot.Diagnostics.CombatActiveAgoMs)
        require.Equal(t, int64(0), snapshot.Diagnostics.LastHitAgoMs)
        require.False(t, snapshot.Diagnostics.UnderAttack)

        bot.ApplyAttack(Attack{
                AttackerID:  268473919,
                TargetIDs:   [AttackTargets]int32{1_000_005},
                TargetCount: 1,
                X:           45000, Y: 50000,
                TargetX: 45100, TargetY: 50100,
        })
        // The played character swung: the fight tracks the target.
        snapshot = bot.Snapshot()
        require.Equal(t, int32(1_000_005),
                snapshot.Diagnostics.FightingTargetID)
        require.Equal(t, int64(0), snapshot.Diagnostics.CombatActiveAgoMs)

        // A mob swings at the character: the hit lands, the fight
        // stays one sided (no fighting target of the character).
        bot.ApplyNpcInfo(NpcInfo{
                ObjectID:   1_000_005,
                TemplateID: 1000003,
                Attackable: true,
                X:          45100, Y: 50100, Z: -3500,
                RunSpeed: 165, WalkSpeed: 55,
        })
        bot.ApplyAttack(Attack{
                AttackerID:  1_000_005,
                TargetIDs:   [AttackTargets]int32{268473919},
                TargetCount: 1,
                X:           45100, Y: 50100,
                TargetX: 45000, TargetY: 50000,
        })

        snapshot = bot.Snapshot()
        require.Equal(t, int32(1_000_005),
                snapshot.Diagnostics.FightingTargetID)
        require.True(t, snapshot.Diagnostics.UnderAttack)
        require.Equal(t, int64(0), snapshot.Diagnostics.LastHitAgoMs)
        require.Equal(t, 1, snapshot.Diagnostics.AttackerCount)
}

// TestDiagnosticsObjectCounts pins the known list summary and the
// attacker tally of the live snapshot fixture: 40 npcs (one dead),
// one ground item and an attackable npc holding the character.
func TestDiagnosticsObjectCounts(t *testing.T) {
        bot := liveSnapshotBot(t)

        bot.ApplyNpcInfo(NpcInfo{
                ObjectID:   2_000_100,
                TemplateID: 1000003,
                Attackable: true,
                X:          45100, Y: 50100, Z: -3500,
                RunSpeed: 165, WalkSpeed: 55,
        })
        // The attacker marks the npc as fighting the character
        // (the chase or swing of the observed world).
        bot.ApplyAttack(Attack{
                AttackerID:  2_000_100,
                TargetIDs:   [AttackTargets]int32{268473919},
                TargetCount: 1,
                X:           45100, Y: 50100,
                TargetX: 45000, TargetY: 50000,
        })

        snapshot := bot.Snapshot()
        counts := snapshot.Diagnostics.Objects
        require.Equal(t, 41, counts.NPCs)
        require.Equal(t, 0, counts.Players)
        require.Equal(t, 1, counts.Items)
        require.Equal(t, 1, counts.Dead)
        require.Len(t, snapshot.Objects, 42)
        // Two attackable npcs hold the character: the fixture swing
        // of 1_000_005 and the added 2_000_100.
        require.Equal(t, 2, snapshot.Diagnostics.AttackerCount)
}

// TestDiagnosticsHuntPublication pins the hunt subview: the
// published struct rides verbatim, the tracker adds the last action
// of NoteAction and the tick age of the publication.
func TestDiagnosticsHuntPublication(t *testing.T) {
        bot := NewBot("acc1")
        snapshot := bot.Snapshot()
        require.Equal(t, HuntDiagnostics{}, snapshot.Diagnostics.Hunt)

        published := HuntDiagnostics{
                TargetID:       1234,
                TargetForMs:    25000,
                SkippedTargets: 3,
                RePaths:        1,
                WaypointsLeft:  12,
                TripForMs:      300000,
        }
        bot.SetHuntDiagnostics(published)
        bot.NoteAction("Hunt: target 1234 does not engage, switching")

        snapshot = bot.Snapshot()
        hunt := snapshot.Diagnostics.Hunt
        require.Equal(t, int32(1234), hunt.TargetID)
        require.Equal(t, int64(25000), hunt.TargetForMs)
        require.Equal(t, 3, hunt.SkippedTargets)
        require.Equal(t, 1, hunt.RePaths)
        require.Equal(t, 12, hunt.WaypointsLeft)
        require.Equal(t, int64(300000), hunt.TripForMs)
        require.Equal(t, int64(0), hunt.TickAgoMs)
        require.Equal(t, "Hunt: target 1234 does not engage, switching",
                hunt.LastAction)
        require.Equal(t, int64(0), hunt.LastActionAgoMs)

        // The note also lands in the rolling event log the dump
        // carries.
        found := false
        for _, event := range snapshot.Events {
                if event.Message ==
                        "Hunt: target 1234 does not engage, switching" {
                        found = true
                }
        }
        require.True(t, found, "the note must land in the event log")

        // A stale publication shows the loop heartbeat age.
        bot.mu.Lock()
        bot.huntPublishedAt = time.Now().Add(-(5*time.Second + 100*time.Millisecond))
        bot.huntLastActionAt = time.Now().Add(-(4 * time.Second))
        bot.mu.Unlock()

        hunt = bot.Snapshot().Diagnostics.Hunt
        require.Equal(t, int64(5000), hunt.TickAgoMs)
        require.Equal(t, int64(4000), hunt.LastActionAgoMs)
}

// TestDiagnosticsResetSession pins the session reset: the hunt
// internals and the phase age reset with the world.
func TestDiagnosticsResetSession(t *testing.T) {
        bot := NewBot("acc1")
        bot.SetPhase("engage")
        bot.SetHuntDiagnostics(HuntDiagnostics{TargetID: 1234})
        bot.NoteAction("Hunt: walking")

        bot.ResetSession()

        snapshot := bot.Snapshot()
        require.Empty(t, snapshot.Phase)
        require.Equal(t, int64(0), snapshot.Diagnostics.PhaseForMs)
        require.Equal(t, int32(0), snapshot.Diagnostics.Hunt.TargetID)
        require.Empty(t, snapshot.Diagnostics.Hunt.LastAction)
        require.Equal(t, int64(0), snapshot.Diagnostics.Hunt.TickAgoMs)
}

// TestDiagnosticsVerbatimCopy pins that the snapshot copy carries
// the diagnostics of the live view unchanged.
func TestDiagnosticsVerbatimCopy(t *testing.T) {
        bot := NewBot("acc1")
        bot.SetPhase("engage")
        bot.SetHuntDiagnostics(HuntDiagnostics{
                TargetID:    1234,
                TargetForMs: 25000,
                RePaths:     2,
        })
        bot.NoteAction("Hunt: resting")

        direct := bot.AppendSnapshotJSON(nil)
        require.Contains(t, string(direct), `"diagnostics":{`)
        require.Contains(t, string(direct), `"lastAction":"Hunt: resting"`)
        require.Contains(t, string(direct), `"targetId":1234`)
}
