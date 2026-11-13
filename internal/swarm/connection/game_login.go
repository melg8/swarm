// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "errors"
    "fmt"
    "os"
    "strconv"
    "time"

    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// Character selection constants. The server drops a CharacterSelect silently
// when it races the creation flow: the updated char list is written before the
// server side selection cache is updated, so a select in that window finds no
// character and is ignored without an answer. The wait for CharSelected is
// therefore bounded and the selection retransmitted; the flood protector allows
// one select per 3 seconds (30 game ticks of 100 ms), which the 5 second wait
// between attempts respects.
const (
    charSelectWait     = 5 * time.Second
    charSelectAttempts = 3
    charCreateOkWait   = 2 * time.Second
    // charListWait bounds one pre-world read outside the bounded selection
    // dance: the character list answer of the AuthLogin exchange and each
    // packet of the creation exchange (see EnsureCharacter). These reads run
    // before the in-world silence watchdog exists, so an unbounded read parks
    // the bot goroutine forever on a half-open server while the supervisor
    // waits for a session that never returns.
    charListWait = 30 * time.Second
)

// Authenticate sends the game AuthLogin packet and waits for the character
// list of the account.
func (gc *GameClient) Authenticate(
    params GameSessionParams,
) (*fromgameserver.CharSelectInfoPacket, error) {
    if err := gc.sendPacket(&togameserver.AuthLogin{
        Login:      params.Account,
        PlayOkID1:  params.PlayOkID1,
        PlayOkID2:  params.PlayOkID2,
        LoginOkID1: params.LoginOkID1,
        LoginOkID2: params.LoginOkID2,
    }); err != nil {
        return nil, fmt.Errorf("failed to send auth login: %w", err)
    }
    gc.logger.Println("Sent game auth login for account " + params.Account)

    // The char list answer bounds like every pre-world read (see charListWait):
    // the silence watchdog of the live session does not exist yet.
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charListWait)); err != nil {
        return nil, fmt.Errorf(
            "failed to set the character list deadline: %w", err)
    }
    payload, err := gc.readPacket(gc.readBuf)
    if err != nil {
        return nil, fmt.Errorf("failed to read character list: %w", err)
    }
    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        return nil, fmt.Errorf(
            "failed to reset the character list deadline: %w", err)
    }
    gc.readBuf = payload
    if len(payload) == 0 {
        return nil, errors.New("empty character list packet")
    }
    if payload[0] != fromgameserver.CharSelectInfoPacketID {
        return nil, fmt.Errorf(
            "unexpected packet id 0x%02x while waiting for characters",
            payload[0])
    }

    charList := fromgameserver.NewCharSelectInfoPacket()
    if err := fromgameserver.ParseCharSelectInfoPacket(
        charList, payload); err != nil {
        return nil, fmt.Errorf("failed to parse character list: %w", err)
    }
    gc.logger.Printf("Received character list with %d characters",
        len(charList.Characters))

    return charList, nil
}

// EnsureCharacter returns the character list that contains the character
// with the requested name, creating the character first when needed.
func (gc *GameClient) EnsureCharacter(
    params CharacterParams,
    charList *fromgameserver.CharSelectInfoPacket,
) (*fromgameserver.CharSelectInfoPacket, error) {
    if _, _, found := charList.FindCharacterByName(params.Name); found {
        return charList, nil
    }

    if err := gc.sendPacket(&togameserver.CharacterCreate{
        Name:      params.Name,
        Race:      params.Race,
        Female:    params.Female,
        ClassID:   params.ClassID,
        INT:       0,
        STR:       0,
        CON:       0,
        MEN:       0,
        DEX:       0,
        WIT:       0,
        HairStyle: params.HairStyle,
        HairColor: params.HairColor,
        Face:      params.Face,
    }); err != nil {
        return nil, fmt.Errorf("failed to send character create: %w", err)
    }
    gc.logger.Println("Sent character create for " + params.Name)

    // The server answers with the creation result and the updated list.
    for {
        done, updated, err := gc.awaitCharacterCreation(params.Name)
        if err != nil {
            return nil, err
        }
        if done {
            // The updated list precedes the trailing CharCreateOk on the wire
            // (the server writes the list from inside the creation handler and
            // the ok after it returns). Draining the ok with a bounded wait
            // guarantees the server side selection cache is populated before
            // the next packet select, closing the silent drop race.
            gc.drainCharCreateOk()

            return updated, nil
        }
    }
}

// drainCharCreateOk consumes the CharCreateOk packet that follows the
// updated character list of a successful creation. The packet always
// follows (the server writes it last), so a bounded wait drains it; a
// lost packet only costs the selection retry of EnterWorld.
func (gc *GameClient) drainCharCreateOk() {
    deadline := time.Now().Add(charCreateOkWait)
    if err := gc.conn.SetReadDeadline(deadline); err != nil {
        gc.logger.Printf("Failed to set the char create ok deadline: %v", err)

        return
    }

    for {
        payload, err := gc.readPacket(gc.readBuf)
        if err != nil {
            gc.readBuf = nil
            if !errors.Is(err, os.ErrDeadlineExceeded) {
                gc.logger.Printf("Failed to drain the char create ok: %v", err)

                return
            }
            gc.logger.Println("Char create ok not drained in time, " +
                "the selection retry will cover it")

            return
        }
        gc.readBuf = payload
        if len(payload) == 0 {
            continue
        }
        if payload[0] == fromgameserver.CharCreateOkPacketID {
            gc.logger.Println("Character create confirmed")

            break
        }
    }

    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        gc.logger.Printf("Failed to reset the char create ok deadline: %v", err)
    }
}

// awaitCharacterCreation reads packets until the creation result resolves:
// the first return value reports completion, the second carries the updated
// character list when the requested character appeared in it. Every read
// bounds by charListWait, since the exchange sits before the in-world silence
// watchdog exists.
func (gc *GameClient) awaitCharacterCreation(
    name string,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charListWait)); err != nil {
        return false, nil, fmt.Errorf(
            "failed to set the creation wait deadline: %w", err)
    }
    payload, err := gc.readPacket(gc.readBuf)
    if err != nil {
        return false, nil, fmt.Errorf("failed to read creation result: %w", err)
    }
    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        return false, nil, fmt.Errorf(
            "failed to reset the creation wait deadline: %w", err)
    }
    gc.readBuf = payload
    if len(payload) == 0 {
        return false, nil, nil
    }

    return gc.handleCharCreatePacket(name, payload)
}

// handleCharCreatePacket dispatches a single packet of the creation flow.
func (gc *GameClient) handleCharCreatePacket(
    name string, payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
    switch payload[0] {
    case fromgameserver.CharCreateOkPacketID:
        if err := fromgameserver.ParseCharCreateOkPacket(payload); err != nil {
            return false, nil, err
        }
        gc.logger.Println("Character " + name + " created")
    case fromgameserver.CharCreateFailPacketID:
        return gc.handleCharCreateFail(payload)
    case fromgameserver.CharSelectInfoPacketID:
        return gc.handleUpdatedCharList(name, payload)
    default:
        gc.logger.Printf("Ignoring packet id 0x%02x while creating character",
            payload[0])
    }

    return false, nil, nil
}

// handleCharCreateFail converts a creation failure packet into an error.
func (gc *GameClient) handleCharCreateFail(
    payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
    fail := fromgameserver.NewCharCreateFailPacket()
    if err := fromgameserver.ParseCharCreateFailPacket(fail,
        payload); err != nil {
        return false, nil, err
    }

    return false, nil, fmt.Errorf(
        "character creation failed: %s", fail.ReasonText())
}

// handleUpdatedCharList checks the updated character list for the name.
func (gc *GameClient) handleUpdatedCharList(
    name string, payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
    updated := fromgameserver.NewCharSelectInfoPacket()
    if err := fromgameserver.ParseCharSelectInfoPacket(
        updated, payload); err != nil {
        return false, nil, fmt.Errorf(
            "failed to parse updated character list: %w", err)
    }
    if _, _, found := updated.FindCharacterByName(name); found {
        return true, updated, nil
    }

    return false, nil, errors.New(
        "created character missing in the updated list")
}

// EnterWorld selects the character slot and requests world entry.
func (gc *GameClient) EnterWorld(slot int32) error {
    // The selection is retransmitted when the server drops it
    // silently (see the charSelectWait constants): the wait is
    // bounded, the select is resent and only repeated failures give
    // up (the reconnect supervisor then rebuilds the session).
    var selected fromgameserver.CharSelectedPacket
    for attempt := 1; attempt <= charSelectAttempts; attempt++ {
        if err := gc.sendPacket(
            &togameserver.CharacterSelect{CharSlot: slot}); err != nil {
            return fmt.Errorf("failed to send character select: %w", err)
        }

        answered, err := gc.awaitCharSelected(&selected)
        if err != nil {
            return err
        }
        if answered {
            break
        }
        if attempt == charSelectAttempts {
            return errors.New("the server did not answer the character " +
                "selection after " + strconv.Itoa(charSelectAttempts) +
                " attempts")
        }
        gc.logger.Printf("CharSelected did not arrive (attempt %d), "+
            "reselecting in %s", attempt, charSelectWait)
    }
    gc.logger.Println("Selected character " + selected.Name)

    if err := gc.sendPacket(&togameserver.EnterWorld{}); err != nil {
        return fmt.Errorf("failed to send enter world: %w", err)
    }
    gc.logger.Println("Sent enter world request")

    // The movement toggle of the official client entry: the server starts every
    // session walking (Creature._isRunning defaults to false), so a bot that
    // never flips it crosses the world at 97 instead of ~170 units per second.
    run := togameserver.NewChangeMoveTypePacket()
    run.TypeRun = 1
    if err := gc.sendPacket(run); err != nil {
        return fmt.Errorf("failed to send run toggle: %w", err)
    }

    return nil
}

// awaitCharSelected waits for the CharSelected packet that allows
// entering the world, bounded by charSelectWait. It reports whether the
// answer arrived; unrelated packets are ignored like before.
func (gc *GameClient) awaitCharSelected(
    selected *fromgameserver.CharSelectedPacket,
) (bool, error) {
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charSelectWait)); err != nil {
        return false, fmt.Errorf(
            "failed to set the char select deadline: %w", err)
    }

    for {
        payload, err := gc.readPacket(gc.readBuf)
        if err != nil {
            gc.readBuf = nil
            if errors.Is(err, os.ErrDeadlineExceeded) {
                return false, nil
            }

            return false, fmt.Errorf(
                "failed to read character selected: %w", err)
        }
        gc.readBuf = payload
        if len(payload) == 0 {
            continue
        }
        if payload[0] == fromgameserver.CharSelectedPacketID {
            if err := fromgameserver.ParseCharSelectedPacket(
                selected, payload); err != nil {
                return false, fmt.Errorf(
                    "failed to parse char selected: %w", err)
            }
            gc.trackerApplySelection(selected)

            if resetErr := gc.conn.SetReadDeadline(
                time.Time{}); resetErr != nil {
                gc.logger.Printf(
                    "Failed to reset the char select deadline: %v",
                    resetErr)
            }

            return true, nil
        }
        gc.logger.Printf("Ignoring packet id 0x%02x while entering world",
            payload[0])
    }
}

// trackerApplySelection feeds the selected character state to the tracker.
func (gc *GameClient) trackerApplySelection(
    selected *fromgameserver.CharSelectedPacket,
) {
    if gc.tracker == nil {
        return
    }
    gc.tracker.SetCharacter(selected.Name, selected.ObjectID, selected.ClassID,
        selected.X, selected.Y, selected.Z,
        selected.CurrentHP, selected.CurrentMP)
}
