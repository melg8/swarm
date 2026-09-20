// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! Tile format of the navigation mesh runtime: the rectangle polygons
//! of one geodata region, their link chains and the spatial index
//! (the v3 bucket grid or the v1/v2 bounding volume tree).
//!
//! Wire versions: 1 (the int32 bounds and spans), 2 (the uint16
//! quantization) and 3 (the columnar layout with the bucket grid).

use crate::NavError;
use std::sync::Arc;

/// Identifies a tile by its region coordinates (the X_Y of the X_Y.nm
/// file name).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Default)]
pub struct RegionKey {
    pub col: i16,
    pub row: i16,
}

// World geometry constants, mirroring internal/swarm/pathfind
// (geometry.go: World.TILE_SIZE and the region anchors of the l2j
// grid). They are duplicated here so the runtime stays a self
// contained leaf package.

/// The cell count of one region side (2048).
pub const REGION_CELLS_SIDE: i32 = 2048;
/// The world size of one geodata cell (16).
pub const CELL_SIZE_WORLD: f64 = 16.0;
/// The world size of one region (32768).
pub const TILE_WORLD_SIZE: f64 = REGION_CELLS_SIDE as f64 * CELL_SIZE_WORLD;
/// The region file name anchor of the world x axis.
pub const TILE_ZERO_COL: i16 = 20;
/// The region file name anchor of the world y axis.
pub const TILE_ZERO_ROW: i16 = 18;
/// The world height the BVTree quantization anchors on.
pub const HEIGHT_FLOOR: f64 = -8192.0;

/// Polygon area ids: the water polys multiply the step cost by the
/// swim area cost.
pub const AREA_GROUND: u8 = 0;
pub const AREA_WATER: u8 = 1;

/// Poly sides: the side of the rectangle a link leaves through. The
/// naming follows the cell axes (SideMinX = the x==X0 edge); the NSWE
/// compass of the geodata maps west/east/north/south onto
/// minx/maxx/miny/maxy respectively.
pub const POLY_SIDE_MIN_X: u8 = 0;
pub const POLY_SIDE_MAX_X: u8 = 1;
pub const POLY_SIDE_MIN_Y: u8 = 2;
pub const POLY_SIDE_MAX_Y: u8 = 3;

const TILE_MAGIC: u32 = 0x3157_4E53; // 'SWN1' little endian
const TILE_VERSION: u32 = 3;
const TILE_HEADER_SIZE: usize = 40;

const POLY_WIRE_SIZE_V1: usize = 32;
const LINK_WIRE_SIZE_V1: usize = 20;
const EXT_LINK_WIRE_SIZE_V1: usize = 12;

const POLY_WIRE_SIZE: usize = 24;
const LINK_WIRE_SIZE: usize = 16;
const EXT_LINK_WIRE_SIZE: usize = 8;

const BV_NODE_WIRE_SIZE: usize = 16;

/// The version 3 grid index: the bucket count per axis and the cell
/// span of one bucket.
pub const GRID_SIDE: usize = 64;
pub const GRID_BUCKET_CELLS: i32 = REGION_CELLS_SIDE / GRID_SIDE as i32;
pub const GRID_BUCKETS: usize = GRID_SIDE * GRID_SIDE;

fn err_bad_tile(msg: String) -> NavError {
    NavError(format!("not a navmesh tile: {msg}"))
}

/// One parsed navigation mesh tile: the rectangle polygons of one
/// geodata region, their link chains and the spatial index. A tile is
/// immutable after decoding and safe for concurrent reads.
#[derive(Debug, Clone)]
pub struct Tile {
    pub col: i16,
    pub row: i16,
    /// The height step the links respect (the 40 unit
    /// HEIGHT_INCREASE_LIMIT the server movement validation uses).
    pub climb: i32,
    /// The rectangle polygons in region local cell bounds
    /// (X0 <= x < X1, Y0 <= y < Y1).
    pub polys: Vec<Poly>,
    /// The flat link store every polygon chains into through
    /// first_link / link.next.
    pub links: Vec<Link>,
    /// The cross region link targets the links with a negative `to`
    /// resolve into.
    pub ext_links: Vec<ExtLink>,
    /// The quantized bounding volume tree over the polygon bounds
    /// (the Detour layout). None for the v3 tiles: the grid replaces
    /// it.
    pub bv_tree: Option<Vec<BVNode>>,
    /// The uniform bucket index of the v3 tiles. None for the v1/v2
    /// tiles: the BVTree serves those.
    pub grid: Option<TileGrid>,
    /// The world anchor of the region (the min corner of cell 0 0),
    /// derived at decode.
    pub world_min_x: f64,
    pub world_min_y: f64,
}

/// The uniform spatial index of the v3 tiles: the 64x64 bucket grid
/// of polygon ids over the region footprint.
#[derive(Debug, Clone)]
pub struct TileGrid {
    pub offsets: Vec<u32>,
    pub entries: Vec<u32>,
}

/// One rectangle navigation polygon.
#[derive(Debug, Clone, Copy)]
pub struct Poly {
    /// Region local cell bounds, half open.
    pub x0: i32,
    pub y0: i32,
    pub x1: i32,
    pub y1: i32,
    /// The corner heights, taken from the geodata cells at the
    /// inside corners. The interior height is the bilinear
    /// interpolation of the four.
    pub h00: i16,
    pub h10: i16,
    pub h01: i16,
    pub h11: i16,
    /// The first link of the chain (-1: the polygon has no links).
    pub first_link: i32,
    /// AreaGround or AreaWater.
    pub area: u8,
}

/// One polygon-to-polygon connection leaving through a side.
#[derive(Debug, Clone, Copy)]
pub struct Link {
    /// The rectangle side the link crosses.
    pub side: u8,
    /// >= 0 the polygon index inside the same tile, < 0 the external
    /// target -(index into ext_links)-1.
    pub to: i32,
    /// The next link of the same polygon (-1: the end).
    pub next: i32,
    /// The inclusive cell range along the crossing axis where the
    /// geodata walls are open.
    pub t0: i32,
    pub t1: i32,
}

/// The cross region target of a link.
#[derive(Debug, Clone, Copy)]
pub struct ExtLink {
    pub col: i32,
    pub row: i32,
    pub poly: u32,
}

/// One node of the tile bounding volume tree: quantized bounds with
/// the leaf escape encoded as a negative index.
#[derive(Debug, Clone, Copy)]
pub struct BVNode {
    pub b_min: [u16; 3],
    pub b_max: [u16; 3],
    pub i: i32,
}

/// Identifies one polygon of one tile: the packed region key and the
/// polygon index (0 is the null reference).
pub type PolyRef = u64;

/// Packs a region key and a polygon index into a reference.
pub fn ref_of(col: i16, row: i16, poly: u32) -> PolyRef {
    ((col as u16 as u64) << 48) | ((row as u16 as u64) << 32) | (poly as u64 + 1)
}

/// Returns the region key of a reference.
pub fn tile_of(ref_: PolyRef) -> (i16, i16) {
    (((ref_ >> 48) as u16) as i16, ((ref_ >> 32) as u16) as i16)
}

/// Returns the polygon index of a reference (-1 for the null
/// reference).
pub fn poly_of(ref_: PolyRef) -> i32 {
    if ref_ == 0 {
        return -1;
    }
    ((ref_ & 0xFFFF_FFFF) as u32 as i32).wrapping_sub(1)
}

/// Maps a world position to its region key.
pub fn region_of_world(x: f64, y: f64) -> RegionKey {
    RegionKey {
        col: (x / TILE_WORLD_SIZE).floor() as i16 + TILE_ZERO_COL,
        row: (y / TILE_WORLD_SIZE).floor() as i16 + TILE_ZERO_ROW,
    }
}

/// Returns the world size of one region tile (32768).
pub fn tile_world_size() -> f64 {
    TILE_WORLD_SIZE
}

/// Returns the region file name anchor of the world x axis.
pub fn tile_zero_col() -> i16 {
    TILE_ZERO_COL
}

/// Returns the region file name anchor of the world y axis.
pub fn tile_zero_row() -> i16 {
    TILE_ZERO_ROW
}

impl Tile {
    /// The world x anchor of the region.
    pub fn world_min_x(&self) -> f64 {
        self.world_min_x
    }

    /// The world y anchor of the region.
    pub fn world_min_y(&self) -> f64 {
        self.world_min_y
    }

    /// The world bounds of one polygon.
    pub fn world_rect(&self, p: &Poly) -> (f64, f64, f64, f64) {
        (
            self.world_min_x + p.x0 as f64 * CELL_SIZE_WORLD,
            self.world_min_y + p.y0 as f64 * CELL_SIZE_WORLD,
            self.world_min_x + p.x1 as f64 * CELL_SIZE_WORLD,
            self.world_min_y + p.y1 as f64 * CELL_SIZE_WORLD,
        )
    }

    /// The bilinear surface height of the polygon at a world position
    /// (the interpolation clamps u and v into [0, 1]).
    pub fn height_at(&self, p: &Poly, world_x: f64, world_y: f64) -> f64 {
        let (x0, y0, x1, y1) = self.world_rect(p);
        let mut u = (world_x - x0) / (x1 - x0);
        let mut v = (world_y - y0) / (y1 - y0);
        u = u.clamp(0.0, 1.0);
        v = v.clamp(0.0, 1.0);
        let h00 = p.h00 as f64;
        let h10 = p.h10 as f64;
        let h01 = p.h01 as f64;
        let h11 = p.h11 as f64;

        (1.0 - u) * (1.0 - v) * h00
            + u * (1.0 - v) * h10
            + (1.0 - u) * v * h01
            + u * v * h11
    }

    /// The point of the polygon surface closest to the world position
    /// in 3D: the position itself with the surface height when the
    /// footprint contains it, otherwise the clamped boundary point
    /// with the interpolated edge height.
    pub fn closest_point(&self, p: &Poly, x: f64, y: f64, _z: f64) -> (f64, f64, f64) {
        let (x0, y0, x1, y1) = self.world_rect(p);
        let px = x.clamp(x0, x1);
        let py = y.clamp(y0, y1);

        (px, py, self.height_at(p, px, py))
    }

    /// The world segment of the link portal: the open span of the
    /// shared edge the funnel may cross.
    pub fn portal(&self, p: &Poly, link: &Link) -> (f64, f64, f64, f64) {
        match link.side {
            POLY_SIDE_MIN_X => (
                self.world_min_x + p.x0 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + link.t0 as f64 * CELL_SIZE_WORLD,
                self.world_min_x + p.x0 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + (link.t1 + 1) as f64 * CELL_SIZE_WORLD,
            ),
            POLY_SIDE_MAX_X => (
                self.world_min_x + p.x1 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + link.t0 as f64 * CELL_SIZE_WORLD,
                self.world_min_x + p.x1 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + (link.t1 + 1) as f64 * CELL_SIZE_WORLD,
            ),
            POLY_SIDE_MIN_Y => (
                self.world_min_x + link.t0 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + p.y0 as f64 * CELL_SIZE_WORLD,
                self.world_min_x + (link.t1 + 1) as f64 * CELL_SIZE_WORLD,
                self.world_min_y + p.y0 as f64 * CELL_SIZE_WORLD,
            ),
            _ => (
                self.world_min_x + link.t0 as f64 * CELL_SIZE_WORLD,
                self.world_min_y + p.y1 as f64 * CELL_SIZE_WORLD,
                self.world_min_x + (link.t1 + 1) as f64 * CELL_SIZE_WORLD,
                self.world_min_y + p.y1 as f64 * CELL_SIZE_WORLD,
            ),
        }
    }
}

/// Decodes the raw bytes of one tile file. The wire versions decode:
/// version 1 (the int32 bounds and spans), version 2 (the uint16
/// quantization) and version 3 (the columnar layout with the bucket
/// grid index).
pub fn decode_tile(data: &[u8]) -> Result<Tile, NavError> {
    if data.len() < TILE_HEADER_SIZE {
        return Err(err_bad_tile(format!("{} bytes is too short", data.len())));
    }
    let magic = u32_le(data, 0);
    if magic != TILE_MAGIC {
        return Err(err_bad_tile(format!("magic 0x{magic:x}")));
    }
    let version = u32_le(data, 4);
    if version < 1 || version > TILE_VERSION {
        return Err(err_bad_tile(format!("version {version}")));
    }

    let col = u32_le(data, 8) as i32;
    let row = u32_le(data, 12) as i32;
    let climb = u32_le(data, 16) as i32;
    let poly_count = u32_le(data, 20) as usize;
    let link_count = u32_le(data, 24) as usize;
    let ext_count = u32_le(data, 28) as usize;
    let index_count = u32_le(data, 32) as usize;

    let mut tile = Tile {
        col: col as i16,
        row: row as i16,
        climb,
        polys: vec![Poly::empty(); poly_count],
        links: vec![Link::empty(); link_count],
        ext_links: vec![ExtLink::empty(); ext_count],
        bv_tree: None,
        grid: None,
        world_min_x: (col as f64 - TILE_ZERO_COL as f64) * TILE_WORLD_SIZE,
        world_min_y: (row as f64 - TILE_ZERO_ROW as f64) * TILE_WORLD_SIZE,
    };
    if version < 3 {
        tile.bv_tree = Some(vec![BVNode::empty(); index_count]);
    }

    let mut offset = TILE_HEADER_SIZE;
    match version {
        1 => {
            decode_polys_v1(data, &mut offset, &mut tile)?;
            decode_links_v1(data, &mut offset, &mut tile)?;
            decode_ext_links_v1(data, &mut offset, &mut tile)?;
            decode_bv_tree(data, &mut offset, &mut tile)?;
        }
        2 => {
            decode_polys_v2(data, &mut offset, &mut tile)?;
            decode_links_v2(data, &mut offset, &mut tile)?;
            decode_ext_links_v2(data, &mut offset, &mut tile)?;
            decode_bv_tree(data, &mut offset, &mut tile)?;
        }
        _ => {
            decode_tile_v3(data, &mut offset, &mut tile)?;
        }
    }
    if offset != data.len() {
        return Err(err_bad_tile(format!(
            "{} trailing bytes",
            data.len() - offset
        )));
    }

    Ok(tile)
}

fn u32_le(data: &[u8], off: usize) -> u32 {
    u32::from_le_bytes([data[off], data[off + 1], data[off + 2], data[off + 3]])
}

fn u16_le(data: &[u8], off: usize) -> u16 {
    u16::from_le_bytes([data[off], data[off + 1]])
}

impl Poly {
    fn empty() -> Self {
        Self {
            x0: 0,
            y0: 0,
            x1: 0,
            y1: 0,
            h00: 0,
            h10: 0,
            h01: 0,
            h11: 0,
            first_link: 0,
            area: 0,
        }
    }
}

impl Link {
    fn empty() -> Self {
        Self {
            side: 0,
            to: 0,
            next: 0,
            t0: 0,
            t1: 0,
        }
    }
}

impl ExtLink {
    fn empty() -> Self {
        Self {
            col: 0,
            row: 0,
            poly: 0,
        }
    }
}

impl BVNode {
    fn empty() -> Self {
        Self {
            b_min: [0; 3],
            b_max: [0; 3],
            i: 0,
        }
    }
}

fn decode_polys_v1(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.polys.len() * POLY_WIRE_SIZE_V1;
    if *offset + size > data.len() {
        return Err(err_bad_tile("polys block truncated".to_string()));
    }
    for (i, poly) in tile.polys.iter_mut().enumerate() {
        let base = *offset + i * POLY_WIRE_SIZE_V1;
        poly.x0 = u32_le(data, base) as i32;
        poly.y0 = u32_le(data, base + 4) as i32;
        poly.x1 = u32_le(data, base + 8) as i32;
        poly.y1 = u32_le(data, base + 12) as i32;
        poly.h00 = u16_le(data, base + 16) as i16;
        poly.h10 = u16_le(data, base + 18) as i16;
        poly.h01 = u16_le(data, base + 20) as i16;
        poly.h11 = u16_le(data, base + 22) as i16;
        poly.first_link = u32_le(data, base + 24) as i32;
        poly.area = data[base + 28];
    }
    *offset += size;

    validate_polys(tile)
}

fn decode_polys_v2(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.polys.len() * POLY_WIRE_SIZE;
    if *offset + size > data.len() {
        return Err(err_bad_tile("polys block truncated".to_string()));
    }
    for (i, poly) in tile.polys.iter_mut().enumerate() {
        let base = *offset + i * POLY_WIRE_SIZE;
        poly.x0 = u16_le(data, base) as i32;
        poly.y0 = u16_le(data, base + 2) as i32;
        poly.x1 = u16_le(data, base + 4) as i32;
        poly.y1 = u16_le(data, base + 6) as i32;
        poly.h00 = u16_le(data, base + 8) as i16;
        poly.h10 = u16_le(data, base + 10) as i16;
        poly.h01 = u16_le(data, base + 12) as i16;
        poly.h11 = u16_le(data, base + 14) as i16;
        poly.first_link = u32_le(data, base + 16) as i32;
        poly.area = data[base + 20];
    }
    *offset += size;

    validate_polys(tile)
}

fn validate_polys(tile: &Tile) -> Result<(), NavError> {
    for (i, poly) in tile.polys.iter().enumerate() {
        if poly.x0 < 0
            || poly.y0 < 0
            || poly.x1 <= poly.x0
            || poly.y1 <= poly.y0
            || poly.x1 > REGION_CELLS_SIDE
            || poly.y1 > REGION_CELLS_SIDE
        {
            return Err(err_bad_tile(format!(
                "poly {} bounds {} {} {} {}",
                i, poly.x0, poly.y0, poly.x1, poly.y1
            )));
        }
    }

    Ok(())
}

fn decode_links_v1(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.links.len() * LINK_WIRE_SIZE_V1;
    if *offset + size > data.len() {
        return Err(err_bad_tile("links block truncated".to_string()));
    }
    for (i, link) in tile.links.iter_mut().enumerate() {
        let base = *offset + i * LINK_WIRE_SIZE_V1;
        link.side = data[base];
        link.to = u32_le(data, base + 4) as i32;
        link.next = u32_le(data, base + 8) as i32;
        link.t0 = u32_le(data, base + 12) as i32;
        link.t1 = u32_le(data, base + 16) as i32;
    }
    *offset += size;

    validate_links(tile)
}

fn decode_links_v2(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.links.len() * LINK_WIRE_SIZE;
    if *offset + size > data.len() {
        return Err(err_bad_tile("links block truncated".to_string()));
    }
    for (i, link) in tile.links.iter_mut().enumerate() {
        let base = *offset + i * LINK_WIRE_SIZE;
        link.side = data[base];
        link.to = u32_le(data, base + 4) as i32;
        link.next = u32_le(data, base + 8) as i32;
        link.t0 = u16_le(data, base + 12) as i32;
        link.t1 = u16_le(data, base + 14) as i32;
    }
    *offset += size;

    validate_links(tile)
}

fn validate_links(tile: &Tile) -> Result<(), NavError> {
    for (i, link) in tile.links.iter().enumerate() {
        if link.side > POLY_SIDE_MAX_Y {
            return Err(err_bad_tile(format!("link {} side {}", i, link.side)));
        }
        if link.to >= tile.polys.len() as i32 || link.to < -(tile.ext_links.len() as i32) {
            return Err(err_bad_tile(format!("link {} target {}", i, link.to)));
        }
        if link.next >= tile.links.len() as i32 {
            return Err(err_bad_tile(format!("link {} next {}", i, link.next)));
        }
        if link.t0 > link.t1 {
            return Err(err_bad_tile(format!(
                "link {} span {}..{}",
                i, link.t0, link.t1
            )));
        }
    }

    Ok(())
}

fn decode_ext_links_v1(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.ext_links.len() * EXT_LINK_WIRE_SIZE_V1;
    if *offset + size > data.len() {
        return Err(err_bad_tile("ext links block truncated".to_string()));
    }
    for (i, ext) in tile.ext_links.iter_mut().enumerate() {
        let base = *offset + i * EXT_LINK_WIRE_SIZE_V1;
        ext.col = u32_le(data, base) as i32;
        ext.row = u32_le(data, base + 4) as i32;
        ext.poly = u32_le(data, base + 8);
    }
    *offset += size;

    Ok(())
}

fn decode_ext_links_v2(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let size = tile.ext_links.len() * EXT_LINK_WIRE_SIZE;
    if *offset + size > data.len() {
        return Err(err_bad_tile("ext links block truncated".to_string()));
    }
    for (i, ext) in tile.ext_links.iter_mut().enumerate() {
        let base = *offset + i * EXT_LINK_WIRE_SIZE;
        ext.col = u16_le(data, base) as i16 as i32;
        ext.row = u16_le(data, base + 2) as i16 as i32;
        ext.poly = u32_le(data, base + 4);
    }
    *offset += size;

    Ok(())
}

fn decode_bv_tree(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let tree_len = tile.bv_tree.as_ref().map_or(0, |t| t.len());
    let size = tree_len * BV_NODE_WIRE_SIZE;
    if *offset + size > data.len() {
        return Err(err_bad_tile("bvtree block truncated".to_string()));
    }
    let tree = tile.bv_tree.as_mut().unwrap();
    for (i, node) in tree.iter_mut().enumerate() {
        let base = *offset + i * BV_NODE_WIRE_SIZE;
        for a in 0..3 {
            node.b_min[a] = u16_le(data, base + a * 2);
            node.b_max[a] = u16_le(data, base + 6 + a * 2);
        }
        node.i = u32_le(data, base + 12) as i32;
    }
    *offset += size;

    Ok(())
}

/// Folds the side and the crossing span into one word (the side 2
/// bits, the t0 and the t1 12 bits each).
pub fn pack_link_span(side: u8, t0: i32, t1: i32) -> u32 {
    side as u32 | (t0 as u32) << 2 | (t1 as u32) << 14
}

/// Splits the packed span word.
pub fn unpack_link_span(word: u32) -> (u8, i32, i32) {
    (
        (word & 3) as u8,
        ((word >> 2) & 0xFFF) as i32,
        ((word >> 14) & 0xFFF) as i32,
    )
}

/// Folds a signed delta into the uvarint range.
pub fn zigzag(v: i64) -> u64 {
    ((v << 1) as u64) ^ ((v >> 63) as u64)
}

/// Unfolds the signed delta.
pub fn unzigzag(v: u64) -> i64 {
    ((v >> 1) as i64) ^ -((v & 1) as i64)
}

/// Reads a Go `binary.Uvarint` value; returns None on a broken
/// stream.
pub fn read_uvarint(data: &[u8], cursor: &mut usize) -> Option<u64> {
    let mut x: u64 = 0;
    let mut s: u32 = 0;
    for i in 0..10 {
        if *cursor >= data.len() {
            return None;
        }
        let b = data[*cursor];
        *cursor += 1;
        if b < 0x80 {
            if i == 9 && b > 1 {
                return None;
            }
            return Some(x | ((b as u64) << s));
        }
        x |= ((b & 0x7f) as u64) << s;
        s += 7;
    }

    None
}

/// Decodes the columnar version 3 sections.
fn decode_tile_v3(data: &[u8], offset: &mut usize, tile: &mut Tile) -> Result<(), NavError> {
    let poly_n = tile.polys.len();
    let link_n = tile.links.len();
    let grid_entries = u32_le(data, 32) as usize;
    let to_stream_size = u32_le(data, 36) as usize;

    if *offset + 8 * 2 * poly_n > data.len() {
        return Err(err_bad_tile("the v3 poly planes truncate".to_string()));
    }
    let plane = |plane: usize, i: usize| -> u16 { u16_le(data, *offset + plane * 2 * poly_n + i * 2) };
    for (i, poly) in tile.polys.iter_mut().enumerate() {
        poly.x0 = plane(0, i) as i32;
        poly.y0 = plane(1, i) as i32;
        poly.x1 = plane(2, i) as i32;
        poly.y1 = plane(3, i) as i32;
        poly.h00 = plane(4, i) as i16;
        poly.h10 = plane(5, i) as i16;
        poly.h01 = plane(6, i) as i16;
        poly.h11 = plane(7, i) as i16;
    }
    *offset += 8 * 2 * poly_n;

    if *offset + poly_n > data.len() {
        return Err(err_bad_tile("the v3 area plane truncates".to_string()));
    }
    for (i, poly) in tile.polys.iter_mut().enumerate() {
        poly.area = data[*offset + i];
    }
    *offset += poly_n;

    let size = 4 * (poly_n + 1);
    if *offset + size > data.len() {
        return Err(err_bad_tile("the v3 link CSR truncates".to_string()));
    }
    let mut chain_offsets = vec![0u32; poly_n + 1];
    for (i, chain) in chain_offsets.iter_mut().enumerate() {
        *chain = u32_le(data, *offset + i * 4);
    }
    if poly_n > 0 && chain_offsets[0] != 0 {
        return Err(err_bad_tile(format!("the v3 link CSR head {}", chain_offsets[0])));
    }
    for i in 0..poly_n {
        if chain_offsets[i] > chain_offsets[i + 1] || chain_offsets[i + 1] > link_n as u32 {
            return Err(err_bad_tile(format!("the v3 link CSR step {i}")));
        }
    }
    if chain_offsets[poly_n] as usize != link_n {
        return Err(err_bad_tile(format!(
            "the v3 link CSR tail {} of {}",
            chain_offsets[poly_n], link_n
        )));
    }
    *offset += size;

    let size = 4 * link_n;
    if *offset + size > data.len() {
        return Err(err_bad_tile("the v3 span plane truncates".to_string()));
    }
    for (i, link) in tile.links.iter_mut().enumerate() {
        let (side, t0, t1) = unpack_link_span(u32_le(data, *offset + i * 4));
        link.side = side;
        link.t0 = t0;
        link.t1 = t1;
    }
    *offset += size;

    if *offset + to_stream_size > data.len() {
        return Err(err_bad_tile("the v3 target stream truncates".to_string()));
    }
    let mut cursor = *offset;
    for i in 0..poly_n {
        let end = chain_offsets[i + 1];
        if end > chain_offsets[i] {
            tile.polys[i].first_link = chain_offsets[i] as i32;
        } else {
            // The isolated surface: the empty chain answers -1 (the
            // wire CSR only carries the non empty chains).
            tile.polys[i].first_link = -1;
        }
        let mut e = chain_offsets[i];
        while e < end {
            let zz = read_uvarint(data, &mut cursor)
                .ok_or_else(|| err_bad_tile("the v3 target stream breaks".to_string()))?;
            let to = unzigzag(zz) + i as i64;
            tile.links[e as usize].to = to as i32;
            if e + 1 < end {
                tile.links[e as usize].next = (e + 1) as i32;
            } else {
                tile.links[e as usize].next = -1;
            }
            e += 1;
        }
    }
    if cursor != *offset + to_stream_size {
        return Err(err_bad_tile("the v3 target stream overruns".to_string()));
    }
    *offset += to_stream_size;

    decode_ext_links_v2(data, offset, tile)?;

    let size = 4 * (GRID_BUCKETS + 1);
    if *offset + size > data.len() {
        return Err(err_bad_tile("the v3 grid offsets truncate".to_string()));
    }
    let mut grid = TileGrid {
        offsets: vec![0u32; GRID_BUCKETS + 1],
        entries: Vec::new(),
    };
    for (i, off) in grid.offsets.iter_mut().enumerate() {
        *off = u32_le(data, *offset + i * 4);
    }
    if grid.offsets[GRID_BUCKETS] as usize != grid_entries {
        return Err(err_bad_tile(format!(
            "the v3 grid tail {} of {}",
            grid.offsets[GRID_BUCKETS], grid_entries
        )));
    }
    *offset += size;

    let mut entries = vec![0u32; grid_entries];
    let mut position = *offset;
    for b in 0..GRID_BUCKETS {
        // The per bucket delta base: the zigzag-free uvarint deltas
        // restart from zero at every bucket (the encode side resets
        // prev the same way).
        let mut prev: u64 = 0;
        let mut e = grid.offsets[b];
        while e < grid.offsets[b + 1] {
            let delta = read_uvarint(data, &mut position)
                .ok_or_else(|| err_bad_tile("the v3 grid stream breaks".to_string()))?;
            prev += delta;
            entries[e as usize] = prev as u32;
            e += 1;
        }
    }
    if position != data.len() {
        return Err(err_bad_tile("the v3 grid stream overruns".to_string()));
    }
    grid.entries = entries;
    tile.grid = Some(grid);
    *offset = data.len();

    validate_polys(tile)?;
    validate_links(tile)
}

/// Returns the side of the neighbor polygon that faces the given
/// side of the shared edge.
pub fn opposite_side(side: u8) -> u8 {
    match side {
        POLY_SIDE_MIN_X => POLY_SIDE_MAX_X,
        POLY_SIDE_MAX_X => POLY_SIDE_MIN_X,
        POLY_SIDE_MIN_Y => POLY_SIDE_MAX_Y,
        _ => POLY_SIDE_MIN_Y,
    }
}

/// The min and the max corner height of a polygon (the bilinear
/// surface stays inside the corner range).
pub fn poly_height_range(poly: &Poly) -> (i16, i16) {
    let mut h_min = poly.h00;
    let mut h_max = poly.h00;
    for h in [poly.h00, poly.h10, poly.h01, poly.h11] {
        if h < h_min {
            h_min = h;
        }
        if h > h_max {
            h_max = h;
        }
    }

    (h_min, h_max)
}

/// Returns the polygon of a reference together with its tile, or
/// None when the tile is unavailable or the index is stale.
pub fn tile_of_ref(ref_: PolyRef, tiles: &dyn Fn(RegionKey) -> Option<Arc<Tile>>) -> Option<(Arc<Tile>, i32)> {
    let (col, row) = tile_of(ref_);
    let tile = tiles(RegionKey { col, row })?;
    let poly = poly_of(ref_);
    if poly < 0 || poly as usize >= tile.polys.len() {
        return None;
    }

    Some((tile, poly))
}
