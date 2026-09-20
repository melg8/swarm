// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! Rust port of the swarm navmesh pathfinding runtime
//! (`internal/swarm/pathfind/navmesh`): tiles of rectangle polygons
//! built offline from the l2j geodata, loaded lazily by [`Mesh`] and
//! answered by the route query (the nearest polygon resolution, the
//! A* corridor search, the funnel string pulling, the HNA* cluster
//! hierarchy).
//!
//! The port mirrors the Go algorithm one to one: the same wire
//! formats, the same costs, the same tie breaking, the same float
//! formulas (Go's `math.Hypot` is ported verbatim for bit parity).

pub mod abstract_graph;
pub mod abstractfile;
pub mod astar;
pub mod avoid;
pub mod corridor;
pub mod funnel;
pub mod hierarchy;
pub mod mesh;
pub mod pocket;
pub mod query;
pub mod smooth;
pub mod tile;

pub use mesh::Mesh;
pub use query::{Filter, Pos, Route, WaterZone};
pub use tile::{
    decode_tile, ref_of, region_of_world, ExtLink, Link, Poly, PolyRef, RegionKey, Tile, TileGrid,
};

/// The error type of the crate: every failure carries a message.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NavError(pub String);

impl std::fmt::Display for NavError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for NavError {}

/// The tile absent marker: a region without a tile file answers this
/// (the link resolution treats it as a wall).
pub fn tile_absent() -> NavError {
    NavError("navmesh tile absent".to_string())
}

/// The missing mesh marker: a position without a usable mesh polygon.
pub fn no_navmesh() -> NavError {
    NavError("no navmesh under the position".to_string())
}
