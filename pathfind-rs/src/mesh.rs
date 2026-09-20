// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The mesh: lazy tile loading from a directory with the LRU bound,
//! the abstract (cluster graph) cache and the hop corridor cache.
//!
//! The Go twin runs the tile decode behind a per key singleflight and
//! the prefetch pools on worker threads; this port keeps the same
//! public behavior with a mutex guarded cache and a bounded worker
//! pool over `std::thread` for the prefetch.

use crate::abstract_graph::{build_abstract, RegionAbstract};
use crate::abstractfile::decode_abstract;
use crate::astar::QueryState;
use crate::hierarchy::hop_cache::HopCache;
use crate::tile::{
    decode_tile, ref_of, RegionKey, Tile, CELL_SIZE_WORLD, TILE_ZERO_COL, TILE_ZERO_ROW,
    TILE_WORLD_SIZE,
};
use crate::{NavError, PolyRef};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

/// Bounds how many tiles stay loaded (zero or less means unlimited).
pub const DEFAULT_MESH_CAPACITY: i32 = 4;

/// Caps the parallel tile decode pool of the prefetch.
pub const MAX_PREFETCH_WORKERS: usize = 4;

/// The tile file extension under the mesh directory.
pub const TILE_FILE_EXT: &str = ".nm";

/// The persistent coarse layer sidecar extension.
pub const ABSTRACT_FILE_EXT: &str = ".ab";

/// The state of one cache entry.
#[derive(Clone, Copy, PartialEq, Eq)]
enum TileState {
    Loaded,
    Failed,
    Missing,
}

/// One loaded, failed or missing tile in the cache.
struct TileEntry {
    tile: Option<Arc<Tile>>,
    error: Option<NavError>,
    state: TileState,
}

impl TileEntry {
    fn answer(&self) -> Result<Arc<Tile>, NavError> {
        match self.state {
            TileState::Loaded => Ok(self.tile.clone().unwrap()),
            TileState::Failed => Err(self.error.clone().unwrap()),
            TileState::Missing => Err(crate::tile_absent()),
        }
    }
}

/// The internal mesh state behind the mutex (the tile cache, the LRU
/// order and the file set).
struct TileCache {
    capacity: i32,
    tiles: HashMap<RegionKey, Arc<TileEntry>>,
    lru: Vec<RegionKey>,
    files: std::collections::HashSet<RegionKey>,
}

/// The abstract cache behind its own mutex.
struct AbstractCache {
    capacity: i32,
    abstracts: HashMap<RegionKey, Option<Arc<RegionAbstract>>>,
    order: Vec<RegionKey>,
}

/// Loads navigation mesh tiles lazily from a directory and answers
/// the navigation queries over them. Safe for concurrent use.
pub struct Mesh {
    dir: String,
    tile_cache: Mutex<TileCache>,
    abstract_cache: Mutex<AbstractCache>,
    hops: Mutex<HopCache>,
    states: Mutex<Vec<QueryState>>,
    coarses: Mutex<Vec<crate::hierarchy::CoarseState>>,
}

impl Mesh {
    /// Creates a mesh over a tile directory. The directory is scanned
    /// once for X_Y.nm files; an empty or missing directory yields a
    /// working mesh whose every query answers the no navmesh error.
    pub fn new(dir: &str) -> Mesh {
        let files = scan_files(dir);
        Mesh {
            dir: dir.to_string(),
            tile_cache: Mutex::new(TileCache {
                capacity: DEFAULT_MESH_CAPACITY,
                tiles: HashMap::new(),
                lru: Vec::with_capacity(DEFAULT_MESH_CAPACITY.max(0) as usize),
                files,
            }),
            abstract_cache: Mutex::new(AbstractCache {
                capacity: crate::abstract_graph::ABSTRACT_CACHE_CAPACITY,
                abstracts: HashMap::new(),
                order: Vec::new(),
            }),
            hops: Mutex::new(HopCache::new()),
            states: Mutex::new(Vec::new()),
            coarses: Mutex::new(Vec::new()),
        }
    }

    /// Overrides the loaded tile bound.
    pub fn set_cache_capacity(&self, capacity: i32) {
        self.tile_cache.lock().unwrap().capacity = capacity;
    }

    /// Overrides the abstract cache bound.
    pub fn set_abstract_capacity(&self, capacity: i32) {
        self.abstract_cache.lock().unwrap().capacity = capacity;
    }

    /// The mesh summary for diagnostics.
    pub fn stats(&self) -> (String, usize, usize) {
        let cache = self.tile_cache.lock().unwrap();

        (self.dir.clone(), cache.files.len(), cache.tiles.len())
    }

    /// Lists the region keys of the tile files found in the mesh
    /// directory, ordered col first then row.
    pub fn tile_files(&self) -> Vec<RegionKey> {
        let cache = self.tile_cache.lock().unwrap();
        let mut keys: Vec<RegionKey> = cache.files.iter().copied().collect();
        keys.sort_by(|a, b| a.col.cmp(&b.col).then(a.row.cmp(&b.row)));

        keys
    }

    /// Returns the loaded tile of a region, loading it on demand. A
    /// missing tile file is a cached miss (not an error): the link
    /// resolution treats it as a wall.
    pub fn tile(&self, key: &RegionKey) -> Result<Arc<Tile>, NavError> {
        {
            let mut cache = self.tile_cache.lock().unwrap();
            if let Some(entry) = cache.tiles.get(key) {
                let entry = entry.clone();
                touch(&mut cache.lru, key);
                evict(&mut cache);
                drop(cache);

                return entry.answer();
            }
        }
        // The decode runs outside the lock (the Go twin runs it
        // behind a per key singleflight; this port decodes once and
        // the concurrent callers merge at the store below).
        let entry = self.decode_tile_entry(key);
        let answer = entry.answer();
        let entry = Arc::new(entry);
        let mut cache = self.tile_cache.lock().unwrap();
        cache.tiles.insert(*key, entry.clone());
        cache.lru.push(*key);
        evict(&mut cache);

        answer
    }

    /// Reads and parses one tile file without holding the mesh lock.
    fn decode_tile_entry(&self, key: &RegionKey) -> TileEntry {
        let present = self.tile_cache.lock().unwrap().files.contains(key);
        if !present {
            return TileEntry {
                tile: None,
                error: None,
                state: TileState::Missing,
            };
        }
        let path = std::path::Path::new(&self.dir)
            .join(format!("{}_{}{}", key.col, key.row, TILE_FILE_EXT));
        let data = match std::fs::read(&path) {
            Ok(data) => data,
            Err(err) => {
                return TileEntry {
                    tile: None,
                    error: Some(NavError(format!(
                        "read navmesh tile {}_{}: {}",
                        key.col, key.row, err
                    ))),
                    state: TileState::Failed,
                };
            }
        };
        // The tile file may be compressed (the navmesh-build -compress
        // output): the magic word decides - the zstd frame is the
        // current format, the gzip frame the legacy packs carry.
        let data = match decompress_tile(&data) {
            Ok(data) => data,
            Err(err) => {
                return TileEntry {
                    tile: None,
                    error: Some(NavError(format!(
                        "decompress navmesh tile {}_{}: {}",
                        key.col, key.row, err
                    ))),
                    state: TileState::Failed,
                };
            }
        };
        match decode_tile(&data) {
            Ok(tile) => TileEntry {
                tile: Some(Arc::new(tile)),
                error: None,
                state: TileState::Loaded,
            },
            Err(err) => TileEntry {
                tile: None,
                error: Some(NavError(format!(
                    "decode navmesh tile {}_{}: {}",
                    key.col, key.row, err
                ))),
                state: TileState::Failed,
            },
        }
    }

    /// Loads the requested tiles into the cache with a bounded
    /// parallel decode pool.
    pub fn prefetch_tiles(&self, keys: &[RegionKey]) {
        let to_load: Vec<RegionKey> = {
            let cache = self.tile_cache.lock().unwrap();
            keys.iter()
                .filter(|key| !cache.tiles.contains_key(key))
                .copied()
                .collect()
        };
        if to_load.is_empty() {
            return;
        }
        if to_load.len() < 2 {
            for key in &to_load {
                let _ = self.tile(key);
            }

            return;
        }
        // The bounded parallel pool (mirrors maxPrefetchWorkers).
        let workers = MAX_PREFETCH_WORKERS.min(to_load.len());
        let next = std::sync::atomic::AtomicUsize::new(0);
        std::thread::scope(|scope| {
            for _ in 0..workers {
                scope.spawn(|| loop {
                    let i = next.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                    if i >= to_load.len() {
                        break;
                    }
                    let _ = self.tile(&to_load[i]);
                });
            }
        });
    }

    /// Loads the region cluster graphs into the abstract cache with a
    /// bounded parallel pool.
    pub fn prefetch_abstracts(&self, keys: &[RegionKey]) {
        let to_load: Vec<RegionKey> = {
            let cache = self.abstract_cache.lock().unwrap();
            keys.iter()
                .filter(|key| !cache.abstracts.contains_key(key))
                .copied()
                .collect()
        };
        for key in &to_load {
            let _ = self.abstract_of(key);
        }
    }

    /// Returns the cached cluster graph of a region, building it over
    /// the decoded tile on demand. A tile that fails to load answers
    /// None (the coarse search treats the region as a wall).
    pub fn abstract_of(&self, key: &RegionKey) -> Option<Arc<RegionAbstract>> {
        {
            let mut cache = self.abstract_cache.lock().unwrap();
            if let Some(abstract_) = cache.abstracts.get(key) {
                let abstract_ = abstract_.clone();
                touch(&mut cache.order, key);
                evict_abstracts(&mut cache);
                drop(cache);

                return abstract_;
            }
        }
        // The persistent coarse layer first: the sidecar answers
        // without decoding the tile.
        let mut cached: Option<Option<Arc<RegionAbstract>>> = None;
        if let Some(sidecar) = self.abstract_sidecar_of(key) {
            cached = Some(Some(Arc::new(sidecar)));
        }
        let abstract_ = match cached {
            Some(abstract_) => abstract_,
            None => match self.tile(key) {
                Ok(tile) => Some(Arc::new(build_abstract(&tile))),
                Err(_) => None,
            },
        };
        let answer = abstract_.clone();
        let mut cache = self.abstract_cache.lock().unwrap();
        cache.abstracts.insert(*key, abstract_);
        cache.order.push(*key);
        evict_abstracts(&mut cache);

        answer
    }

    /// Loads the region abstract from the X_Y.ab sidecar without
    /// decoding the tile (None means the sidecar is absent, stale or
    /// unreadable).
    fn abstract_sidecar_of(&self, key: &RegionKey) -> Option<RegionAbstract> {
        let sidecar_path = std::path::Path::new(&self.dir)
            .join(format!("{}_{}{}", key.col, key.row, ABSTRACT_FILE_EXT));
        let data = std::fs::read(&sidecar_path).ok()?;
        let abstract_ = decode_abstract(&data).ok()?;
        let tile_path = std::path::Path::new(&self.dir)
            .join(format!("{}_{}{}", key.col, key.row, TILE_FILE_EXT));
        let info = std::fs::metadata(&tile_path).ok()?;
        if info.len() as i64 != abstract_.tile_size {
            return None;
        }
        if abstract_.tile_checksums {
            if tile_checksums_match(&tile_path, &abstract_) {
                return Some(abstract_);
            }

            return None;
        }
        let modified = info.modified().ok()?;
        let stored = std::time::SystemTime::UNIX_EPOCH
            + std::time::Duration::from_nanos(abstract_.tile_mod_time_unix_nano as u64);
        if modified != stored {
            return None;
        }

        Some(abstract_)
    }

    /// Takes a pooled query state.
    pub(crate) fn acquire_state(&self) -> Box<QueryState> {
        let mut pool = self.states.lock().unwrap();
        Box::new(pool.pop().unwrap_or_default())
    }

    /// Returns a query state to the pool (the buffers keep their
    /// capacity for the next query).
    pub(crate) fn release_state(&self, state: Box<QueryState>) {
        self.states.lock().unwrap().push(*state);
    }

    /// Takes a pooled coarse (cluster level) search state.
    pub(crate) fn acquire_coarse(&self) -> Box<crate::hierarchy::CoarseState> {
        let mut pool = self.coarses.lock().unwrap();
        Box::new(pool.pop().unwrap_or_default())
    }

    /// Returns a coarse search state to the pool.
    pub(crate) fn release_coarse(&self, state: Box<crate::hierarchy::CoarseState>) {
        self.coarses.lock().unwrap().push(*state);
    }

    /// Answers the cached corridor of a hop key.
    pub(crate) fn cached_hop(&self, key: crate::hierarchy::HopKey) -> Option<Vec<PolyRef>> {
        self.hops.lock().unwrap().cached(key)
    }

    /// Caches the corridor of a portal pair.
    pub(crate) fn store_hop(
        &self,
        key: crate::hierarchy::HopKey,
        corridor: &[PolyRef],
    ) {
        self.hops.lock().unwrap().store(key, corridor);
    }

    /// Returns the loaded tile holding a reference (loading it on
    /// demand). A missing or failed tile answers None: the caller
    /// treats the link as a wall.
    pub fn tile_of_ref(&self, ref_: PolyRef) -> Option<Arc<Tile>> {
        let (col, row) = crate::tile::tile_of(ref_);
        self.tile(&RegionKey { col, row }).ok()
    }

    /// Returns the polygon of a reference together with its tile, or
    /// None when the tile is unavailable or the index is stale.
    pub fn poly_of_ref(&self, ref_: PolyRef) -> Option<(Arc<Tile>, u32)> {
        let poly = crate::tile::poly_of(ref_);
        if poly < 0 {
            return None;
        }
        let tile = self.tile_of_ref(ref_)?;
        if poly as usize >= tile.polys.len() {
            return None;
        }

        Some((tile, poly as u32))
    }

    /// The public alias of poly_of_ref (the Go PolyOf).
    pub fn poly_of(&self, ref_: PolyRef) -> Option<(Arc<Tile>, u32)> {
        self.poly_of_ref(ref_)
    }

    /// Names the neighbor a link leads to: an internal link resolves
    /// inside the same tile, an external link loads the neighbor
    /// region tile and validates its polygon index.
    pub fn resolve_link(&self, tile: &Tile, link: &crate::tile::Link) -> PolyRef {
        if link.to >= 0 {
            if link.to as usize >= tile.polys.len() {
                return 0;
            }

            return ref_of(tile.col, tile.row, link.to as u32);
        }
        let ext_idx = (-link.to - 1) as usize;
        if ext_idx >= tile.ext_links.len() {
            return 0;
        }
        let ext = &tile.ext_links[ext_idx];
        if ext.poly == 0xFFFF_FFFF {
            return 0;
        }
        let neighbor = match self.tile(&RegionKey {
            col: ext.col as i16,
            row: ext.row as i16,
        }) {
            Ok(neighbor) => neighbor,
            Err(_) => return 0,
        };
        if ext.poly as usize >= neighbor.polys.len() {
            return 0;
        }

        ref_of(ext.col as i16, ext.row as i16, ext.poly)
    }

    /// Resolves one link of a node polygon to its target reference,
    /// tile and polygon index: the internal link stays inside the
    /// node tile (no mesh lookup, the caller's Arc rides through),
    /// the external one resolves through the mesh. A link the mesh
    /// cannot resolve answers None - the caller treats it as a wall.
    pub fn link_target(
        &self,
        tile: &Arc<Tile>,
        link: &crate::tile::Link,
    ) -> Option<(PolyRef, Arc<Tile>, u32)> {
        if link.to >= 0 {
            if link.to as usize >= tile.polys.len() {
                return None;
            }

            return Some((
                ref_of(tile.col, tile.row, link.to as u32),
                tile.clone(),
                link.to as u32,
            ));
        }
        let target_ref = self.resolve_link(tile, link);
        if target_ref == 0 {
            return None;
        }
        let (target_tile, target_poly) = self.poly_of_ref(target_ref)?;

        Some((target_ref, target_tile, target_poly))
    }
}

/// Decompresses the tile bytes by the magic word: zstd, gzip or raw.
fn decompress_tile(data: &[u8]) -> Result<Vec<u8>, String> {
    if data.len() >= 4 && data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD
    {
        let out = zstd::stream::decode_all(std::io::Cursor::new(data))
            .map_err(|err| format!("zstd decode: {err}"))?;

        return Ok(out);
    }
    if data.len() >= 2 && data[0] == 0x1F && data[1] == 0x8B {
        let mut out = Vec::new();
        let mut decoder = flate2::read::GzDecoder::new(data);
        std::io::copy(&mut decoder, &mut out).map_err(|err| format!("gzip decode: {err}"))?;

        return Ok(out);
    }

    Ok(data.to_vec())
}

/// Counts the tile files of the directory.
fn scan_files(dir: &str) -> std::collections::HashSet<RegionKey> {
    let mut files = std::collections::HashSet::new();
    let entries = match std::fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(_) => return files,
    };
    for entry in entries.flatten() {
        let name = entry.file_name();
        let name = name.to_string_lossy();
        let base = match name.strip_suffix(TILE_FILE_EXT) {
            Some(base) => base,
            None => continue,
        };
        let (col_text, row_text) = match base.split_once('_') {
            Some(pair) => pair,
            None => continue,
        };
        let col: i32 = match col_text.parse() {
            Ok(col) => col,
            Err(_) => continue,
        };
        let row: i32 = match row_text.parse() {
            Ok(row) => row,
            Err(_) => continue,
        };
        if !(-32768..=32767).contains(&col) || !(-32768..=32767).contains(&row) {
            continue;
        }
        files.insert(RegionKey {
            col: col as i16,
            row: row as i16,
        });
    }

    files
}

/// Moves a key to the back of the LRU queue.
fn touch(lru: &mut Vec<RegionKey>, key: &RegionKey) {
    if let Some(pos) = lru.iter().position(|candidate| candidate == key) {
        lru.remove(pos);
        lru.push(*key);
    }
}

/// Drops the least recently used entries beyond the capacity.
fn evict(cache: &mut TileCache) {
    while cache.capacity > 0 && cache.lru.len() > cache.capacity as usize {
        let oldest = cache.lru.remove(0);
        cache.tiles.remove(&oldest);
    }
}

/// Drops the oldest abstracts beyond the cache bound.
fn evict_abstracts(cache: &mut AbstractCache) {
    while cache.capacity > 0 && cache.order.len() > cache.capacity as usize {
        let oldest = cache.order.remove(0);
        cache.abstracts.remove(&oldest);
    }
}

/// Reads the two file ends and compares them with the sidecar
/// recorded checksums.
fn tile_checksums_match(path: &std::path::Path, abstract_: &RegionAbstract) -> bool {
    const TILE_CHECKSUM_WINDOW: usize = 512;
    let data = match std::fs::read(path) {
        Ok(data) => data,
        Err(_) => return false,
    };
    let head_end = TILE_CHECKSUM_WINDOW.min(data.len());
    let head = crc32fast::hash(&data[..head_end]);
    if head != abstract_.tile_head_crc {
        return false;
    }
    let tail_start = data.len().saturating_sub(TILE_CHECKSUM_WINDOW);
    let tail = crc32fast::hash(&data[tail_start..]);

    tail == abstract_.tile_tail_crc
}

// The world geometry helpers the callers import from the mesh level.
pub fn tile_world_anchor(col: i16, row: i16) -> (f64, f64) {
    (
        (col as f64 - TILE_ZERO_COL as f64) * TILE_WORLD_SIZE,
        (row as f64 - TILE_ZERO_ROW as f64) * TILE_WORLD_SIZE,
    )
}

pub const CELL_SIZE: f64 = CELL_SIZE_WORLD;
