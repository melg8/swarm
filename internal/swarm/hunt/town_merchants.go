// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// npcDisplayOffset mirrors the display id offset the Mobius server adds
// to the npc id of every NpcInfo packet (AbstractNpcInfo writeImpl).
const npcDisplayOffset = 1000000

// townNpc is a town npc the trip machinery navigates to: a shop
// merchant of the sell trips or a guard of the deleveling.
type townNpc struct {
    TemplateID int32
    Name       string
    X          int32
    Y          int32
    Z          int32
}

// zeroTownNpc is the not-found sentinel of the merchant and guard
// searches.
var zeroTownNpc = townNpc{TemplateID: 0, Name: "", X: 0, Y: 0, Z: 0}

// townMerchants are the shop merchants of the known towns. Any merchant
// accepts the sale of any sellable item (the inventory sell list), so
// the bot simply walks to the nearest one; the list grows with the
// farming areas of the deployment. Coordinates from the Mobius C1
// spawn data (ElvenTerritory/ElvenVillageNPCs.xml). TemplateID is the
// client display id the NpcInfo packet carries: the C1 spawn ids of the
// traders (30147..30150) map to the CT0 display ids through
// CT0_to_C4_ids.txt of the npc stats.
var townMerchants = []townNpc{
    {TemplateID: 7147, Name: "Unoren", X: 44667, Y: 46896, Z: -2982},
    {TemplateID: 7148, Name: "Ariel", X: 44683, Y: 46952, Z: -2981},
    {TemplateID: 7149, Name: "Creamees", X: 42700, Y: 50057, Z: -2984},
    {TemplateID: 7150, Name: "Herbiel", X: 42766, Y: 50037, Z: -2984},
}

// dionMerchants are the shop merchants of the Town of Dion the 20-25
// band shopping trip targets, from docs/band_20_25_survey.md. The
// coordinates are the spawn positions of spawns/Dion/DionNPCs.xml;
// TemplateID is the CT0-to-C4 display id (the npcdata.npcBuyLists map
// resolves their buylists). The elven village merchants stay the
// default of the 1-19 band; the hunt loop switches to the Dion
// merchants when the active zone region is Dion (see
// shopCatalogForRegion).
var dionMerchants = []townNpc{
    {TemplateID: 7060, Name: "Sabrin", X: 17999, Y: 144484, Z: -3048},
    {TemplateID: 7061, Name: "Casey", X: 17948, Y: 144560, Z: -3048},
    {TemplateID: 7062, Name: "Sonia", X: 19313, Y: 146229, Z: -3048},
    {TemplateID: 7063, Name: "Lara", X: 19223, Y: 146228, Z: -3048},
}

// merchantStands are the customer stand points of the merchants that
// trade behind a counter (the curated rows of the counter round: the
// elven village and the Dion quarter). The merchant spawn itself is no
// stand point: the spawn cell sits inside the roofed stall (the
// geodata pack models the stall interior as roof-only cells over the
// missing floor - the cell answers blocked, nobody can stand on it
// directly, the user report of the shop quarter round), and the mesh
// route to the spawn ends wherever the nearest floor poly happens to
// sit - for Unoren that was the outer side of the stall front 42 units
// north-west, not the customer side. The stand is the cell just
// beyond the counter front along the merchant facing heading (the
// counter sits on the facing side of the stall, the customer cell on
// the other side of it), derived and verified with cmd/counterprobe
// (-mode detect names the counter direction, -mode stands and
// -mode scan pin the cell: every entry routes found with the plan
// ending exactly on the cell, inside the interaction distance of the
// spawn). A corrected or a new entry is one table row plus a rerun
// of the probe; the z of every entry is the pack floor of the cell
// so the exact search resolves it onto the right deck. The world
// wide coverage of the same machinery rides the generated
// merchantStandsWorld table (merchant_stands_world.go).
var merchantStands = map[int32]pathfind.Vec3{
    // Unoren, the customer corridor west of the counter front, at the
    // corridor gate latitude (the gate cell row the east approach
    // lines pass through; deeper rows clip the walled counter corner
    // on the diagonal strides).
    7147: {X: 44584, Y: 46944, Z: -2984},
    // Ariel, the same corridor at her row.
    7148: {X: 44584, Y: 46952, Z: -2984},
    // Creamees, the open floor beyond the counter front south-east.
    7149: {X: 42727, Y: 50115, Z: -2984},
    // Herbiel, the open floor beyond the counter front south-east.
    7150: {X: 42798, Y: 50101, Z: -2984},
    // Sabrin, the customer corridor east of the counter front.
    7060: {X: 18072, Y: 144488, Z: -3040},
    // Casey, the same corridor at her row.
    7061: {X: 18044, Y: 144560, Z: -3040},
    // Sonia, the customer corridor north of the counter front.
    7062: {X: 19320, Y: 146168, Z: -3064},
    // Lara, the same corridor at her row.
    7063: {X: 19224, Y: 146168, Z: -3064},
}

// merchantStandPoint returns the customer stand point of the merchant
// when a stand table knows it, the spawn point otherwise. The curated
// counter table answers first (the hand pinned elven and Dion rows),
// the generated world table second (the counterprobe world pass rows
// of merchant_stands_world.go) and the spawn fallback covers the
// spawns the pack serves directly plus the flat placeholder regions.
// The walk planning of the merchant stops targets the stand (the
// character stops face to face with the merchant across the counter);
// the interaction and approach gates keep measuring the spawn (the
// stand sits 64-80 units from it, well inside the 250 interaction
// distance).
func merchantStandPoint(npc townNpc) pathfind.Vec3 {
    if stand, ok := merchantStands[npc.TemplateID]; ok {
        return stand
    }
    if stand, ok := merchantStandsWorld[npc.TemplateID]; ok {
        return stand
    }

    return townNpcPosition(npc)
}

// merchantsForRegion returns the shop merchants of the region the
// loop farms near: the elven village set is the default and the Dion
// set serves the 20-25 band region. The same set builds the region
// shop catalog (see shopCatalogForRegion), so the trip targets, the
// sell stop pick and the purchase plan always name the same town's
// traders.
func merchantsForRegion(region string) []townNpc {
    if region == regionDion {
        return dionMerchants
    }

    return townMerchants
}

// nearestMerchant returns the town merchant of the active region
// closest to the point.
func (l *Loop) nearestMerchant(
    selfX int32, selfY int32,
) (townNpc, bool) {
    best := zeroTownNpc
    bestDist := math.MaxFloat64
    found := false
    for _, merchant := range merchantsForRegion(l.zoneRegion) {
        dist := math.Hypot(
            float64(merchant.X-selfX), float64(merchant.Y-selfY))
        if dist < bestDist {
            bestDist = dist
            best = merchant
            found = true
        }
    }

    return best, found
}

// merchantTemplates lists the packet template ids of the active
// region's town merchants: the sell stop pick accepts any of them
// (every vendor accepts the sale of any sellable item), while the buy
// stops name their own trader through stopMerchantTemplates.
func (l *Loop) merchantTemplates() []int32 {
    return townMerchantTemplates(merchantsForRegion(l.zoneRegion))
}

// townMerchantTemplates lists the packet template ids of the merchant
// set (the NpcInfo tracker ids carry the display id offset).
func townMerchantTemplates(merchants []townNpc) []int32 {
    templates := make([]int32, 0, len(merchants))
    for _, merchant := range merchants {
        templates = append(templates, merchant.TemplateID+npcDisplayOffset)
    }

    return templates
}

// townNpcPosition returns the spawn point of the npc.
func townNpcPosition(npc townNpc) pathfind.Vec3 {
    return pathfind.Vec3{
        X: float64(npc.X),
        Y: float64(npc.Y),
        Z: float64(npc.Z),
    }
}

// npcApproachPoint computes the ground click target for a town npc
// approach: a point npcApproachOffset units from the npc toward the
// bot, so the Bresenham click line the server validates stays outside
// the building walls. The z is the npc's z: the server resolves the
// target cell layer from it, picking the ground floor the npc stands
// on. When the bot already stands within the offset distance, the
// bot's own x and y are returned (the click collapses to a no-op the
// caller skips in favor of the talk click).
func npcApproachPoint(
    npcX, npcY, npcZ, selfX, selfY int32,
) (int32, int32, int32) {
    dx := float64(selfX - npcX)
    dy := float64(selfY - npcY)
    dist := math.Hypot(dx, dy)
    if dist < 1 {
        return npcX, npcY, npcZ
    }
    frac := npcApproachOffset / dist
    if frac >= 1 {
        return selfX, selfY, npcZ
    }
    ax := float64(npcX) + dx*frac
    ay := float64(npcY) + dy*frac

    return int32(math.Round(ax)), int32(math.Round(ay)), npcZ
}

// merchantWithinExactRange reports whether the merchant is close
// enough for the exact segment: the distance gate of exactSegmentMaxDistance
// (see the constant comment - the far exact searches are the
// exhaustive flood class).
func (l *Loop) merchantWithinExactRange(npc townNpc) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dx := float64(npc.X - selfX)
    dy := float64(npc.Y - selfY)
    dz := float64(npc.Z - selfZ)

    return math.Sqrt(dx*dx+dy*dy+dz*dz) <= exactSegmentMaxDistance
}
