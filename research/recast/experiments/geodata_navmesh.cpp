// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Experiment C+D: the real geodata converter and benchmark.
//
// Reads one l2j region file (the elven village region 21_19 by
// default), converts every layer of every cell into a Recast
// heightfield span (the walkable surface is the span TOP, exactly the
// l2j layer height), marks the spans below the C1 water level as the
// WATER area, runs the full Recast build pipeline and answers the
// queries of the bridge_over_water experiment on REAL data:
//
//   - the same x/y different z disambiguation on a real bridge column,
//   - the village -> under-the-bridge route with and without swimming,
//   - the audits: how faithful the mesh is (the NSWE walls the
//     height-only import cannot see), how healthy the poly link graph
//     is (one-way links, isolated polys) and how many stacked-surface
//     merge hazards the region contains,
//   - the build cost and the query microbenchmark over random pairs,
//     written to a pairs file the Go engine benchmark replays.
//
// The naive import (every layer a span, water/ground areas) FAILS on
// the real region: the elven lands contain stacked walkable surfaces
// that are additionally within climb range of each other (terraces,
// interior floors, ramps under decks) - the region flood fill merges
// them into regions whose 2D footprint self overlaps, rcBuildContours
// answers with "multiple outlines", mergeHoles fails and
// rcBuildPolyMesh dies on a 137k vertex contour. The fix this
// experiment implements is the SHEET decomposition: the walkable
// layers are partitioned into 2D manifold sheets (at most one span
// per column per sheet, flooded top down over the within-climb
// geometric continuity), every sheet gets its own area id through
// greedy graph coloring of the sheet adjacency (the contour machinery
// then never sees a self overlapping region), and the poly links the
// sheets produce at their shared cell borders are validated against
// the height rule afterwards (links between vertically stacked
// surfaces die).
//
// Tunables (compile-time): CONTOUR_MAX_ERROR, MIN_REGION_AREA,
// MERGE_REGION_AREA, WALKABLE_HEIGHT_VOX, REGIONS_MONOTONE.

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <algorithm>
#include <vector>

#include "DetourNavMesh.h"
#include "DetourNavMeshBuilder.h"
#include "DetourNavMeshQuery.h"
#include "Recast.h"

namespace {

// The l2j multilayer cells carry duplicate noise layers (the same
// surface written twice with +-8 jitter, plus orphaned pockets). The
// dedup rule below merges the within-16 duplicates of one cell (no
// real walkable geometry stacks two surfaces 16 units apart in one
// cell) and the sheet drop rule removes the islands no walk can
// reach anyway (no neighbour layer within the climb range: the
// current grid engine cannot reach them either).
constexpr int REGION_CELLS = 2048;        // cells per region side
constexpr int DEDUP_DELTA = 16;
#ifndef MIN_SHEET_LAYERS
constexpr int MIN_SHEET_LAYERS = 4;
#endif
constexpr int BLOCKS = 256;              // blocks per region side
constexpr float CELL_SIZE = 16.0f;
constexpr float CELL_HEIGHT = 8.0f;
constexpr int VOX_CLIMB = 5;             // 40 units, the max passable step
#ifndef WALKABLE_HEIGHT_VOX
constexpr int WALKABLE_HEIGHT_VOX = 1;   // L2 has no ceiling rule
#endif
constexpr float WATER_LEVEL = -3780.0f;  // the C1 water surface
constexpr float REGION_MIN_X = 32768.0f; // region 21_19 world anchor
constexpr float REGION_MIN_Z = 32768.0f;
constexpr float WORLD_MIN[3] = {REGION_MIN_X, -8192.0f, REGION_MIN_Z};
constexpr float WORLD_MAX[3] = {REGION_MIN_X + REGION_CELLS * CELL_SIZE,
                                8192.0f,
                                REGION_MIN_Z + REGION_CELLS * CELL_SIZE};

constexpr unsigned char AREA_GROUND = 63; // RC_WALKABLE_AREA
constexpr unsigned char AREA_WATER = 62;
constexpr unsigned short POLYFLAGS_WALK = 1;
constexpr unsigned short POLYFLAGS_SWIM = 2;

#ifndef CONTOUR_MAX_ERROR
constexpr float CONTOUR_MAX_ERROR = 1.3f;
#endif
#ifndef MIN_REGION_AREA
constexpr int MIN_REGION_AREA = 1;
#endif
#ifndef MERGE_REGION_AREA
constexpr int MERGE_REGION_AREA = 1;
#endif

// CellLayer is one l2j cell layer: the surface height and the NSWE
// wall flags (N=8, S=4, W=2, E=1 - a set bit is an open direction).
struct CellLayer {
    int16_t h;
    uint8_t nswe;
};

// RegionData holds the parsed region: per cell a contiguous slice of
// the layer pool. Two arrays: spanOff (cells+1 offsets) + layers.
struct RegionData {
    // The l2j block order is x-strip major (blocks of one x strip
    // cover all y first), so the layers of neighbouring cells sit
    // far apart in the array: every cell carries its own offset and
    // count instead of a range into a shared prefix array.
    std::vector<uint32_t> cellOff; // per cell: start index into layers
    std::vector<uint16_t> cellCnt; // per cell: layer count
    std::vector<CellLayer> layers;
    std::vector<uint32_t> cellIndexOf;
    int stackedColumns = 0;   // cells with 2+ layers
    int underwaterLayers = 0; // layers below the water level

    // indexOf returns the cell index (cx * REGION_CELLS + cy) of the
    // layer instance j.
    size_t indexOf(uint32_t j) const {
        return cellIndexOf[j];
    }
};

class LogContext final : public rcContext {
public:
    LogContext() : rcContext(true) {}

protected:
    void doLog(const rcLogCategory /*category*/, const char* msg,
               const int /*len*/) override {
        std::printf("[recast] %s\n", msg);
    }
};

uint16_t le16(const unsigned char* p) {
    return static_cast<uint16_t>(p[0] | (p[1] << 8));
}

// parseRegion mirrors the Go parser of internal/swarm/pathfind
// (region.go): headerless, 65536 blocks, flat/complex/multilayer.
bool parseRegion(const char* path, RegionData& out) {
    FILE* f = std::fopen(path, "rb");
    if (!f) {
        std::printf("FATAL: cannot open %s\n", path);
        return false;
    }
    std::fseek(f, 0, SEEK_END);
    const long size = std::ftell(f);
    std::fseek(f, 0, SEEK_SET);
    std::vector<unsigned char> data(static_cast<size_t>(size));
    if (std::fread(data.data(), 1, data.size(), f) != data.size()) {
        std::printf("FATAL: short read of %s\n", path);
        std::fclose(f);
        return false;
    }
    std::fclose(f);

    const int cells = REGION_CELLS * REGION_CELLS;
    out.cellOff.assign(static_cast<size_t>(cells), 0);
    out.cellCnt.assign(static_cast<size_t>(cells), 0);
    out.cellIndexOf.reserve(static_cast<size_t>(cells) * 2);
    out.layers.reserve(static_cast<size_t>(cells) * 2);

    size_t pos = 0;
    for (int block = 0; block < BLOCKS * BLOCKS; ++block) {
        if (pos >= data.size()) {
            std::printf("FATAL: truncated at block %d\n", block);
            return false;
        }
        const uint8_t kind = data[pos++];
        const int blockX = block / BLOCKS;
        const int blockY = block % BLOCKS;
        if (kind == 0) { // flat: one 2 byte height for 64 cells
            if (pos + 2 > data.size()) {
                std::printf("FATAL: truncated flat block %d\n", block);
                return false;
            }
            const int16_t h = static_cast<int16_t>(le16(&data[pos]));
            pos += 2;
            for (int cell = 0; cell < 64; ++cell) {
                const int cx = blockX * 8 + cell / 8;
                const int cy = blockY * 8 + cell % 8;
                const size_t idx = static_cast<size_t>(cx) * REGION_CELLS +
                                   static_cast<size_t>(cy);
                out.cellOff[idx] = static_cast<uint32_t>(out.layers.size());
                out.cellCnt[idx] = 1;
                out.cellIndexOf.push_back(static_cast<uint32_t>(idx));
                out.layers.push_back(CellLayer{h, 0x0F});
            }
        } else if (kind == 1) { // complex: 64 words
            if (pos + 128 > data.size()) {
                std::printf("FATAL: truncated complex block %d\n", block);
                return false;
            }
            for (int cell = 0; cell < 64; ++cell) {
                const uint16_t w = le16(&data[pos]);
                pos += 2;
                const int cx = blockX * 8 + cell / 8;
                const int cy = blockY * 8 + cell % 8;
                const size_t idx = static_cast<size_t>(cx) * REGION_CELLS +
                                   static_cast<size_t>(cy);
                out.cellOff[idx] = static_cast<uint32_t>(out.layers.size());
                out.cellCnt[idx] = 1;
                out.cellIndexOf.push_back(static_cast<uint32_t>(idx));
                out.layers.push_back(CellLayer{
                    static_cast<int16_t>(w & 0xFFF0) >> 1,
                    static_cast<uint8_t>(w & 0x0F)});
            }
        } else if (kind == 2) { // multilayer: per cell count + words
            for (int cell = 0; cell < 64; ++cell) {
                if (pos >= data.size()) {
                    std::printf("FATAL: truncated multilayer block %d\n",
                                block);
                    return false;
                }
                const int count = data[pos++];
                if (count == 0 || count > 125) {
                    std::printf("FATAL: layer count %d at block %d\n", count,
                                block);
                    return false;
                }
                if (pos + static_cast<size_t>(count) * 2 > data.size()) {
                    std::printf("FATAL: truncated multilayer cell\n");
                    return false;
                }
                const int cx = blockX * 8 + cell / 8;
                const int cy = blockY * 8 + cell % 8;
                const size_t idx = static_cast<size_t>(cx) * REGION_CELLS +
                                   static_cast<size_t>(cy);
                out.cellOff[idx] = static_cast<uint32_t>(out.layers.size());
                out.cellCnt[idx] = static_cast<uint16_t>(count);
                for (int i = 0; i < count; ++i) {
                    const uint16_t w = le16(&data[pos]);
                    pos += 2;
                    out.cellIndexOf.push_back(static_cast<uint32_t>(idx));
                    out.layers.push_back(CellLayer{
                        static_cast<int16_t>(w & 0xFFF0) >> 1,
                        static_cast<uint8_t>(w & 0x0F)});
                }
            }
        } else {
            std::printf("FATAL: invalid block kind %d at %zu\n", kind,
                        pos - 1);
            return false;
        }
    }
    if (pos != data.size()) {
        std::printf("FATAL: %zu trailing bytes\n", data.size() - pos);
        return false;
    }
    for (int i = 0; i < cells; ++i) {
        const uint32_t off = out.cellOff[static_cast<size_t>(i)];
        const uint16_t cnt = out.cellCnt[static_cast<size_t>(i)];
        if (cnt >= 2) {
            ++out.stackedColumns;
        }
        for (uint32_t j = off; j < off + cnt; ++j) {
            if (out.layers[j].h < WATER_LEVEL) {
                ++out.underwaterLayers;
            }
        }
    }

    return true;
}

// dedupLayers rebuilds the region layers so that every cell holds at
// most one layer per 16 unit band (the highest of the band wins: the
// surface the character stands on). Returns the layer count removed.
int dedupLayers(RegionData& rd) {
    std::vector<CellLayer> kept;
    kept.reserve(rd.layers.size());
    std::vector<uint32_t> keptCell(rd.cellIndexOf.size(), 0);
    const int cells = REGION_CELLS * REGION_CELLS;
    for (int i = 0; i < cells; ++i) {
        const uint32_t from = rd.cellOff[static_cast<size_t>(i)];
        const uint16_t cnt = rd.cellCnt[static_cast<size_t>(i)];
        rd.cellOff[static_cast<size_t>(i)] =
            static_cast<uint32_t>(kept.size());
        rd.cellCnt[static_cast<size_t>(i)] = 0;
        for (uint32_t j = from; j < from + cnt; ++j) {
            const uint16_t already = rd.cellCnt[static_cast<size_t>(i)];
            if (already > 0) {
                const CellLayer& prev =
                    kept[rd.cellOff[static_cast<size_t>(i)] + already - 1];
                if (std::abs(static_cast<int>(prev.h) -
                             static_cast<int>(rd.layers[j].h)) <
                    DEDUP_DELTA) {
                    if (rd.layers[j].h > prev.h) {
                        kept[rd.cellOff[static_cast<size_t>(i)] +
                             already - 1] = rd.layers[j];
                    }
                    continue;
                }
            }
            kept.push_back(rd.layers[j]);
            keptCell[kept.size() - 1] = static_cast<uint32_t>(i);
            ++rd.cellCnt[static_cast<size_t>(i)];
        }
    }
    const int removed = static_cast<int>(rd.layers.size()) -
                        static_cast<int>(kept.size());
    rd.layers = std::move(kept);
    rd.cellIndexOf = std::move(keptCell);

    return removed;
}

// cellLayers returns the layer slice of a local cell.
const CellLayer* cellLayers(const RegionData& rd, int cx, int cy,
                            int& count) {
    const size_t idx = static_cast<size_t>(cx) * REGION_CELLS +
                       static_cast<size_t>(cy);
    const uint32_t off = rd.cellOff[idx];
    count = static_cast<int>(rd.cellCnt[idx]);
    return &rd.layers[off];
}

// The audits run over the parsed region before the mesh build.
struct Audits {
    // Span pairs the height-only import would connect although the
    // NSWE walls block them: the invisible-wall fidelity loss.
    int nsweLost = 0;
    // Connected span pairs where at least one side carries a span
    // above it: the stacked-surface merge hazards.
    int mergeHazards = 0;
    // Samples for the report.
    std::vector<int> nsweSamples;
    std::vector<int> hazardSamples;
};

// nsweBlocked reports whether the passage from cell (x,y) layer a to
// the neighbour in dx,dy with layer b is walled, mirroring the
// wallsOpen rule of the swarm search.
bool nsweBlocked(const CellLayer& from, const CellLayer& to, int dx,
                 int dy) {
    const uint8_t fromOpen = from.nswe;
    const uint8_t toOpen = to.nswe;
    if (dy < 0 && !(fromOpen & 0x08)) return true; // north
    if (dy > 0 && !(fromOpen & 0x04)) return true; // south
    if (dx > 0 && !(fromOpen & 0x01)) return true; // east
    if (dx < 0 && !(fromOpen & 0x02)) return true; // west
    if (dy < 0 && !(toOpen & 0x04)) return true;   // reverse south
    if (dy > 0 && !(toOpen & 0x08)) return true;   // reverse north
    if (dx > 0 && !(toOpen & 0x02)) return true;   // reverse west
    if (dx < 0 && !(toOpen & 0x01)) return true;   // reverse east
    return false;
}

void runAudits(const RegionData& rd, Audits& audits) {
    static constexpr int MAX_CLIMB = 40;
    for (int cy = 0; cy < REGION_CELLS; ++cy) {
        for (int cx = 0; cx < REGION_CELLS; ++cx) {
            int countA = 0;
            const CellLayer* a = cellLayers(rd, cx, cy, countA);
            const int dxs[2] = {1, 0};
            const int dys[2] = {0, 1};
            for (int d = 0; d < 2; ++d) {
                const int nx = cx + dxs[d];
                const int ny = cy + dys[d];
                if (nx >= REGION_CELLS || ny >= REGION_CELLS) {
                    continue;
                }
                int countB = 0;
                const CellLayer* b = cellLayers(rd, nx, ny, countB);
                for (int i = 0; i < countA; ++i) {
                    for (int j = 0; j < countB; ++j) {
                        const int diff = std::abs(
                            static_cast<int>(a[i].h) -
                            static_cast<int>(b[j].h));
                        if (diff > MAX_CLIMB) {
                            continue;
                        }
                        // Both layers fully blocked cells never become
                        // spans at all; partially walled ones do.
                        if (a[i].nswe != 0 && b[j].nswe != 0 &&
                            nsweBlocked(a[i], b[j], dxs[d], dys[d])) {
                            ++audits.nsweLost;
                            if (audits.nsweSamples.size() < 8) {
                                audits.nsweSamples.push_back(
                                    cx * 65536 * 16 + cy);
                            }
                        }
                        if (i < countA - 1 || j < countB - 1) {
                            ++audits.mergeHazards;
                            if (audits.hazardSamples.size() < 8) {
                                audits.hazardSamples.push_back(
                                    cx * 65536 * 16 + cy);
                            }
                        }
                    }
                }
            }
        }
    }
}

// The bridge finder: walkable deck cells with water directly below
// whose lateral neighbours do not carry a deck-with-water - narrow
// strips are the bridges of the floating village.
struct BridgeSpot {
    int cx = 0;
    int cy = 0;
    int deckH = 0;
    int waterH = 0;
};

void findBridges(const RegionData& rd, std::vector<BridgeSpot>& out) {
    for (int cy = 8; cy < REGION_CELLS - 8; ++cy) {
        for (int cx = 8; cx < REGION_CELLS - 8; ++cx) {
            int count = 0;
            const CellLayer* ls = cellLayers(rd, cx, cy, count);
            if (count < 2) {
                continue;
            }
            int deckIdx = -1;
            int waterIdx = -1;
            for (int i = 0; i < count; ++i) {
                if (ls[i].h > WATER_LEVEL && ls[i].nswe != 0) {
                    deckIdx = i;
                }
                if (ls[i].h < WATER_LEVEL && ls[i].nswe != 0) {
                    waterIdx = i;
                }
            }
            if (deckIdx < 0 || waterIdx < 0) {
                continue;
            }
            // The deck must ride clearly above the water and have no
            // walkable layer in between (a pure two surface column).
            int between = 0;
            for (int i = 0; i < count; ++i) {
                if (ls[i].h < ls[deckIdx].h && ls[i].h > ls[waterIdx].h &&
                    ls[i].nswe != 0) {
                    ++between;
                }
            }
            if (between > 0) {
                continue;
            }
            // Narrow in x: the x neighbours carry no deck-with-water.
            int narrow = 0;
            for (int dx = -1; dx <= 1; dx += 2) {
                int nc = 0;
                const CellLayer* nls = cellLayers(rd, cx + dx, cy, nc);
                bool hasDeck = false;
                for (int i = 0; i < nc; ++i) {
                    if (nls[i].h > WATER_LEVEL && nls[i].nswe != 0) {
                        hasDeck = true;
                    }
                }
                if (!hasDeck) {
                    ++narrow;
                }
            }
            if (narrow == 0) {
                continue;
            }
            out.push_back(BridgeSpot{cx, cy, ls[deckIdx].h, ls[waterIdx].h});
        }
    }
}

// worldOf converts local cell coordinates + height to world space.
void worldOf(int cx, int cy, int h, float* out) {
    out[0] = REGION_MIN_X + cx * CELL_SIZE + CELL_SIZE / 2;
    out[1] = static_cast<float>(h);
    out[2] = REGION_MIN_Z + cy * CELL_SIZE + CELL_SIZE / 2;
}

// closestHeightOf picks the layer height closest to refZ.
int closestHeightOf(const RegionData& rd, int cx, int cy, int refZ) {
    int count = 0;
    const CellLayer* ls = cellLayers(rd, cx, cy, count);
    int best = ls[0].h;
    for (int i = 1; i < count; ++i) {
        if (std::abs(static_cast<int>(ls[i].h) - refZ) <
            std::abs(best - refZ)) {
            best = ls[i].h;
        }
    }
    return best;
}

// The sheet decomposition: walkable layers partitioned into 2D
// manifolds. sheetOf is indexed like rd.layers (the global layer
// order), one sheet id per layer instance.
struct SheetData {
    std::vector<int32_t> sheetOf;
    int sheetCount = 0;
    std::vector<uint8_t> sheetClass;  // 0 ground, 1 water
    std::vector<uint8_t> sheetArea;   // the Recast area id per sheet
    std::vector<bool> sheetDropped;   // the unreachable island filter
};

// assignSheets floods the sheets top down: the highest unassigned
// layer seeds a new sheet, the fill moves to the neighbour column
// layers within the climb range of the same wetness class whenever
// the sheet does not already occupy the neighbour column. The NSWE
// walls are deliberately ignored here: the sheets are geometric
// surfaces, the walls become poly links that the validation step
// filters afterwards.
void assignSheets(const RegionData& rd, SheetData& out) {
    const size_t layerCount = rd.layers.size();
    out.sheetOf.assign(layerCount, -1);
    // Bucket the layer indices by height (heights are multiples of 8,
    // the voxel grid covers [-8192, 8192)).
    std::vector<std::vector<uint32_t>> buckets(2048);
    for (size_t j = 0; j < layerCount; ++j) {
        const int bucket = (static_cast<int>(rd.layers[j].h) + 8192) / 8;
        if (rd.layers[j].nswe != 0) {
            buckets[static_cast<size_t>(bucket)].push_back(
                static_cast<uint32_t>(j));
        }
    }
    std::vector<uint32_t> stack;
    for (int bucket = 2047; bucket >= 0; --bucket) {
        for (const uint32_t seed : buckets[static_cast<size_t>(bucket)]) {
            if (out.sheetOf[seed] >= 0) {
                continue;
            }
            const int sheet = out.sheetCount++;
            out.sheetOf[seed] = sheet;
            stack.clear();
            stack.push_back(seed);
            while (!stack.empty()) {
                const uint32_t cur = stack.back();
                stack.pop_back();
                // The column of the current layer.
                const size_t idx = rd.indexOf(cur);
                const int cx = static_cast<int>(idx / REGION_CELLS);
                const int cy = static_cast<int>(idx % REGION_CELLS);
                const int h = rd.layers[cur].h;
                const uint8_t wet =
                    h < WATER_LEVEL ? 1 : 0;
                const int dxs[4] = {1, -1, 0, 0};
                const int dys[4] = {0, 0, 1, -1};
                for (int d = 0; d < 4; ++d) {
                    const int nx = cx + dxs[d];
                    const int ny = cy + dys[d];
                    if (nx < 0 || ny < 0 || nx >= REGION_CELLS ||
                        ny >= REGION_CELLS) {
                        continue;
                    }
                    const size_t nIdx = static_cast<size_t>(nx) *
                                            REGION_CELLS +
                                        static_cast<size_t>(ny);
                    const uint32_t off = rd.cellOff[nIdx];
                    const uint16_t cnt = rd.cellCnt[nIdx];
                    for (uint32_t k = off; k < off + cnt; ++k) {
                        if (out.sheetOf[k] >= 0 ||
                            rd.layers[k].nswe == 0) {
                            continue;
                        }
                        if ((rd.layers[k].h < WATER_LEVEL ? 1 : 0) != wet) {
                            continue;
                        }
                        const int diff = std::abs(
                            static_cast<int>(rd.layers[k].h) - h);
                        if (diff > 40) {
                            continue;
                        }
                        // The sheet may occupy the neighbour column
                        // only once: skip when it already holds a
                        // layer there.
                        bool occupied = false;
                        for (uint32_t m = off; m < off + cnt; ++m) {
                            if (out.sheetOf[m] == sheet) {
                                occupied = true;
                                break;
                            }
                        }
                        if (occupied) {
                            continue;
                        }
                        out.sheetOf[k] = sheet;
                        stack.push_back(k);
                    }
                }
            }
        }
    }
    out.sheetClass.assign(static_cast<size_t>(out.sheetCount), 0);
    std::vector<uint32_t> sizes(static_cast<size_t>(out.sheetCount), 0);
    for (size_t j = 0; j < layerCount; ++j) {
        const int32_t sheet = out.sheetOf[j];
        if (sheet >= 0) {
            if (rd.layers[j].h < WATER_LEVEL) {
                out.sheetClass[static_cast<size_t>(sheet)] = 1;
            }
            ++sizes[static_cast<size_t>(sheet)];
        }
    }
    // The island filter: sheets no walk can reach (their fill found
    // no within-climb neighbour) become dropped - their layers join
    // no span and no region.
    size_t droppedSheets = 0;
    uint64_t droppedLayers = 0;
    out.sheetDropped.assign(static_cast<size_t>(out.sheetCount), false);
    for (int s = 0; s < out.sheetCount; ++s) {
        if (sizes[static_cast<size_t>(s)] <
            static_cast<uint32_t>(MIN_SHEET_LAYERS)) {
            out.sheetDropped[static_cast<size_t>(s)] = true;
            ++droppedSheets;
            droppedLayers += sizes[static_cast<size_t>(s)];
        }
    }
    std::printf("sheet drop: %zu islands with %llu layers removed"
                " (< %d layers)\n",
                droppedSheets,
                static_cast<unsigned long long>(droppedLayers),
                MIN_SHEET_LAYERS);
}

// colorSheets assigns Recast area ids so that column adjacent sheets
// never share an id (the region flood fill would merge them again).
// Ground sheets use 47..62, water sheets 31..46: any ground/water pair
// differs by construction, the palettes leave 16 colors per class.
bool colorSheets(const RegionData& rd, SheetData& sheets) {
    const int n = sheets.sheetCount;
    std::vector<std::vector<int32_t>> adj(static_cast<size_t>(n));
    for (int cy = 0; cy < REGION_CELLS; ++cy) {
        for (int cx = 0; cx < REGION_CELLS; ++cx) {
            const int dxs[2] = {1, 0};
            const int dys[2] = {0, 1};
            for (int d = 0; d < 2; ++d) {
                const int nx = cx + dxs[d];
                const int ny = cy + dys[d];
                if (nx >= REGION_CELLS || ny >= REGION_CELLS) {
                    continue;
                }
                const size_t a = static_cast<size_t>(cx) * REGION_CELLS +
                                 static_cast<size_t>(cy);
                const size_t b = static_cast<size_t>(nx) * REGION_CELLS +
                                 static_cast<size_t>(ny);
                const uint32_t offA = rd.cellOff[a];
                const uint32_t offB = rd.cellOff[b];
                const uint16_t cntA = rd.cellCnt[a];
                const uint16_t cntB = rd.cellCnt[b];
                for (uint32_t i = offA; i < offA + cntA; ++i) {
                    const int32_t sa = sheets.sheetOf[i];
                    if (sa < 0 || sheets.sheetDropped[
                                        static_cast<size_t>(sa)]) {
                        continue;
                    }
                    for (uint32_t j = offB; j < offB + cntB; ++j) {
                        const int32_t sb = sheets.sheetOf[j];
                        if (sb < 0 || sb == sa ||
                            sheets.sheetDropped[static_cast<size_t>(sb)]) {
                            continue;
                        }
                        adj[static_cast<size_t>(sa)].push_back(sb);
                        adj[static_cast<size_t>(sb)].push_back(sa);
                    }
                }
            }
        }
    }
    for (auto& list : adj) {
        std::sort(list.begin(), list.end());
        list.erase(std::unique(list.begin(), list.end()), list.end());
    }
    sheets.sheetArea.assign(static_cast<size_t>(n), 0);
    for (int s = 0; s < n; ++s) {
        if (sheets.sheetDropped[static_cast<size_t>(s)]) {
            continue;
        }
        const bool wet = sheets.sheetClass[static_cast<size_t>(s)] != 0;
        const int base = wet ? 31 : 47;
        const int colors = 16;
        bool used[16] = {};
        for (const int32_t nb : adj[static_cast<size_t>(s)]) {
            if (sheets.sheetDropped[static_cast<size_t>(nb)]) {
                continue;
            }
            const int area = sheets.sheetArea[static_cast<size_t>(nb)];
            if (area >= base && area < base + colors) {
                used[area - base] = true;
            }
        }
        int color = -1;
        for (int c = 0; c < colors; ++c) {
            if (!used[c]) {
                color = c;
                break;
            }
        }
        if (color < 0) {
            std::printf("FATAL: sheet coloring exhausted (%d sheets)\n", n);
            return false;
        }
        sheets.sheetArea[static_cast<size_t>(s)] =
            static_cast<uint8_t>(base + color);
    }

    return true;
}

// validateLinks kills the poly links whose two surfaces sit further
// than the climb range apart at the shared edge: without this the
// stacked sheets would teleport characters between floors.
int validateLinks(dtNavMesh& mesh) {
    const dtMeshTile* tile = mesh.getTileAt(0, 0, 0);
    if (!tile) {
        return -1;
    }
    const int n = tile->header->polyCount;
    int killed = 0;
    for (int i = 0; i < n; ++i) {
        dtPoly* poly = &tile->polys[i];
        unsigned int prev = DT_NULL_LINK;
        for (unsigned int li = poly->firstLink; li != DT_NULL_LINK;
             /* advance in the loop body */) {
            const dtLink& link = tile->links[li];
            bool kill = false;
            if (link.side == 0xff && link.ref) {
                const dtMeshTile* nbTile = nullptr;
                const dtPoly* nb = nullptr;
                if (dtStatusSucceed(mesh.getTileAndPolyByRef(
                        link.ref, &nbTile, &nb)) &&
                    nbTile == tile && nb) {
                    // The neighbour edge that points back at us.
                    const int selfIdx = static_cast<int>(poly - tile->polys);
                    for (unsigned int nj = 0;
                         nj < static_cast<unsigned int>(nb->vertCount);
                         ++nj) {
                        const unsigned int back = nb->neis[nj] & 0x7fffffff;
                        if (back != 0 &&
                            static_cast<int>(back) - 1 == selfIdx) {
                            // Both polys keep their own copy of the
                            // shared edge with their own surface z.
                            const int vj = (link.edge + 1) % poly->vertCount;
                            const float* va =
                                &tile->verts[poly->verts[link.edge] * 3];
                            const float* vb =
                                &tile->verts[poly->verts[vj] * 3];
                            const float* na =
                                &tile->verts[nb->verts[nj] * 3];
                            const float* nbv =
                                &tile->verts[nb->verts[(nj + 1) %
                                                       nb->vertCount] * 3];
                            const float zA = (va[1] + vb[1]) * 0.5f;
                            const float zB = (na[1] + nbv[1]) * 0.5f;
                            if (std::abs(zA - zB) > 48.0f) {
                                kill = true;
                            }
                            break;
                        }
                    }
                }
            }
            if (kill) {
                // Splice the link out of the poly's list.
                if (prev == DT_NULL_LINK) {
                    poly->firstLink = link.next;
                } else {
                    tile->links[prev].next = link.next;
                }
                ++killed;
                li = link.next;
            } else {
                prev = li;
                li = link.next;
            }
        }
    }
    return killed;
}

struct BuildResult {
    unsigned char* data = nullptr;
    int dataSize = 0;
    rcHeightfield* hf = nullptr;
    rcCompactHeightfield* chf = nullptr;
    rcContourSet* cset = nullptr;
    rcPolyMesh* pmesh = nullptr;
    rcPolyMeshDetail* dmesh = nullptr;
    int waterPolys = 0;
};

// buildNavMesh imports the sheet colored layers into the Recast
// pipeline.
bool buildNavMesh(LogContext& ctx, const RegionData& rd,
                  const SheetData& sheets, BuildResult& out) {
    out.hf = rcAllocHeightfield();
    if (!rcCreateHeightfield(&ctx, *out.hf, REGION_CELLS, REGION_CELLS,
                             WORLD_MIN, WORLD_MAX, CELL_SIZE, CELL_HEIGHT)) {
        return false;
    }
    for (int cy = 0; cy < REGION_CELLS; ++cy) {
        for (int cx = 0; cx < REGION_CELLS; ++cx) {
            const size_t idx0 = static_cast<size_t>(cx) * REGION_CELLS +
                                static_cast<size_t>(cy);
            int count = 0;
            const CellLayer* ls = cellLayers(rd, cx, cy, count);
            for (int i = 0; i < count; ++i) {
                if (ls[i].nswe == 0) {
                    continue; // a fully blocked cell never walks
                }
                const uint32_t layerIdx = rd.cellOff[idx0] +
                                          static_cast<uint32_t>(i);
                const int32_t sheet = sheets.sheetOf[layerIdx];
                if (sheet < 0 || sheets.sheetDropped[
                                      static_cast<size_t>(sheet)]) {
                    continue;
                }
                const uint8_t area = sheets.sheetArea[
                    static_cast<size_t>(sheet)];
                const int vox = (static_cast<int>(ls[i].h) + 8192) / 8;
                if (!rcAddSpan(&ctx, *out.hf, cx, cy,
                               static_cast<unsigned short>(vox - 1),
                               static_cast<unsigned short>(vox), area, 0)) {
                    return false;
                }
            }
        }
    }
    out.chf = rcAllocCompactHeightfield();
    if (!rcBuildCompactHeightfield(&ctx, WALKABLE_HEIGHT_VOX, VOX_CLIMB,
                                   *out.hf, *out.chf)) {
        return false;
    }
    if (!rcBuildDistanceField(&ctx, *out.chf)) {
        return false;
    }
    bool ok = false;
#ifdef REGIONS_MONOTONE
    ok = rcBuildRegionsMonotone(&ctx, *out.chf, 0, MIN_REGION_AREA,
                                MERGE_REGION_AREA);
#else
    ok = rcBuildRegions(&ctx, *out.chf, 0, MIN_REGION_AREA,
                        MERGE_REGION_AREA);
#endif
    if (!ok) {
        return false;
    }
    out.cset = rcAllocContourSet();
    if (!rcBuildContours(&ctx, *out.chf, CONTOUR_MAX_ERROR, 12, *out.cset)) {
        return false;
    }
    out.pmesh = rcAllocPolyMesh();
    if (!rcBuildPolyMesh(&ctx, *out.cset, 6, *out.pmesh)) {
        return false;
    }
    out.dmesh = rcAllocPolyMeshDetail();
    if (!rcBuildPolyMeshDetail(&ctx, *out.pmesh, *out.chf, CELL_SIZE * 6.0f,
                               CELL_HEIGHT, *out.dmesh)) {
        return false;
    }
    std::vector<unsigned short> flags(static_cast<size_t>(out.pmesh->npolys));
    for (int i = 0; i < out.pmesh->npolys; ++i) {
        const uint8_t area = out.pmesh->areas[i];
        if (area >= 31 && area < 47) {
            flags[static_cast<size_t>(i)] = POLYFLAGS_SWIM;
            ++out.waterPolys;
        } else {
            flags[static_cast<size_t>(i)] = POLYFLAGS_WALK;
        }
    }
    dtNavMeshCreateParams params;
    std::memset(&params, 0, sizeof(params));
    params.verts = out.pmesh->verts;
    params.vertCount = out.pmesh->nverts;
    params.polys = out.pmesh->polys;
    params.polyFlags = flags.data();
    params.polyAreas = out.pmesh->areas;
    params.polyCount = out.pmesh->npolys;
    params.nvp = out.pmesh->nvp;
    params.detailMeshes = out.dmesh->meshes;
    params.detailVerts = out.dmesh->verts;
    params.detailVertsCount = out.dmesh->nverts;
    params.detailTris = out.dmesh->tris;
    params.detailTriCount = out.dmesh->ntris;
    params.walkableHeight = WALKABLE_HEIGHT_VOX * CELL_HEIGHT;
    params.walkableRadius = 8.0f;
    params.walkableClimb = VOX_CLIMB * CELL_HEIGHT;
    params.cs = CELL_SIZE;
    params.ch = CELL_HEIGHT;
    std::memcpy(params.bmin, WORLD_MIN, sizeof(float) * 3);
    std::memcpy(params.bmax, WORLD_MAX, sizeof(float) * 3);
    params.buildBvTree = true;
    if (!dtCreateNavMeshData(&params, &out.data, &out.dataSize)) {
        return false;
    }

    return true;
}

// graphAudit checks the poly link graph health: one-way links and
// isolated polys are the silent killers of a stacked-surface mesh.
void graphAudit(dtNavMesh& mesh, std::FILE* report) {
    const dtMeshTile* tile = mesh.getTileAt(0, 0, 0);
    if (!tile) {
        std::fprintf(report, "graph audit: no tile\n");
        return;
    }
    const int n = tile->header->polyCount;
    std::vector<std::vector<int>> adj(static_cast<size_t>(n));
    int oneWay = 0;
    for (int i = 0; i < n; ++i) {
        const dtPoly* p = &tile->polys[i];
        for (unsigned int li = p->firstLink; li != DT_NULL_LINK;
             li = tile->links[li].next) {
            const dtPolyRef nb = tile->links[li].ref;
            if (!nb) {
                continue;
            }
            const dtMeshTile* nbTile = nullptr;
            const dtPoly* nbPoly = nullptr;
            if (dtStatusFailed(mesh.getTileAndPolyByRef(nb, &nbTile,
                                                        &nbPoly)) ||
                !nbPoly) {
                continue;
            }
            const int nbIdx = static_cast<int>(nbPoly - nbTile->polys);
            adj[static_cast<size_t>(i)].push_back(nbIdx);
        }
    }
    for (int i = 0; i < n; ++i) {
        for (const int nb : adj[static_cast<size_t>(i)]) {
            bool back = false;
            for (const int back2 : adj[static_cast<size_t>(nb)]) {
                if (back2 == i) {
                    back = true;
                    break;
                }
            }
            if (!back) {
                ++oneWay;
            }
        }
    }
    // The largest weakly connected component.
    std::vector<int> component(static_cast<size_t>(n), -1);
    int components = 0;
    int largest = 0;
    for (int i = 0; i < n; ++i) {
        if (component[static_cast<size_t>(i)] >= 0) {
            continue;
        }
        std::vector<int> stack;
        stack.push_back(i);
        component[static_cast<size_t>(i)] = components;
        int size = 0;
        while (!stack.empty()) {
            const int cur = stack.back();
            stack.pop_back();
            ++size;
            for (const int nb : adj[static_cast<size_t>(cur)]) {
                if (component[static_cast<size_t>(nb)] < 0) {
                    component[static_cast<size_t>(nb)] = components;
                    stack.push_back(nb);
                }
            }
        }
        largest = std::max(largest, size);
        ++components;
    }
    std::fprintf(report,
                 "graph audit: %d polys, %d one-way links, %d components"
                 " (largest %d)\n",
                 n, oneWay, components, largest);
}

// contourPoints sums the vertex counts of every traced contour.
int contourPoints(const rcContourSet& cset) {
    int total = 0;
    for (int i = 0; i < cset.nconts; ++i) {
        total += cset.conts[i].nverts;
    }
    return total;
}

// describePoly prints a poly's area and representative height.
void describePoly(const dtNavMesh& mesh, const dtPolyRef ref) {
    const dtMeshTile* tile = nullptr;
    const dtPoly* poly = nullptr;
    if (dtStatusFailed(mesh.getTileAndPolyByRef(ref, &tile, &poly)) ||
        !poly) {
        std::printf("poly ?");
        return;
    }
    const float* v = &tile->verts[poly->verts[0] * 3];
    std::printf("poly %llu area=%s z~%.0f",
                static_cast<unsigned long long>(ref),
                poly->getArea() >= 31 && poly->getArea() < 47 ? "WATER"
                                                              : "GROUND",
                v[1]);
}

} // namespace

int main(int argc, char** argv) {
    std::setvbuf(stdout, nullptr, _IONBF, 0);
    const char* regionPath =
        argc > 1 ? argv[1] : "../../data/geodata/21_19.l2j";
    const char* pairsPath =
        argc > 2 ? argv[2] : "results/query_pairs_21_19.txt";
    const char* tilePath =
        argc > 3 ? argv[3] : "results/navmesh_21_19.bin";
    std::printf("region file: %s\n", regionPath);

    RegionData rd;
    const auto parseBegan = std::chrono::steady_clock::now();
    if (!parseRegion(regionPath, rd)) {
        return 1;
    }
    const double parseMs = std::chrono::duration<double, std::milli>(
                                std::chrono::steady_clock::now() -
                                parseBegan)
                                .count();
    const int deduped = dedupLayers(rd);
    {
        // Recompute the quick stats after the dedup.
        int stacked = 0;
        int underwater = 0;
        const int cells = REGION_CELLS * REGION_CELLS;
        for (int i = 0; i < cells; ++i) {
            if (rd.cellCnt[static_cast<size_t>(i)] >= 2) {
                ++stacked;
            }
            const uint32_t off = rd.cellOff[static_cast<size_t>(i)];
            const uint16_t cnt = rd.cellCnt[static_cast<size_t>(i)];
            for (uint32_t j = off; j < off + cnt; ++j) {
                if (rd.layers[j].h < WATER_LEVEL) {
                    ++underwater;
                }
            }
        }
        rd.stackedColumns = stacked;
        rd.underwaterLayers = underwater;
    }
    std::printf("parse: %.0f ms, %zu layers after dedup (%d duplicates"
                " dropped), %d stacked columns, %d underwater layers\n",
                parseMs, rd.layers.size(), deduped, rd.stackedColumns,
                rd.underwaterLayers);

    Audits audits;
    const auto auditBegan = std::chrono::steady_clock::now();
    runAudits(rd, audits);
    const double auditMs = std::chrono::duration<double, std::milli>(
                                std::chrono::steady_clock::now() - auditBegan)
                                .count();
    std::printf("audits: %.0f ms, nswe-lost pairs %d, merge hazards %d\n",
                auditMs, audits.nsweLost, audits.mergeHazards);

    std::vector<BridgeSpot> bridges;
    findBridges(rd, bridges);
    std::printf("bridge-like deck-over-water columns: %zu\n", bridges.size());

    SheetData sheets;
    const auto sheetBegan = std::chrono::steady_clock::now();
    assignSheets(rd, sheets);
    const double sheetMs = std::chrono::duration<double, std::milli>(
                                std::chrono::steady_clock::now() -
                                sheetBegan)
                                .count();
    {
        std::vector<uint32_t> sizes(static_cast<size_t>(sheets.sheetCount),
                                    0);
        for (const int32_t sheet : sheets.sheetOf) {
            if (sheet >= 0) {
                ++sizes[static_cast<size_t>(sheet)];
            }
        }
        std::sort(sizes.begin(), sizes.end(),
                  std::greater<uint32_t>());
        size_t big100 = 0;
        size_t big1000 = 0;
        size_t big10000 = 0;
        uint64_t covered = 0;
        for (const uint32_t sz : sizes) {
            if (sz >= 100) {
                ++big100;
            }
            if (sz >= 1000) {
                ++big1000;
            }
            if (sz >= 10000) {
                ++big10000;
            }
            covered += sz;
        }
        std::printf("sheet sizes: largest");
        for (size_t i = 0; i < 5 && i < sizes.size(); ++i) {
            std::printf(" %u", sizes[i]);
        }
        std::printf(", sheets>=100: %zu, >=1000: %zu, >=10000: %zu,"
                    " covered layers %llu\n",
                    big100, big1000, big10000,
                    static_cast<unsigned long long>(covered));
        {
            uint64_t smallCover = 0;
            uint64_t smallCover4 = 0;
            uint64_t smallCover16 = 0;
            uint64_t smallCover64 = 0;
            size_t count1 = 0;
            for (const uint32_t sz : sizes) {
                if (sz < 4) {
                    ++count1;
                }
                if (sz < 4) {
                    smallCover4 += sz;
                }
                if (sz < 16) {
                    smallCover16 += sz;
                }
                if (sz < 64) {
                    smallCover64 += sz;
                }
                if (sz < 100) {
                    smallCover += sz;
                }
            }
            std::printf("    tiny sheet coverage: <4: %llu layers (%zu"
                        " sheets), <16: %llu, <64: %llu, <100: %llu\n",
                        static_cast<unsigned long long>(smallCover4),
                        count1,
                        static_cast<unsigned long long>(smallCover16),
                        static_cast<unsigned long long>(smallCover64),
                        static_cast<unsigned long long>(smallCover));
        }
    }
    std::printf("sheets: %.0f ms, %d sheets (%d water class)\n", sheetMs,
                sheets.sheetCount,
                static_cast<int>(std::count(sheets.sheetClass.begin(),
                                            sheets.sheetClass.end(), 1)));
    if (!colorSheets(rd, sheets)) {
        return 1;
    }

    LogContext ctx;
    BuildResult build;
    const auto buildBegan = std::chrono::steady_clock::now();
    if (!buildNavMesh(ctx, rd, sheets, build)) {
        std::printf("FATAL: navmesh build failed\n");
        return 1;
    }
    const double buildMs = std::chrono::duration<double, std::milli>(
                                std::chrono::steady_clock::now() - buildBegan)
                                .count();
    std::printf(
        "build: %.0f ms total, compact spans %d, %d regions, %d polys"
        " (%d water), %d verts, %d contour points, navmesh data %d"
        " bytes\n",
        buildMs, build.chf->spanCount, build.chf->maxRegions,
        build.pmesh->npolys, build.waterPolys, build.pmesh->nverts,
        contourPoints(*build.cset), build.dataSize);
    std::printf("build rc timer total: %.0f ms\n",
                ctx.getAccumulatedTime(RC_TIMER_TOTAL) / 1000.0);

    // Export the raw Detour tile for the Go runtime prototype.
    {
        std::FILE* tile = std::fopen(tilePath, "wb");
        if (!tile ||
            std::fwrite(build.data, 1, static_cast<size_t>(build.dataSize),
                        tile) != static_cast<size_t>(build.dataSize)) {
            std::printf("FATAL: cannot write %s\n", tilePath);
            return 1;
        }
        std::fclose(tile);
        std::printf("tile exported: %s (%d bytes)\n", tilePath,
                    build.dataSize);
    }

    dtNavMesh mesh;
    if (dtStatusFailed(mesh.init(build.data, build.dataSize, 0))) {
        std::printf("FATAL: dtNavMesh::init failed\n");
        return 1;
    }
    dtNavMeshQuery query;
    if (dtStatusFailed(query.init(&mesh, 8192))) {
        std::printf("FATAL: dtNavMeshQuery::init failed\n");
        return 1;
    }

    const int killedLinks = validateLinks(mesh);
    std::printf("link validation: killed %d vertical links\n",
                killedLinks);
    graphAudit(mesh, stdout);

    dtQueryFilter permissive;
    permissive.setIncludeFlags(POLYFLAGS_WALK | POLYFLAGS_SWIM);
    for (int area = 31; area < 47; ++area) {
        permissive.setAreaCost(area, 3.0f); // water sheets swim slower
    }
    for (int area = 47; area < 63; ++area) {
        permissive.setAreaCost(area, 1.0f);
    }
    dtQueryFilter dry;
    dry.setIncludeFlags(POLYFLAGS_WALK);
    dry.setAreaCost(AREA_GROUND, 1.0f);

    // The village start: the temp10 dump cell of the acceptance round.
    const int villageCx = 812; // world 45768
    const int villageCy = 1067; // world 49848
    const int villageH = closestHeightOf(rd, villageCx, villageCy, -3056);
    float village[3];
    worldOf(villageCx, villageCy, villageH, village);
    std::printf("village start: (%.0f, %.0f, %.0f)\n", village[0],
                village[1], village[2]);

    // The bridge target: the bridge-like column closest to the village.
    int bestIdx = -1;
    double bestDist = 1e18;
    for (size_t i = 0; i < bridges.size(); ++i) {
        const double dx = bridges[i].cx - villageCx;
        const double dy = bridges[i].cy - villageCy;
        const double dist = dx * dx + dy * dy;
        if (dist < bestDist) {
            bestDist = dist;
            bestIdx = static_cast<int>(i);
        }
    }
    if (bestIdx < 0) {
        std::printf("FATAL: no bridge-like column found\n");
        return 1;
    }
    const BridgeSpot& bridge = bridges[static_cast<size_t>(bestIdx)];
    float underBridge[3];
    worldOf(bridge.cx, bridge.cy, bridge.waterH, underBridge);
    float onDeck[3];
    worldOf(bridge.cx, bridge.cy, bridge.deckH, onDeck);
    std::printf("bridge column: (%.0f, %.0f) deck z %d, water z %d"
                " (delta %d)\n",
                underBridge[0], underBridge[2], bridge.deckH, bridge.waterH,
                bridge.deckH - bridge.waterH);

    // [1] The same x/y different z disambiguation on real data.
    constexpr float EXT[3] = {32.0f, 600.0f, 32.0f};
    dtPolyRef ref = 0;
    float nearest[3] = {0, 0, 0};
    query.findNearestPoly(underBridge, EXT, &permissive, &ref, nearest);
    std::printf("\n[1] nearest poly under the bridge (%.0f, %.0f, %.0f): ",
                underBridge[0], underBridge[1], underBridge[2]);
    describePoly(mesh, ref);
    std::printf(" -> nearest z %.0f\n", nearest[1]);
    query.findNearestPoly(onDeck, EXT, &permissive, &ref, nearest);
    std::printf("    nearest poly on the deck (%.0f, %.0f, %.0f): ",
                onDeck[0], onDeck[1], onDeck[2]);
    describePoly(mesh, ref);
    std::printf(" -> nearest z %.0f\n", nearest[1]);

    // [2] The village -> under-bridge route, swim allowed.
    dtPolyRef startRef = 0;
    dtPolyRef endRef = 0;
    float startPt[3];
    float endPt[3];
    query.findNearestPoly(village, EXT, &permissive, &startRef, startPt);
    query.findNearestPoly(underBridge, EXT, &permissive, &endRef, endPt);
    dtPolyRef pathRefs[256];
    int pathCount = 0;
    dtStatus status = query.findPath(startRef, endRef, startPt, endPt,
                                      &permissive, pathRefs, &pathCount,
                                      256);
    std::printf("\n[2] village -> under the bridge, swim allowed"
                " (water 3x): ");
    if (dtStatusSucceed(status)) {
        std::printf("%d polys, partial: %s\n", pathCount,
                    dtStatusDetail(status, DT_PARTIAL_RESULT) ? "yes" : "no");
        float straight[3 * 256];
        unsigned char straightFlags[256];
        dtPolyRef straightRefs[256];
        int straightCount = 0;
        query.findStraightPath(startPt, endPt, pathRefs, pathCount,
                               straight, straightFlags, straightRefs,
                               &straightCount, 256, 0);
        std::printf("    straight path %d corners:\n", straightCount);
        for (int i = 0; i < straightCount; ++i) {
            const float* p = &straight[i * 3];
            std::printf("    [%d] (%.0f, %.0f, %.0f)\n", i, p[0], p[1],
                        p[2]);
        }
    } else {
        std::printf("not found, status 0x%x\n", status);
    }

    // [3] The dry form.
    dtPolyRef dryPath[256];
    int dryCount = 0;
    status = query.findPath(startRef, endRef, startPt, endPt, &dry, dryPath,
                            &dryCount, 256);
    std::printf("\n[3] the same route with the dry filter: ");
    if (dtStatusSucceed(status)) {
        std::printf("%d polys, partial: %s\n", dryCount,
                    dtStatusDetail(status, DT_PARTIAL_RESULT) ? "yes"
                                                              : "no");
        if (dryCount > 0) {
            float bestPos[3] = {0, 0, 0};
            query.closestPointOnPoly(dryPath[dryCount - 1], underBridge,
                                     bestPos, nullptr);
            std::printf("    closest reachable dry point: (%.0f, %.0f,"
                        " %.0f)\n",
                        bestPos[0], bestPos[1], bestPos[2]);
        }
    } else {
        std::printf("not found, status 0x%x\n", status);
    }

    // [4] The water escape: under the bridge -> the village deck.
    dtQueryFilter escape;
    escape.setIncludeFlags(POLYFLAGS_WALK | POLYFLAGS_SWIM);
    for (int area = 31; area < 47; ++area) {
        escape.setAreaCost(area, 8.0f);
    }
    for (int area = 47; area < 63; ++area) {
        escape.setAreaCost(area, 1.0f);
    }
    dtPolyRef escPath[256];
    int escCount = 0;
    status = query.findPath(endRef, startRef, endPt, startPt, &escape,
                            escPath, &escCount, 256);
    std::printf("\n[4] water escape under the bridge -> village deck: ");
    if (dtStatusSucceed(status)) {
        std::printf("%d polys, partial: %s\n", escCount,
                    dtStatusDetail(status, DT_PARTIAL_RESULT) ? "yes" : "no");
    } else {
        std::printf("not found, status 0x%x\n", status);
    }

    // [5] The random pair microbenchmark + the pairs file for the Go
    // engine comparison.
    {
        std::FILE* pairs = std::fopen(pairsPath, "w");
        if (!pairs) {
            std::printf("FATAL: cannot write %s\n", pairsPath);
            return 1;
        }
        std::fprintf(pairs, "# region 21_19 pairs (Recast axis order): x1 height1 y1 x2 height2 y2\n");
        uint64_t rng = 42;
        auto nextRand = [&rng]() {
            rng = rng * 6364136223846793005ULL + 1442695040888963407ULL;
            return static_cast<uint32_t>(rng >> 33);
        };
        const int PAIRS = 200;
        int found = 0;
        double totalUs = 0;
        int reported = 0;
        for (int i = 0; i < PAIRS; ++i) {
            float pa[3];
            float pb[3];
            dtPolyRef ra = 0;
            dtPolyRef rb = 0;
            bool havePair = false;
            for (int attempt = 0; attempt < 40 && !havePair; ++attempt) {
                const int cx = 64 + static_cast<int>(nextRand() % 1920);
                const int cy = 64 + static_cast<int>(nextRand() % 1920);
                const int layer = static_cast<int>(nextRand());
                int count = 0;
                const CellLayer* ls = cellLayers(rd, cx, cy, count);
                if (count == 0 || ls[0].nswe == 0) {
                    continue;
                }
                const CellLayer& la = ls[layer % count];
                if (la.nswe == 0) {
                    continue;
                }
                worldOf(cx, cy, la.h, pa);
                const int cx2 = 64 + static_cast<int>(nextRand() % 1920);
                const int cy2 = 64 + static_cast<int>(nextRand() % 1920);
                const int layer2 = static_cast<int>(nextRand());
                int count2 = 0;
                const CellLayer* ls2 = cellLayers(rd, cx2, cy2, count2);
                if (count2 == 0 || ls2[0].nswe == 0) {
                    continue;
                }
                const CellLayer& lb = ls2[layer2 % count2];
                if (lb.nswe == 0) {
                    continue;
                }
                worldOf(cx2, cy2, lb.h, pb);
                if (!query.findNearestPoly(pa, EXT, &permissive, &ra,
                                           nearest) ||
                    !ra) {
                    continue;
                }
                if (!query.findNearestPoly(pb, EXT, &permissive, &rb,
                                           nearest) ||
                    !rb) {
                    continue;
                }
                havePair = true;
            }
            if (!havePair) {
                continue;
            }
            const auto began = std::chrono::steady_clock::now();
            const dtStatus s = query.findPath(ra, rb, pa, pb, &permissive,
                                              pathRefs, &pathCount, 256);
            float straight[3 * 256];
            unsigned char straightFlags[256];
            dtPolyRef straightRefs[256];
            int straightCount = 0;
            if (dtStatusSucceed(s)) {
                query.findStraightPath(pa, pb, pathRefs, pathCount,
                                       straight, straightFlags,
                                       straightRefs, &straightCount, 256,
                                       0);
                ++found;
            }
            totalUs += std::chrono::duration<double, std::micro>(
                           std::chrono::steady_clock::now() - began)
                           .count();
            if (reported < 0) {
                // The pairs file carries every pair (also the failed
                // ones: the Go engine must answer them the same way).
                reported += 0;
            }
            std::fprintf(pairs, "%.0f %.0f %.0f %.0f %.0f %.0f\n", pa[0],
                         pa[1], pa[2], pb[0], pb[1], pb[2]);
        }
        std::fclose(pairs);
        std::printf("\n[5] random pairs: %d/%d routable, avg %.1f us per"
                    " findPath+findStraightPath\n",
                    found, PAIRS,
                    found > 0 ? totalUs / static_cast<double>(found) : 0.0);
    }

    // [6] The hard pair microbenchmark in isolation.
    {
        constexpr int ITER = 5000;
        const auto began = std::chrono::steady_clock::now();
        float straight[3 * 256];
        unsigned char straightFlags[256];
        dtPolyRef straightRefs[256];
        int straightCount = 0;
        for (int i = 0; i < ITER; ++i) {
            query.findPath(startRef, endRef, startPt, endPt, &permissive,
                           pathRefs, &pathCount, 256);
            query.findStraightPath(startPt, endPt, pathRefs, pathCount,
                                   straight, straightFlags, straightRefs,
                                   &straightCount, 256, 0);
        }
        const double us = std::chrono::duration<double, std::micro>(
                              std::chrono::steady_clock::now() - began)
                              .count() /
                          ITER;
        std::printf("[6] the hard pair (village -> under the bridge):"
                    " %.1f us per query\n",
                    us);
    }

    rcFreeHeightField(build.hf);
    rcFreeCompactHeightfield(build.chf);
    rcFreeContourSet(build.cset);
    rcFreePolyMesh(build.pmesh);
    rcFreePolyMeshDetail(build.dmesh);
    dtFree(build.data);
    return 0;
}
