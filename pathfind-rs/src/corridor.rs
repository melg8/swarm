// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The corridor region shortcut: the priced flood around the search
//! corridor, the chord walk over the open spans and the greedy
//! farthest visible merge of the funnel waypoints.

use crate::astar::{go_hypot, poly_cost, ZoneIndex};
use crate::funnel::{rect_center, FunnelWp};
use crate::mesh::Mesh;
use crate::query::{Filter, Pos};
use crate::smooth::{poly_wall_clear, wall_spans_of, WallCache};
use crate::tile::{
    PolyRef, POLY_SIDE_MAX_X, POLY_SIDE_MAX_Y, POLY_SIDE_MIN_X, POLY_SIDE_MIN_Y,
};
use std::collections::HashMap;
use std::sync::Arc;

/// The priced deviation the shortcut region grows beyond the search
/// corridor.
pub const CORRIDOR_SLACK: f64 = 96.0;

/// Caps how far the greedy scan walks back per anchor before it
/// accepts the next waypoint.
pub const SMOOTH_SCAN_WINDOW: usize = 256;

impl Mesh {
    /// Grows the search corridor into the walk region the shortcut
    /// chords may thread: a multi source flood from the chain
    /// polygons over the open links, priced like the search step and
    /// capped at the slack over the free chain. The avoid queries
    /// keep the bare chain.
    pub fn corridor_region(&self, corridor: &[PolyRef], filter: &Filter) -> HashMap<PolyRef, ()> {
        let mut region: HashMap<PolyRef, ()> = HashMap::with_capacity(corridor.len() * 2);
        for ref_ in corridor {
            region.insert(*ref_, ());
        }
        if !filter.avoid.is_empty() || corridor.len() < 2 {
            return region;
        }
        let zones = if filter.water_zones.is_empty() {
            None
        } else {
            Some(ZoneIndex::new(&filter.water_zones))
        };
        // The frontier entries: the reference and the accumulated
        // price.
        let mut frontier: Vec<(PolyRef, f64)> =
            Vec::with_capacity(corridor.len() * 4);
        for ref_ in corridor {
            frontier.push((*ref_, 0.0));
        }
        while !frontier.is_empty() {
            // The linear minimum pops the cheapest frontier entry (the
            // frontier of one region growth stays small, the heap is
            // not worth the code).
            let mut best = 0usize;
            for i in 1..frontier.len() {
                if frontier[i].1 < frontier[best].1 {
                    best = i;
                }
            }
            let current = frontier.swap_remove(best);
            if current.1 > CORRIDOR_SLACK {
                continue;
            }
            let (tile, poly_idx) = match self.poly_of_ref(current.0) {
                Some(pair) => pair,
                None => continue,
            };
            let poly = &tile.polys[poly_idx as usize];
            let from_cost = poly_cost(&tile, poly, zones.as_ref(), filter);
            let mut li = poly.first_link;
            while li >= 0 && (li as usize) < tile.links.len() {
                let link = tile.links[li as usize];
                li = link.next;
                let (target_ref, target_tile, target_poly) = match self.link_target(&tile, &link)
                {
                    Some(triple) => triple,
                    None => continue,
                };
                if region.contains_key(&target_ref) {
                    continue;
                }
                let target_cost = poly_cost(
                    &target_tile,
                    &target_tile.polys[target_poly as usize],
                    zones.as_ref(),
                    filter,
                );
                let (ax, ay, bx, by) = tile.portal(poly, &link);
                let mid_x = (ax + bx) * 0.5;
                let mid_y = (ay + by) * 0.5;
                let mid = Pos {
                    x: mid_x,
                    y: mid_y,
                    z: tile.height_at(poly, mid_x, mid_y),
                };
                let (cx0, cy0) = rect_center(&tile, poly);
                let step = go_hypot(mid.x - cx0, mid.y - cy0) * (from_cost + target_cost) * 0.5;
                let acc = current.1 + step;
                if acc > CORRIDOR_SLACK {
                    continue;
                }
                region.insert(target_ref, ());
                frontier.push((target_ref, acc));
            }
        }

        region
    }

    /// Answers whether the straight segment walks the region: from
    /// the start polygon it exits every rectangle through an open
    /// link span whose span holds the crossing, every polygon it
    /// enters belongs to the region, and the visited polygon list
    /// rides along for the wall clearance check of the caller.
    pub fn region_chord(
        &self,
        start_ref: PolyRef,
        a: Pos,
        b: Pos,
        region: &HashMap<PolyRef, ()>,
        visited: &mut Vec<PolyRef>,
    ) -> bool {
        let mut current = start_ref;
        let mut cx = a.x;
        let mut cy = a.y;
        visited.clear();
        const EPS: f64 = 1e-9;
        for _ in 0..1024 {
            if !region.contains_key(&current) {
                return false;
            }
            let (tile, poly_idx) = match self.poly_of_ref(current) {
                Some(pair) => pair,
                None => return false,
            };
            let poly = &tile.polys[poly_idx as usize];
            visited.push(current);
            let (x0, y0, x1, y1) = tile.world_rect(poly);
            if b.x >= x0 - EPS && b.x <= x1 + EPS && b.y >= y0 - EPS && b.y <= y1 + EPS {
                return true;
            }
            let dx = b.x - cx;
            let dy = b.y - cy;
            let mut exit_t = f64::MAX;
            let mut exit_side = 0u8;
            let mut exit_x = 0.0;
            let mut exit_y = 0.0;
            for side in [POLY_SIDE_MIN_X, POLY_SIDE_MAX_X, POLY_SIDE_MIN_Y, POLY_SIDE_MAX_Y] {
                let t = match side {
                    POLY_SIDE_MIN_X => {
                        if dx.abs() < 1e-12 {
                            continue;
                        }
                        (x0 - cx) / dx
                    }
                    POLY_SIDE_MAX_X => {
                        if dx.abs() < 1e-12 {
                            continue;
                        }
                        (x1 - cx) / dx
                    }
                    POLY_SIDE_MIN_Y => {
                        if dy.abs() < 1e-12 {
                            continue;
                        }
                        (y0 - cy) / dy
                    }
                    _ => {
                        if dy.abs() < 1e-12 {
                            continue;
                        }
                        (y1 - cy) / dy
                    }
                };
                if t <= EPS || t >= exit_t {
                    continue;
                }
                let px = cx + t * dx;
                let py = cy + t * dy;
                if px < x0 - EPS || px > x1 + EPS || py < y0 - EPS || py > y1 + EPS {
                    continue;
                }
                exit_t = t;
                exit_side = side;
                exit_x = px;
                exit_y = py;
            }
            if exit_t == f64::MAX {
                return false;
            }
            let next = self.region_cross(&tile, poly, exit_side, exit_x, exit_y);
            if next == 0 {
                return false;
            }
            current = next;
            cx = exit_x;
            cy = exit_y;
        }

        false
    }

    /// Resolves the polygon across the open link whose span holds the
    /// crossing point on the given side (zero: the crossing is a wall
    /// or outside every span).
    pub fn region_cross(
        &self,
        tile: &Arc<crate::Tile>,
        poly: &crate::tile::Poly,
        side: u8,
        x: f64,
        y: f64,
    ) -> PolyRef {
        const EPS: f64 = 1e-7;
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = &tile.links[li as usize];
            li = link.next;
            if link.side != side {
                continue;
            }
            let (mut lo, mut hi) = (
                tile.world_min_y + link.t0 as f64 * crate::tile::CELL_SIZE_WORLD,
                tile.world_min_y + (link.t1 + 1) as f64 * crate::tile::CELL_SIZE_WORLD,
            );
            if side == POLY_SIDE_MIN_Y || side == POLY_SIDE_MAX_Y {
                lo = tile.world_min_x + link.t0 as f64 * crate::tile::CELL_SIZE_WORLD;
                hi = tile.world_min_x + (link.t1 + 1) as f64 * crate::tile::CELL_SIZE_WORLD;
            }
            let along = if side == POLY_SIDE_MIN_Y || side == POLY_SIDE_MAX_Y { x } else { y };
            if along < lo - EPS || along > hi + EPS {
                continue;
            }
            let (target_ref, _target_tile, _target_poly) = match self.link_target(tile, link) {
                Some(triple) => triple,
                None => continue,
            };

            return target_ref;
        }

        0
    }

    /// Folds the funnel waypoints into the longest chords the
    /// corridor region allows (the greedy farthest visible walk of
    /// the shortcut pass).
    pub fn shorten_corridor_waypoints(
        &self,
        corridor: &[PolyRef],
        wps: &[FunnelWp],
        filter: &Filter,
    ) -> Vec<Pos> {
        if wps.is_empty() {
            return Vec::new();
        }
        let clearance = filter.waypoint_clearance;
        if wps.len() < 3 || clearance <= 0.0 || corridor.len() < 2 {
            return crate::funnel::funnel_positions(wps);
        }
        let region = self.corridor_region(corridor, filter);
        let mut walls: WallCache = HashMap::new();
        let mut merged: Vec<Pos> = Vec::with_capacity(wps.len());
        merged.push(wps[0].pos);
        let mut anchor = 0usize;
        let mut visited: Vec<PolyRef> = Vec::with_capacity(32);
        while anchor < wps.len() - 1 {
            let last = wps.len() - 1;
            let mut far = anchor + 1;
            if far + SMOOTH_SCAN_WINDOW < last {
                far += SMOOTH_SCAN_WINDOW;
            } else {
                far = last;
            }
            let mut chosen = anchor + 1;
            for k in (anchor + 2..=far).rev() {
                if !self.chord_walks_region(
                    corridor,
                    &wps[anchor],
                    &wps[k],
                    &region,
                    &mut walls,
                    &mut visited,
                    filter,
                ) {
                    continue;
                }
                chosen = k;

                break;
            }
            merged.push(wps[chosen].pos);
            anchor = chosen;
        }

        merged
    }

    /// Answers whether the straight chord from the waypoint `from` to
    /// the waypoint `to` walks the corridor region safely.
    pub fn chord_walks_region(
        &self,
        corridor: &[PolyRef],
        from: &FunnelWp,
        to: &FunnelWp,
        region: &HashMap<PolyRef, ()>,
        walls: &mut WallCache,
        visited: &mut Vec<PolyRef>,
        filter: &Filter,
    ) -> bool {
        let start = from.portal.max(0) as usize;
        if start >= corridor.len() {
            return false;
        }
        if !self.region_chord(corridor[start], from.pos, to.pos, region, visited) {
            return false;
        }
        for ref_ in visited.iter() {
            let spans = match wall_spans_of(self, *ref_, walls) {
                Some(spans) => spans,
                None => return false,
            };
            if !poly_wall_clear(spans, from.pos, to.pos, filter.waypoint_clearance) {
                return false;
            }
        }

        true
    }
}
