// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "errors"
    "fmt"
    "unicode/utf16"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/melg8/swarm/internal/swarm/state"
)

// handleClientPacket dispatches one decrypted client packet by state.
func (gc *gameConn) handleClientPacket(payload []byte) error {
    opcode := payload[0]
    switch gc.currentState() {
    case gameStateHandshake:
        // The handshake completes before the read loop starts; a packet
        // arriving here means a desynchronized client.
        gc.server.logger.Printf(
            "game#%d: packet 0x%02x before the handshake finished",
            gc.id, opcode)

    case gameStateAuthed:
        if opcode == gameOpAuthLogin {
            return gc.handleAuthLogin(payload)
        }
    case gameStateChars:
        switch opcode {
        case gameOpCharacterSelect:
            return gc.handleCharacterSelect(payload)
        case gameOpCharacterCreate:
            return gc.handleCharacterCreate()
        case gameOpNewCharacter, gameOpCharacterDelete:
            gc.server.logger.Printf(
                "game#%d: character management packet 0x%02x "+
                    "ignored by the emulation",
                gc.id, opcode)

            return nil
        }
    case gameStateSelected:
        if opcode == gameOpEnterWorld {
            return gc.handleEnterWorld()
        }
        if opcode == gameOpCharacterSelect {
            // The client is still at (or back on) the char
            // select screen: the unsolicited answer of the
            // auto select may not have taken (the packet raced
            // the client state), and the user's own double
            // click is served the same way - idempotent.
            return gc.handleCharacterSelect(payload)
        }
        gc.queueClientPacket(payload)

        return nil
    case gameStateWorld:
        return gc.transitToServer(payload)
    }

    gc.server.logger.Printf("game#%d: ignored packet 0x%02x in state %d",
        gc.id, opcode, gc.state)

    return nil
}

// handleAuthLogin accepts any session keys, resolves the bot session to
// serve (waiting for one to enter the world) and answers with the one
// character char list.
func (gc *gameConn) handleAuthLogin(payload []byte) error {
    login := readGameAuthLogin(payload)
    if login == "" {
        hexLen := min(len(payload), authLoginHexDumpLimit)
        gc.server.logger.Printf(
            "game#%d: auth login packet unreadable (len %d, decrypted % x): "+
                "continuing, any pair is accepted",
            gc.id, len(payload), payload[:hexLen])
        login = authLoginFallbackAccount
    }
    gc.server.logger.Printf(
        "game#%d: auth login for account %q (any pair is accepted)",
        gc.id, login)

    gc.session = gc.server.resolveSession()
    if gc.session == nil {
        return errors.New("no bot session available for the client")
    }
    if gc.session.tracker.Status() != state.StatusOnline {
        return errors.New("the bot session is not in the world yet")
    }
    gc.server.logger.Printf("game#%d: attached to bot session %q",
        gc.id, gc.session.id)

    charList, err := gc.buildCharacterList()
    if err != nil {
        return fmt.Errorf("failed to build the char list: %w", err)
    }
    if err := gc.sendToClient(charList); err != nil {
        return err
    }
    gc.setState(gameStateChars)

    return nil
}

// handleCharacterSelect answers a client character select with the
// recorded CharSelected packet of the bot session, patched to the live
// tracker state; a user click cancels the pending auto select of a
// dance (see serveCharSelectedLive).
func (gc *gameConn) handleCharacterSelect(payload []byte) error {
    slot := int32(0)
    if len(payload) >= 5 {
        slot = int32(uint32(payload[1]) | uint32(payload[2])<<8 |
            uint32(payload[3])<<16 | uint32(payload[4])<<24)
    }
    gc.cancelAutoSelect()
    if err := gc.serveCharSelectedLive(); err != nil {
        return err
    }

    live := gc.currentSession().tracker.SelfSnapshot()
    gc.server.logger.Printf(
        "game#%d: char select (slot %d) answered with the recorded packet, "+
            "live state %d %d %d hp %.0f/%.0f level %d",
        gc.id, slot, live.X, live.Y, live.Z, live.CurHP, live.MaxHP,
        live.Level)

    return nil
}

// serveCharSelectedLive serves the char selected answer of the current
// bot session, rewritten with the live snapshot (the newest WebUI
// selection wins). It is idempotent: the read loop, the auto select
// timer and a re-click after a lost answer all ride this path.
func (gc *gameConn) serveCharSelectedLive() error {
    if next := gc.server.tryResolveSelectedSession(); next != nil {
        gc.setSession(next)
    }
    session := gc.currentSession()
    if session == nil {
        return errors.New("no bot session available for the char selection")
    }

    gc.mu.Lock()
    if gc.state != gameStateChars && gc.state != gameStateSelected {
        gc.mu.Unlock()

        return nil // the client is not selecting a character
    }
    seq := session.recorder.FirstPacketSeq(charSelectedOpcode)
    if seq == 0 {
        gc.mu.Unlock()

        return errors.New("no char selected packet recorded for the session")
    }
    selected := session.recorder.Entry(seq)
    if selected == nil {
        gc.mu.Unlock()

        return errors.New("the recorded char selected entry is gone")
    }
    live := session.tracker.SelfSnapshot()
    answer := patchCharSelectedLive(selected, live)
    gc.charSelectedSeq = seq
    gc.state = gameStateSelected
    gc.mu.Unlock()

    return gc.sendToClient(answer)
}

// handleCharacterCreate refuses creation attempts: the emulated account
// offers exactly one character.
func (gc *gameConn) handleCharacterCreate() error {
    gc.server.logger.Printf(
        "game#%d: character create refused (the proxy serves one character)",
        gc.id)
    writer := packet.NewWriter()
    if err := writer.WriteInt8(gameOpCharCreateFail); err != nil {
        return err
    }
    if err := writer.WriteInt32(charCreateFailTooMany); err != nil {
        return err
    }

    return gc.sendToClient(writer.Bytes())
}

// handleEnterWorld flushes the client packets queued between the char
// selection and the world entry, then hands the stream to the relay: a
// fresh connection starts the relay goroutine, while a re-entry after a
// restart dance wakes the parked relay (it owns the lifecycle) with the
// replay sequence of the just served char selected answer.
func (gc *gameConn) handleEnterWorld() error {
    gc.server.logger.Printf("game#%d: enter world, starting the session relay",
        gc.id)

    gc.mu.Lock()
    if gc.state != gameStateSelected {
        gc.mu.Unlock()

        return nil // not selecting: a stray re-entry
    }
    gc.state = gameStateWorld
    seq := gc.charSelectedSeq
    relayLive := gc.relayLive
    gc.relayLive = true
    gc.mu.Unlock()

    for _, queued := range gc.queue {
        if err := gc.transitToServer(queued); err != nil {
            return err
        }
    }
    gc.queue = nil

    if relayLive {
        // A restart dance parked the relay on this connection:
        // the re-entry is its signal to continue.
        select {
        case gc.enterWorldCh <- seq:
        case <-gc.done:
        }

        return nil
    }

    go gc.runRelay()

    return nil
}

// queueClientPacket buffers a client packet of the selected state until
// the world entry.
func (gc *gameConn) queueClientPacket(payload []byte) {
    copied := make([]byte, len(payload))
    copy(copied, payload)
    gc.queue = append(gc.queue, copied)
}

// authLoginFallbackAccount is the placeholder account name used when
// the login string of the game AuthLogin packet cannot be decoded: the
// emulated server accepts any account, so a cosmetic field must never
// fail the client connection.
const authLoginFallbackAccount = "<unreadable>"

// authLoginHexDumpLimit bounds the decrypted hex dump logged for an
// unreadable auth login packet.
const authLoginHexDumpLimit = 48

// authLoginPrefixLen is the byte size of the opcode plus the short
// length prefix of the fallback auth login layout.
const authLoginPrefixLen = 3

// readGameAuthLogin extracts the login string of the game AuthLogin
// packet: [opcode 0x08][login: utf16][4 session key ints]. The session
// key values are accepted as whatever the client carries. The login is
// cosmetic, so the reader accepts both known utf16 layouts (null
// terminated like the Mobius readString, and short length prefixed) and
// degrades to an empty string instead of failing on anything else.
func readGameAuthLogin(payload []byte) string {
    if len(payload) < 2 {
        return ""
    }

    // The documented Mobius layout: a null terminated UTF-16LE string.
    login, err := packet.NewReader(payload[1:]).ReadStringFromUtf16Format()
    if err == nil {
        return login
    }

    // The fallback layout: [short char count][UTF-16LE data].
    if len(payload) >= authLoginPrefixLen {
        charCount := int(uint16(payload[1]) | uint16(payload[2])<<8)
        if charCount > 0 && authLoginPrefixLen+charCount*2 <= len(payload) {
            data := payload[authLoginPrefixLen : authLoginPrefixLen+charCount*2]
            units := make([]uint16, 0, charCount)
            for i := 0; i+1 < len(data); i += 2 {
                units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
            }

            return string(utf16.Decode(units))
        }
    }

    return ""
}
