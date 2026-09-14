// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Experiment A+B: the synthetic "floating village over water" world.
//
// Reproduces the elven village topology the swarm grid pathfinder
// struggles with: a village deck riding a second walkable layer
// above a swimmable lake bed, a bridge strip crossing the water at
// deck height, and a navigation target that sits on the water
// directly UNDER the bridge - the same (x, y) as the bridge deck
// column but 288 world units below it. The run proves:
//
//   1. findNearestPoly disambiguates the stacked layers by 3D
//      distance: the under-bridge target binds to the water poly,
//      the same x/y at deck z binds to the bridge poly.
//   2. findPath routes deck -> bridge -> shore -> water -> target
//      when the swim area is allowed (water cost 3x, the swarm
//      waterCostMultiplier) and answers a partial dry path when it
//      is not.
//   3. the water escape (water -> deck, the swarm
//      FindWaterEscape BFS) is an ordinary findPath with a water
//      area cost - no dedicated flood fill needed.
//   4. query cost: microseconds per findPath on a mesh of a few
//      hundred polygons.

#include <chrono>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>

#include "DetourNavMesh.h"
#include "DetourNavMeshBuilder.h"
#include "DetourNavMeshQuery.h"
#include "Recast.h"

namespace {

// World scale mirrors the L2 geodata constants of the swarm engine:
// 16x16 world unit columns, 8 unit height quantization (the l2j
// wire format quantizes heights to multiples of 8).
constexpr float CELL_SIZE = 16.0f;
constexpr float CELL_HEIGHT = 8.0f;
constexpr int GRID = 192;              // columns per axis
constexpr int VOX_CLIMB = 5;           // 40 unit max passable step / 8
constexpr int VOX_AGENT_HEIGHT = 2;    // 16 units of clearance
constexpr float WATER_LEVEL = -160.0f; // local water surface

// The contour build knobs under diagnosis: the link graph of the
// stacked surfaces (the deck over the walkable ramp) breaks with
// simplification/maxEdgeLen values the demo uses by default.
#ifndef CONTOUR_MAX_ERROR
#define CONTOUR_MAX_ERROR 1.0f
#endif
#ifndef CONTOUR_MAX_EDGE
#define CONTOUR_MAX_EDGE 12
#endif

// Custom span areas (anything below RC_WALKABLE_AREA=63 is a custom
// area; the higher id wins when spans merge).
constexpr unsigned char AREA_GROUND = 63; // RC_WALKABLE_AREA
constexpr unsigned char AREA_WATER = 62;

// Poly flags, the RecastDemo convention.
constexpr unsigned short POLYFLAGS_WALK = 1;
constexpr unsigned short POLYFLAGS_SWIM = 2;

constexpr float WORLD_MIN[3] = {0.0f, -4096.0f, 0.0f};
constexpr float WORLD_MAX[3] = {GRID * CELL_SIZE, 1024.0f, GRID * CELL_SIZE};

// The log context prints the Recast build log and the timers.
class LogContext final : public rcContext {
public:
    LogContext() : rcContext(true) {}

protected:
    void doLog(const rcLogCategory /*category*/, const char* msg,
               const int /*len*/) override {
        std::printf("[recast] %s\n", msg);
    }
};

// baseHeight is the terrain without the island and the bridge.
int baseHeight(const int cz) {
    if (cz < 24) {
        return 0;     // the mainland plateau at deck level
    }
    if (cz < 42) {
        return -(cz - 23) * 16; // the shore ramp down to the lake bed
    }
    return -288;      // the flat lake bed (under the water level)
}

bool isIsland(const int cx, const int cz) {
    return cx >= 48 && cx < 144 && cz >= 48 && cz < 144;
}

bool isBridge(const int cx, const int cz) {
    return cx >= 88 && cx < 93 && cz >= 24 && cz < 48;
}

// The bridge abutment: the ramp surface does not pass UNDER the
// bridge deck. This detail is the difference between a clean
// multi layer mesh and a broken one: when a stacked surface sits
// within walkableClimb of the deck at a lateral edge (the first
// draft of this world had the deck 2..4 voxels above the ramp at
// the bridge start), the region flood fill merges the two stacked
// surfaces into ONE region whose 2D footprint self overlaps - the
// traced contours come out mangled and the poly link graph breaks
// silently (one way links, unreachable water). The real elven
// village geodata keeps the decks and the beds far apart (hundreds
// of units), which is what the abutment models here.
bool isAbutment(const int cx, const int cz) {
    return isBridge(cx, cz) && cz < 42;
}

// addTerrainSpan adds one walkable span whose TOP sits exactly at
// world z (the walkable surface, the l2j layer height).
int g_addedSpans = 0;

void addTerrainSpan(rcContext& ctx, rcHeightfield& hf, const int cx,
                    const int cz, const int z, const unsigned char area) {
    const int vox = (z - static_cast<int>(WORLD_MIN[1])) /
                    static_cast<int>(CELL_HEIGHT);
    const bool ok = rcAddSpan(&ctx, hf, cx, cz,
                              static_cast<unsigned short>(vox - 1),
                              static_cast<unsigned short>(vox), area, 0);
    if (!ok) {
        std::printf("FATAL: rcAddSpan failed at %d %d\n", cx, cz);
        std::exit(1);
    }
    ++g_addedSpans;
}

// buildNavMesh runs the full Recast pipeline over the synthetic
// world and returns the Detour mesh (ownership transfers to the
// caller through the out parameter).
unsigned char* buildNavMesh(LogContext& ctx, rcHeightfield& hf,
                            rcCompactHeightfield& chf,
                            rcContourSet& cset, rcPolyMesh& pmesh,
                            rcPolyMeshDetail& dmesh, int* outDataSize) {
    for (int cz = 0; cz < GRID; ++cz) {
        for (int cx = 0; cx < GRID; ++cx) {
            if (!isAbutment(cx, cz)) {
                addTerrainSpan(ctx, hf, cx, cz, baseHeight(cz),
                               baseHeight(cz) < WATER_LEVEL ? AREA_WATER
                                                            : AREA_GROUND);
            }
            if (isIsland(cx, cz) || isBridge(cx, cz)) {
                addTerrainSpan(ctx, hf, cx, cz, 0, AREA_GROUND);
            }
        }
    }

    if (!rcBuildCompactHeightfield(&ctx, VOX_AGENT_HEIGHT, VOX_CLIMB, hf,
                                   chf)) {
        std::printf("FATAL: rcBuildCompactHeightfield failed\n");
        std::exit(1);
    }
    if (!rcBuildDistanceField(&ctx, chf)) {
        std::printf("FATAL: rcBuildDistanceField failed\n");
        std::exit(1);
    }
    if (!rcBuildRegions(&ctx, chf, 0 /*border*/, 8 /*min region*/,
                        20 /*merge region*/)) {
        std::printf("FATAL: rcBuildRegions failed\n");
        std::exit(1);
    }
    if (!rcBuildContours(&ctx, chf, CONTOUR_MAX_ERROR, CONTOUR_MAX_EDGE,
                         cset)) {
        std::printf("FATAL: rcBuildContours failed\n");
        std::exit(1);
    }
    if (!rcBuildPolyMesh(&ctx, cset, 6 /*max verts per poly*/, pmesh)) {
        std::printf("FATAL: rcBuildPolyMesh failed\n");
        std::exit(1);
    }
    if (!rcBuildPolyMeshDetail(&ctx, pmesh, chf, CELL_SIZE * 6.0f,
                               CELL_HEIGHT, dmesh)) {
        std::printf("FATAL: rcBuildPolyMeshDetail failed\n");
        std::exit(1);
    }

    // Map the pmesh areas to poly flags: water polys are swimmable
    // only, ground polys are walkable only (the RecastDemo marking).
    unsigned short* flags = new unsigned short[pmesh.npolys];
    int waterPolys = 0;
    for (int i = 0; i < pmesh.npolys; ++i) {
        if (pmesh.areas[i] == AREA_WATER) {
            flags[i] = POLYFLAGS_SWIM;
            ++waterPolys;
        } else {
            flags[i] = POLYFLAGS_WALK;
        }
    }
    std::printf("mesh: %d polygons (%d water, %d ground), %d vertices\n",
                pmesh.npolys, waterPolys, pmesh.npolys - waterPolys,
                pmesh.nverts);

    dtNavMeshCreateParams params;
    std::memset(&params, 0, sizeof(params));
    params.verts = pmesh.verts;
    params.vertCount = pmesh.nverts;
    params.polys = pmesh.polys;
    params.polyFlags = flags;
    params.polyAreas = pmesh.areas;
    params.polyCount = pmesh.npolys;
    params.nvp = pmesh.nvp;
    params.detailMeshes = dmesh.meshes;
    params.detailVerts = dmesh.verts;
    params.detailVertsCount = dmesh.nverts;
    params.detailTris = dmesh.tris;
    params.detailTriCount = dmesh.ntris;
    params.walkableHeight = VOX_AGENT_HEIGHT * CELL_HEIGHT;
    params.walkableRadius = 8.0f;
    params.walkableClimb = VOX_CLIMB * CELL_HEIGHT;
    params.cs = CELL_SIZE;
    params.ch = CELL_HEIGHT;
    std::memcpy(params.bmin, WORLD_MIN, sizeof(float) * 3);
    std::memcpy(params.bmax, WORLD_MAX, sizeof(float) * 3);
    params.buildBvTree = true;

    unsigned char* data = nullptr;
    int dataSize = 0;
    if (!dtCreateNavMeshData(&params, &data, &dataSize)) {
        std::printf("FATAL: dtCreateNavMeshData failed\n");
        std::exit(1);
    }
    delete[] flags;
    *outDataSize = dataSize;

    return data;
}

// polyCenter dumps a polygon for the report.
void describePoly(const dtNavMesh& mesh, const dtPolyRef ref) {
    const dtMeshTile* tile = nullptr;
    const dtPoly* poly = nullptr;
    if (dtStatusFailed(mesh.getTileAndPolyByRef(ref, &tile, &poly))) {
        std::printf("poly ?");
        return;
    }
    const char* area = poly->getArea() == AREA_WATER ? "WATER" : "GROUND";
    const float* p = &tile->verts[poly->verts[0] * 3];
    std::printf("poly %llu area=%s flags=0x%x z~%.0f",
                static_cast<unsigned long long>(ref), area, poly->flags,
                p[1]);
}

struct QueryResult {
    dtPolyRef path[64];
    int pathCount = 0;
    float straight[3 * 64];
    unsigned char straightFlags[64];
    dtPolyRef straightRefs[64];
    int straightCount = 0;
    dtStatus status = 0;
    float nearestStart[3];
    float nearestEnd[3];
    dtPolyRef startRef = 0;
    dtPolyRef endRef = 0;
};

// runPath locates the endpoint polys with a permissive filter and
// then searches with the given filter - the two filter pattern the
// water experiments need (a dry search still binds the start poly
// on the water first).
QueryResult runPath(dtNavMeshQuery& query, const float* start,
                    const float* end, const dtQueryFilter& locate,
                    const dtQueryFilter& search) {
    QueryResult out;
    constexpr float LOCATE_EXTENT[3] = {32.0f, 700.0f, 32.0f};
    dtStatus status = query.findNearestPoly(
        start, LOCATE_EXTENT, &locate, &out.startRef, out.nearestStart);
    if (dtStatusFailed(status) || !out.startRef) {
        out.status = status;
        return out;
    }
    status = query.findNearestPoly(
        end, LOCATE_EXTENT, &locate, &out.endRef, out.nearestEnd);
    if (dtStatusFailed(status) || !out.endRef) {
        out.status = status;
        return out;
    }
    status = query.findPath(out.startRef, out.endRef, out.nearestStart,
                            out.nearestEnd, &search, out.path,
                            &out.pathCount, 64);
    out.status = status;
    if (dtStatusSucceed(status) && out.pathCount > 0) {
        query.findStraightPath(
            out.nearestStart, out.nearestEnd, out.path, out.pathCount,
            out.straight, out.straightFlags, out.straightRefs,
            &out.straightCount, 64, DT_STRAIGHTPATH_ALL_CROSSINGS);
    }
    return out;
}

// microbench times findPath+findStraightPath iterations.
double microbench(dtNavMeshQuery& query, const QueryResult& probe,
                  const dtQueryFilter& search) {
    constexpr int ITER = 20000;
    const auto began = std::chrono::steady_clock::now();
    dtPolyRef path[64];
    int pathCount = 0;
    float straight[3 * 64];
    unsigned char straightFlags[64];
    dtPolyRef straightRefs[64];
    int straightCount = 0;
    for (int i = 0; i < ITER; ++i) {
        query.findPath(probe.startRef, probe.endRef, probe.nearestStart,
                       probe.nearestEnd, &search, path, &pathCount, 64);
        query.findStraightPath(probe.nearestStart, probe.nearestEnd, path,
                               pathCount, straight, straightFlags,
                               straightRefs, &straightCount, 64, 0);
    }
    const auto elapsed = std::chrono::steady_clock::now() - began;
    return std::chrono::duration<double, std::micro>(elapsed).count() / ITER;
}

void printStraight(const QueryResult& r) {
    std::printf("  straight path (%d corners):\n", r.straightCount);
    for (int i = 0; i < r.straightCount; ++i) {
        const float* p = &r.straight[i * 3];
        const char* area = "";
        if (r.straightFlags[i] & DT_STRAIGHTPATH_END) {
            area = " END";
        }
        std::printf("    [%d] (%.0f, %.0f, %.0f)%s\n", i, p[0], p[1], p[2],
                    area);
    }
}

} // namespace

int main() {
    std::setvbuf(stdout, nullptr, _IONBF, 0);
    LogContext ctx;
    rcHeightfield* hf = rcAllocHeightfield();
    if (!rcCreateHeightfield(&ctx, *hf, GRID, GRID, WORLD_MIN, WORLD_MAX,
                             CELL_SIZE, CELL_HEIGHT)) {
        std::printf("FATAL: rcCreateHeightfield failed\n");
        return 1;
    }

    const auto buildBegan = std::chrono::steady_clock::now();
    rcCompactHeightfield* chf = rcAllocCompactHeightfield();
    rcContourSet* cset = rcAllocContourSet();
    rcPolyMesh* pmesh = rcAllocPolyMesh();
    rcPolyMeshDetail* dmesh = rcAllocPolyMeshDetail();
    int dataSize = 0;
    unsigned char* data = buildNavMesh(ctx, *hf, *chf, *cset, *pmesh, *dmesh,
                                       &dataSize);
    const auto buildElapsed =
        std::chrono::duration<double, std::milli>(
            std::chrono::steady_clock::now() - buildBegan)
            .count();
    std::printf("build: heightfield %d spans, compact %d spans, %d regions"
                " -> %d polys, navmesh data %d bytes, %.1f ms total\n",
                g_addedSpans, chf->spanCount, chf->maxRegions, pmesh->npolys,
                dataSize, buildElapsed);

    dtNavMesh mesh;
    if (dtStatusFailed(mesh.init(data, dataSize, 0))) {
        std::printf("FATAL: dtNavMesh::init failed\n");
        return 1;
    }
    dtNavMeshQuery query;
    if (dtStatusFailed(query.init(&mesh, 2048))) {
        std::printf("FATAL: dtNavMeshQuery::init failed\n");
        return 1;
    }

    // The permissive filter: everything allowed, water costs 3x (the
    // swarm waterCostMultiplier).
    dtQueryFilter permissive;
    permissive.setIncludeFlags(POLYFLAGS_WALK | POLYFLAGS_SWIM);
    permissive.setAreaCost(AREA_GROUND, 1.0f);
    permissive.setAreaCost(AREA_WATER, 3.0f);

    // The dry filter: the swim area excluded, the shore walks of the
    // hunt loop.
    dtQueryFilter dry;
    dry.setIncludeFlags(POLYFLAGS_WALK);
    dry.setAreaCost(AREA_GROUND, 1.0f);

    // The escape filter: everything allowed, water heavily priced so
    // the route leaves it at the nearest shore.
    dtQueryFilter escape;
    escape.setIncludeFlags(POLYFLAGS_WALK | POLYFLAGS_SWIM);
    escape.setAreaCost(AREA_GROUND, 1.0f);
    escape.setAreaCost(AREA_WATER, 8.0f);

    // ------------------------------------------------------------------
    // 1. The same x/y, two different z: nearest poly disambiguation.
    const float underBridge[3] = {90 * 16.0f + 8, -288.0f, 45 * 16.0f + 8};
    const float onDeck[3] = {90 * 16.0f + 8, 0.0f, 45 * 16.0f + 8};
    const float midHeight[3] = {90 * 16.0f + 8, -100.0f, 45 * 16.0f + 8};
    constexpr float EXT[3] = {32.0f, 700.0f, 32.0f};

    dtPolyRef underBridgeRef = 0;
    dtPolyRef& ref = underBridgeRef;
    float nearest[3] = {0, 0, 0};
    query.findNearestPoly(underBridge, EXT, &permissive, &ref, nearest);
    std::printf("\n[1] nearest poly of (%.0f, %.0f, %.0f) [UNDER the"
                " bridge, on the water]: ",
                underBridge[0], underBridge[1], underBridge[2]);
    describePoly(mesh, ref);
    std::printf(" -> nearest z %.0f\n", nearest[1]);

    query.findNearestPoly(onDeck, EXT, &permissive, &ref, nearest);
    std::printf("    nearest poly of (%.0f, %.0f, %.0f) [ON the bridge"
                " deck]: ",
                onDeck[0], onDeck[1], onDeck[2]);
    describePoly(mesh, ref);
    std::printf(" -> nearest z %.0f\n", nearest[1]);

    query.findNearestPoly(midHeight, EXT, &permissive, &ref, nearest);
    std::printf("    nearest poly of (%.0f, %.0f, %.0f) [mid height"
                " between the two]: ",
                midHeight[0], midHeight[1], midHeight[2]);
    describePoly(mesh, ref);
    std::printf(" -> nearest z %.0f\n", nearest[1]);

    // ------------------------------------------------------------------
    // 2. The village -> under the bridge route with swimming allowed.
    const float village[3] = {96 * 16.0f + 8, 0.0f, 96 * 16.0f + 8};
    QueryResult swim = runPath(query, village, underBridge, permissive,
                               permissive);
    std::printf("\n[2] village deck (%.0f, %.0f, %.0f) -> under the bridge"
                " (%.0f, %.0f, %.0f), swim allowed (water 3x):\n",
                village[0], village[1], village[2], underBridge[0],
                underBridge[1], underBridge[2]);
    if (dtStatusSucceed(swim.status)) {
        const bool partial =
            dtStatusDetail(swim.status, DT_PARTIAL_RESULT);
        std::printf("  found (%s): %d polys in the corridor:",
                    partial ? "PARTIAL" : "full", swim.pathCount);
        for (int i = 0; i < swim.pathCount; ++i) {
            std::printf("\n    ");
            describePoly(mesh, swim.path[i]);
        }
        std::printf("\n");
        printStraight(swim);
    } else {
        std::printf("  NOT FOUND (status 0x%x)\n", swim.status);
    }
    {
        // The default filter passes everything: does the search reach
        // the water target then?
        dtQueryFilter defaults;
        dtPolyRef defaultPath[64];
        int defaultCount = 0;
        const dtStatus defaultStatus = query.findPath(
            swim.startRef, swim.endRef, swim.nearestStart, swim.nearestEnd,
            &defaults, defaultPath, &defaultCount, 64);
        std::printf("  default filter findPath: status 0x%x, %d polys,"
                    " partial: %s\n",
                    defaultStatus, defaultCount,
                    dtStatusDetail(defaultStatus, DT_PARTIAL_RESULT) ? "yes"
                                                                     : "no");
    }
    std::printf("  raw status 0x%x, out-of-nodes: %s, partial: %s\n",
                swim.status,
                dtStatusDetail(swim.status, DT_OUT_OF_NODES) ? "yes" : "no",
                dtStatusDetail(swim.status, DT_PARTIAL_RESULT) ? "yes"
                                                               : "no");
    // The sliced variant of the same swim search with the iteration
    // count: how far does the A* actually explore?
    {
        int doneIters = 0;
        dtStatus sliceStatus = query.initSlicedFindPath(
            swim.startRef, swim.endRef, swim.nearestStart, swim.nearestEnd,
            &permissive);
        sliceStatus = query.updateSlicedFindPath(10000, &doneIters);
        dtPolyRef sliced[64];
        int slicedCount = 0;
        const dtStatus finStatus =
            query.finalizeSlicedFindPath(sliced, &slicedCount, 64);
        std::printf("  sliced: %d iters, finalize status 0x%x, %d polys,"
                    " partial: %s\n",
                    doneIters, finStatus, slicedCount,
                    dtStatusDetail(finStatus, DT_PARTIAL_RESULT) ? "yes"
                                                                 : "no");
    }
    std::printf("  avg query %.2f us (findPath+findStraightPath, 20k"
                " iterations)\n",
                microbench(query, swim, permissive));

    // The link dump of the last corridor poly and the target water poly:
    // are there ANY links between the ground and the water polys?
    {
        const dtMeshTile* tile = nullptr;
        const dtPoly* poly = nullptr;
        mesh.getTileAndPolyByRef(swim.path[swim.pathCount - 1], &tile,
                                 &poly);
        std::printf("  links of the last corridor poly:");
        for (unsigned int i = poly->firstLink; i != DT_NULL_LINK;
             i = tile->links[i].next) {
            std::printf(" ->poly %u", tile->links[i].ref);
        }
        std::printf("\n");
        // Count the cross area links of the whole mesh.
        int crossAreaLinks = 0;
        int totalLinks = 0;
        {
            const dtMeshTile* t = mesh.getTileAt(0, 0, 0);
            for (int pi = 0; t && pi < t->header->polyCount; ++pi) {
                const dtPoly* pp = &t->polys[pi];
                for (unsigned int li = pp->firstLink; li != DT_NULL_LINK;
                     li = t->links[li].next) {
                    ++totalLinks;
                    const dtPoly* other = nullptr;
                    const dtMeshTile* otherTile = nullptr;
                    mesh.getTileAndPolyByRef(t->links[li].ref, &otherTile,
                                             &other);
                    if (other && other->getArea() != pp->getArea()) {
                        ++crossAreaLinks;
                    }
                }
            }
        }
        std::printf("  mesh links: %d total, %d cross-area\n", totalLinks,
                    crossAreaLinks);

        // Isolation census: how many polys of each area have no links.
        int isolatedWater = 0;
        int isolatedGround = 0;
        int linkedWater = 0;
        int linkedGround = 0;
        const dtMeshTile* t = mesh.getTileAt(0, 0, 0);
        for (int pi = 0; t && pi < t->header->polyCount; ++pi) {
            const dtPoly* pp = &t->polys[pi];
            const bool linked = pp->firstLink != DT_NULL_LINK;
            if (pp->getArea() == AREA_WATER) {
                linked ? ++linkedWater : ++isolatedWater;
            } else {
                linked ? ++linkedGround : ++isolatedGround;
            }
        }
        std::printf("  water polys: %d linked, %d isolated; ground polys:"
                    " %d linked, %d isolated\n",
                    linkedWater, isolatedWater, linkedGround,
                    isolatedGround);
        // The cross-area link pairs and the component census of the
        // target water poly: a BFS over the link graph.
        {
            const dtMeshTile* t2 = mesh.getTileAt(0, 0, 0);
            const dtPolyRef baseRef = mesh.getPolyRefBase(t2);
            std::printf("  cross-area link pairs:");
            for (int pi = 0; t2 && pi < t2->header->polyCount; ++pi) {
                const dtPoly* pp = &t2->polys[pi];
                for (unsigned int li = pp->firstLink; li != DT_NULL_LINK;
                     li = t2->links[li].next) {
                    const dtPoly* other = nullptr;
                    const dtMeshTile* otherTile = nullptr;
                    mesh.getTileAndPolyByRef(t2->links[li].ref, &otherTile,
                                             &other);
                    if (other && other->getArea() != pp->getArea()) {
                        std::printf(" %u(%s)->%u(%s)", baseRef + pi,
                                    pp->getArea() == AREA_WATER ? "W" : "G",
                                    t2->links[li].ref,
                                    other->getArea() == AREA_WATER ? "W"
                                                                   : "G");
                    }
                }
            }
            std::printf("\n");

            // BFS from the target water poly over the link graph.
            const dtPolyRef startPoly = underBridgeRef;
            bool seen[512] = {};
            dtPolyRef frontier[512];
            int frontCount = 0;
            frontier[frontCount++] = startPoly;
            seen[startPoly] = true;
            int reached = 1;
            while (frontCount > 0) {
                const dtPolyRef current = frontier[--frontCount];
                const dtMeshTile* ct = nullptr;
                const dtPoly* cp = nullptr;
                mesh.getTileAndPolyByRef(current, &ct, &cp);
                if (!cp) {
                    continue;
                }
                for (unsigned int li = cp->firstLink; li != DT_NULL_LINK;
                     li = ct->links[li].next) {
                    const dtPolyRef nb = ct->links[li].ref;
                    if (nb < 512 && !seen[nb]) {
                        seen[nb] = true;
                        frontier[frontCount++] = nb;
                        ++reached;
                    }
                }
            }
            std::printf("  BFS from the target water poly 144 reaches %d"
                        " polys\n",
                        reached);
            // The island deck poly is the one the village start
            // binds to; resolve it the same way.
            dtPolyRef villageRef = 0;
            query.findNearestPoly(village, EXT, &permissive, &villageRef,
                                  nearest);
            const bool islandDeckSeen = villageRef < 512 && seen[villageRef];
            std::printf("  island deck poly %u reachable from it: %s\n",
                        villageRef, islandDeckSeen ? "YES" : "NO");

            // The reverse BFS: from the island deck poly. If the two
            // directions disagree, the graph has one-way links.
            bool seen2[512] = {};
            dtPolyRef frontier2[512];
            int front2 = 0;
            frontier2[front2++] = villageRef;
            seen2[villageRef] = true;
            int reached2 = 1;
            while (front2 > 0) {
                const dtPolyRef current = frontier2[--front2];
                const dtMeshTile* ct = nullptr;
                const dtPoly* cp = nullptr;
                mesh.getTileAndPolyByRef(current, &ct, &cp);
                if (!cp) {
                    continue;
                }
                for (unsigned int li = cp->firstLink; li != DT_NULL_LINK;
                     li = ct->links[li].next) {
                    const dtPolyRef nb = ct->links[li].ref;
                    if (nb < 512 && !seen2[nb]) {
                        seen2[nb] = true;
                        frontier2[front2++] = nb;
                        ++reached2;
                    }
                }
            }
            std::printf("  reverse BFS from the island poly %u reaches %d"
                        " polys, target water poly in it: %s\n",
                        villageRef, reached2,
                        seen2[underBridgeRef] ? "YES" : "NO");
            // The frontier polys: reachable from the island but with a
            // neighbour the water-side BFS owns - the one-way edge.
            for (int r = 0; r < 512; ++r) {
                if (!seen2[r]) {
                    continue;
                }
                const dtMeshTile* rt = nullptr;
                const dtPoly* rp = nullptr;
                mesh.getTileAndPolyByRef(r, &rt, &rp);
                if (!rp) {
                    continue;
                }
                for (unsigned int li = rp->firstLink; li != DT_NULL_LINK;
                     li = rt->links[li].next) {
                    const dtPolyRef nb = rt->links[li].ref;
                    if (nb < 512 && seen[nb] && !seen2[nb]) {
                        std::printf("  one-way frontier: poly %u(area %s)"
                                    " -> poly %u(area %s)\n",
                                    r, rp->getArea() == AREA_WATER ? "W"
                                                                   : "G",
                                    nb,
                                    seen[nb] && (nb & 0x7f) < 67
                                        ? "?"
                                        : "?");
                    }
                }
            }
        }
    }

    // ------------------------------------------------------------------
    // 3. The dry form: the target only swimming reaches.
    QueryResult dryRun = runPath(query, village, underBridge, permissive, dry);
    QueryResult partial = runPath(query, village, underBridge, permissive,
                                  permissive);
    std::printf("\n[3] the same route with the dry filter (swim"
                " excluded):\n");
    if (dtStatusSucceed(dryRun.status)) {
        const bool partial =
            dtStatusDetail(dryRun.status, DT_PARTIAL_RESULT);
        std::printf("  found (%s): %d polys:",
                    partial ? "PARTIAL - the closest dry poly"
                            : "FULL (unexpected)",
                    dryRun.pathCount);
        for (int i = 0; i < dryRun.pathCount; ++i) {
            std::printf("\n    ");
            describePoly(mesh, dryRun.path[i]);
        }
        std::printf("\n");
        printStraight(dryRun);
    } else {
        std::printf("  not found, status 0x%x (expected: the target poly"
                    " fails the filter)\n",
                    dryRun.status);
    }

    // The partial path form: the sliced search runs with the dry
    // filter over the permissively located endpoints and finalizes
    // with the closest-reachable-poly answer.
    dtQueryFilter drySearch;
    drySearch.setIncludeFlags(POLYFLAGS_WALK);
    drySearch.setAreaCost(AREA_GROUND, 1.0f);
    if (partial.startRef && partial.endRef) {
        dtStatus sliceStatus = query.initSlicedFindPath(
            partial.startRef, partial.endRef, partial.nearestStart,
            partial.nearestEnd, &drySearch);
        if (dtStatusSucceed(sliceStatus)) {
            int doneIters = 0;
            do {
                sliceStatus = query.updateSlicedFindPath(64, &doneIters);
            } while (dtStatusInProgress(sliceStatus));
            if (dtStatusFailed(sliceStatus)) {
                std::printf("  sliced dry search failed, status 0x%x\n",
                            sliceStatus);
            } else {
                dtPolyRef furthest[64];
                int furthestCount = 0;
                const dtStatus partialStatus = query.finalizeSlicedFindPath(
                    furthest, &furthestCount, 64);
                if (dtStatusFailed(partialStatus)) {
                    // The target never entered the closed set: ask for
                    // the partial corridor instead.
                    furthestCount = 0;
                }
                dtPolyRef partialPath[64];
                int partialCount = 0;
                query.finalizeSlicedFindPathPartial(
                    furthest, furthestCount, partialPath, &partialCount, 64);
                if (partialCount > 0) {
                    float bestPos[3] = {0, 0, 0};
                    query.closestPointOnPoly(
                        partialPath[partialCount - 1], underBridge, bestPos,
                        nullptr);
                    std::printf(
                        "  partial dry answer: %d polys, closest reachable"
                        " point (%.0f, %.0f, %.0f)\n",
                        partialCount, bestPos[0], bestPos[1], bestPos[2]);
                } else {
                    std::printf("  partial dry corridor empty\n");
                }
            }
        }
    }

    // ------------------------------------------------------------------
    // 4. The water escape: from the water under the bridge back onto
    // the deck, water priced 8x.
    QueryResult out = runPath(query, underBridge, village, permissive,
                              escape);
    std::printf("\n[4] water escape: under the bridge -> village deck"
                " (water cost 8x):\n");
    if (dtStatusSucceed(out.status)) {
        std::printf("  found: %d polys\n", out.pathCount);
        printStraight(out);
    } else {
        std::printf("  NOT FOUND (status 0x%x)\n", out.status);
    }

    // ------------------------------------------------------------------
    // 5. The raycast: the straight line off the island cliff.
    const float islandCenter[3] = {96 * 16.0f + 8, 0.0f, 96 * 16.0f + 8};
    const float offCliff[3] = {96 * 16.0f + 8, 0.0f, 24 * 16.0f + 8};
    dtPolyRef startRef = 0;
    float startPt[3];
    query.findNearestPoly(islandCenter, EXT, &permissive, &startRef,
                          startPt);
    float t = 0;
    float hitNormal[3] = {0, 0, 0};
    dtPolyRef rayPath[64];
    int rayCount = 0;
    const dtStatus rayStatus = query.raycast(
        startRef, islandCenter, offCliff, &permissive, &t, hitNormal,
        rayPath, &rayCount, 64);
    std::printf("\n[5] raycast island center -> mainland (%.0f, %.0f): ",
                offCliff[0], offCliff[2]);
    if (dtStatusSucceed(rayStatus)) {
        if (t < 1.0f) {
            std::printf("HIT at t=%.2f (the cliff edge), normal (%.1f, %.1f,"
                        " %.1f), %d polys walked\n",
                        t, hitNormal[0], hitNormal[1], hitNormal[2],
                        rayCount);
        } else {
            std::printf("clear, %d polys walked\n", rayCount);
        }
    } else {
        std::printf("failed\n");
    }

    // Cleanup.
    rcFreeHeightField(hf);
    rcFreeCompactHeightfield(chf);
    rcFreeContourSet(cset);
    rcFreePolyMesh(pmesh);
    rcFreePolyMeshDetail(dmesh);
    dtFree(data);
    return 0;
}
