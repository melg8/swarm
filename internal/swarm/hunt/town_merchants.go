// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// townNpc is a town npc the trip machinery navigates to: a shop
// merchant of the sell trips or a guard of the deleveling.
type townNpc struct {
    TemplateID int32
    Name       string
    X          int32
    Y          int32
    Z          int32
}

// zeroTownNpc is the not-found sentinel of the merchant and guard searches.
var zeroTownNpc = townNpc{TemplateID: 0, Name: "", X: 0, Y: 0, Z: 0}

// townMerchants are the shop merchants of the elven village, the default set of
// the 1-19 band; any merchant accepts the sale of any sellable item, so the bot
// walks to the nearest one. The coordinates come from the Mobius C1 spawn data
// (ElvenVillageNPCs.xml) and TemplateID is the display id the NpcInfo packet
// carries (the C1 spawn ids 30147..30150 map through CT0_to_C4_ids.txt).
var townMerchants = []townNpc{
    {TemplateID: 7147, Name: "Unoren", X: 44667, Y: 46896, Z: -2982},
    {TemplateID: 7148, Name: "Ariel", X: 44683, Y: 46952, Z: -2981},
    {TemplateID: 7149, Name: "Creamees", X: 42700, Y: 50057, Z: -2984},
    {TemplateID: 7150, Name: "Herbiel", X: 42766, Y: 50037, Z: -2984},
}

// dionMerchants are the shop merchants of the Town of Dion the 20-25 band trip
// targets (docs/band_20_25_survey.md): the spawn positions of
// spawns/Dion/DionNPCs.xml with their CT0-to-C4 display ids. The loop switches
// to them when the zone region is Dion (shopCatalogForRegion).
var dionMerchants = []townNpc{
    {TemplateID: 7060, Name: "Sabrin", X: 17999, Y: 144484, Z: -3048},
    {TemplateID: 7061, Name: "Casey", X: 17948, Y: 144560, Z: -3048},
    {TemplateID: 7062, Name: "Sonia", X: 19313, Y: 146229, Z: -3048},
    {TemplateID: 7063, Name: "Lara", X: 19223, Y: 146228, Z: -3048},
}

// merchantStands are the curated customer stands of the merchants that trade
// behind a counter. The spawn cell sits inside the roofed stall (roof only
// cells, nobody can stand there) and the mesh route to it ends wherever the
// nearest floor poly sits, so each stand is the cell just beyond the counter
// front along the merchant facing heading, derived and verified with
// cmd/counterprobe. Every z is the pack floor of the cell and the generated
// merchantStandsWorld table covers the rest of the world (docs/hunting.md).
var merchantStands = map[int32]pathfind.Vec3{
    // Unoren, the corridor west of the counter front at the gate row (deeper
    // rows clip the walled corner on the diagonal strides).
    7147: {X: 44584, Y: 46944, Z: -2984},
    // Ariel, the same corridor at her row.
    7148: {X: 44584, Y: 46952, Z: -2984},
    // Creamees, the open floor beyond the counter front south-east.
    7149: {X: 42727, Y: 50115, Z: -2984},
    // Herbiel, the open floor beyond the counter front south-east.
    7150: {X: 42798, Y: 50101, Z: -2984},
    // Sabrin, the corridor east of the counter front.
    7060: {X: 18072, Y: 144488, Z: -3040},
    // Casey, the same corridor at her row.
    7061: {X: 18044, Y: 144560, Z: -3040},
    // Sonia, the corridor north of the counter front.
    7062: {X: 19320, Y: 146168, Z: -3064},
    // Lara, the same corridor at her row.
    7063: {X: 19224, Y: 146168, Z: -3064},
}

// merchantStandPoint returns the customer stand of the merchant: the curated
// table first, the generated world table second and the spawn point otherwise
// (the spawns the pack serves directly plus the flat placeholder regions). The
// walks target the stand while the interaction and approach gates keep
// measuring the spawn, which sits 64-80 units away.
func merchantStandPoint(npc townNpc) pathfind.Vec3 {
    if stand, ok := merchantStands[npc.TemplateID]; ok {
        return stand
    }
    if stand, ok := merchantStandsWorld[npc.TemplateID]; ok {
        return stand
    }

    return townNpcPosition(npc)
}

// merchantsForRegion returns the shop merchants of the region the loop farms
// near: the elven village set by default, the Dion set for the 20-25 band. The
// same set builds the region catalog, so the trip targets, the sell stop pick
// and the purchase plan name one town's traders.
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

// merchantTemplates lists the packet template ids of the active region's
// merchants: the sell stop accepts any of them, the buy stops name their own
// trader through stopMerchantTemplates.
func (l *Loop) merchantTemplates() []int32 {
    return townMerchantTemplates(merchantsForRegion(l.zoneRegion))
}

// townMerchantTemplates lists the packet template ids of the merchant
// set (the NpcInfo tracker ids carry the display id offset).
func townMerchantTemplates(merchants []townNpc) []int32 {
    templates := make([]int32, 0, len(merchants))
    for _, merchant := range merchants {
        templates = append(templates,
            merchant.TemplateID+npcdata.NPCDisplayOffset)
    }

    return templates
}

func townNpcPosition(npc townNpc) pathfind.Vec3 {
    return pathfind.Vec3{
        X: float64(npc.X),
        Y: float64(npc.Y),
        Z: float64(npc.Z),
    }
}

// npcApproachPoint computes the ground click target for a town npc approach: a
// point npcApproachOffset units from the npc toward the bot, so the Bresenham
// line the server validates stays outside the walls, carrying the npc z so the
// server resolves its ground floor. A bot already inside the offset gets its
// own x and y back (the click collapses and the caller talks instead).
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

// merchantWithinExactRange reports whether the merchant is close enough for the
// exact segment: the exactSegmentMaxDistance gate (the far exact searches are
// the exhaustive flood class).
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
