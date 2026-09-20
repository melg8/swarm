// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The avoid areas of the corridor search: the recovery bans of the
//! hunt loop ported onto the rectangle granularity of the mesh.

use crate::query::Pos;
use crate::tile::Poly;

/// One banned world disk of a search: the recovery ban the hunt loop
/// derives from its freeze reports.
#[derive(Debug, Clone, Copy)]
pub struct AvoidCircle {
    pub center_x: f64,
    pub center_y: f64,
    pub radius: f64,
}

/// Prices the escape polygons of the ban that holds the search start.
pub const AVOID_ESCAPE_MULTIPLIER: f64 = 6.0;

/// Bounds the escape polygons of the own ban.
pub const AVOID_ESCAPE_RADIUS: f64 = 256.0;

/// Prices the steps onto a foreign banned polygon whose own portal
/// segment stays clear of the circle.
pub const AVOID_GRAZED_MULTIPLIER: f64 = AVOID_ESCAPE_MULTIPLIER * AVOID_ESCAPE_MULTIPLIER;

/// Classifies one polygon against the avoid circles of a search.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AvoidState {
    /// No circle touches the footprint.
    Free,
    /// A wall under the ban rules.
    Wall,
    /// Own ban near the start: priced.
    Escape,
}

/// The per-search avoid context, derived once from the filter circles
/// and the raw start position.
pub struct AvoidCtx {
    areas: Vec<AvoidCircle>,
    escape_idx: isize,
    start: Pos,
    active: bool,
}

/// Derives the avoid context. The escape index is the first circle
/// containing the start position.
pub fn new_avoid_ctx(areas: &[AvoidCircle], start: Pos) -> AvoidCtx {
    let mut ctx = AvoidCtx {
        areas: areas.to_vec(),
        escape_idx: -1,
        start,
        active: !areas.is_empty(),
    };
    if !ctx.active {
        return ctx;
    }
    for (i, area) in areas.iter().enumerate() {
        if go_hypot(start.x - area.center_x, start.y - area.center_y) <= area.radius {
            ctx.escape_idx = i as isize;

            break;
        }
    }

    ctx
}

/// The avoid context of the searches without bans.
pub fn no_avoid() -> AvoidCtx {
    AvoidCtx {
        areas: Vec::new(),
        escape_idx: -1,
        start: Pos::ZERO,
        active: false,
    }
}

impl AvoidCtx {
    /// Classifies one polygon of one tile against the circles of the
    /// context:
    ///
    /// - a polygon touched by a FOREIGN ban is a wall, even when the
    ///   own escape ring overlaps it too,
    /// - a polygon touched only by the own ban is an escape polygon
    ///   while its footprint reaches within avoidEscapeRadius of the
    ///   start, a wall beyond it,
    /// - a polygon no circle touches is free.
    pub fn state(&self, tile: &crate::Tile, poly: &Poly) -> AvoidState {
        if !self.active {
            return AvoidState::Free;
        }
        let (x0, y0, x1, y1) = tile.world_rect(poly);
        let mut own = false;
        for (i, area) in self.areas.iter().enumerate() {
            if !circle_overlaps_rect(area, x0, y0, x1, y1) {
                continue;
            }
            if i as isize != self.escape_idx {
                return AvoidState::Wall;
            }
            own = true;
        }
        if !own {
            return AvoidState::Free;
        }
        if rect_point_dist(x0, y0, x1, y1, self.start.x, self.start.y) <= AVOID_ESCAPE_RADIUS {
            return AvoidState::Escape;
        }

        AvoidState::Wall
    }

    /// Reports whether a FOREIGN ban circle touches the polygon
    /// footprint.
    pub fn foreign_touched(&self, tile: &crate::Tile, poly: &Poly) -> bool {
        if !self.active {
            return false;
        }
        let (x0, y0, x1, y1) = tile.world_rect(poly);
        for (i, area) in self.areas.iter().enumerate() {
            if i as isize == self.escape_idx {
                continue;
            }
            if circle_overlaps_rect(area, x0, y0, x1, y1) {
                return true;
            }
        }

        false
    }

    /// Reports whether the walkable portal segment crosses a foreign
    /// ban circle.
    pub fn portal_banned(&self, ax: f64, ay: f64, bx: f64, by: f64) -> bool {
        if !self.active {
            return false;
        }
        let dx = bx - ax;
        let dy = by - ay;
        for (i, area) in self.areas.iter().enumerate() {
            if i as isize == self.escape_idx {
                continue;
            }
            let mut t = 0.0;
            let len2 = dx * dx + dy * dy;
            if len2 > 0.0 {
                t = (((area.center_x - ax) * dx + (area.center_y - ay) * dy) / len2)
                    .clamp(0.0, 1.0);
            }
            let px = ax + dx * t;
            let py = ay + dy * t;
            if go_hypot(area.center_x - px, area.center_y - py) <= area.radius {
                return true;
            }
        }

        false
    }
}

/// Reports whether a circle and an axis aligned rectangle share any
/// point (the standard clamp test).
fn circle_overlaps_rect(c: &AvoidCircle, x0: f64, y0: f64, x1: f64, y1: f64) -> bool {
    let px = c.center_x.clamp(x0, x1);
    let py = c.center_y.clamp(y0, y1);
    let dx = c.center_x - px;
    let dy = c.center_y - py;

    dx * dx + dy * dy <= c.radius * c.radius
}

/// The planar distance from a point to an axis aligned rectangle
/// (zero inside).
fn rect_point_dist(x0: f64, y0: f64, x1: f64, y1: f64, px: f64, py: f64) -> f64 {
    let dx = (x0 - px).max(px - x1).max(0.0);
    let dy = (y0 - py).max(py - y1).max(0.0);

    go_hypot(dx, dy)
}

use crate::astar::go_hypot;
