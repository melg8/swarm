// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The funnel algorithm (string pulling) over the corridor portals,
//! with the wall abutting span end clearance.

use crate::astar::go_hypot;
use crate::mesh::Mesh;
use crate::query::Pos;
use crate::tile::{
    opposite_side, CELL_SIZE_WORLD, POLY_SIDE_MAX_X, POLY_SIDE_MAX_Y, POLY_SIDE_MIN_X,
    POLY_SIDE_MIN_Y, PolyRef,
};

/// The 2D coincidence threshold of the funnel.
pub const FUNNEL_EPSILON: f64 = 1e-4;

/// One waypoint of the raw funnel answer: the position and the
/// corridor index of the portal the waypoint sits on (-1 for the
/// start projection before the first portal, len(corridor)-1 for the
/// end projection).
#[derive(Debug, Clone, Copy)]
pub struct FunnelWp {
    pub pos: Pos,
    pub portal: i32,
}

/// Strips the funnel waypoints down to their positions.
pub fn funnel_positions(wps: &[FunnelWp]) -> Vec<Pos> {
    wps.iter().map(|wp| wp.pos).collect()
}

impl Mesh {
    /// The funnel answer with the corridor walk addresses (the portal
    /// index of every waypoint) the shortcut pass replays.
    pub fn straight_path_wps(
        &self,
        corridor: &[PolyRef],
        start_pos: Pos,
        end_pos: Pos,
        clearance: f64,
    ) -> Vec<FunnelWp> {
        if corridor.is_empty() {
            return Vec::new();
        }
        let first = match self.poly_of_ref(corridor[0]) {
            Some((tile, poly)) => (tile, poly),
            None => return Vec::new(),
        };
        let (sx, sy, sz) =
            first
                .0
                .closest_point(&first.0.polys[first.1 as usize], start_pos.x, start_pos.y, start_pos.z);
        let closest_start = Pos {
            x: sx,
            y: sy,
            z: sz,
        };
        let last = match self.poly_of_ref(corridor[corridor.len() - 1]) {
            Some((tile, poly)) => (tile, poly),
            None => return Vec::new(),
        };
        let (ex, ey, ez) = last.0.closest_point(
            &last.0.polys[last.1 as usize],
            end_pos.x,
            end_pos.y,
            end_pos.z,
        );
        let closest_end = Pos { x: ex, y: ey, z: ez };

        let mut waypoints: Vec<FunnelWp> = Vec::with_capacity(corridor.len() + 1);
        append_wp(&mut waypoints, closest_start, -1);

        if corridor.len() > 1 {
            self.run_funnel(corridor, closest_start, closest_end, clearance, &mut waypoints);
        }

        append_wp(&mut waypoints, closest_end, (corridor.len() - 1) as i32);

        waypoints
    }

    /// The funnel state machine over the corridor portals.
    ///
    /// The cone side labels follow the local travel direction; the
    /// signed area convention here is the standard cross product
    /// (positive = counter-clockwise left), the negative of the
    /// Detour dtTriArea2D - every comparison flips with it.
    fn run_funnel(
        &self,
        corridor: &[PolyRef],
        closest_start: Pos,
        closest_end: Pos,
        clearance: f64,
        waypoints: &mut Vec<FunnelWp>,
    ) {
        let mut cone = FunnelCone {
            apex: closest_start,
            left: closest_start,
            right: closest_start,
            apex_index: 0,
            left_index: 0,
            right_index: 0,
        };

        let mut i = 0usize;
        while i < corridor.len() {
            let (left, right, ok) = self.corridor_portal(corridor, i, closest_end, clearance);
            if !ok {
                // A broken chain ends the walk at the last connected
                // polygon.
                return;
            }
            if i == 0 && dist_pt_seg_sqr_2d(cone.apex, left, right) < FUNNEL_EPSILON {
                i += 1;

                continue;
            }
            let (apex, inverted) = cone.narrow_right(right, i);
            if inverted {
                append_wp(waypoints, apex, cone.apex_index as i32);
                i = cone.apex_index + 1;

                continue;
            }
            let (apex, inverted) = cone.narrow_left(left, i);
            if inverted {
                append_wp(waypoints, apex, cone.apex_index as i32);
                i = cone.apex_index + 1;

                continue;
            }
            i += 1;
        }
    }

    /// Returns the left and right portal points of the corridor
    /// transition at index i, pulled inward from the wall abutting
    /// span ends.
    fn corridor_portal(
        &self,
        corridor: &[PolyRef],
        i: usize,
        closest_end: Pos,
        clearance: f64,
    ) -> (Pos, Pos, bool) {
        if i + 1 >= corridor.len() {
            return (closest_end, closest_end, true);
        }
        let (tile, poly, link) = match self.link_between(corridor[i], corridor[i + 1]) {
            Some(triple) => triple,
            None => return (Pos::ZERO, Pos::ZERO, false),
        };
        let (next_tile, next_poly) = match self.poly_of_ref(corridor[i + 1]) {
            Some(pair) => pair,
            None => return (Pos::ZERO, Pos::ZERO, false),
        };
        let (ax, ay, bx, by) = tile.portal(&tile.polys[poly as usize], &link);
        let (ax, ay, bx, by) = self.shrunk_portal_span(
            &tile,
            poly,
            &link,
            &next_tile,
            next_poly,
            ax,
            ay,
            bx,
            by,
            clearance,
        );
        let (left, right) = label_portal_ends(
            &tile,
            &tile.polys[poly as usize],
            &next_tile,
            &next_tile.polys[next_poly as usize],
            ax,
            ay,
            bx,
            by,
        );

        (left, right, true)
    }

    /// Finds the link of one polygon that leads to the next polygon
    /// of the corridor.
    pub fn link_between(
        &self,
        ref_: PolyRef,
        next: PolyRef,
    ) -> Option<(std::sync::Arc<crate::Tile>, u32, crate::tile::Link)> {
        let (tile, poly) = self.poly_of_ref(ref_)?;
        let mut li = tile.polys[poly as usize].first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = tile.links[li as usize];
            li = link.next;
            if self.resolve_link(&tile, &link) == next {
                return Some((tile, poly, link));
            }
        }

        None
    }

    /// Pulls the portal span ends inward from the walls that abut
    /// them: the capsule clearance of the turning pivots.
    pub fn shrunk_portal_span(
        &self,
        tile: &std::sync::Arc<crate::Tile>,
        poly: u32,
        link: &crate::tile::Link,
        next_tile: &std::sync::Arc<crate::Tile>,
        next_poly: u32,
        ax: f64,
        ay: f64,
        bx: f64,
        by: f64,
        clearance: f64,
    ) -> (f64, f64, f64, f64) {
        if clearance <= 0.0 {
            return (ax, ay, bx, by);
        }
        let low_pull = self.span_end_wall(tile, poly, link, next_tile, next_poly, true);
        let high_pull = self.span_end_wall(tile, poly, link, next_tile, next_poly, false);
        if !low_pull && !high_pull {
            return (ax, ay, bx, by);
        }
        let dx = bx - ax;
        let dy = by - ay;
        let length = go_hypot(dx, dy);
        if length < 1e-6 {
            return (ax, ay, bx, by);
        }
        let mut low = 0.0;
        let mut high = 0.0;
        if low_pull {
            low = clearance;
        }
        if high_pull {
            high = clearance;
        }
        if low + high >= length {
            let mid_x = (ax + bx) * 0.5;
            let mid_y = (ay + by) * 0.5;

            return (mid_x, mid_y, mid_x, mid_y);
        }
        let ux = dx / length;
        let uy = dy / length;

        (
            ax + ux * low,
            ay + uy * low,
            bx - ux * high,
            by - uy * high,
        )
    }

    /// Answers whether a wall of the walkable union abuts one end of
    /// the link portal span along the shared edge.
    pub fn span_end_wall(
        &self,
        tile: &std::sync::Arc<crate::Tile>,
        poly: u32,
        link: &crate::tile::Link,
        next_tile: &std::sync::Arc<crate::Tile>,
        next_poly: u32,
        low: bool,
    ) -> bool {
        let cell = if low { link.t0 - 1 } else { link.t1 + 1 };
        if span_side_walled(tile, &tile.polys[poly as usize], link.side, cell, low) {
            return true;
        }
        // The neighbor side of the edge: the continuation cell maps
        // into the neighbor tile grid.
        let next_cell = if !std::sync::Arc::ptr_eq(tile, next_tile) {
            map_cell_across_tiles(tile, next_tile, link.side, cell)
        } else {
            cell
        };

        span_side_walled(
            next_tile,
            &next_tile.polys[next_poly as usize],
            opposite_side(link.side),
            next_cell,
            low,
        )
    }
}

/// The narrowing cone of the funnel algorithm: the apex and the two
/// portal points it advanced through, with the corridor indices the
/// restarts jump back to.
struct FunnelCone {
    apex: Pos,
    left: Pos,
    right: Pos,
    apex_index: usize,
    left_index: usize,
    right_index: usize,
}

impl FunnelCone {
    /// Advances the right cone side through the right point of portal
    /// i. Reports the new apex when the cone inverts on the left side.
    fn narrow_right(&mut self, right: Pos, i: usize) -> (Pos, bool) {
        if tri_area_2d(self.apex, self.right, right) < 0.0 {
            return (Pos::ZERO, false);
        }
        if same_2d(self.apex, self.right) || tri_area_2d(self.apex, self.left, right) < 0.0 {
            self.right = right;
            self.right_index = i;

            return (Pos::ZERO, false);
        }
        self.apex = self.left;
        self.apex_index = self.left_index;
        let apex = self.apex;
        self.left = apex;
        self.right = apex;
        self.left_index = self.apex_index;
        self.right_index = self.apex_index;

        (apex, true)
    }

    /// Advances the left cone side through the left point of portal
    /// i. Reports the new apex when the cone inverts on the right
    /// side.
    fn narrow_left(&mut self, left: Pos, i: usize) -> (Pos, bool) {
        if tri_area_2d(self.apex, self.left, left) > 0.0 {
            return (Pos::ZERO, false);
        }
        if same_2d(self.apex, self.left) || tri_area_2d(self.apex, self.right, left) > 0.0 {
            self.left = left;
            self.left_index = i;

            return (Pos::ZERO, false);
        }
        self.apex = self.right;
        self.apex_index = self.right_index;
        let apex = self.apex;
        self.left = apex;
        self.right = apex;
        self.left_index = self.apex_index;
        self.right_index = self.apex_index;

        (apex, true)
    }
}

/// Returns the portal endpoints labeled by the local travel
/// direction: the endpoint on the left of the corridor crossing is
/// the left cone side, the other one the right.
fn label_portal_ends(
    tile: &crate::Tile,
    poly: &crate::tile::Poly,
    next_tile: &crate::Tile,
    next_poly: &crate::tile::Poly,
    ax: f64,
    ay: f64,
    bx: f64,
    by: f64,
) -> (Pos, Pos) {
    let mid_x = (ax + bx) * 0.5;
    let mid_y = (ay + by) * 0.5;
    let (cx0, cy0) = rect_center(tile, poly);
    let (cx1, cy1) = rect_center(next_tile, next_poly);
    let dx = cx1 - cx0;
    let dy = cy1 - cy0;
    // The cross product of the travel direction with the endpoint
    // offset picks the left endpoint.
    let cross_a = dx * (ay - mid_y) - dy * (ax - mid_x);
    if cross_a > 0.0 {
        let left = Pos {
            x: ax,
            y: ay,
            z: tile.height_at(poly, ax, ay),
        };
        let right = Pos {
            x: bx,
            y: by,
            z: tile.height_at(poly, bx, by),
        };

        (left, right)
    } else {
        let left = Pos {
            x: bx,
            y: by,
            z: tile.height_at(poly, bx, by),
        };
        let right = Pos {
            x: ax,
            y: ay,
            z: tile.height_at(poly, ax, ay),
        };

        (left, right)
    }
}

/// The world center of a rectangle polygon.
pub fn rect_center(tile: &crate::Tile, poly: &crate::tile::Poly) -> (f64, f64) {
    let (x0, y0, x1, y1) = tile.world_rect(poly);

    ((x0 + x1) * 0.5, (y0 + y1) * 0.5)
}

/// Answers whether the shared edge at the continuation cell walls on
/// the given polygon's side.
fn span_side_walled(
    tile: &crate::Tile,
    poly: &crate::tile::Poly,
    side: u8,
    cell: i32,
    low: bool,
) -> bool {
    if side_range_covers(poly, side, cell) {
        return !poly_side_link_covers(tile, poly, side, cell);
    }

    poly_corner_walled(tile, poly, side, low)
}

/// Answers whether the polygon's side of the given orientation spans
/// the cell coordinate.
fn side_range_covers(poly: &crate::tile::Poly, side: u8, cell: i32) -> bool {
    let (lo, hi) = if side == POLY_SIDE_MIN_Y || side == POLY_SIDE_MAX_Y {
        (poly.x0, poly.x1)
    } else {
        (poly.y0, poly.y1)
    };

    cell >= lo && cell < hi
}

/// Answers whether one of the polygon's links on the given side opens
/// the crossing at the cell coordinate.
fn poly_side_link_covers(
    tile: &crate::Tile,
    poly: &crate::tile::Poly,
    side: u8,
    cell: i32,
) -> bool {
    let mut li = poly.first_link;
    while li >= 0 && (li as usize) < tile.links.len() {
        let link = &tile.links[li as usize];
        li = link.next;
        if link.side == side && cell >= link.t0 && cell <= link.t1 {
            return true;
        }
    }

    false
}

/// Answers whether the polygon boundary walls along the perpendicular
/// side that meets the shared edge at the span end corner.
fn poly_corner_walled(
    tile: &crate::Tile,
    poly: &crate::tile::Poly,
    edge_side: u8,
    low: bool,
) -> bool {
    let (perp, corner) = match edge_side {
        POLY_SIDE_MIN_X => {
            let perp = if low { POLY_SIDE_MIN_Y } else { POLY_SIDE_MAX_Y };

            (perp, poly.x0)
        }
        POLY_SIDE_MAX_X => {
            let perp = if low { POLY_SIDE_MIN_Y } else { POLY_SIDE_MAX_Y };

            (perp, poly.x1 - 1)
        }
        POLY_SIDE_MIN_Y => {
            let perp = if low { POLY_SIDE_MIN_X } else { POLY_SIDE_MAX_X };

            (perp, poly.y0)
        }
        _ => {
            let perp = if low { POLY_SIDE_MIN_X } else { POLY_SIDE_MAX_X };

            (perp, poly.y1 - 1)
        }
    };

    !poly_side_link_covers(tile, poly, perp, corner)
}

/// Maps the continuation cell coordinate of the from tile into the
/// neighbor tile grid along the side axis.
fn map_cell_across_tiles(
    from_tile: &crate::Tile,
    to_tile: &crate::Tile,
    side: u8,
    cell: i32,
) -> i32 {
    let (world, origin) = if side == POLY_SIDE_MIN_X || side == POLY_SIDE_MAX_X {
        (
            from_tile.world_min_y + cell as f64 * CELL_SIZE_WORLD,
            to_tile.world_min_y,
        )
    } else {
        (
            from_tile.world_min_x + cell as f64 * CELL_SIZE_WORLD,
            to_tile.world_min_x,
        )
    };

    ((world - origin) / CELL_SIZE_WORLD).round() as i32
}

/// Appends a funnel corner with its corridor portal unless it
/// duplicates the previous one in 2D.
pub fn append_wp(waypoints: &mut Vec<FunnelWp>, point: Pos, portal: i32) {
    if let Some(last) = waypoints.last() {
        if same_2d(last.pos, point) {
            return;
        }
    }
    waypoints.push(FunnelWp { pos: point, portal });
}

/// The signed 2D triangle area with the standard cross product
/// convention.
pub fn tri_area_2d(a: Pos, b: Pos, c: Pos) -> f64 {
    (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x)
}

/// Reports whether two positions coincide in 2D.
pub fn same_2d(a: Pos, b: Pos) -> bool {
    (a.x - b.x).abs() < FUNNEL_EPSILON && (a.y - b.y).abs() < FUNNEL_EPSILON
}

/// The squared 2D distance of a point to a segment.
pub fn dist_pt_seg_sqr_2d(pt: Pos, a: Pos, b: Pos) -> f64 {
    let dx = b.x - a.x;
    let dy = b.y - a.y;
    if dx == 0.0 && dy == 0.0 {
        let ex = pt.x - a.x;
        let ey = pt.y - a.y;

        return ex * ex + ey * ey;
    }
    let t = ((pt.x - a.x) * dx + (pt.y - a.y) * dy) / (dx * dx + dy * dy);
    let t = t.clamp(0.0, 1.0);
    let ex = a.x + t * dx - pt.x;
    let ey = a.y + t * dy - pt.y;

    ex * ex + ey * ey
}
