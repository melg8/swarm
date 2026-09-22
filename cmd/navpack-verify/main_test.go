// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com
//
// SPDX-License-Identifier: MIT

package main

import (
    "bytes"
    "encoding/json"
    "os"
    "path/filepath"
    "strconv"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The corpus round of the navpack-verify tool (owner issue #27): the
// tests pin the generate determinism, the replay gate (identical
// packs pass, any answer drift fails) and the two pack compare form
// of the transport round - over a synthetic two region world the
// tests build in place of the 166 region deployment pack.

// rectSpec is one rectangle polygon of a synthetic region: uniform
// height over the cell bounds.
type rectSpec struct {
    x0, y0, x1, y1 int32
    h              int16
}

// linkSpec is one link of a synthetic region: the source polygon, the
// side and the open portal span.
type linkSpec struct {
    poly   int32
    side   uint8
    to     int32
    t0, t1 int32
}

// writeRegion encodes one synthetic region tile into the pack
// directory: the full flat rectangle world with the internal links.
func writeRegion(
    t *testing.T, dir, name string, col, row int16,
    rects []rectSpec, links []linkSpec,
) {
    t.Helper()
    tile := &navmesh.Tile{
        Col: col, Row: row, Climb: 40,
        Polys:    make([]navmesh.Poly, len(rects)),
        Links:    make([]navmesh.Link, len(links)),
        ExtLinks: nil,
    }
    for i, rect := range rects {
        tile.Polys[i] = navmesh.Poly{
            X0: rect.x0, Y0: rect.y0, X1: rect.x1, Y1: rect.y1,
            H00: rect.h, H10: rect.h, H01: rect.h, H11: rect.h,
            FirstLink: -1, Area: navmesh.AreaGround,
        }
    }
    for i, spec := range links {
        tile.Links[i] = navmesh.Link{
            Side: spec.side, To: spec.to, Next: -1,
            T0: spec.t0, T1: spec.t1,
        }
        poly := &tile.Polys[spec.poly]
        if poly.FirstLink < 0 {
            poly.FirstLink = int32(i)
        } else {
            chain := poly.FirstLink
            for tile.Links[chain].Next >= 0 {
                chain = tile.Links[chain].Next
            }
            tile.Links[chain].Next = int32(i)
        }
    }
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, name), data, 0o600))
}

// flatWorld builds a pack of one flat region (21_19, the region the
// anchor set overlaps): every query inside the region answers the
// same found route over the plain rectangle.
func flatWorld(t *testing.T, h int16) string {
    t.Helper()
    dir := t.TempDir()
    writeRegion(t, dir, "21_19.nm", 21, 19,
        []rectSpec{{x0: 0, y0: 0, x1: 2048, y1: 2048, h: h}},
        []linkSpec{})

    return dir
}

// generateCorpus runs the generate mode over the pack and reads the
// written corpus back.
func generateCorpus(
    t *testing.T, meshDir string, samples int,
) *corpusFile {
    t.Helper()
    out := filepath.Join(t.TempDir(), "corpus.json")
    require.NoError(t, runGenerate([]string{
        "-mesh", meshDir, "-out", out,
        "-seed", "1", "-samples", strconv.Itoa(samples),
    }))
    corpus, err := loadCorpus(out)
    require.NoError(t, err)

    return corpus
}

// TestGenerateRecordsTheAnswers pins the generate mode: the corpus
// carries the anchors, the corridors and the samples over the pack
// regions, and the flat world answers the inside queries with found
// routes (the answer shape the replay gates on).
func TestGenerateRecordsTheAnswers(t *testing.T) {
    world := flatWorld(t, 0)
    corpus := generateCorpus(t, world, 8)

    require.Equal(t, corpusVersion, corpus.Version)
    require.Equal(t, []string{"21_19"}, corpus.Regions)
    kinds := map[string]int{}
    for _, q := range corpus.Queries {
        kinds[q.Kind]++
    }
    require.Positive(t, kinds[kindAnchor],
        "the anchor set rides every corpus")
    require.Positive(t, kinds[kindCorridor],
        "the corridors cover the pack regions")
    require.Equal(t, 8, kinds[kindSample],
        "the sample count rides the flag")
    require.Positive(t, countVerdict(corpus.Queries, verdictFound),
        "the flat world routes the inside queries")
}

// TestGenerateIsDeterministic pins the generator contract: the same
// pack, seed and sample count produce the same query set - the
// recorded answers of one run replay against the queries of another.
func TestGenerateIsDeterministic(t *testing.T) {
    world := flatWorld(t, 0)
    first := generateCorpus(t, world, 8)
    second := generateCorpus(t, world, 8)

    require.Equal(t, first.Queries, second.Queries,
        "the query set and the answers must repeat exactly")
    require.Equal(t, first.Regions, second.Regions)
}

// TestReplayPassesOnTheRecordedPack pins the happy path of the gate:
// the corpus replays over the pack it was recorded from with zero
// drift.
func TestReplayPassesOnTheRecordedPack(t *testing.T) {
    world := flatWorld(t, 0)
    corpusPath := filepath.Join(t.TempDir(), "corpus.json")
    require.NoError(t, runGenerate([]string{
        "-mesh", world, "-out", corpusPath,
        "-seed", "1", "-samples", "8",
    }))
    require.NoError(t, runReplay([]string{
        "-mesh", world, "-corpus", corpusPath,
    }))
}

// TestReplayDetectsTheAnswerDrift pins the regression gate: a pack
// whose answers moved (the surface height change of the structural
// rounds) fails the replay - the verdict stays found here, the moved
// waypoints flip the stream hash.
func TestReplayDetectsTheAnswerDrift(t *testing.T) {
    recorded := flatWorld(t, 0)
    corpusPath := filepath.Join(t.TempDir(), "corpus.json")
    require.NoError(t, runGenerate([]string{
        "-mesh", recorded, "-out", corpusPath,
        "-seed", "1", "-samples", "8",
    }))

    mutated := flatWorld(t, -40)
    err := runReplay([]string{"-mesh", mutated, "-corpus", corpusPath})
    require.Error(t, err,
        "the surface change must fail the replay")
    require.ErrorContains(t, err, "drifted")
}

// TestCompareKeepsThePairwiseContract pins the transport round form:
// two identical packs answer ALL QUERIES IDENTICAL, a moved surface
// fails with the per query mismatches. The corpus binds the query set
// (the plain anchor form refuses on the synthetic world - the anchors
// are real world elven coordinates - so the drift signal comes from
// the recorded samples).
func TestCompareKeepsThePairwiseContract(t *testing.T) {
    old := flatWorld(t, 0)
    same := flatWorld(t, 0)
    moved := flatWorld(t, -40)
    corpusPath := filepath.Join(t.TempDir(), "corpus.json")
    require.NoError(t, runGenerate([]string{
        "-mesh", old, "-out", corpusPath,
        "-seed", "1", "-samples", "8",
    }))

    require.NoError(t, runCompare([]string{
        "-old", old, "-new", same, "-corpus", corpusPath,
    }))
    err := runCompare([]string{
        "-old", old, "-new", moved, "-corpus", corpusPath,
    })
    require.Error(t, err, "the pairwise gate must catch the drift")
    require.ErrorContains(t, err, "mismatch")
}

// TestUsageListsTheModes pins the argument surface: the usage text
// the operator sees names all three modes.
func TestUsageListsTheModes(t *testing.T) {
    require.Contains(t, usage, "generate")
    require.Contains(t, usage, "replay")
    require.Contains(t, usage, "compare")
}

// TestCorpusJSONRoundTrip pins the corpus wire: the file decodes
// without unknown field refusals after the encode (the schema the
// future rounds share).
func TestCorpusJSONRoundTrip(t *testing.T) {
    corpus := corpusFile{
        Version:   corpusVersion,
        Generator: "navpack-verify/1",
        Seed:      1,
        Regions:   []string{"21_19"},
        Queries: []corpusQuery{{
            ID: "anchor-1", Kind: kindAnchor,
            Start: corpusPos{X: 46880, Y: 50752, Z: -2889},
            Goal:  corpusPos{X: 47595, Y: 51569, Z: -2992},
            Answer: corpusAnswer{
                Verdict: verdictFound, Waypoints: 3,
                SHA: "abc", First: corpusPos{X: 1}, Last: corpusPos{X: 2},
            },
        }},
    }
    data, err := json.Marshal(corpus)
    require.NoError(t, err)
    dec := json.NewDecoder(bytes.NewReader(data))
    dec.DisallowUnknownFields()
    var back corpusFile
    require.NoError(t, dec.Decode(&back))
    require.Equal(t, corpus, back)
}
