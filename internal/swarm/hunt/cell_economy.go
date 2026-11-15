// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "sync"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// The ground economy prices a hunting cell by the adena its mobs pay per
// second of work. The C1 tables tie the adena to the mob level (the drop
// curve and the level gap scale) and the work to the hp at the character
// damage rate plus the incoming damage (the rest and the emergency logouts),
// so a higher band is only worth its longer fight (docs/hunting_cells.md).

// The economy constants of the picker.
const (
    // cellKillOverhead is the non-fight time of one kill before the risk
    // term: the loot walk, the pull and the target pick (the run5 cost per
    // kill 21.3 s over a 9.9 s fight, the town share removed).
    cellKillOverhead = 6.0
    // cellRiskPerPAtk is the rest and emergency logout time one point of mob
    // physical attack adds to a kill: the observed cost minus the fight and
    // the overhead ran 5.4 s at pAtk 22.5 and 12.1 s at pAtk 41.
    cellRiskPerPAtk = 0.30
    // combatRateRefPdef is the defence the damage rate meter normalizes to,
    // so the rate transfers across grounds with different mob defence.
    combatRateRefPdef = 100.0
    // combatRateDecay keeps the meter on the recent fights: every sample
    // decays the sums, so a gear upgrade replaces the old rate within a few
    // hundred kills.
    combatRateDecay = 0.99
    // combatRateMinSamples and combatRateMinSeconds gate the measured rate:
    // the first fights warm it up and the flat capacity holds meanwhile.
    combatRateMinSamples = 3
    combatRateMinSeconds = 10.0
    // adenaGapEnd is the DropAdenaMaxLevelDifference (Rates.ini): the adena
    // chance bottoms out at adenaGapFloor there.
    adenaGapEnd = int32(15)
    // adenaGapFloor is the DropAdenaMinLevelGapChance (Rates.ini, percent at
    // the widest gap, a tenth).
    adenaGapFloor = 0.10
)

// cellEconomy is the expected economy of one kill on a cell ground, weighted
// by the spawned mass of the species inside the income window.
type cellEconomy struct {
    // mass is the expected population inside the window.
    mass float64
    // hp, pdef and patk are the mean combat values of one kill.
    hp   float64
    pdef float64
    patk float64
    // adena is the mean adena of one kill after the level gap scale.
    adena float64
}

// noEconomy is the not-found answer (exhaustruct wants the plain var).
var noEconomy cellEconomy

// adenaGapChance returns the fraction of the adena drop the server pays for a
// mob at the given level against the character level: the full drop until
// adenaFullLevelGap, then the linear scale down to adenaGapFloor at
// adenaGapEnd (NpcTemplate.calculateGroupDrops, Rates.ini DropAdena*).
func adenaGapChance(charLevel int32, mobLevel int32) float64 {
    gap := charLevel - mobLevel
    if gap <= adenaFullLevelGap {
        return 1
    }
    if gap >= adenaGapEnd {
        return adenaGapFloor
    }
    span := float64(adenaGapEnd - adenaFullLevelGap)

    return 1 - (1-adenaGapFloor)*float64(gap-adenaFullLevelGap)/span
}

// cellWindowEconomy prices the income window of a cell at the character
// level: the spawned mass, the mean hp, defence, attack and adena of one
// kill. A species without a combat profile is skipped; false when the window
// holds no priced species.
func cellWindowEconomy(cell Cell, level int32) (cellEconomy, bool) {
    low, high := cellWindowBounds(level)
    var economy cellEconomy
    for index := range cell.Mobs {
        mob := &cell.Mobs[index]
        if mob.Count <= 0 || mob.Level < low || mob.Level > high {
            continue
        }
        profile, ok := npcdata.NPCCombatProfile(mob.TemplateID)
        if !ok || profile.HP <= 0 {
            continue
        }
        weight := float64(mob.Count)
        economy.mass += weight
        economy.hp += profile.HP * weight
        economy.pdef += profile.PDef * weight
        economy.patk += profile.PAtk * weight
        economy.adena += profile.Adena * adenaGapChance(level, mob.Level) *
            weight
    }
    if economy.mass <= 0 {
        return noEconomy, false
    }
    economy.hp /= economy.mass
    economy.pdef /= economy.mass
    economy.patk /= economy.mass
    economy.adena /= economy.mass

    return economy, true
}

// combatRateMeter accumulates the fleet's observed physical damage rate: the
// def normalized hp removed per second. A kill feeds the hp of its ground and
// the fought seconds, so the same character rate prices every ground.
type combatRateMeter struct {
    mu      sync.Mutex
    hp      float64
    seconds float64
    samples int
}

// globalCombatRate is the process wide meter, shared by the bots and the
// sessions of the fleet (the gear and the level are the same kit).
var globalCombatRate combatRateMeter

// add folds one fight into the meter: the mean hp of the ground at its mean
// defence and the seconds the fight ran.
func (m *combatRateMeter) add(hp float64, pdef float64, seconds float64) {
    if hp <= 0 || pdef <= 0 || seconds <= 0 {
        return
    }
    m.mu.Lock()
    m.hp = m.hp*combatRateDecay + hp*pdef/combatRateRefPdef
    m.seconds = m.seconds*combatRateDecay + seconds
    m.samples++
    m.mu.Unlock()
}

// rate returns the def normalized hp per second, zero until the meter holds
// enough fights to trust.
func (m *combatRateMeter) rate() float64 {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.samples < combatRateMinSamples || m.seconds < combatRateMinSeconds {
        return 0
    }

    return m.hp / m.seconds
}

// reset clears the meter (the tests).
func (m *combatRateMeter) reset() {
    m.mu.Lock()
    m.hp = 0
    m.seconds = 0
    m.samples = 0
    m.mu.Unlock()
}

// snapshot returns the sample count and the accumulated sums (the tests).
func (m *combatRateMeter) snapshot() (int, float64, float64) {
    m.mu.Lock()
    defer m.mu.Unlock()

    return m.samples, m.hp, m.seconds
}

// groundKillCapacity returns the kills per minute one hunter sustains on the
// ground: the fight at the measured damage rate plus the loot and risk
// overhead, capped by the flat solo capacity before the first fights.
func groundKillCapacity(economy cellEconomy) float64 {
    capacity := cellKillCapacity
    rate := globalCombatRate.rate()
    if rate <= 0 {
        return capacity
    }
    fight := economy.hp * economy.pdef / combatRateRefPdef / rate
    seconds := fight + cellKillOverhead + cellRiskPerPAtk*economy.patk
    if seconds > 0 {
        capacity = math.Min(capacity, 60.0/seconds)
    }

    return capacity
}

// noteCellCombat feeds the observed fight of the held ground into the damage
// rate meter: the ground price carries the hp and the defence, so a ground
// switch needs no per object template lookup.
func (l *Loop) noteCellCombat(seconds float64) {
    hunter := l.cell
    if hunter == nil || hunter.picked < 0 || seconds <= 0 {
        return
    }
    economy, ok := cellWindowEconomy(hunter.cells[hunter.picked], hunter.level)
    if !ok {
        return
    }
    globalCombatRate.add(economy.hp, economy.pdef, seconds)
}
