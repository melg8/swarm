// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "encoding/binary"
    "fmt"
    "math"
    "time"
)

// The abstract sidecar format: the persistent coarse layer of one
// region (docs/fastpath_research.md section 2). The pack build
// serializes the cluster graph next to the tile (the X_Y.ab
// sidecar); the runtime loads it instead of re-scanning the decoded
// tile, so the coarse search starts warm and the tile decodes stay
// inside the refinement hops.
//
// The wire: a 56 byte header, the node records (the inline edge
// index lists), the edge records and the run length encoded link
// components. The derived structures (the cluster index map and the
// per node class dedupe) rebuild at the decode - the edge dedupe of
// the build keeps one edge per class, so the first class mapping per
// stored edge reconstructs the map exactly.
//
// Version 2 adds the tile checksum guard (the head and the tail
// CRC32 of the tile file bytes in the header bytes 48..56): the
// deployment copies that lose the file mtimes (a plain cp or an
// archive extraction) keep serving the sidecars, the mtime only
// packs stay on the version 1 guard.

const (
    abstractMagic        = 0x31424153 // 'SAB1' little endian
    abstractVersion      = 2
    abstractVersionV1    = 1 // the mtime only guard (the legacy sidecars)
    abstractHeaderSize   = 56
    abstractEdgeWireSize = 64
)

// EncodeAbstract serializes a region cluster graph.
func EncodeAbstract(abstract *regionAbstract) ([]byte, error) {
    if abstract == nil || len(abstract.nodes) == 0 {
        return nil, fmt.Errorf("%w: the empty abstract", ErrBadTile)
    }
    nodeBytes := 0
    for i := range abstract.nodes {
        nodeBytes += 8 + len(abstract.nodes[i].edges)*4
    }
    runs := compRunsOf(abstract.comps)
    size := abstractHeaderSize + nodeBytes +
        len(abstract.edges)*abstractEdgeWireSize + len(runs)*8
    data := make([]byte, size)

    put32 := func(off int, v uint32) {
        binary.LittleEndian.PutUint32(data[off:], v)
    }
    put16 := func(off int, v uint16) {
        binary.LittleEndian.PutUint16(data[off:], v)
    }
    put32(0, abstractMagic)
    if abstract.TileChecksums {
        put32(4, abstractVersion)
    } else {
        put32(4, abstractVersionV1)
    }
    put32(8, uint32(int32(abstract.key.Col)))
    put32(12, uint32(int32(abstract.key.Row)))
    put32(16, uint32(abstract.polys))
    put32(20, uint32(len(abstract.nodes)))
    put32(24, uint32(len(abstract.edges)))
    put32(28, uint32(len(runs)))
    binary.LittleEndian.PutUint64(data[32:], uint64(abstract.TileSize))
    binary.LittleEndian.PutUint64(data[40:],
        uint64(abstract.TileModTime.UnixNano()))
    if abstract.TileChecksums {
        put32(48, abstract.TileHeadCRC)
        put32(52, abstract.TileTailCRC)
    }

    offset := abstractHeaderSize
    for i := range abstract.nodes {
        node := &abstract.nodes[i]
        data[offset] = byte(node.id)
        put16(offset+2, uint16(len(node.edges)))
        offset += 8
        for _, edgeIdx := range node.edges {
            put32(offset, uint32(edgeIdx))
            offset += 4
        }
    }
    for i := range abstract.edges {
        edge := &abstract.edges[i]
        base := offset + i*abstractEdgeWireSize
        put16(base, uint16(int16(edge.to.Col)))
        put16(base+2, uint16(int16(edge.to.Row)))
        data[base+4] = byte(edge.to.ID)
        binary.LittleEndian.PutUint64(data[base+8:], uint64(edge.toRef))
        put32(base+16, uint32(edge.link))
        binary.LittleEndian.PutUint64(data[base+20:], math.Float64bits(edge.mid.X))
        binary.LittleEndian.PutUint64(data[base+28:], math.Float64bits(edge.mid.Y))
        binary.LittleEndian.PutUint64(data[base+36:], math.Float64bits(edge.mid.Z))
        put32(base+44, edge.srcComp)
        put32(base+48, edge.dstComp)
        put16(base+52, uint16(int16(edge.extCol)))
        put16(base+54, uint16(int16(edge.extRow)))
        put32(base+56, edge.extPoly)
        data[base+60] = edge.srcArea
        data[base+61] = edge.dstArea
    }
    offset += len(abstract.edges) * abstractEdgeWireSize
    for _, run := range runs {
        put32(offset, run.value)
        put32(offset+4, run.count)
        offset += 8
    }

    return data, nil
}

// DecodeAbstract parses one abstract sidecar.
func DecodeAbstract(data []byte) (*regionAbstract, error) {
    if len(data) < abstractHeaderSize {
        return nil, fmt.Errorf("%w: the abstract %d bytes is too short",
            ErrBadTile, len(data))
    }
    if magic := binary.LittleEndian.Uint32(data[0:]); magic != abstractMagic {
        return nil, fmt.Errorf("%w: the abstract magic 0x%x", ErrBadTile,
            magic)
    }
    if version := binary.LittleEndian.Uint32(data[4:]); version !=
        abstractVersion && version != abstractVersionV1 {
        return nil, fmt.Errorf("%w: the abstract version %d", ErrBadTile,
            version)
    }
    checksums := binary.LittleEndian.Uint32(data[4:]) == abstractVersion
    u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(data[off:]) }
    col := int16(u32(8))
    row := int16(u32(12))
    polys := int(u32(16))
    nodeCount := int(u32(20))
    edgeCount := int(u32(24))
    runCount := int(u32(28))
    tileSize := int64(binary.LittleEndian.Uint64(data[32:]))
    tileModTime := time.Unix(0,
        int64(binary.LittleEndian.Uint64(data[40:])))

    abstract := &regionAbstract{
        key:   RegionKey{Col: col, Row: row},
        index: make(map[clusterID]int32, nodeCount),
        edges: make([]abstractEdge, 0, edgeCount),
        comps: make([]uint32, polys),
        polys: polys,

        TileSize:      tileSize,
        TileModTime:   tileModTime,
        TileChecksums: checksums,
        TileHeadCRC:   binary.LittleEndian.Uint32(data[48:]),
        TileTailCRC:   binary.LittleEndian.Uint32(data[52:]),
    }

    offset := abstractHeaderSize
    for i := 0; i < nodeCount; i++ {
        if offset+8 > len(data) {
            return nil, fmt.Errorf("%w: the abstract nodes truncate",
                ErrBadTile)
        }
        id := clusterID(data[offset])
        edgeN := int(binary.LittleEndian.Uint16(data[offset+2:]))
        offset += 8
        if offset+edgeN*4 > len(data) {
            return nil, fmt.Errorf("%w: the abstract node edges truncate",
                ErrBadTile)
        }
        node := abstractNode{
            id:      id,
            edges:   make([]int32, 0, edgeN),
            classes: make(map[abstractClassKey]int32, edgeN),
        }
        for j := 0; j < edgeN; j++ {
            idx := int32(binary.LittleEndian.Uint32(data[offset:]))
            offset += 4
            if idx < 0 || int(idx) >= edgeCount {
                return nil, fmt.Errorf(
                    "%w: the abstract edge index %d", ErrBadTile, idx)
            }
            node.edges = append(node.edges, idx)
        }
        abstract.nodes = append(abstract.nodes, node)
        abstract.index[id] = int32(len(abstract.nodes) - 1)
    }
    if offset+edgeCount*abstractEdgeWireSize > len(data) {
        return nil, fmt.Errorf("%w: the abstract edges truncate",
            ErrBadTile)
    }
    for i := 0; i < edgeCount; i++ {
        base := offset + i*abstractEdgeWireSize
        edge := abstractEdge{
            to: clusterKey{
                Col: int16(binary.LittleEndian.Uint16(data[base:])),
                Row: int16(binary.LittleEndian.Uint16(data[base+2:])),
                ID:  clusterID(data[base+4]),
            },
            toRef: PolyRef(binary.LittleEndian.Uint64(data[base+8:])),
            link:  int32(binary.LittleEndian.Uint32(data[base+16:])),
            mid: Pos{
                X: math.Float64frombits(
                    binary.LittleEndian.Uint64(data[base+20:])),
                Y: math.Float64frombits(
                    binary.LittleEndian.Uint64(data[base+28:])),
                Z: math.Float64frombits(
                    binary.LittleEndian.Uint64(data[base+36:])),
            },
            srcComp: binary.LittleEndian.Uint32(data[base+44:]),
            dstComp: binary.LittleEndian.Uint32(data[base+48:]),
            extCol: int16(binary.LittleEndian.Uint16(
                data[base+52:])),
            extRow: int16(binary.LittleEndian.Uint16(
                data[base+54:])),
            extPoly: binary.LittleEndian.Uint32(data[base+56:]),
            srcArea: data[base+60],
            dstArea: data[base+61],
        }
        abstract.edges = append(abstract.edges, edge)
    }
    offset += edgeCount * abstractEdgeWireSize
    if offset+runCount*8 > len(data) {
        return nil, fmt.Errorf("%w: the abstract components truncate",
            ErrBadTile)
    }
    cursor := 0
    for i := 0; i < runCount; i++ {
        base := offset + i*8
        value := binary.LittleEndian.Uint32(data[base:])
        count := int(binary.LittleEndian.Uint32(data[base+4:]))
        for j := 0; j < count; j++ {
            if cursor >= polys {
                return nil, fmt.Errorf(
                    "%w: the abstract components overflow", ErrBadTile)
            }
            abstract.comps[cursor] = value
            cursor++
        }
    }

    // The class dedupe rebuilds from the stored edges: the build kept
    // one edge per class, the first mapping per stored edge
    // reconstructs the map.
    for i := range abstract.nodes {
        node := &abstract.nodes[i]
        for _, edgeIdx := range node.edges {
            edge := &abstract.edges[edgeIdx]
            node.classes[abstractClassKey{
                to:      edge.to,
                srcArea: edge.srcArea,
                dstArea: edge.dstArea,
                srcComp: edge.srcComp,
            }] = edgeIdx
        }
    }

    return abstract, nil
}

// compRun is one run of the link component array.
type compRun struct {
    value uint32
    count uint32
}

// compRunsOf folds the component array into the runs.
func compRunsOf(comps []uint32) []compRun {
    runs := make([]compRun, 0, len(comps)/8+1)
    for _, value := range comps {
        if n := len(runs); n > 0 && runs[n-1].value == value {
            runs[n-1].count++

            continue
        }
        runs = append(runs, compRun{value: value, count: 1})
    }

    return runs
}
