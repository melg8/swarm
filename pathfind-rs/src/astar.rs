// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The corridor search over the mesh: the pooled per-query state,
//! the open addressing reference index, the binary heap and the
//! expansion loop (the water pricing and the avoid walls included).

use crate::avoid::AvoidCtx;
use crate::query::{dist3, Filter, Pos, WaterZone};
use crate::tile::{AREA_WATER, PolyRef, RegionKey};
use crate::{Mesh, Tile};
use std::collections::HashMap;
use std::sync::Arc;

/// Bounds the A* node count of one flat search.
pub const MAX_QUERY_NODES: usize = 65536;

/// Marks the root of the parent chain.
const NO_PARENT: u32 = u32::MAX;
const NOT_IN_HEAP: i32 = -1;

/// One node of the corridor search: the polygon with its tile
/// (resolved once at relaxation), the entry position, the parent, the
/// costs and the open heap bookkeeping.
#[derive(Clone)]
pub struct AstarNode {
    pub ref_: PolyRef,
    pub parent: u32,
    pub pos: Pos,
    pub g: f64,
    pub f: f64,
    pub closed: bool,
    pub heap_idx: i32,
    pub tile: Option<Arc<Tile>>,
    pub poly: u32,
}

/// The pooled per-query search state: the node array, the
/// reference-to-node index, the open heap and the best partial node
/// tracking.
pub struct QueryState {
    pub nodes: Vec<AstarNode>,
    pub index: RefIndex,
    pub open: Vec<u32>,
    pub best: u32,
    pub best_h: f64,
    /// The bucket index of the filter water zone cuboids, built once
    /// per search for the zone aware swim pricing (None without the
    /// zone table).
    pub zones: Option<ZoneIndex>,
}

impl Default for QueryState {
    fn default() -> Self {
        let mut state = QueryState {
            nodes: Vec::new(),
            index: RefIndex::default(),
            open: Vec::new(),
            best: 0,
            best_h: 0.0,
            zones: None,
        };
        state.index.init(0);

        state
    }
}

impl QueryState {
    /// Empties the state for reuse.
    pub fn reset(&mut self) {
        self.nodes.clear();
        self.index.reset();
        self.open.clear();
        self.best = 0;
        self.best_h = f64::MAX;
        self.zones = None;
    }

    /// Appends a fresh node for a reference (the caller guarantees
    /// the reference is new).
    fn create(
        &mut self,
        ref_: PolyRef,
        parent: u32,
        pos: Pos,
        g: f64,
        h: f64,
        tile: Option<Arc<Tile>>,
        poly: u32,
    ) -> u32 {
        self.nodes.push(AstarNode {
            ref_,
            parent,
            pos,
            g,
            f: g + h,
            closed: false,
            heap_idx: NOT_IN_HEAP,
            tile,
            poly,
        });
        let idx = (self.nodes.len() - 1) as u32;
        self.index.insert(ref_, idx);
        if h < self.best_h {
            self.best_h = h;
            self.best = idx;
        }

        idx
    }

    /// Inserts a node index into the open heap.
    fn push(&mut self, idx: u32) {
        if self.nodes[idx as usize].heap_idx >= 0 {
            return;
        }
        self.open.push(idx);
        let mut i = (self.open.len() - 1) as i32;
        self.nodes[idx as usize].heap_idx = i;
        while i > 0 {
            let parent = (i - 1) / 2;
            if self.less(self.open[i as usize], self.open[parent as usize]) {
                self.swap(i, parent);
                i = parent;
            } else {
                break;
            }
        }
    }

    /// Restores the heap property of a node whose score improved.
    fn fix(&mut self, idx: u32) {
        let i = self.nodes[idx as usize].heap_idx;
        if i < 0 {
            self.push(idx);

            return;
        }
        let mut i = i;
        while i > 0 {
            let parent = (i - 1) / 2;
            if self.less(self.open[i as usize], self.open[parent as usize]) {
                self.swap(i, parent);
                i = parent;
            } else {
                break;
            }
        }
    }

    /// Removes and returns the best open node index.
    fn pop(&mut self) -> Option<u32> {
        if self.open.is_empty() {
            return None;
        }
        let top = self.open[0];
        self.nodes[top as usize].heap_idx = NOT_IN_HEAP;
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
                    && self.less(self.open[left as usize], self.open[best as usize])
                {
                    best = left;
                }
                if (right as usize) < self.open.len()
                    && self.less(self.open[right as usize], self.open[best as usize])
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

    /// Orders two heap entries by their f score.
    fn less(&self, a: u32, b: u32) -> bool {
        self.nodes[a as usize].f < self.nodes[b as usize].f
    }

    /// Exchanges two heap entries and fixes their indices.
    fn swap(&mut self, i: i32, j: i32) {
        self.open.swap(i as usize, j as usize);
        let a = self.open[i as usize];
        let b = self.open[j as usize];
        self.nodes[a as usize].heap_idx = i;
        self.nodes[b as usize].heap_idx = j;
    }

    /// Walks the parent chain of a node back to the root and returns
    /// the start-to-node polygon corridor.
    fn corridor_of(&self, idx: u32) -> Vec<PolyRef> {
        let mut depth = 0usize;
        let mut node = idx;
        while node != NO_PARENT {
            depth += 1;
            node = self.nodes[node as usize].parent;
        }
        let mut corridor = vec![0 as PolyRef; depth];
        let mut node = idx;
        while node != NO_PARENT {
            depth -= 1;
            corridor[depth] = self.nodes[node as usize].ref_;
            node = self.nodes[node as usize].parent;
        }

        corridor
    }
}

/// The open addressing reference-to-node table the flat search
/// relaxes through. The epoch stamp makes the reset O(1).
#[derive(Default)]
pub struct RefIndex {
    slots: Vec<RefSlot>,
    mask: u64,
    count: usize,
    epoch: u32,
}

/// One open addressing cell: the reference key, the node index it
/// created and the query generation it belongs to.
#[derive(Clone, Copy, Default)]
struct RefSlot {
    ref_: PolyRef,
    node: u32,
    epoch: u32,
}

/// Bounds the smallest table.
const REF_INDEX_MIN_SLOTS: usize = 1 << 13;

impl RefIndex {
    /// Sizes the table to the power of two that keeps the default
    /// load under 0.7 at the requested node count.
    pub fn init(&mut self, nodes: usize) {
        let mut size = REF_INDEX_MIN_SLOTS;
        while size < (nodes + 1) * 2 {
            size <<= 1;
        }
        self.slots = vec![RefSlot::default(); size];
        self.mask = (size - 1) as u64;
        self.count = 0;
        self.epoch = 1;
    }

    /// Starts a new query generation: one integer bump.
    pub fn reset(&mut self) {
        self.count = 0;
        self.epoch = self.epoch.wrapping_add(1);
        // The epoch wrap recycles the table.
        if self.epoch == 0 {
            for slot in self.slots.iter_mut() {
                slot.epoch = 0;
            }
            self.epoch = 1;
        }
    }

    /// Returns the node index of a reference (false when the search
    /// has not created it in this generation).
    pub fn find(&self, ref_: PolyRef) -> Option<u32> {
        let mut h = hash_ref(ref_);
        loop {
            let slot = &self.slots[(h & self.mask) as usize];
            if slot.epoch != self.epoch {
                return None;
            }
            if slot.ref_ == ref_ {
                return Some(slot.node);
            }
            h += 1;
        }
    }

    /// Records a freshly created node. The table doubles at the 0.7
    /// load and rehashes the live generation only.
    pub fn insert(&mut self, ref_: PolyRef, node: u32) {
        if self.count * 10 >= self.slots.len() * 7 {
            self.grow();
        }
        let mut h = hash_ref(ref_);
        loop {
            let slot = &mut self.slots[(h & self.mask) as usize];
            if slot.epoch != self.epoch {
                slot.ref_ = ref_;
                slot.node = node;
                slot.epoch = self.epoch;
                self.count += 1;

                return;
            }
            h += 1;
        }
    }

    /// Doubles the table and carries the live generation's entries.
    fn grow(&mut self) {
        let old = std::mem::take(&mut self.slots);
        self.slots = vec![RefSlot::default(); old.len() * 2];
        self.mask = (self.slots.len() - 1) as u64;
        self.count = 0;
        let epoch = self.epoch;
        for slot in &old {
            if slot.epoch == epoch {
                self.insert(slot.ref_, slot.node);
            }
        }
    }
}

/// The splitmix64 finalizer: the packed references differ mostly in
/// the low bits, the avalanche spreads both into the table index.
fn hash_ref(v: u64) -> u64 {
    let mut v = v;
    v ^= v >> 33;
    v = v.wrapping_mul(0xff51_afd7_ed55_8ccd);
    v ^= v >> 33;
    v = v.wrapping_mul(0xc4ce_b9fe_1a85_ec53);
    v ^= v >> 33;

    v
}

/// The bucket grid side of the water zone index: the world splits
/// into 16384 unit cells.
const ZONE_CELL_SHIFT: i32 = 14;

/// Buckets the water zone cuboids by their x/y footprint.
pub struct ZoneIndex {
    cells: HashMap<u64, Vec<WaterZone>>,
}

impl ZoneIndex {
    /// Builds the bucket grid of the water zone cuboids.
    pub fn new(boxes: &[WaterZone]) -> ZoneIndex {
        let mut cells: HashMap<u64, Vec<WaterZone>> = HashMap::new();
        for box_ in boxes {
            let x0 = (box_.min_x as i64) >> ZONE_CELL_SHIFT;
            let x1 = (box_.max_x as i64) >> ZONE_CELL_SHIFT;
            let y0 = (box_.min_y as i64) >> ZONE_CELL_SHIFT;
            let y1 = (box_.max_y as i64) >> ZONE_CELL_SHIFT;
            for x in x0..=x1 {
                for y in y0..=y1 {
                    let key = ((x as u64) << 32) | ((y as u32) as u64);
                    cells.entry(key).or_default().push(*box_);
                }
            }
        }

        ZoneIndex { cells }
    }

    /// Reports whether the server prices the swim at a world point
    /// whose bed height is z.
    pub fn covered(&self, x: f64, y: f64, z: f64) -> bool {
        let key = (((x as i64) >> ZONE_CELL_SHIFT) as u64) << 32
            | ((((y as i64) >> ZONE_CELL_SHIFT) as u32) as u64);
        if let Some(boxes) = self.cells.get(&key) {
            for box_ in boxes {
                if x >= box_.min_x
                    && x <= box_.max_x
                    && y >= box_.min_y
                    && y <= box_.max_y
                    && z <= box_.max_z
                {
                    return true;
                }
            }
        }

        false
    }
}

/// Prices one polygon of the search: the land rate for the ground,
/// the swim price for the water polygons the armed zone data covers,
/// the plain land rate for the water polygons it omits.
pub fn poly_cost(
    tile: &Tile,
    poly: &crate::tile::Poly,
    zones: Option<&ZoneIndex>,
    filter: &Filter,
) -> f64 {
    if poly.area != AREA_WATER {
        return 1.0;
    }
    let zones = match zones {
        None => return filter.water_cost,
        Some(z) => z,
    };
    let (x0, y0, x1, y1) = tile.world_rect(poly);
    let cx = (x0 + x1) * 0.5;
    let cy = (y0 + y1) * 0.5;
    if zones.covered(cx, cy, tile.height_at(poly, cx, cy)) {
        return filter.water_cost;
    }

    1.0
}

/// The outcome of the corridor search.
#[derive(Debug, Clone, Default)]
pub struct AstarResult {
    pub corridor: Vec<PolyRef>,
    /// The effective end position of the corridor: the search end for
    /// a reached goal, the entry position of the best partial node
    /// for the closest-reachable answer.
    pub end: Pos,
    pub reached: bool,
    pub partial: bool,
    pub explored: usize,
    pub capped: bool,
}

/// Selects the stop condition of the corridor search: the search
/// stops at one target polygon; the approach radius widens the goal.
#[derive(Clone, Copy)]
pub struct AstarGoal {
    pub target: PolyRef,
    /// The 3D radius of the approach goal; zero keeps the exact
    /// target contract.
    pub approach: f64,
}

impl AstarGoal {
    /// Reports whether a settled polygon satisfies the goal.
    fn reached(&self, _poly: Option<&crate::tile::Poly>, ref_: PolyRef) -> bool {
        ref_ == self.target
    }

    /// Reports whether a settled polygon already satisfies an
    /// approach radius goal.
    fn approach_reached(&self, tile: Option<&Arc<Tile>>, poly: u32, end: Pos) -> bool {
        if self.approach <= 0.0 {
            return false;
        }
        let tile = match tile {
            None => return false,
            Some(t) => t,
        };
        let poly = &tile.polys[poly as usize];
        let (cx, cy, cz) = tile.closest_point(poly, end.x, end.y, end.z);
        let dx = end.x - cx;
        let dy = end.y - cy;
        let dz = end.z - cz;

        (dx * dx + dy * dy + dz * dz).sqrt() <= self.approach
    }
}

impl Mesh {
    /// Runs the corridor search over the mesh. The step cost is the
    /// 3D distance between entry positions times the averaged area
    /// costs of the two polygons; the heuristic of the ordinary
    /// search is the 3D distance to the end position. When the goal
    /// is not reached the answer carries the corridor to the node
    /// closest to the end position.
    pub fn astar(
        &self,
        state: &mut QueryState,
        goal: AstarGoal,
        start_ref: PolyRef,
        start_pos: Pos,
        end_pos: Pos,
        filter: &Filter,
        avoid: &AvoidCtx,
        budget: usize,
        allow: Option<&crate::hierarchy::ConfinedSet>,
    ) -> AstarResult {
        state.reset();
        if !filter.water_zones.is_empty() {
            state.zones = Some(ZoneIndex::new(&filter.water_zones));
        }
        let start_h = dist3(start_pos, end_pos);
        let (start_tile, start_poly) = match self.poly_of_ref(start_ref) {
            Some(pair) => pair,
            None => return AstarResult::default(),
        };
        state.create(
            start_ref,
            NO_PARENT,
            start_pos,
            0.0,
            start_h,
            Some(start_tile.clone()),
            start_poly,
        );
        state.best_h = start_h;
        state.push(0);

        let mut result = AstarResult::default();
        loop {
            let idx = match state.pop() {
                None => break,
                Some(idx) => idx,
            };
            {
                let node = &mut state.nodes[idx as usize];
                if node.closed {
                    continue;
                }
                node.closed = true;
            }
            result.explored += 1;
            let node_tile = state.nodes[idx as usize].tile.clone();
            let node_poly = state.nodes[idx as usize].poly;
            if goal.reached(
                node_tile
                    .as_ref()
                    .map(|t| &t.polys[node_poly as usize]),
                state.nodes[idx as usize].ref_,
            ) || goal.approach_reached(node_tile.as_ref(), node_poly, end_pos)
            {
                result.corridor = state.corridor_of(idx);
                result.reached = true;
                result.end = end_pos;

                return result;
            }
            if state.nodes.len() >= budget {
                result.capped = true;

                break;
            }
            self.expand(state, idx, end_pos, filter, avoid, allow);
        }

        if state.best != 0 {
            result.corridor = state.corridor_of(state.best);
            result.partial = true;
            result.end = state.nodes[state.best as usize].pos;
        }

        result
    }

    /// Relaxes every link of one settled node. The internal links
    /// resolve inside the node tile without any mesh lookup, the
    /// external one resolves through the mesh.
    fn expand(
        &self,
        state: &mut QueryState,
        idx: u32,
        end_pos: Pos,
        filter: &Filter,
        avoid: &AvoidCtx,
        allow: Option<&crate::hierarchy::ConfinedSet>,
    ) {
        let node_tile = state.nodes[idx as usize].tile.clone();
        let node_poly = state.nodes[idx as usize].poly;
        let tile = match &node_tile {
            Some(t) => t.clone(),
            None => return,
        };
        let poly = &tile.polys[node_poly as usize];
        let from_cost = poly_cost(&tile, poly, state.zones.as_ref(), filter);
        let mut li = poly.first_link;
        while li >= 0 && (li as usize) < tile.links.len() {
            let link = tile.links[li as usize];
            li = link.next;
            let (target_ref, target_tile, target_poly) = match self.link_target(&tile, &link) {
                Some(triple) => triple,
                None => continue,
            };            if let Some(allow) = allow {
                if !allow.allows(
                    RegionKey {
                        col: target_tile.col,
                        row: target_tile.row,
                    },
                    &target_tile.polys[target_poly as usize],
                ) {
                    continue;
                }
            }
            let ban = avoid.state(&target_tile, &target_tile.polys[target_poly as usize]);
            let (ax, ay, bx, by) = tile.portal(poly, &link);
            let mut portal_clear = false;
            if ban == crate::avoid::AvoidState::Wall {
                if avoid.portal_banned(ax, ay, bx, by)
                    || !avoid.foreign_touched(&target_tile, &target_tile.polys[target_poly as usize])
                    || !filter.avoid_grazed
                {
                    continue;
                }
                portal_clear = true;
            }
            let mid_x = (ax + bx) * 0.5;
            let mid_y = (ay + by) * 0.5;
            let mid_z = tile.height_at(poly, mid_x, mid_y);
            let mid = Pos {
                x: mid_x,
                y: mid_y,
                z: mid_z,
            };
            let target_cost = poly_cost(
                &target_tile,
                &target_tile.polys[target_poly as usize],
                state.zones.as_ref(),
                filter,
            );
            let mut g =
                state.nodes[idx as usize].g + dist3(state.nodes[idx as usize].pos, mid)
                    * (from_cost + target_cost)
                    * 0.5;
            if ban == crate::avoid::AvoidState::Escape {
                // The ban that holds the start: the only honest route
                // out of it crosses its own ground - expensive, never
                // sealed.
                g *= crate::avoid::AVOID_ESCAPE_MULTIPLIER;
            } else if portal_clear {
                // The grazed ground of a foreign ban.
                g *= crate::avoid::AVOID_GRAZED_MULTIPLIER;
            }
            let h = dist3(mid, end_pos);
            relax(
                state,
                idx,
                target_ref,
                Some(target_tile),
                target_poly,
                mid,
                g,
                h,
            );
        }
    }
}

/// Relaxes one link target of the corridor search: a reference the
/// search has not seen yet becomes a fresh node, an open one improves
/// through the cheaper parent.
fn relax(
    state: &mut QueryState,
    idx: u32,
    target_ref: PolyRef,
    target_tile: Option<Arc<Tile>>,
    target_poly: u32,
    mid: Pos,
    g: f64,
    h: f64,
) {
    let existing = match state.index.find(target_ref) {
        None => {
            let created = state.create(target_ref, idx, mid, g, h, target_tile, target_poly);
            state.push(created);

            return;
        }
        Some(existing) => existing,
    };
    if state.nodes[existing as usize].closed {
        return;
    }
    {
        let other = &mut state.nodes[existing as usize];
        if g < other.g {
            other.g = g;
            other.f = g + h;
            other.parent = idx;
            other.pos = mid;
        } else {
            return;
        }
    }
    state.fix(existing);
}

/// The 2D hypot port of Go's `math.Hypot` (the amd64 algorithm:
/// p * Sqrt(1+q*q) after the abs/swap normalization) for bit parity
/// with the Go twin.
pub fn go_hypot(p: f64, q: f64) -> f64 {
    let mut p = p.abs();
    let mut q = q.abs();
    if p.is_infinite() || q.is_infinite() {
        return f64::INFINITY;
    }
    if p.is_nan() || q.is_nan() {
        return f64::NAN;
    }
    if p < q {
        std::mem::swap(&mut p, &mut q);
    }
    if p == 0.0 {
        return 0.0;
    }
    let qq = q / p;

    p * (1.0 + qq * qq).sqrt()
}
