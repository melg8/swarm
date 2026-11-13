// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const SetupGaugePacketID = 0x85

// gaugeColorRed is the RED gauge color of the SetupGauge packet (the
// SetupGauge.RED constant of the Mobius server; the other colors are
// BLUE 0 for skill casts, CYAN 2 for the water breath and 3 for the
// feed bar of a mount). Only the red gauge carries a bow cycle.
const gaugeColorRed = 1

// SetupGaugePacket announces a gauge bar of the played character. The packet
// carries no object id (it goes only to the affected client), so every parsed
// gauge belongs to the self. Wire (SetupGauge.writeImpl): [opcode 0x85]
// [color: 4][current: 4][max: 4]. The RED gauge is the bow shot cycle: the
// server sends SetupGauge(RED, sAtk + reuse) from Creature.doAttackHitByBow
// when it commits the shot, so the max is the full disable window.
type SetupGaugePacket struct {
    Color   int32
    Current int32
    Max     int32
}

func NewSetupGaugePacket() *SetupGaugePacket {
    return &SetupGaugePacket{Color: 0, Current: 0, Max: 0}
}

func ParseSetupGaugePacket(p *SetupGaugePacket, data []byte) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, SetupGaugePacketID); err != nil {
        return err
    }
    if err := readInt32Fields(reader, &p.Color, &p.Current,
        &p.Max); err != nil {
        return fmt.Errorf("failed to read setup gauge: %w", err)
    }

    return nil
}

// IsBowCycle reports whether the gauge is the red bow shot gauge.
func (p *SetupGaugePacket) IsBowCycle() bool {
    return p.Color == gaugeColorRed && p.Max > 0
}
