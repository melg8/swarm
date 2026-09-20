// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The pocket escape: the route answer for a start standing on ground
//! the link graph cannot leave.

use crate::astar::go_hypot;
use crate::avoid::{AvoidCtx, AvoidState};
use crate::mesh::Mesh;
use crate::query::{Pos, Route};
use crate::tile::{ref_of, AREA_WATER, PolyRef};
use std::collections::HashSet;

/// Bounds the link component the escape serves.
const POCKET_MAX_SIDE: f64 = 2048.0;

/// Bounds the world ring the exit search scans around the pocket box.
const POCKET_EXIT_RADIUS: f64 = 768.0;

/// Prices the water polygons of the exit search.
const POCKET_EXIT_WATER_PENALTY: f64 = 8.0;

/// Prices the exit ground under a foreign recovery ban.
const POCKET_EXIT_BAN_PENALTY: f64 = 4.0;

/// Bounds the height window of the exit candidates.
const POCKET_EXIT_Z_WINDOW: f64 = 1024.0;

/// The squared horizontal displacement the exit boundary must carry
/// from the standing point.
const POCKET_EXIT_HORIZONTAL_FLOOR_SQ: f64 = 1.0;

/// The depths of the aim march past the pocket boundary.
const POCKET_EXIT_MARCH_STEPS: [f64; 3] = [288.0, 256.0, 224.0];

/// The flooded link component of a pocket start: the seen polygons,
/// their bounding box in world units and the start polygon's own
/// world rect.
struct PocketComponent {
    seen: HashSet<PolyRef>,
    min_x: f64,
    min_y: f64,
    max_x: f64,
    max_y: f64,
    sx0: f64,
    sy0: f64,
    sx1: f64,
    sy1: f64,
}

impl PocketComponent {
    /// Grows the component box over the polygon's world rect: false
    /// when the rect would stretch the box past the pocket side.
    fn admit(&mut self, tile: &crate::Tile, poly: &crate::tile::Poly) -> bool {
        let (x0, y0, x1, y1) = tile.world_rect(poly);
        let n_min_x = self.min_x.min(x0);
        let n_min_y = self.min_y.min(y0);
        let n_max_x = self.max_x.max(x1);
        let n_max_y = self.max_y.max(y1);
        if n_max_x - n_min_x > POCKET_MAX_SIDE || n_max_y - n_min_y > POCKET_MAX_SIDE {
            return false;
        }
        self.min_x = n_min_x;
        self.min_y = n_min_y;
        self.max_x = n_max_x;
        self.max_y = n_max_y;

        true
    }
}

impl Mesh {
    /// Answers the walk out of a stranded start, None when the start
    /// is not a pocket (the component left the pocket box or no
    /// connected exit exists within the radius): the caller keeps the
    /// plain search answer in that case.
    pub fn pocket_escape_inner(
        &self,
        start_ref: PolyRef,
        start_pos: Pos,
        avoid: &AvoidCtx,
    ) -> Option<Route> {
        let component = self.flood_pocket_component(start_ref)?;
        let (exit, ok) = self.pocket_exit(&component, start_pos, avoid);
        if !ok {
            return None;
        }
        if exit.x < component.sx1
            && exit.x > component.sx0
            && exit.y < component.sy1
            && exit.y > component.sy0
        {
            // The aim collapsed back into the start footprint: the
            // plan would click the character's own position and never
            // move - the honest no exit answer keeps the plain search
            // verdict.
            return None;
        }

        Some(Route {
            found: false,
            partial: true,
            waypoints: vec![exit],
            raw_waypoints: None,
            corridor: Vec::new(),
            explored: component.seen.len(),
            hierarchical: false,
            pocket_escape: true,
        })
    }

    /// Floods the link component of startRef and returns it with its
    /// bounding box, None when the component outgrows the pocket box.
    fn flood_pocket_component(&self, start_ref: PolyRef) -> Option<PocketComponent> {
        let (col, row) = crate::tile::tile_of(start_ref);
        let tile = self
            .tile(&crate::tile::RegionKey { col, row })
            .ok()?;
        let start_idx = crate::tile::poly_of(start_ref);
        if start_idx < 0 || start_idx as usize >= tile.polys.len() {
            return None;
        }
        let poly = &tile.polys[start_idx as usize];
        let (x0, y0, x1, y1) = tile.world_rect(poly);
        if x1 - x0 > POCKET_MAX_SIDE || y1 - y0 > POCKET_MAX_SIDE {
            return None;
        }
        let mut component = PocketComponent {
            seen: HashSet::from([start_ref]),
            min_x: x0,
            min_y: y0,
            max_x: x1,
            max_y: y1,
            sx0: x0,
            sy0: y0,
            sx1: x1,
            sy1: y1,
        };
        let mut frontier: Vec<PolyRef> = vec![start_ref];
        while !frontier.is_empty() {
            let mut next: Vec<PolyRef> = Vec::new();
            for ref_ in frontier {
                match self.flood_step(&mut component, ref_)? {
                    Some(grown) => next.extend(grown),
                    None => return None,
                }
            }
            frontier = next;
        }

        Some(component)
    }

    /// Visits one frontier polygon: every unvisited link target that
    /// keeps the component inside the pocket box joins the component
    /// and the next frontier. None (the second answer false) aborts
    /// the flood with the honest ground verdict.
    fn flood_step(
        &self,
        component: &mut PocketComponent,
        ref_: PolyRef,
    ) -> Option<Option<Vec<PolyRef>>> {
        let (col, row) = crate::tile::tile_of(ref_);
        let tile = match self.tile(&crate::tile::RegionKey { col, row }) {
            Ok(tile) => tile,
            Err(_) => return Some(None),
        };
        let idx = crate::tile::poly_of(ref_);
        if idx < 0 || idx as usize >= tile.polys.len() {
            return Some(None);
        }
        let poly = &tile.polys[idx as usize];
        let mut next = Vec::new();
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = tile.links[li as usize];
            li = link.next;
            let (target, ttile, tpoly) = match self.link_target(&tile, &link) {
                Some(triple) => triple,
                None => continue,
            };
            if component.seen.contains(&target) {
                continue;
            }
            if !component.admit(&ttile, &ttile.polys[tpoly as usize]) {
                // The component would outgrow the pocket box: the
                // honest ground verdict aborts the flood.
                return None;
            }
            component.seen.insert(target);
            next.push(target);
        }

        Some(Some(next))
    }

    /// Finds the walkable exit aim of the pocket: the closest
    /// connected ground outside the component seeded as the boundary,
    /// the aim marched deep into that ground along the horizontal exit
    /// direction and snapped onto the real surface.
    fn pocket_exit(
        &self,
        component: &PocketComponent,
        start_pos: Pos,
        avoid: &AvoidCtx,
    ) -> (Pos, bool) {
        let (boundary, ok) = self.pocket_boundary(component, start_pos, avoid);
        if !ok {
            return (Pos::ZERO, false);
        }

        (self.pocket_aim(component, start_pos, boundary), true)
    }

    /// Scans the world ring around the pocket box for the closest
    /// point of the connected ground outside the component.
    fn pocket_boundary(
        &self,
        component: &PocketComponent,
        start_pos: Pos,
        avoid: &AvoidCtx,
    ) -> (Pos, bool) {
        let min_x = component.min_x - POCKET_EXIT_RADIUS;
        let min_y = component.min_y - POCKET_EXIT_RADIUS;
        let max_x = component.max_x + POCKET_EXIT_RADIUS;
        let max_y = component.max_y + POCKET_EXIT_RADIUS;
        let min_z = start_pos.z - POCKET_EXIT_Z_WINDOW;
        let max_z = start_pos.z + POCKET_EXIT_Z_WINDOW;

        let mut best_dist = f64::MAX;
        let mut boundary = Pos::ZERO;
        let mut found = false;
        let mut candidates: Vec<i32> = Vec::new();
        for key in crate::query::region_keys_of_box(min_x, min_y, max_x, max_y) {
            let tile = match self.tile(&key) {
                Ok(tile) => tile,
                Err(_) => continue,
            };
            candidates.clear();
            crate::query::tile_query_polys(
                &tile,
                min_x,
                min_y,
                max_x,
                max_y,
                min_z,
                max_z,
                &mut candidates,
            );
            for &pi in candidates.iter() {
                let ref_ = ref_of(key.col, key.row, pi as u32);
                if component.seen.contains(&ref_) {
                    continue;
                }
                let poly = &tile.polys[pi as usize];
                if poly.first_link < 0 {
                    // Never exit into another island: the escape must
                    // land on ground the next plan cycle routes from.
                    continue;
                }
                let (cx, cy, cz) = tile.closest_point(poly, start_pos.x, start_pos.y, start_pos.z);
                let dx = start_pos.x - cx;
                let dy = start_pos.y - cy;
                if dx * dx + dy * dy < POCKET_EXIT_HORIZONTAL_FLOOR_SQ {
                    // The ground directly under the standing point:
                    // not a walkable exit.
                    continue;
                }
                let dz = start_pos.z - cz;
                let mut dist = (dx * dx + dy * dy + dz * dz).sqrt();
                if poly.area == AREA_WATER {
                    dist *= POCKET_EXIT_WATER_PENALTY;
                }
                if avoid.state(&tile, poly) == AvoidState::Wall {
                    dist *= POCKET_EXIT_BAN_PENALTY;
                }
                if dist < best_dist {
                    best_dist = dist;
                    boundary = Pos {
                        x: cx,
                        y: cy,
                        z: cz,
                    };
                    found = true;
                }
            }
        }

        (boundary, found)
    }

    /// Carries the boundary point deep into the exit ground along the
    /// horizontal exit direction and snaps it onto the real surface.
    fn pocket_aim(&self, component: &PocketComponent, start_pos: Pos, boundary: Pos) -> Pos {
        let dir_x = boundary.x - start_pos.x;
        let dir_y = boundary.y - start_pos.y;
        let length = go_hypot(dir_x, dir_y);
        if length < 1.0 {
            return boundary;
        }
        for march in POCKET_EXIT_MARCH_STEPS {
            let frac = march / length;
            let aim = Pos {
                x: boundary.x + dir_x * frac,
                y: boundary.y + dir_y * frac,
                z: boundary.z,
            };
            let (ref_, pos) = match self.find_nearest_poly(aim) {
                Some(pair) => pair,
                None => continue,
            };
            if component.seen.contains(&ref_) {
                continue;
            }
            let (_tile, poly_idx) = match self.poly_of_ref(ref_) {
                Some(pair) => pair,
                None => continue,
            };
            let tile = match self.tile_of_ref(ref_) {
                Some(tile) => tile,
                None => continue,
            };
            if tile.polys[poly_idx as usize].first_link < 0 {
                continue;
            }

            return pos;
        }

        boundary
    }
}

/// The pocket escape entry the route query calls (the free function
/// form the Go twin uses).
pub fn pocket_escape(mesh: &Mesh, start_ref: PolyRef, start_pos: Pos, avoid: &AvoidCtx) -> Option<Route> {
    mesh.pocket_escape_inner(start_ref, start_pos, avoid)
}
