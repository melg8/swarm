// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The route query API: the nearest polygon resolution, the A*
//! corridor search entry and the funnel answer assembly.

use crate::astar::{AstarGoal, MAX_QUERY_NODES};
use crate::avoid::{new_avoid_ctx, AvoidCircle};
use crate::hierarchy::{hier_worthy, route_hierarchical};
use crate::mesh::Mesh;
use crate::pocket::pocket_escape;
use crate::funnel::funnel_positions;
use crate::tile::{
    poly_height_range, ref_of, region_of_world, PolyRef, RegionKey, Tile, CELL_SIZE_WORLD,
    GRID_BUCKET_CELLS, GRID_SIDE, HEIGHT_FLOOR,
};

/// A world position: `x` and `y` are the horizontal world axes, `z`
/// is the height.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Pos {
    pub x: f64,
    pub y: f64,
    pub z: f64,
}

impl Pos {
    pub const ZERO: Pos = Pos {
        x: 0.0,
        y: 0.0,
        z: 0.0,
    };
}

impl Default for Pos {
    fn default() -> Self {
        Pos::ZERO
    }
}

/// One server water zone cuboid (the ZoneCuboid of the C1 water.xml
/// data): the x/y box and the water surface the server tests the swim
/// state against.
#[derive(Debug, Clone, Copy)]
pub struct WaterZone {
    pub min_x: f64,
    pub max_x: f64,
    pub min_y: f64,
    pub max_y: f64,
    /// The box bottom of the zone data (kept for the honest table).
    pub min_z: f64,
    /// The water surface of the zone (the zone data maxZ).
    pub max_z: f64,
}

/// Prices the areas of a search (the water and the avoid semantics of
/// the grid engine, see the Go twin).
#[derive(Debug, Clone)]
pub struct Filter {
    /// Multiplies the step cost of the water polygons (the swim
    /// pricing).
    pub water_cost: f64,
    /// The server water zone cuboids of the world; empty prices every
    /// water polygon at `water_cost`.
    pub water_zones: Vec<WaterZone>,
    /// The recovery bans of the hunt loop; empty bans nothing.
    pub avoid: Vec<AvoidCircle>,
    /// Pulls the funnel pivots inward from the portal span ends a
    /// wall abuts before the string pulling. Zero keeps the exact
    /// pivots.
    pub waypoint_clearance: f64,
    /// Runs the shortcut pass over the funnel answer.
    pub smooth: bool,
    /// Prices the foreign banned ground whose own portal segment stays
    /// clear of the circle instead of sealing it.
    pub avoid_grazed: bool,
}

impl Default for Filter {
    fn default() -> Self {
        Filter {
            water_cost: 2.3,
            water_zones: Vec::new(),
            avoid: Vec::new(),
            waypoint_clearance: 0.0,
            smooth: false,
            avoid_grazed: false,
        }
    }
}

impl Filter {
    /// The priced search: the water polygons stay walkable at the
    /// measured swim rate.
    pub fn default_filter() -> Filter {
        Filter::default()
    }
}

/// One navigation answer: the waypoints the walker follows, the
/// polygon corridor they never leave and the search statistics.
#[derive(Debug, Clone, Default)]
pub struct Route {
    /// Whether the requested destination was reached.
    pub found: bool,
    /// The closest-reachable answer of a search whose destination is
    /// unreachable under the filter.
    pub partial: bool,
    /// The walk answer: the smoothed funnel when the filter arms the
    /// shortcut pass, the raw funnel otherwise.
    pub waypoints: Vec<Pos>,
    /// The unsmoothed funnel answer, populated when the shortcut pass
    /// ran.
    pub raw_waypoints: Option<Vec<Pos>>,
    pub corridor: Vec<PolyRef>,
    pub explored: usize,
    /// The cluster level route answer (the coarse chain search plus
    /// the refinement hops instead of one flat search).
    pub hierarchical: bool,
    /// The stranded start answer (pocket.rs): the single waypoint is
    /// the walk out of the pocket toward the nearest connected
    /// ground.
    pub pocket_escape: bool,
}

/// The query extents of the nearest polygon resolution: two cells of
/// horizontal slack around the query point and the stacked-layer
/// disambiguation window vertically.
const NEAREST_HALF_XZ: f64 = 64.0;
const NEAREST_HALF_Z: f64 = 600.0;

/// The 3D distance between two positions.
pub fn dist3(a: Pos, b: Pos) -> f64 {
    let dx = a.x - b.x;
    let dy = a.y - b.y;
    let dz = a.z - b.z;

    (dx * dx + dy * dy + dz * dz).sqrt()
}

impl Mesh {
    /// Searches the walkable route from start to end under the
    /// filter: the exact destination contract.
    pub fn route(&self, start: Pos, end: Pos, filter: &Filter) -> Result<Route, crate::NavError> {
        self.route_approach(start, end, 0.0, filter)
    }

    /// Searches the walkable route from start to end succeeding on
    /// the first polygon whose closest surface point lies within the
    /// approach radius (3D) of the end position.
    pub fn route_approach(
        &self,
        start: Pos,
        end: Pos,
        approach_radius: f64,
        filter: &Filter,
    ) -> Result<Route, crate::NavError> {
        // The cold path prefetch: the endpoint tiles decode before
        // the nearest poly resolution (sequential in this port; the
        // Go twin runs the bounded parallel pool).
        let start_key = region_of_world(start.x, start.y);
        let end_key = region_of_world(end.x, end.y);
        self.prefetch_tiles(&[start_key, end_key]);

        let (start_ref, start_pos) = self
            .find_nearest_poly(start)
            .ok_or_else(crate::no_navmesh)?;
        let (end_ref, end_pos) = self.find_nearest_poly(end).ok_or_else(crate::no_navmesh)?;

        let mut route = Route {
            found: false,
            partial: false,
            waypoints: Vec::new(),
            raw_waypoints: None,
            corridor: Vec::new(),
            explored: 0,
            hierarchical: false,
            pocket_escape: false,
        };

        let mut state = self.acquire_state();

        if start_ref == end_ref {
            route.corridor = vec![start_ref];
            let corridor = route.corridor.clone();
            self.answer_waypoints(&mut route, &corridor, start_pos, end_pos, filter);
            route.found = true;
            self.release_state(state);

            return Ok(route);
        }

        // The long routes go through the hierarchy (see hier_worthy);
        // the avoid banned queries stay flat.
        if filter.avoid.is_empty() && hier_worthy(start_ref, end_ref, filter) {
            if let Some(hierarchical) = route_hierarchical(
                self,
                start_ref,
                start_pos,
                end_ref,
                end_pos,
                approach_radius,
                filter,
                &mut state,
            ) {
                self.release_state(state);

                return Ok(hierarchical);
            }
        }

        let avoid = new_avoid_ctx(&filter.avoid, start);
        let mut result = self.astar(
            &mut state,
            AstarGoal {
                target: end_ref,
                approach: approach_radius,
            },
            start_ref,
            start_pos,
            end_pos,
            filter,
            &avoid,
            MAX_QUERY_NODES,
            None,
        );
        // The capped escalation: a flat search that hit the node
        // budget never saw the target side of the corridor - the
        // hierarchy answers those. The avoid banned queries rerun
        // with the raised budget instead.
        if result.capped && filter.avoid.is_empty() {
            if let Some(hierarchical) = route_hierarchical(
                self,
                start_ref,
                start_pos,
                end_ref,
                end_pos,
                approach_radius,
                filter,
                &mut state,
            ) {
                if hierarchical.found {
                    self.release_state(state);

                    return Ok(hierarchical);
                }
            }
        } else if result.capped {
            result = self.astar(
                &mut state,
                AstarGoal {
                    target: end_ref,
                    approach: approach_radius,
                },
                start_ref,
                start_pos,
                end_pos,
                filter,
                &avoid,
                MAX_QUERY_NODES * 8,
                None,
            );
        }
        // The pocket escape: a start the corridor search cannot leave
        // answers the closest reachable route OUT of the spot.
        if !result.reached {
            if let Some(escape) = pocket_escape(self, start_ref, start_pos, &avoid) {
                let mut escape = escape;
                escape.explored += result.explored;
                self.release_state(state);

                return Ok(escape);
            }
        }
        route.explored = result.explored;
        route.corridor = result.corridor.clone();
        if result.reached {
            route.found = true;
            self.answer_waypoints(&mut route, &result.corridor, start_pos, end_pos, filter);
        } else if result.partial && result.corridor.len() > 1 {
            route.partial = true;
            // The partial answer funnels toward the original end.
            self.answer_waypoints(&mut route, &result.corridor, start_pos, end_pos, filter);
        } else {
            route.corridor = Vec::new();
        }

        self.release_state(state);

        Ok(route)
    }

    /// Fills the route waypoints from the corridor: the raw funnel
    /// answer, folded through the corridor region shortcut when the
    /// filter arms it.
    pub fn answer_waypoints(
        &self,
        route: &mut Route,
        corridor: &[PolyRef],
        start_pos: Pos,
        end_pos: Pos,
        filter: &Filter,
    ) {
        let wps = self.straight_path_wps(corridor, start_pos, end_pos, filter.waypoint_clearance);
        if filter.smooth && filter.waypoint_clearance > 0.0 {
            route.raw_waypoints = Some(funnel_positions(&wps));
            route.waypoints = self.shorten_corridor_waypoints(corridor, &wps, filter);

            return;
        }
        route.waypoints = funnel_positions(&wps);
    }

    /// Returns the polygon the position stands on together with the
    /// closest surface point. The polygon CONTAINING the query x/y
    /// wins first (the closest surface z among the stacked
    /// candidates), the pure 3D nearest of the query window answers
    /// only when no polygon covers the x/y.
    pub fn find_nearest_poly(&self, pos: Pos) -> Option<(PolyRef, Pos)> {
        let min_x = pos.x - NEAREST_HALF_XZ;
        let max_x = pos.x + NEAREST_HALF_XZ;
        let min_y = pos.y - NEAREST_HALF_XZ;
        let max_y = pos.y + NEAREST_HALF_XZ;
        let min_z = pos.z - NEAREST_HALF_Z;
        let max_z = pos.z + NEAREST_HALF_Z;

        let mut best_ref: PolyRef = 0;
        let mut best_pos = Pos::ZERO;
        let mut best_dist = f64::MAX;
        let mut column_ref: PolyRef = 0;
        let mut column_pos = Pos::ZERO;
        let mut column_dist = f64::MAX;
        let mut column_found = false;
        let mut candidates: Vec<i32> = Vec::with_capacity(32);
        for key in region_keys_of_box(min_x, min_y, max_x, max_y) {
            let tile = match self.tile(&key) {
                Ok(t) => t,
                Err(_) => continue,
            };
            candidates.clear();
            tile_query_polys(
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
                let poly = &tile.polys[pi as usize];
                let (cx, cy, cz) = tile.closest_point(poly, pos.x, pos.y, pos.z);
                let dx = pos.x - cx;
                let dy = pos.y - cy;
                let dz = pos.z - cz;
                let dist = dx * dx + dy * dy + dz * dz;
                if dist < best_dist {
                    best_dist = dist;
                    best_ref = ref_of(key.col, key.row, pi as u32);
                    best_pos = Pos {
                        x: cx,
                        y: cy,
                        z: cz,
                    };
                }
                // The column containment: the query x/y inside the
                // polygon rect (the half open cell bounds).
                let (x0, y0, x1, y1) = tile.world_rect(poly);
                if pos.x < x0 || pos.x >= x1 || pos.y < y0 || pos.y >= y1 {
                    continue;
                }
                let z_dist = dz * dz;
                if z_dist < column_dist {
                    column_dist = z_dist;
                    column_ref = ref_of(key.col, key.row, pi as u32);
                    column_pos = Pos {
                        x: cx,
                        y: cy,
                        z: cz,
                    };
                    column_found = true;
                }
            }
        }
        if column_found {
            return Some((column_ref, column_pos));
        }
        if best_ref != 0 {
            return Some((best_ref, best_pos));
        }

        None
    }
}

/// Lists the region keys a world box may touch (at most four).
pub fn region_keys_of_box(min_x: f64, min_y: f64, max_x: f64, max_y: f64) -> Vec<RegionKey> {
    let k0 = region_of_world(min_x, min_y);
    let k1 = region_of_world(max_x, max_y);
    let mut keys: Vec<RegionKey> = Vec::with_capacity(4);
    for key in [k0, k1] {
        if !keys.contains(&key) {
            keys.push(key);
        }
    }

    keys
}

/// Walks the spatial index of a tile and appends the polygon indices
/// whose bounds overlap the world query box. The v3 tiles walk the
/// bucket grid, the v1/v2 tiles walk the bounding volume tree. An
/// empty index degrades to every polygon.
pub fn tile_query_polys(
    tile: &Tile,
    min_x: f64,
    min_y: f64,
    max_x: f64,
    max_y: f64,
    min_z: f64,
    max_z: f64,
    out: &mut Vec<i32>,
) {
    if tile.grid.is_some() {
        tile_query_polys_grid(tile, min_x, min_y, max_x, max_y, min_z, max_z, out);

        return;
    }
    match &tile.bv_tree {
        None => {
            for i in 0..tile.polys.len() {
                out.push(i as i32);
            }
        }
        Some(tree) if tree.is_empty() => {
            for i in 0..tile.polys.len() {
                out.push(i as i32);
            }
        }
        Some(tree) => {
            let mut qmin = [0u16; 3];
            let mut qmax = [0u16; 3];
            let lo = [
                min_x - tile.world_min_x,
                min_z - HEIGHT_FLOOR,
                min_y - tile.world_min_y,
            ];
            let hi = [
                max_x - tile.world_min_x,
                max_z - HEIGHT_FLOOR,
                max_y - tile.world_min_y,
            ];
            for a in 0..3 {
                let l = quantize_bv(lo[a]);
                let h = quantize_bv(hi[a]);
                // The even/odd lowest bit keeps overlapping quantized
                // bounds strictly separated (the Detour trick).
                qmin[a] = l & 0xFFFE;
                qmax[a] = h | 1;
            }

            let mut i = 0usize;
            while i < tree.len() {
                let node = &tree[i];
                let overlap = quant_overlap(&qmin, &qmax, &node.b_min, &node.b_max);
                let leaf = node.i >= 0;
                if leaf && overlap {
                    out.push(node.i);
                }
                if overlap || leaf {
                    i += 1;
                } else {
                    i += (-node.i) as usize;
                }
            }
        }
    }
}

/// Converts a world offset into the quantized BVTree units.
fn quantize_bv(world: f64) -> u16 {
    let cells = world / CELL_SIZE_WORLD;
    if cells < 0.0 {
        return 0;
    }
    if cells > 65535.0 {
        return 65535;
    }

    cells as u16
}

/// Walks the bucket grid of a v3 tile: the buckets the query
/// footprint touches list their polygons, the height window prunes
/// the stacked surfaces the 2D grid cannot separate.
fn tile_query_polys_grid(
    tile: &Tile,
    min_x: f64,
    min_y: f64,
    max_x: f64,
    max_y: f64,
    min_z: f64,
    max_z: f64,
    out: &mut Vec<i32>,
) {
    let grid = tile.grid.as_ref().unwrap();
    let bucket = |world: f64, anchor: f64| -> usize {
        let cells = ((world - anchor) / CELL_SIZE_WORLD).floor() as i32;
        let b = cells / GRID_BUCKET_CELLS;
        if b < 0 {
            return 0;
        }
        if b as usize >= GRID_SIDE {
            return GRID_SIDE - 1;
        }

        b as usize
    };
    let bx0 = bucket(min_x, tile.world_min_x);
    let bx1 = bucket(max_x, tile.world_min_x);
    let by0 = bucket(min_y, tile.world_min_y);
    let by1 = bucket(max_y, tile.world_min_y);
    for by in by0..=by1 {
        let base = by * GRID_SIDE;
        for bx in bx0..=bx1 {
            let b = base + bx;
            for e in grid.offsets[b]..grid.offsets[b + 1] {
                let pi = grid.entries[e as usize] as i32;
                let poly = &tile.polys[pi as usize];
                let (h_min, h_max) = poly_height_range(poly);
                if (h_max as f64) < min_z || (h_min as f64) > max_z {
                    continue;
                }
                out.push(pi);
            }
        }
    }
}

/// Reports whether two quantized bounds overlap.
fn quant_overlap(a_min: &[u16; 3], a_max: &[u16; 3], b_min: &[u16; 3], b_max: &[u16; 3]) -> bool {
    for a in 0..3 {
        if a_min[a] > b_max[a] || a_max[a] < b_min[a] {
            return false;
        }
    }

    true
}

impl Route {
    /// Sums the waypoint walk length of the route.
    pub fn length(&self) -> f64 {
        let mut sum = 0.0;
        for i in 1..self.waypoints.len() {
            sum += dist3(self.waypoints[i - 1], self.waypoints[i]);
        }

        sum
    }
}

/// The public route stats for the harnesses: the walk length of a
/// waypoint slice.
pub fn route_length_of(waypoints: &[Pos]) -> f64 {
    let mut sum = 0.0;
    for i in 1..waypoints.len() {
        sum += dist3(waypoints[i - 1], waypoints[i]);
    }

    sum
}
