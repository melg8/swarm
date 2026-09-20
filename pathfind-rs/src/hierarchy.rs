// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The hierarchical route query (the HNA* model over the cluster
//! graph): the coarse search runs the A* over the abstract nodes and
//! answers with the chain of portal edges it crossed; the refinement
//! then searches the real mesh hop by hop through the chain - every
//! hop is one ordinary corridor search from the previous portal
//! midpoint to the next portal polygon, budgeted and cached.

use crate::abstract_graph::{AbstractEdge, AbstractEdgeRef, ClusterKey, CLUSTER_CELLS_SIDE, CLUSTERS_PER_SIDE};
use crate::astar::{poly_cost, AstarGoal, QueryState, ZoneIndex};
use crate::avoid::no_avoid;
use crate::mesh::Mesh;
use crate::query::{dist3, Filter, Pos, Route};
use crate::tile::{poly_of, tile_of, AREA_WATER, PolyRef, RegionKey};
use std::collections::HashMap;

/// Bounds the cluster level A* of one query.
pub const MAX_COARSE_NODES: usize = 65536;

/// Bounds one refinement hop.
pub const MAX_HOPPER_NODES: usize = 32768;

/// Bounds the ban and replan cycles of one query.
pub const MAX_COARSE_ATTEMPTS: usize = 3;

/// Bounds the component gated guide attempt.
pub const GATED_COARSE_POPS: usize = 4096;

/// Bounds the corridor confined fallback search.
pub const MAX_CONFINED_NODES: usize = 1 << 20;

/// Caps the line corridor abstract prefetch.
pub const MAX_LINE_PREFETCH_REGIONS: i32 = 24;

/// The outcome of the cluster level search.
#[derive(Debug, Clone, Default)]
pub struct CoarseResult {
    /// The crossed portal chain, start to goal.
    pub edges: Vec<AbstractEdgeRef>,
    /// The cluster key walk of the same chain (one more entry than
    /// the edges: the start cluster first).
    pub clusters: Vec<ClusterKey>,
    pub explored: usize,
    pub reached: bool,
    /// The gated guide budget stop.
    pub gave_up: bool,
    /// The geometric length of the chain walk.
    pub estimate: f64,
}

/// Carries one hierarchical route query.
pub struct HierQuery<'a> {
    pub start_ref: PolyRef,
    pub start_pos: Pos,
    pub end_ref: PolyRef,
    pub end_pos: Pos,
    pub approach: f64,
    pub filter: &'a Filter,
    pub state: &'a mut QueryState,
    pub coarse: &'a mut CoarseState,
    pub bans: HashMap<AbstractEdgeRef, bool>,
    /// The bucket index of the filter water zone cuboids, built once
    /// per hierarchical query for the priced guide.
    pub zones: Option<ZoneIndex>,
}

impl<'a> HierQuery<'a> {
    /// Prices one coarse guide edge landing at pos: the zone aware
    /// swim rate the filter arms.
    fn water_multiplier(&self, at: Pos) -> f64 {
        match &self.zones {
            None => self.filter.water_cost,
            Some(zones) => {
                if zones.covered(at.x, at.y, at.z) {
                    self.filter.water_cost
                } else {
                    1.0
                }
            }
        }
    }
}

/// Reports whether the route goes through the hierarchy: the avoid
/// bans wall polygons at the mesh granularity the coarse graph cannot
/// see, so those queries stay flat; the cross tile queries go
/// hierarchical first, the same tile queries stay flat.
pub fn hier_worthy(start_ref: PolyRef, end_ref: PolyRef, filter: &Filter) -> bool {
    if !filter.avoid.is_empty() {
        return false;
    }
    let (sc, sr) = tile_of(start_ref);
    let (ec, er) = tile_of(end_ref);

    sc != ec || sr != er
}

impl Mesh {
    /// Runs the hierarchical query: the coarse chain and the hop
    /// refinement, stitched into the ordinary route answer. The
    /// answer is None when the hierarchy declines.
    pub fn route_hierarchical_inner(
        &self,
        q: &mut HierQuery,
    ) -> Option<Route> {
        let (col, row) = tile_of(q.start_ref);
        if self.abstract_of(&RegionKey { col, row }).is_none() {
            return None;
        }

        // The line corridor prefetch: the regions the straight
        // segment crosses are the common frontier.
        let line_keys = line_region_keys(q.start_pos, q.end_pos);
        self.prefetch_abstracts(&line_keys);

        let mut route = Route {
            found: false,
            partial: false,
            waypoints: Vec::new(),
            raw_waypoints: None,
            corridor: Vec::new(),
            explored: 0,
            hierarchical: true,
            pocket_escape: false,
        };
        // The gated attempts first: the component verified chains
        // with the ban and replan cycles of a failed hop.
        let mut chain = CoarseResult::default();
        let mut gave_up = false;
        for _ in 0..MAX_COARSE_ATTEMPTS {
            chain = self.coarse_chain(q, true);
            route.explored += chain.explored;
            if chain.gave_up {
                gave_up = true;

                break;
            }
            if chain.edges.is_empty() {
                // The cluster level found nothing at all: the flat
                // search holds the honest answer for this start.
                return None;
            }
            if self.refine_chain(q, &chain, &mut route) {
                return Some(self.contest_flat(q, &chain, route));
            }
            // A hop failed: its exit edge is banned, the replan walks
            // a different portal chain.
        }
        if gave_up {
            // The gated guide stranded: the gate free chain samples
            // the crossings plain and the refinement verifies them on
            // the real mesh.
            q.bans.clear();
            for _ in 0..MAX_COARSE_ATTEMPTS {
                chain = self.coarse_chain(q, false);
                route.explored += chain.explored;
                if chain.edges.is_empty() {
                    return None;
                }
                if self.refine_chain(q, &chain, &mut route) {
                    return Some(self.contest_flat(q, &chain, route));
                }
            }
        }

        // The attempts are exhausted: the confined search threads the
        // real mesh through the chain clusters instead.
        if route.corridor.is_empty() && self.confined_route(q, &chain.clusters, &mut route) {
            return Some(route);
        }
        if route.corridor.is_empty() {
            return None;
        }
        route.partial = true;
        let corridor = route.corridor.clone();
        self.answer_waypoints(&mut route, &corridor, q.start_pos, q.end_pos, q.filter);

        Some(route)
    }

    /// Runs the cluster level A* and returns the portal edge chain
    /// from the start cluster toward the goal cluster.
    fn coarse_chain(&self, q: &mut HierQuery, gated: bool) -> CoarseResult {
        let mut result = CoarseResult::default();
        let straight = dist3(q.start_pos, q.end_pos);
        let (col, row) = tile_of(q.start_ref);
        let start_key = cluster_key_of_pos_q(col, row, q.start_pos);
        let (goal_col, goal_row) = tile_of(q.end_ref);
        let goal_key = cluster_key_of_pos_q(goal_col, goal_row, q.end_pos);

        q.coarse.reset();
        let start_abstract = match self.abstract_of(&RegionKey { col, row }) {
            Some(a) => a,
            None => return result,
        };
        if !start_abstract.index.contains_key(&start_key.id) {
            return result;
        }
        let goal_abstract = match self.abstract_of(&RegionKey {
            col: goal_col,
            row: goal_row,
        }) {
            Some(a) => a,
            None => return result,
        };
        // The search states carry the component the crossing stood
        // on.
        let start_idx = poly_of(q.start_ref);
        let start_comp = if start_idx >= 0 && (start_idx as usize) < start_abstract.comps.len() {
            start_abstract.comps[start_idx as usize]
        } else {
            0
        };
        let end_idx = poly_of(q.end_ref);
        let goal_comp = if end_idx >= 0 && (end_idx as usize) < goal_abstract.comps.len() {
            goal_abstract.comps[end_idx as usize]
        } else {
            0
        };
        q.coarse.nodes.push(CoarseNode {
            key: start_key,
            entry_poly: start_idx.max(0) as u32,
            entry_comp: start_comp,
            parent: -1,
            edge_ref: AbstractEdgeRef::default(),
            pos: q.start_pos,
            g: 0.0,
            f: dist3(q.start_pos, q.end_pos),
            closed: false,
            heap_idx: -1,
        });
        q.coarse.index.insert(
            CoarseNodeKey {
                key: start_key,
                edge: AbstractEdgeRef::default(),
            },
            0,
        );
        q.coarse.push(0);

        loop {
            let idx = match q.coarse.pop() {
                None => break,
                Some(idx) => idx,
            };
            if q.coarse.nodes[idx as usize].closed {
                continue;
            }
            q.coarse.nodes[idx as usize].closed = true;
            result.explored += 1;

            if gated {
                if q.coarse.nodes[idx as usize].entry_comp == 0 {
                    // The lazy entry resolution: the crossing target
                    // poly's link component lives in the node tile
                    // abstract.
                    let node_key = q.coarse.nodes[idx as usize].key;
                    let node_poly = q.coarse.nodes[idx as usize].entry_poly;
                    if let Some(abstract_) = self.abstract_of(&RegionKey {
                        col: node_key.col,
                        row: node_key.row,
                    }) {
                        if (node_poly as usize) < abstract_.comps.len() {
                            q.coarse.nodes[idx as usize].entry_comp =
                                abstract_.comps[node_poly as usize];
                        }
                    }
                }
                // The settled component merge: another crossing into
                // this cluster already expanded the same component.
                let comp_key = CoarseCompKey {
                    key: q.coarse.nodes[idx as usize].key,
                    comp: q.coarse.nodes[idx as usize].entry_comp,
                };
                if let Some(&best) = q.coarse.settled.get(&comp_key) {
                    if q.coarse.nodes[idx as usize].g >= best {
                        continue;
                    }
                }
                let node_g = q.coarse.nodes[idx as usize].g;
                q.coarse.settled.insert(comp_key, node_g);
                if q.coarse.nodes[idx as usize].key == goal_key
                    && q.coarse.nodes[idx as usize].entry_comp == goal_comp
                {
                    result.reached = true;
                    let (edges, clusters) = coarse_chain_walk(q.coarse, idx);
                    result.estimate = self.chain_estimate(q, &edges);
                    result.edges = edges;
                    result.clusters = clusters;

                    return result;
                }
            } else if q.coarse.nodes[idx as usize].key == goal_key {
                // The gate free pass settles clusters plain (the
                // classic HPA* chain).
                result.reached = true;
                let (edges, clusters) = coarse_chain_walk(q.coarse, idx);
                result.estimate = self.chain_estimate(q, &edges);
                result.edges = edges;
                result.clusters = clusters;

                return result;
            }
            if q.coarse.nodes.len() >= MAX_COARSE_NODES {
                break;
            }
            if gated
                && (result.explored >= GATED_COARSE_POPS
                    || q.coarse.nodes[idx as usize].f > 1.5 * straight + 8192.0)
            {
                // The gated guide gave up: the gate free retry takes
                // over.
                result.gave_up = true;

                return result;
            }
            self.expand_coarse(q, idx);
        }
        if q.coarse.best >= 0 {
            let (edges, clusters) = coarse_chain_walk(q.coarse, q.coarse.best);
            result.estimate = self.chain_estimate(q, &edges);
            result.edges = edges;
            result.clusters = clusters;
        }

        result
    }

    /// Relaxes the coarse edges of one settled cluster node.
    fn expand_coarse(&self, q: &mut HierQuery, idx: i32) {
        let node_key = q.coarse.nodes[idx as usize].key;
        let abstract_ = match self.abstract_of(&RegionKey {
            col: node_key.col,
            row: node_key.row,
        }) {
            Some(a) => a,
            None => return,
        };
        let local = match abstract_.index.get(&node_key.id) {
            Some(&local) => local,
            None => return,
        };
        let node_entry_comp = q.coarse.nodes[idx as usize].entry_comp;
        let node_g = q.coarse.nodes[idx as usize].g;
        let node_pos = q.coarse.nodes[idx as usize].pos;
        let edge_indices = abstract_.nodes[local as usize].edges.clone();
        for ei in edge_indices {
            let edge = &abstract_.edges[ei as usize];
            let ref_ = AbstractEdgeRef {
                region: abstract_.key,
                index: ei,
            };
            if q.bans.get(&ref_).copied().unwrap_or(false) {
                continue;
            }
            if edge.src_comp != node_entry_comp {
                // The component gate: no polygon path connects the
                // crossing the search stands on with this crossing.
                continue;
            }
            let node_key = CoarseNodeKey {
                key: edge.to,
                edge: ref_,
            };
            let mut cost = dist3(node_pos, edge.mid);
            if edge.dst_area == AREA_WATER {
                // The priced guide: the water crossing pays the swim
                // rate the zone data arms.
                cost *= q.water_multiplier(edge.mid);
            }
            let g = node_g + cost;
            let h = dist3(edge.mid, q.end_pos);
            let seen = q.coarse.index.get(&node_key).copied();
            if let Some(existing) = seen {
                let known = &q.coarse.nodes[existing as usize];
                if known.closed {
                    // The re-armed component expansion: the closed
                    // state re-opens when the new g is strictly
                    // cheaper.
                    if known.g <= g {
                        continue;
                    }
                    let known = &mut q.coarse.nodes[existing as usize];
                    known.closed = false;
                    known.g = g;
                    known.f = g + h;
                    known.parent = idx;
                    known.edge_ref = ref_;
                    known.pos = edge.mid;
                    q.coarse.push(existing);

                    continue;
                }
            }
            match seen {
                None => {
                    let created = q.coarse.nodes.len() as i32;
                    q.coarse.nodes.push(CoarseNode {
                        key: edge.to,
                        entry_poly: edge_entry_poly(edge),
                        entry_comp: 0,
                        parent: idx,
                        edge_ref: ref_,
                        pos: edge.mid,
                        g,
                        f: g + h,
                        closed: false,
                        heap_idx: -1,
                    });
                    q.coarse.index.insert(node_key, created);
                    q.coarse.push(created);
                    if h < q.coarse.best_h {
                        q.coarse.best_h = h;
                        q.coarse.best = created;
                    }
                }
                Some(existing) => {
                    let known = &mut q.coarse.nodes[existing as usize];
                    if g < known.g {
                        known.g = g;
                        known.f = g + h;
                        known.parent = idx;
                        known.edge_ref = ref_;
                        known.pos = edge.mid;
                        q.coarse.fix(existing);
                    }
                }
            }
        }
    }

    /// Loads one coarse edge by reference.
    pub fn hier_edge(&self, ref_: AbstractEdgeRef) -> Option<AbstractEdge> {
        let abstract_ = self.abstract_of(&ref_.region)?;

        abstract_.edges.get(ref_.index as usize).copied()
    }

    /// Resolves the crossing target polygon of an external edge.
    pub fn resolve_edge_target(&self, ref_: AbstractEdgeRef, edge: &AbstractEdge) -> PolyRef {
        let tile = match self.tile(&ref_.region) {
            Ok(tile) => tile,
            Err(_) => return 0,
        };
        if edge.link as usize >= tile.links.len() {
            return 0;
        }
        let link = tile.links[edge.link as usize];
        match self.link_target(&tile, &link) {
            Some((target, _, _)) => target,
            None => 0,
        }
    }

    /// Walks the coarse edge chain and fills the route with the
    /// stitched refinement corridors. The answer reports whether the
    /// route is finished.
    fn refine_chain(&self, q: &mut HierQuery, chain: &CoarseResult, route: &mut Route) -> bool {
        // The chain region prefetch: the first regions of the walk
        // decode in parallel before the hops start.
        let first_keys = chain_region_keys(chain, crate::mesh::MAX_PREFETCH_WORKERS);
        self.prefetch_tiles(&first_keys);
        let mut corridor: Vec<PolyRef> = Vec::with_capacity(64 + chain.edges.len() * 8);
        let mut current_ref = q.start_ref;
        let mut current_pos = q.start_pos;
        let mut prev_edge = AbstractEdgeRef::default();
        for i in 0..chain.edges.len() {
            // The rolling look ahead: the tile two clusters ahead
            // decodes while this hop searches.
            if i + 2 < chain.clusters.len() {
                self.prefetch_ahead(RegionKey {
                    col: chain.clusters[i + 2].col,
                    row: chain.clusters[i + 2].row,
                });
            }
            let exit_ref = chain.edges[i];
            let exit_edge = match self.hier_edge(exit_ref) {
                Some(edge) => edge,
                None => {
                    q.bans.insert(exit_ref, true);

                    return false;
                }
            };
            let last = i == chain.edges.len() - 1;
            let (goal_ref, goal_pos) = if !last {
                let mut goal_ref = exit_edge.to_ref;
                if goal_ref == 0 {
                    goal_ref = self.resolve_edge_target(exit_ref, &exit_edge);
                    if goal_ref == 0 {
                        q.bans.insert(exit_ref, true);

                        return false;
                    }
                }
                (goal_ref, exit_edge.mid)
            } else {
                (q.end_ref, q.end_pos)
            };
            let segment = self.hop_corridor(
                q,
                current_ref,
                current_pos,
                prev_edge,
                exit_ref,
                goal_ref,
                goal_pos,
                i > 0,
                last,
            );
            let segment = match segment {
                Some(segment)
                    if segment.reached || (last && segment.partial) =>
                {
                    segment
                }
                _ => {
                    // The hop into the next portal failed: ban the
                    // exit edge and replan (the last hop keeps its
                    // partial answer).
                    q.bans.insert(exit_ref, true);

                    return false;
                }
            };
            route.explored += segment.explored;
            if last && segment.reached {
                route.found = true;
            }
            let mut start = 0usize;
            if !corridor.is_empty()
                && !segment.corridor.is_empty()
                && corridor[corridor.len() - 1] == segment.corridor[0]
            {
                start = 1;
            }
            corridor.extend_from_slice(&segment.corridor[start..]);
            current_ref = goal_ref;
            current_pos = goal_pos;
            prev_edge = exit_ref;
        }

        route.corridor = corridor;
        route.partial = !route.found;
        let corridor_clone = route.corridor.clone();
        self.answer_waypoints(route, &corridor_clone, q.start_pos, q.end_pos, q.filter);

        true
    }

    /// Answers one portal to portal corridor: the cached intra-edge
    /// answer when the pair ran before, a fresh budgeted search
    /// otherwise.
    fn hop_corridor(
        &self,
        q: &mut HierQuery,
        from_ref: PolyRef,
        from_pos: Pos,
        entry: AbstractEdgeRef,
        exit: AbstractEdgeRef,
        to_ref: PolyRef,
        to_pos: Pos,
        cacheable: bool,
        last: bool,
    ) -> Option<HopSegment> {
        let key = HopKey { from: entry, to: exit };
        if cacheable {
            if let Some(cached) = self.cached_hop(key) {
                return Some(HopSegment {
                    corridor: cached,
                    explored: 0,
                    reached: true,
                    partial: false,
                });
            }
        }

        let segment = self.run_hop(q, from_ref, from_pos, to_ref, to_pos, last)?;
        if cacheable && segment.reached && !segment.corridor.is_empty() {
            self.store_hop(key, &segment.corridor);
        }

        Some(segment)
    }

    /// Searches one hop on the mesh: the ordinary corridor search
    /// with the hop budget. A search that hit the node budget retries
    /// with the quadrupled budget before the caller bans the edge.
    fn run_hop(
        &self,
        q: &mut HierQuery,
        from_ref: PolyRef,
        from_pos: Pos,
        to_ref: PolyRef,
        to_pos: Pos,
        last: bool,
    ) -> Option<HopSegment> {
        let mut goal = AstarGoal {
            target: to_ref,
            approach: 0.0,
        };
        if last {
            goal.approach = q.approach;
        }
        let mut budget = MAX_HOPPER_NODES;
        let mut explored = 0usize;
        let mut retries = 0usize;
        loop {
            let result = self.astar(
                q.state,
                goal,
                from_ref,
                from_pos,
                to_pos,
                q.filter,
                &no_avoid(),
                budget,
                None,
            );
            if result.corridor.is_empty() {
                return None;
            }
            explored += result.explored;
            if result.reached || !result.capped || retries >= 2 || budget >= MAX_CONFINED_NODES {
                return Some(HopSegment {
                    corridor: result.corridor,
                    explored,
                    reached: result.reached,
                    partial: result.partial,
                });
            }
            // The progress guard: a capped search that never left the
            // start side names a crossing the mesh does not serve.
            if dist3(from_pos, result.end) < 0.25 * dist3(from_pos, to_pos) {
                return Some(HopSegment {
                    corridor: result.corridor,
                    explored,
                    reached: result.reached,
                    partial: result.partial,
                });
            }
            budget *= 4;
            retries += 1;
        }
    }

    /// Runs the fallback search: one fine A* over the mesh with the
    /// expansion confined to the coarse chain clusters.
    fn confined_route(&self, q: &mut HierQuery, clusters: &[ClusterKey], route: &mut Route) -> bool {
        if clusters.is_empty() {
            return false;
        }
        let allow = new_confined_set(clusters);
        let result = self.astar(
            q.state,
            AstarGoal {
                target: q.end_ref,
                approach: q.approach,
            },
            q.start_ref,
            q.start_pos,
            q.end_pos,
            q.filter,
            &no_avoid(),
            MAX_CONFINED_NODES,
            Some(&allow),
        );
        if result.corridor.is_empty() {
            return false;
        }
        if !result.reached && !result.partial {
            return false;
        }
        route.corridor = result.corridor;
        route.explored += result.explored;
        route.found = result.reached;
        route.partial = !result.reached;
        let corridor = route.corridor.clone();
        self.answer_waypoints(route, &corridor, q.start_pos, q.end_pos, q.filter);

        true
    }

    /// Hands the refined answer to the exact competitors when the
    /// refinement outran its own chain estimate.
    fn contest_flat(&self, q: &mut HierQuery, chain: &CoarseResult, mut route: Route) -> Route {
        if !route.found || route.corridor.is_empty() {
            return route;
        }
        let length = route.length();
        let result = self.astar(
            q.state,
            AstarGoal {
                target: q.end_ref,
                approach: q.approach,
            },
            q.start_ref,
            q.start_pos,
            q.end_pos,
            q.filter,
            &no_avoid(),
            crate::astar::MAX_QUERY_NODES,
            None,
        );
        if result.reached && !result.corridor.is_empty() {
            let mut flat = Route {
                found: true,
                partial: false,
                waypoints: Vec::new(),
                raw_waypoints: None,
                corridor: result.corridor,
                explored: result.explored,
                hierarchical: false,
                pocket_escape: false,
            };
            let corridor = flat.corridor.clone();
            self.answer_waypoints(&mut flat, &corridor, q.start_pos, q.end_pos, q.filter);
            if flat.length() < length {
                return flat;
            }

            return route;
        }
        // The exact budget cannot finish the corridor: the confined
        // search threads the chain clusters with the full polygon
        // freedom.
        let mut confined = Route::default();
        if !self.confined_route(q, &chain.clusters, &mut confined) || !confined.found {
            return route;
        }
        if corridor_cost_of(self, &confined.corridor, q)
            < corridor_cost_of(self, &route.corridor, q)
        {
            confined.hierarchical = true;
            route.explored += confined.explored;

            return confined;
        }

        route
    }

    /// Sums the geometric walk of the chain: the start, the portal
    /// midpoints and the end in sequence.
    fn chain_estimate(&self, q: &HierQuery, edges: &[AbstractEdgeRef]) -> f64 {
        let mut prev = q.start_pos;
        let mut sum = 0.0;
        for ref_ in edges {
            let edge = match self.hier_edge(*ref_) {
                Some(edge) => edge,
                None => return 0.0,
            };
            sum += dist3(prev, edge.mid);
            prev = edge.mid;
        }
        sum += dist3(prev, q.end_pos);

        sum
    }

    /// Fires one background tile load (the rolling look ahead of the
    /// hop refinement).
    fn prefetch_ahead(&self, key: RegionKey) {
        self.prefetch_tiles(&[key]);
    }
}

/// One refinement answer: the polygon corridor plus the search
/// bookkeeping.
struct HopSegment {
    corridor: Vec<PolyRef>,
    explored: usize,
    reached: bool,
    partial: bool,
}

/// Names one cached refinement corridor: the entry edge and the exit
/// edge.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct HopKey {
    pub from: AbstractEdgeRef,
    pub to: AbstractEdgeRef,
}

/// The hop corridor cache (the HNA* intra-edge cache).
pub mod hop_cache {
    use super::HopKey;
    use crate::tile::PolyRef;
    use std::collections::HashMap;
    use std::collections::VecDeque;

    /// Bounds the cached refinement corridors.
    pub const HOP_CACHE_CAPACITY: usize = 4096;

    /// The LRU bounded corridor cache of the portal pair answers.
    #[derive(Default)]
    pub struct HopCache {
        hops: HashMap<HopKey, Vec<PolyRef>>,
        order: VecDeque<HopKey>,
    }

    impl HopCache {
        pub fn new() -> HopCache {
            HopCache {
                hops: HashMap::new(),
                order: VecDeque::new(),
            }
        }

        /// Answers the cached corridor of a hop key.
        pub fn cached(&self, key: HopKey) -> Option<Vec<PolyRef>> {
            self.hops.get(&key).cloned()
        }

        /// Caches the corridor of a portal pair.
        pub fn store(&mut self, key: HopKey, corridor: &[PolyRef]) {
            if self.hops.contains_key(&key) {
                return;
            }
            if self.order.len() >= HOP_CACHE_CAPACITY {
                let oldest = self.order.pop_front().unwrap();
                self.hops.remove(&oldest);
            }
            self.order.push_back(key);
            self.hops.insert(key, corridor.to_vec());
        }
    }
}

/// Names the polygon index whose link component the crossing enters
/// through.
fn edge_entry_poly(edge: &AbstractEdge) -> u32 {
    if edge.ext_col != 0 || edge.ext_row != 0 {
        return edge.ext_poly;
    }
    if edge.to_ref != 0 {
        let poly = poly_of(edge.to_ref);
        if poly >= 0 {
            return poly as u32;
        }
    }

    0
}

/// Walks the parent chain and returns the crossed edge refs and the
/// cluster keys from the start cluster to the node.
fn coarse_chain_walk(
    state: &CoarseState,
    idx: i32,
) -> (Vec<AbstractEdgeRef>, Vec<ClusterKey>) {
    let mut edges = Vec::new();
    let mut clusters = Vec::new();
    let mut n = idx;
    while n >= 0 {
        let node = &state.nodes[n as usize];
        clusters.push(node.key);
        if node.parent >= 0 {
            edges.push(node.edge_ref);
        }
        n = node.parent;
    }
    // The chains walked goal first; reverse into the start order.
    edges.reverse();
    clusters.reverse();

    (edges, clusters)
}

/// The allowed cluster set of the fallback search: one bitmap per
/// region, the coarse chain clusters marked.
pub struct ConfinedSet {
    regions: HashMap<RegionKey, Box<ClusterBitmap>>,
}

/// Marks the allowed clusters of one region.
pub type ClusterBitmap = [[bool; CLUSTERS_PER_SIDE as usize]; CLUSTERS_PER_SIDE as usize];

/// Marks the chain clusters with the one cluster dilation.
pub fn new_confined_set(clusters: &[ClusterKey]) -> ConfinedSet {
    let mut set = ConfinedSet {
        regions: HashMap::new(),
    };
    let add = |set: &mut ConfinedSet, col: i16, row: i16, cx: i32, cy: i32| {
        if cx < 0 || cx >= CLUSTERS_PER_SIDE || cy < 0 || cy >= CLUSTERS_PER_SIDE {
            return;
        }
        let region_key = RegionKey { col, row };
        let bm = set.regions.entry(region_key).or_insert_with(|| {
            Box::new([[false; CLUSTERS_PER_SIDE as usize]; CLUSTERS_PER_SIDE as usize])
        });
        bm[cx as usize][cy as usize] = true;
    };
    for key in clusters {
        let cx = (key.id >> 4) as i32;
        let cy = (key.id & 0x0F) as i32;
        for dx in -1i32..=1 {
            for dy in -1i32..=1 {
                let mut nx = cx + dx;
                let mut ny = cy + dy;
                let mut col = key.col;
                let mut row = key.row;
                if nx < 0 {
                    col -= 1;
                    nx += CLUSTERS_PER_SIDE;
                }
                if nx >= CLUSTERS_PER_SIDE {
                    col += 1;
                    nx -= CLUSTERS_PER_SIDE;
                }
                if ny < 0 {
                    row -= 1;
                    ny += CLUSTERS_PER_SIDE;
                }
                if ny >= CLUSTERS_PER_SIDE {
                    row += 1;
                    ny -= CLUSTERS_PER_SIDE;
                }
                add(&mut set, col, row, nx, ny);
            }
        }
    }

    set
}

impl ConfinedSet {
    /// Reports whether the polygon footprint touches any allowed
    /// cluster of its region.
    pub fn allows(&self, key: RegionKey, p: &crate::tile::Poly) -> bool {
        let bm = match self.regions.get(&key) {
            Some(bm) => bm,
            None => return false,
        };
        let x0 = p.x0 / CLUSTER_CELLS_SIDE;
        let x1 = (p.x1 - 1) / CLUSTER_CELLS_SIDE;
        let y0 = p.y0 / CLUSTER_CELLS_SIDE;
        let y1 = (p.y1 - 1) / CLUSTER_CELLS_SIDE;
        if x1 - x0 >= CLUSTERS_PER_SIDE - 1 && y1 - y0 >= CLUSTERS_PER_SIDE - 1 {
            return true;
        }
        for cx in x0..=x1 {
            for cy in y0..=y1 {
                if bm[cx as usize][cy as usize] {
                    return true;
                }
            }
        }

        false
    }
}

/// Lists the regions the straight segment crosses.
pub fn line_region_keys(start: Pos, end: Pos) -> Vec<RegionKey> {
    let sc = crate::tile::region_of_world(start.x, start.y);
    let ec = crate::tile::region_of_world(end.x, end.y);
    let mut span = ((ec.col - sc.col).abs() + (ec.row - sc.row).abs()) as i32;
    if span == 0 {
        return Vec::new();
    }
    if span > MAX_LINE_PREFETCH_REGIONS {
        span = MAX_LINE_PREFETCH_REGIONS;
    }
    let mut seen: std::collections::HashSet<RegionKey> =
        std::collections::HashSet::with_capacity(span as usize + 2);
    let mut keys: Vec<RegionKey> = Vec::with_capacity(span as usize + 2);
    for i in 0..=(span + 1) {
        let t = i as f64 / (span + 1) as f64;
        let key = crate::tile::region_of_world(
            start.x + (end.x - start.x) * t,
            start.y + (end.y - start.y) * t,
        );
        if seen.insert(key) {
            keys.push(key);
        }
    }

    keys
}

/// Names the distinct regions of the coarse chain walk.
fn chain_region_keys(chain: &CoarseResult, bound: usize) -> Vec<RegionKey> {
    let mut keys: Vec<RegionKey> = Vec::with_capacity(chain.clusters.len());
    let mut seen: std::collections::HashSet<RegionKey> =
        std::collections::HashSet::with_capacity(chain.clusters.len());
    for cluster in &chain.clusters {
        let key = RegionKey {
            col: cluster.col,
            row: cluster.row,
        };
        if !seen.insert(key) {
            continue;
        }
        keys.push(key);
        if keys.len() >= bound {
            break;
        }
    }

    keys
}

/// Prices the corridor under the search filter: every polygon pays
/// its polyCost weighted by the share of the center walk.
fn corridor_cost_of(mesh: &Mesh, corridor: &[PolyRef], q: &HierQuery) -> f64 {
    if corridor.is_empty() {
        return f64::MAX;
    }
    let mut centers: Vec<Pos> = Vec::with_capacity(corridor.len());
    for ref_ in corridor {
        let (tile, poly_idx) = match mesh.poly_of_ref(*ref_) {
            Some(pair) => pair,
            None => return f64::MAX,
        };
        let poly = &tile.polys[poly_idx as usize];
        let (x0, y0, x1, y1) = tile.world_rect(poly);
        let cx = (x0 + x1) * 0.5;
        let cy = (y0 + y1) * 0.5;
        let z = tile.height_at(poly, cx, cy);
        centers.push(Pos {
            x: cx,
            y: cy,
            z,
        });
    }
    let mut sum = 0.0;
    for (i, ref_) in corridor.iter().enumerate() {
        let (tile, poly_idx) = match mesh.poly_of_ref(*ref_) {
            Some(pair) => pair,
            None => return f64::MAX,
        };
        let poly = &tile.polys[poly_idx as usize];
        let back = if i > 0 {
            dist3(centers[i - 1], centers[i])
        } else {
            0.0
        };
        let ahead = if i + 1 < corridor.len() {
            dist3(centers[i], centers[i + 1])
        } else {
            0.0
        };
        let mut weight = back + ahead;
        if i == 0 {
            weight = ahead;
        }
        if i + 1 == corridor.len() {
            weight = back;
        }
        sum += poly_cost(&tile, poly, q.zones.as_ref(), q.filter) * weight * 0.5;
    }

    sum
}

/// The cluster key helper of the query module (the position form).
fn cluster_key_of_pos_q(col: i16, row: i16, pos: Pos) -> ClusterKey {
    crate::abstract_graph::cluster_key_of_pos(col, row, pos.x, pos.y)
}

/// The coarse node identity: the cluster and the crossing the search
/// entered it through.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct CoarseNodeKey {
    pub key: ClusterKey,
    pub edge: AbstractEdgeRef,
}

/// The settled (cluster, entry component) identity of the pop time
/// dedupe.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct CoarseCompKey {
    pub key: ClusterKey,
    pub comp: u32,
}

/// One node of the cluster level search: the (cluster, entering
/// crossing) pair.
#[derive(Debug, Clone, Copy)]
pub struct CoarseNode {
    pub key: ClusterKey,
    pub entry_poly: u32,
    pub entry_comp: u32,
    pub parent: i32,
    pub edge_ref: AbstractEdgeRef,
    pub pos: Pos,
    pub g: f64,
    pub f: f64,
    pub closed: bool,
    pub heap_idx: i32,
}

/// The pooled search state of the cluster level.
#[derive(Default)]
pub struct CoarseState {
    pub nodes: Vec<CoarseNode>,
    pub index: HashMap<CoarseNodeKey, i32>,
    pub settled: HashMap<CoarseCompKey, f64>,
    pub open: Vec<i32>,
    pub best: i32,
    pub best_h: f64,
}

impl CoarseState {
    /// Empties the coarse state for reuse.
    pub fn reset(&mut self) {
        self.nodes.clear();
        self.index.clear();
        self.settled.clear();
        self.open.clear();
        self.best = -1;
        self.best_h = f64::MAX;
    }

    /// Inserts a node index into the open heap.
    pub fn push(&mut self, idx: i32) {
        if self.nodes[idx as usize].heap_idx >= 0 {
            return;
        }
        self.open.push(idx);
        let mut i = (self.open.len() - 1) as i32;
        self.nodes[idx as usize].heap_idx = i;
        while i > 0 {
            let parent = (i - 1) / 2;
            if self.nodes[self.open[i as usize] as usize].f
                < self.nodes[self.open[parent as usize] as usize].f
            {
                self.swap(i, parent);
                i = parent;
            } else {
                break;
            }
        }
    }

    /// Restores the heap property of a node whose score improved.
    pub fn fix(&mut self, idx: i32) {
        let i = self.nodes[idx as usize].heap_idx;
        if i < 0 {
            self.push(idx);

            return;
        }
        let mut i = i;
        while i > 0 {
            let parent = (i - 1) / 2;
            if self.nodes[self.open[i as usize] as usize].f
                < self.nodes[self.open[parent as usize] as usize].f
            {
                self.swap(i, parent);
                i = parent;
            } else {
                break;
            }
        }
    }

    /// Removes and returns the best open node index.
    pub fn pop(&mut self) -> Option<i32> {
        if self.open.is_empty() {
            return None;
        }
        let top = self.open[0];
        self.nodes[top as usize].heap_idx = -1;
        let last = self.open[self.open.len() - 1];
        self.open.pop();
        if !self.open.is_empty() {
            self.open[0] = last;
            self.nodes[last as usize].heap_idx = 0;
            let mut i: i32 = 0;
            loop {
                let left = 2 * i + 1;
                let right = 2 * i + 2;
                let mut best = i;
                if (left as usize) < self.open.len()
                    && self.nodes[self.open[left as usize] as usize].f
                        < self.nodes[self.open[best as usize] as usize].f
                {
                    best = left;
                }
                if (right as usize) < self.open.len()
                    && self.nodes[self.open[right as usize] as usize].f
                        < self.nodes[self.open[best as usize] as usize].f
                {
                    best = right;
                }
                if best == i {
                    break;
                }
                self.swap(i, best);
                i = best;
            }
        }

        Some(top)
    }

    /// Exchanges two heap entries and fixes their indices.
    fn swap(&mut self, i: i32, j: i32) {
        self.open.swap(i as usize, j as usize);
        let a = self.open[i as usize];
        let b = self.open[j as usize];
        self.nodes[a as usize].heap_idx = i;
        self.nodes[b as usize].heap_idx = j;
    }
}

/// The public hierarchical route entry: builds the query over the
/// pooled coarse state and runs the phases in order.
pub fn route_hierarchical(
    mesh: &Mesh,
    start_ref: PolyRef,
    start_pos: Pos,
    end_ref: PolyRef,
    end_pos: Pos,
    approach: f64,
    filter: &Filter,
    state: &mut QueryState,
) -> Option<Route> {
    let mut coarse = mesh.acquire_coarse();
    let mut query = HierQuery {
        start_ref,
        start_pos,
        end_ref,
        end_pos,
        approach,
        filter,
        state,
        coarse: &mut coarse,
        bans: HashMap::new(),
        zones: None,
    };
    if !filter.water_zones.is_empty() {
        query.zones = Some(ZoneIndex::new(&filter.water_zones));
    }
    let result = mesh.route_hierarchical_inner(&mut query);
    mesh.release_coarse(coarse);

    result
}

