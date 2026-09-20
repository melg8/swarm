// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The hierarchical route layer (the HNA* adaptation): the polygon
//! mesh stays the level 0 geometry, the cluster graph is the level 1
//! abstraction above it. One abstract node is a square block of
//! clusterCellsSide x clusterCellsSide geodata cells; one abstract
//! edge is a mesh link whose portal crosses a cluster boundary,
//! carrying the crossing midpoint the refinement hops search through.

use crate::query::Pos;
use crate::tile::{
    ref_of, REGION_CELLS_SIDE, TILE_ZERO_COL, TILE_ZERO_ROW, TILE_WORLD_SIZE, CELL_SIZE_WORLD,
    POLY_SIDE_MAX_X, POLY_SIDE_MIN_X, POLY_SIDE_MIN_Y, PolyRef, RegionKey, Tile,
};
use std::collections::HashMap;

/// The cell count of one cluster side (128 cells = 2048 world units).
pub const CLUSTER_CELLS_SIDE: i32 = 128;

/// The cluster count of one region side (16).
pub const CLUSTERS_PER_SIDE: i32 = REGION_CELLS_SIDE / CLUSTER_CELLS_SIDE;

/// Marks the target area an external link cannot name without
/// decoding the neighbour tile.
pub const UNKNOWN_AREA: u8 = 0xFF;

/// The abstract cache bound (the LRU keeps the working set of the
/// running queries).
pub const ABSTRACT_CACHE_CAPACITY: i32 = 32;

/// Names one abstract node: the cluster block of a region.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct ClusterKey {
    pub col: i16,
    pub row: i16,
    pub id: ClusterId,
}

/// Packs the cluster indices inside a region (CX<<4|CY).
pub type ClusterId = u8;

/// Returns the cluster id of a region local cell.
pub fn cluster_of_cell(cx: i32, cy: i32) -> ClusterId {
    (((cx / CLUSTER_CELLS_SIDE) as u8) << 4) | ((cy / CLUSTER_CELLS_SIDE) as u8)
}

/// Returns the cluster key of a world position (the position clamps
/// into the region).
pub fn cluster_key_of_pos(col: i16, row: i16, x: f64, y: f64) -> ClusterKey {
    let min_x = (col as f64 - TILE_ZERO_COL as f64) * TILE_WORLD_SIZE;
    let min_y = (row as f64 - TILE_ZERO_ROW as f64) * TILE_WORLD_SIZE;
    let cx = ((x - min_x) / CELL_SIZE_WORLD).floor() as i32;
    let cy = ((y - min_y) / CELL_SIZE_WORLD).floor() as i32;
    let cx = clamp_cell(cx);
    let cy = clamp_cell(cy);

    ClusterKey {
        col,
        row,
        id: cluster_of_cell(cx, cy),
    }
}

/// Folds a region local cell coordinate into the region.
pub fn clamp_cell(c: i32) -> i32 {
    c.clamp(0, REGION_CELLS_SIDE - 1)
}

/// Identifies one coarse edge of the whole world: the region that
/// owns it plus the local edge index.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub struct AbstractEdgeRef {
    pub region: RegionKey,
    pub index: i32,
}

/// One cluster to cluster portal: the mesh link whose portal span
/// crosses a cluster boundary.
#[derive(Debug, Clone, Copy)]
pub struct AbstractEdge {
    pub to: ClusterKey,
    /// The target polygon of the crossing when the link is internal;
    /// zero defers to the link resolution (external target).
    pub to_ref: PolyRef,
    /// The index into the source tile's link store: the refinement
    /// resolves the target through it.
    pub link: i32,
    /// The portal midpoint: the world position the hop searches
    /// through.
    pub mid: Pos,
    /// The link component of the source polygon; the component of the
    /// target polygon.
    pub src_comp: u32,
    pub dst_comp: u32,
    /// The neighbour side of an external crossing. Zero poly marks
    /// the internal edges.
    pub ext_col: i16,
    pub ext_row: i16,
    pub ext_poly: u32,
    pub src_area: u8,
    pub dst_area: u8,
}

impl Default for AbstractEdge {
    fn default() -> Self {
        AbstractEdge {
            to: ClusterKey {
                col: 0,
                row: 0,
                id: 0,
            },
            to_ref: 0,
            link: 0,
            mid: Pos::ZERO,
            src_comp: 0,
            dst_comp: 0,
            ext_col: 0,
            ext_row: 0,
            ext_poly: 0,
            src_area: 0,
            dst_area: 0,
        }
    }
}

/// One cluster of the region abstract graph.
#[derive(Debug, Clone, Default)]
pub struct AbstractNode {
    pub id: ClusterId,
    pub edges: Vec<i32>,
    /// Dedupes the parallel crossings: one representative edge per
    /// target cluster and area pair.
    pub classes: HashMap<AbstractClassKey, i32>,
}

/// Dedupes the parallel edges of one node: the target cluster, the
/// area pair of the crossing and the source component.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct AbstractClassKey {
    pub to: ClusterKey,
    pub src_area: u8,
    pub dst_area: u8,
    pub src_comp: u32,
}

/// The level 1 graph of one region: the clusters with polygons as
/// nodes, the boundary crossing portals as edges.
#[derive(Debug, Default)]
pub struct RegionAbstract {
    pub key: RegionKey,
    pub nodes: Vec<AbstractNode>,
    pub index: HashMap<ClusterId, i32>,
    pub edges: Vec<AbstractEdge>,
    /// The link component id per polygon of the source tile.
    pub comps: Vec<u32>,
    /// The polygon count of the source tile.
    pub polys: usize,
    /// The tile file stat the sidecar was built against.
    pub tile_size: i64,
    pub tile_mod_time_unix_nano: i64,
    pub tile_checksums: bool,
    pub tile_head_crc: u32,
    pub tile_tail_crc: u32,
}

/// Scans one decoded tile into its cluster graph (the exported path
/// of the pack sidecar pass and the tests).
pub fn build_abstract(tile: &Tile) -> RegionAbstract {
    let mut abstract_ = RegionAbstract {
        key: RegionKey {
            col: tile.col,
            row: tile.row,
        },
        nodes: Vec::new(),
        index: HashMap::with_capacity((CLUSTERS_PER_SIDE * CLUSTERS_PER_SIDE) as usize),
        edges: Vec::new(),
        comps: link_components(tile),
        polys: tile.polys.len(),
        tile_size: 0,
        tile_mod_time_unix_nano: 0,
        tile_checksums: false,
        tile_head_crc: 0,
        tile_tail_crc: 0,
    };
    mark_clusters(tile, &mut abstract_);
    for pi in 0..tile.polys.len() {
        let poly = tile.polys[pi];
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = tile.links[li as usize];
            let current = li;
            li = link.next;
            let source_id = cluster_of_edge_source(&poly, &link);
            let (edge, ok) = abstract_edge_of(tile, &abstract_.comps, pi as i32, current, &link);
            if !ok {
                continue;
            }
            if edge.to.col == tile.col && edge.to.row == tile.row && edge.to.id == source_id {
                // The link stays inside one cluster: the coarse graph
                // does not see it.
                continue;
            }
            let node_idx = node_of(tile, &mut abstract_, source_id);
            let class = AbstractClassKey {
                to: edge.to,
                src_area: edge.src_area,
                dst_area: edge.dst_area,
                src_comp: edge.src_comp,
            };
            if let Some(&idx) = abstract_.nodes[node_idx as usize].classes.get(&class) {
                // Keep the crossing closest to the side center: the
                // representative of the open span the funnel routes
                // through.
                if edge_crossing_distance(tile, source_id, &edge)
                    < edge_crossing_distance(tile, source_id, &abstract_.edges[idx as usize])
                {
                    abstract_.edges[idx as usize] = edge;
                }

                continue;
            }
            let edge_idx = abstract_.edges.len() as i32;
            abstract_.nodes[node_idx as usize].classes.insert(class, edge_idx);
            abstract_.nodes[node_idx as usize].edges.push(edge_idx);
            abstract_.edges.push(edge);
        }
    }

    abstract_
}

/// Labels every polygon of the tile with its link component (union
/// find over the internal links).
pub fn link_components(tile: &Tile) -> Vec<u32> {
    let n = tile.polys.len();
    let mut parent: Vec<u32> = (0..n as u32).collect();

    fn find(parent: &mut Vec<u32>, mut x: u32) -> u32 {
        while parent[x as usize] != x {
            let grandparent = parent[parent[x as usize] as usize];
            parent[x as usize] = grandparent;
            x = grandparent;
        }

        x
    }

    for pi in 0..n {
        let poly = &tile.polys[pi];
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = &tile.links[li as usize];
            li = link.next;
            if link.to >= 0 && (link.to as usize) < tile.polys.len() {
                let ra = find(&mut parent, pi as u32);
                let rb = find(&mut parent, link.to as u32);
                if ra != rb {
                    parent[rb as usize] = ra;
                }
            }
        }
    }
    let mut comps = vec![0u32; n];
    let mut ids: HashMap<u32, u32> = HashMap::new();
    for pi in 0..n {
        let root = find(&mut parent, pi as u32);
        let next_id = (ids.len() + 1) as u32;
        let id = *ids.entry(root).or_insert(next_id);
        comps[pi] = id;
    }

    comps
}

/// Measures how far a crossing sits from the center of the cluster
/// side it leaves through (the dedupe metric).
fn edge_crossing_distance(tile: &Tile, source_id: ClusterId, edge: &AbstractEdge) -> f64 {
    let cx = ((source_id >> 4) as i32) * CLUSTER_CELLS_SIDE;
    let cy = ((source_id & 0x0F) as i32) * CLUSTER_CELLS_SIDE;
    let half = CLUSTER_CELLS_SIDE / 2;
    let x = (cx + half) as f64 * CELL_SIZE_WORLD + tile.world_min_x();
    let y = (cy + half) as f64 * CELL_SIZE_WORLD + tile.world_min_y();
    let dx = edge.mid.x - x;
    let dy = edge.mid.y - y;

    dx * dx + dy * dy
}

/// Creates a node for every cluster block a polygon footprint
/// touches.
fn mark_clusters(tile: &Tile, abstract_: &mut RegionAbstract) {
    for pi in 0..tile.polys.len() {
        let poly = &tile.polys[pi];
        let x0 = poly.x0 / CLUSTER_CELLS_SIDE;
        let x1 = (poly.x1 - 1) / CLUSTER_CELLS_SIDE;
        let y0 = poly.y0 / CLUSTER_CELLS_SIDE;
        let y1 = (poly.y1 - 1) / CLUSTER_CELLS_SIDE;
        for cx in x0..=x1 {
            for cy in y0..=y1 {
                node_of(tile, abstract_, cluster_of_cell(cx, cy));
            }
        }
    }
}

/// Returns the node index of a cluster id, creating it on first use.
/// The tile argument carries the region key the nodes report (the Go
/// twin reads it off the abstract).
fn node_of(_tile: &Tile, abstract_: &mut RegionAbstract, id: ClusterId) -> i32 {
    if let Some(&ni) = abstract_.index.get(&id) {
        return ni;
    }
    abstract_.nodes.push(AbstractNode {
        id,
        edges: Vec::new(),
        classes: HashMap::new(),
    });
    let idx = (abstract_.nodes.len() - 1) as i32;
    abstract_.index.insert(id, idx);

    idx
}

/// Derives the coarse edge of one mesh link: the crossing midpoint,
/// the source and target clusters and the target polygon reference.
fn abstract_edge_of(
    tile: &Tile,
    comps: &[u32],
    poly: i32,
    link_idx: i32,
    link: &crate::tile::Link,
) -> (AbstractEdge, bool) {
    let p = &tile.polys[poly as usize];
    let (ax, ay, bx, by) = tile.portal(p, link);
    let edge = AbstractEdge {
        to: ClusterKey {
            col: 0,
            row: 0,
            id: 0,
        },
        to_ref: 0,
        link: link_idx,
        mid: Pos {
            x: (ax + bx) * 0.5,
            y: (ay + by) * 0.5,
            z: tile.height_at(p, (ax + bx) * 0.5, (ay + by) * 0.5),
        },
        src_comp: comps[poly as usize],
        dst_comp: 0,
        src_area: p.area,
        dst_area: UNKNOWN_AREA,
        ext_col: 0,
        ext_row: 0,
        ext_poly: 0,
    };
    // The cell across the shared edge decides the target cluster.
    let (mut cx, mut cy) = edge_source_cell(p, link);
    match link.side {
        POLY_SIDE_MIN_X => cx -= 1,
        POLY_SIDE_MAX_X => cx += 1,
        POLY_SIDE_MIN_Y => cy -= 1,
        _ => cy += 1,
    }
    if link.to >= 0 {
        return abstract_internal_edge(tile, comps, link, edge, cx, cy);
    }

    abstract_external_edge(tile, link, edge, cx, cy)
}

/// Finishes the coarse edge of an internal link.
fn abstract_internal_edge(
    tile: &Tile,
    comps: &[u32],
    link: &crate::tile::Link,
    mut edge: AbstractEdge,
    cx: i32,
    cy: i32,
) -> (AbstractEdge, bool) {
    if link.to as usize >= tile.polys.len() {
        return (AbstractEdge::default(), false);
    }
    let target = &tile.polys[link.to as usize];
    edge.to = ClusterKey {
        col: tile.col,
        row: tile.row,
        id: cluster_of_cell(clamp_cell(cx), clamp_cell(cy)),
    };
    edge.to_ref = ref_of(tile.col, tile.row, link.to as u32);
    edge.dst_comp = comps[link.to as usize];
    edge.dst_area = target.area;

    (edge, true)
}

/// Finishes the coarse edge of an external link: the neighbour cell
/// mirrors onto the opposite border of the neighbour region, its
/// cluster folds the border coordinate.
fn abstract_external_edge(
    tile: &Tile,
    link: &crate::tile::Link,
    mut edge: AbstractEdge,
    cx: i32,
    cy: i32,
) -> (AbstractEdge, bool) {
    let ext_idx = (-link.to - 1) as usize;
    if ext_idx >= tile.ext_links.len() {
        return (AbstractEdge::default(), false);
    }
    let ext = &tile.ext_links[ext_idx];
    if ext.poly == 0xFFFF_FFFF {
        return (AbstractEdge::default(), false);
    }
    let (ncx, ncy) = match link.side {
        POLY_SIDE_MIN_X => (REGION_CELLS_SIDE - 1, cy),
        POLY_SIDE_MAX_X => (0, cy),
        POLY_SIDE_MIN_Y => (cx, REGION_CELLS_SIDE - 1),
        _ => (cx, 0),
    };
    edge.to = ClusterKey {
        col: ext.col as i16,
        row: ext.row as i16,
        id: cluster_of_cell(clamp_cell(ncx), clamp_cell(ncy)),
    };
    edge.ext_col = ext.col as i16;
    edge.ext_row = ext.row as i16;
    edge.ext_poly = ext.poly;

    (edge, true)
}

/// Returns the cluster of the cell just inside the source polygon at
/// the crossing (the source node of the edge).
pub fn cluster_of_edge_source(p: &crate::tile::Poly, link: &crate::tile::Link) -> ClusterId {
    let (cx, cy) = edge_source_cell(p, link);

    cluster_of_cell(clamp_cell(cx), clamp_cell(cy))
}

/// Returns the region local cell of the polygon just inside the
/// crossing portal (before the step across the side).
fn edge_source_cell(p: &crate::tile::Poly, link: &crate::tile::Link) -> (i32, i32) {
    let span = (link.t0 + link.t1) / 2;
    match link.side {
        POLY_SIDE_MIN_X => (p.x0, span),
        POLY_SIDE_MAX_X => (p.x1 - 1, span),
        POLY_SIDE_MIN_Y => (span, p.y0),
        _ => (span, p.y1 - 1),
    }
}
