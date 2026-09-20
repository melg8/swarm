// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The shortcut pass of the route answer (the smoothing): the funnel
//! waypoints merge into the longest chords the corridor geometry
//! allows.

use crate::mesh::Mesh;
use crate::query::Pos;
use crate::tile::{
    CELL_SIZE_WORLD, POLY_SIDE_MAX_X, POLY_SIDE_MAX_Y, POLY_SIDE_MIN_X, POLY_SIDE_MIN_Y, PolyRef,
};
use std::collections::HashMap;

/// Relaxes the wall distance rule by a hair: the funnel pivots sit
/// exactly one radius off the wall edges and the chords through them
/// must survive the float rounding.
pub const SMOOTH_EPSILON: f64 = 1e-6;

/// One closed wall portion of a polygon side in world coordinates.
#[derive(Debug, Clone, Copy)]
pub struct WallSpan {
    pub ax: f64,
    pub ay: f64,
    pub bx: f64,
    pub by: f64,
}

/// The wall span cache of one shortcut pass.
pub type WallCache = HashMap<PolyRef, Option<Box<[Vec<WallSpan>; 4]>>>;

/// Returns the wall spans of one polygon, computing them once per
/// shortcut pass.
pub fn wall_spans_of<'a>(
    mesh: &Mesh,
    ref_: PolyRef,
    walls: &'a mut WallCache,
) -> Option<&'a [Vec<WallSpan>; 4]> {
    if !walls.contains_key(&ref_) {
        let spans = mesh.compute_wall_spans(ref_);
        walls.insert(ref_, spans);
    }

    walls
        .get(&ref_)
        .expect("the entry was just inserted")
        .as_deref()
}

impl Mesh {
    /// Derives the closed wall portions of the polygon sides: every
    /// side spans the whole rectangle edge; the link chain carves the
    /// open spans out of it; the complement is the wall.
    pub fn compute_wall_spans(&self, ref_: PolyRef) -> Option<Box<[Vec<WallSpan>; 4]>> {
        let (tile, poly_idx) = self.poly_of_ref(ref_)?;
        let poly = &tile.polys[poly_idx as usize];

        let mut open: [Vec<(i32, i32)>; 4] = [Vec::new(), Vec::new(), Vec::new(), Vec::new()];
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = &tile.links[li as usize];
            li = link.next;
            open[link.side as usize].push((link.t0, link.t1 + 1));
        }

        let mut spans: Box<[Vec<WallSpan>; 4]> = Box::new([
            Vec::new(),
            Vec::new(),
            Vec::new(),
            Vec::new(),
        ]);
        for &side in &[POLY_SIDE_MIN_X, POLY_SIDE_MAX_X, POLY_SIDE_MIN_Y, POLY_SIDE_MAX_Y] {
            let (lo, hi) = if side == POLY_SIDE_MIN_X || side == POLY_SIDE_MAX_X {
                (poly.y0, poly.y1)
            } else {
                (poly.x0, poly.x1)
            };
            for gap in complement_spans(lo, hi, &open[side as usize]) {
                spans[side as usize].push(side_span_segment(&tile, &poly, side, gap.0, gap.1));
            }
        }

        Some(spans)
    }
}

/// Maps a cell range of one polygon side onto the world segment of
/// the rectangle edge.
fn side_span_segment(
    tile: &crate::Tile,
    poly: &crate::tile::Poly,
    side: u8,
    c0: i32,
    c1: i32,
) -> WallSpan {
    let wx = |c: i32| tile.world_min_x + c as f64 * CELL_SIZE_WORLD;
    let wy = |c: i32| tile.world_min_y + c as f64 * CELL_SIZE_WORLD;

    match side {
        POLY_SIDE_MIN_X => WallSpan {
            ax: wx(poly.x0),
            ay: wy(c0),
            bx: wx(poly.x0),
            by: wy(c1),
        },
        POLY_SIDE_MAX_X => WallSpan {
            ax: wx(poly.x1),
            ay: wy(c0),
            bx: wx(poly.x1),
            by: wy(c1),
        },
        POLY_SIDE_MIN_Y => WallSpan {
            ax: wx(c0),
            ay: wy(poly.y0),
            bx: wx(c1),
            by: wy(poly.y0),
        },
        _ => WallSpan {
            ax: wx(c0),
            ay: wy(poly.y1),
            bx: wx(c1),
            by: wy(poly.y1),
        },
    }
}

/// Returns the gaps the open spans leave inside the cell range
/// [lo, hi): the closed portions of the side.
pub fn complement_spans(lo: i32, hi: i32, open: &[(i32, i32)]) -> Vec<(i32, i32)> {
    if hi <= lo {
        return Vec::new();
    }
    let mut sorted: Vec<(i32, i32)> = open.to_vec();
    sorted.sort_by_key(|span| span.0);

    let mut gaps = Vec::with_capacity(sorted.len() + 1);
    let mut cursor = lo;
    for span in sorted {
        if span.1 <= cursor {
            continue;
        }
        if span.0 > cursor {
            let end = span.0.min(hi);
            gaps.push((cursor, end));
        }
        if span.1 > cursor {
            cursor = span.1;
        }
        if cursor >= hi {
            break;
        }
    }
    if cursor < hi {
        gaps.push((cursor, hi));
    }

    gaps
}

/// Answers whether the segment keeps the clearance from every wall
/// span of the polygon (2D).
pub fn poly_wall_clear(spans: &[Vec<WallSpan>; 4], a: Pos, b: Pos, clearance: f64) -> bool {
    let limit = clearance - SMOOTH_EPSILON;
    if limit <= 0.0 {
        return true;
    }
    let limit_sq = limit * limit;
    for side in 0..4 {
        for span in &spans[side] {
            if seg_seg_dist_sqr_2d(a.x, a.y, b.x, b.y, span.ax, span.ay, span.bx, span.by)
                < limit_sq
            {
                return false;
            }
        }
    }

    true
}

/// The squared distance between two 2D segments (the clamped closest
/// points of Ericson's Real-Time Collision Detection, the degenerate
/// cases included).
pub fn seg_seg_dist_sqr_2d(
    ax: f64, ay: f64, bx: f64, by: f64,
    cx: f64, cy: f64, dx: f64, dy: f64,
) -> f64 {
    let d1x = bx - ax;
    let d1y = by - ay;
    let d2x = dx - cx;
    let d2y = dy - cy;
    let rx = ax - cx;
    let ry = ay - cy;
    let a = d1x * d1x + d1y * d1y;
    let e = d2x * d2x + d2y * d2y;
    let f = d2x * rx + d2y * ry;

    const TINY: f64 = 1e-12;
    let mut s = 0.0;
    let mut t;
    if a <= TINY && e <= TINY {
        s = 0.0;
        t = 0.0;
    } else if a <= TINY {
        s = 0.0;
        t = clamp01(f / e);
    } else if e <= TINY {
        t = 0.0;
        let c = d1x * rx + d1y * ry;
        s = clamp01(-c / a);
    } else {
        let b = d1x * d2x + d1y * d2y;
        let denom = a * e - b * b;
        let c = d1x * rx + d1y * ry;
        if denom > TINY {
            s = clamp01((b * f - c * e) / denom);
        }
        t = (b * s + f) / e;
        if t < 0.0 {
            t = 0.0;
            s = clamp01(-c / a);
        } else if t > 1.0 {
            t = 1.0;
            s = clamp01((b - c) / a);
        }
    }

    let px = ax + d1x * s - (cx + d2x * t);
    let py = ay + d1y * s - (cy + d2y * t);

    px * px + py * py
}

/// Clamps the value into [0, 1].
fn clamp01(v: f64) -> f64 {
    v.clamp(0.0, 1.0)
}
