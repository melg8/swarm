// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The Rust half of the Go-vs-Rust pathfind comparison: the same
//! protocol as the Go `cmd/pathbench` (generate | warm | cold over
//! the shared pair file).
//!
//! The binary wraps the global allocator with a counting one so the
//! total allocated bytes ride the results JSON next to the Go
//! runtime.MemStats numbers.

use serde::Deserialize;
use std::alloc::{GlobalAlloc, Layout, System};
use std::sync::atomic::{AtomicU64, Ordering};
use swarm_navmesh::{Filter, Mesh, Pos};

static TOTAL_ALLOC: AtomicU64 = AtomicU64::new(0);
static MALLOC_COUNT: AtomicU64 = AtomicU64::new(0);

struct CountingAlloc;

unsafe impl GlobalAlloc for CountingAlloc {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        TOTAL_ALLOC.fetch_add(layout.size() as u64, Ordering::Relaxed);
        MALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        System.alloc(layout)
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        System.dealloc(ptr, layout)
    }
}

#[global_allocator]
static GLOBAL: CountingAlloc = CountingAlloc;

#[derive(Debug, Clone, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct PairSpec {
    name: String,
    #[serde(default)]
    start: [f64; 3],
    #[serde(default)]
    end: [f64; 3],
    #[serde(default)]
    smooth: bool,
    #[serde(default)]
    clearance: f64,
    #[serde(default = "zero3")]
    bound_start: [f64; 3],
    #[serde(default = "zero3")]
    bound_end: [f64; 3],
    #[serde(default)]
    expected: Expected,
}

fn zero3() -> [f64; 3] {
    [0.0; 3]
}

#[derive(Debug, Clone, Copy, Default, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct Expected {
    #[serde(default)]
    found: bool,
    #[serde(default)]
    partial: bool,
    #[serde(default)]
    hierarchical: bool,
    #[serde(default)]
    corridor: i64,
    #[serde(default)]
    waypoints: usize,
    #[serde(default)]
    length: f64,
    #[serde(default)]
    explored: usize,
    #[serde(default)]
    pocket_escape: bool,
}

/ SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The Rust half of the Go-vs-Rust pathfind comparison: the same
//! protocol as the Go `cmd/pathbench` (generate | warm | cold over
//! the shared pair file).
//!
//! The binary wraps the global allocator with a counting one so the
//! total allocated bytes ride the results JSON next to the Go
//! runtime.MemStats numbers.

use serde::Deserialize;
use std::alloc::{GlobalAlloc, Layout, System};
use std::sync::atomic::{AtomicU64, Ordering};
use swarm_navmesh::{Filter, Mesh, Pos};

static TOTAL_ALLOC: AtomicU64 = AtomicU64::new(0);
static MALLOC_COUNT: AtomicU64 = AtomicU64::new(0);

struct CountingAlloc;

unsafe impl GlobalAlloc for CountingAlloc {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        TOTAL_ALLOC.fetch_add(layout.size() as u64, Ordering::Relaxed);
        MALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        System.alloc(layout)
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        System.dealloc(ptr, layout)
    }
}

#[global_allocator]
static GLOBAL: CountingAlloc = CountingAlloc;

#[derive(Debug, Clone, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct PairSpec {
    name: String,
    #[serde(default)]
    start: [f64; 3],
    #[serde(default)]
    end: [f64; 3],
    #[serde(default)]
    smooth: bool,
    #[serde(default)]
    clearance: f64,
    #[serde(default = "zero3")]
    bound_start: [f64; 3],
    #[serde(default = "zero3")]
    bound_end: [f64; 3],
    #[serde(default)]
    expected: Expected,
}

fn zero3() -> [f64; 3] {
    [0.0; 3]
}

#[derive(Debug, Clone, Copy, Default, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct Expected {
    #[serde(default)]
    found: bool,
    #[serde(default)]
    partial: bool,
    #[serde(default)]
    hierarchical: bool,
    #[serde(default)]
    corridor: i64,
    #[serde(default)]
    waypoints: usize,
    #[serde(default)]
    length: f64,
    #[serde(default)]
    explored: usize,
    #[serde(default)]
    pocket_escape: bool,
}

#[derive(Debug, Clone, Deserialize)]
struct SeedPair {
    name: String,
    start: [f64; 3],
    end: [f64; 3],
    #[serde(default)]
    smooth: bool,
    #[serde(default)]
    clearance: f64,
}

#[derive(Debug, Clone, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct PairResult {
    name: String,
    found: bool,
    partial: bool,
    hierarchical: bool,
    corridor: usize,
    waypoints: usize,
    length: f64,
    explored: usize,
    rounds: usize,
    total_ns: u64,
    min_ns: u64,
    median_ns: u64,
    max_ns: u64,
    mismatches: usize,
    #[serde(skip_serializing_if = "String::is_empty")]
    first_note: String,
    /// The per round samples buffer (kept out of the JSON; drained
    /// before the push).
    #[serde(skip)]
    samples: Vec<u64>,
}

#[derive(Debug, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct BenchOutput {
    engine: String,
    mode: String,
    rounds: usize,
    pairs: Vec<PairResult>,
    total_timed_ns: u64,
    peak_rss_kb: i64,
    total_alloc_bytes: u64,
    mallocs: u64,
    verify_errors: usize,
    warmup_ns: u64,
}

fn dist3(a: Pos, b: Pos) -> f64 {
    let (dx, dy, dz) = (a.x - b.x, a.y - b.y, a.z - b.z);
    (dx * dx + dy * dy + dz * dz).sqrt()
}

fn route_length(wps: &[Pos]) -> f64 {
    let mut sum = 0.0;
    for i in 1..wps.len() {
        sum += dist3(wps[i - 1], wps[i]);
    }

    sum
}

fn pos_of(v: [f64; 3]) -> Pos {
    Pos {
        x: v[0],
        y: v[1],
        z: v[2],
    }
}

fn filter_of(spec: &PairSpec) -> Filter {
    Filter {
        smooth: spec.smooth,
        waypoint_clearance: spec.clearance,
        ..Filter::default_filter()
    }
}

fn peak_rss_kb() -> i64 {
    if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
        for line in status.lines() {
            if let Some(rest) = line.strip_prefix("VmHWM:") {
                let kb: i64 = rest
                    .split_whitespace()
                    .next()
                    .and_then(|v| v.parse().ok())
                    .unwrap_or(0);

                return kb;
            }
        }
    }

    0
}

/// Resolves a world position onto the mesh (the bindPos ladder of
/// the Go generator).
fn bind_pos(mesh: &Mesh, pos: [f64; 3]) -> Option<Pos> {
    let mut z = -12000.0f64;
    while z <= 12000.0 {
        let probe = Pos {
            x: pos[0],
            y: pos[1],
            z,
        };
        if let Some((ref_, snapped)) = mesh.find_nearest_poly(probe) {
            if ref_ != 0 {
                return Some(snapped);
            }
        }
        z += 800.0;
    }
    let directions: [[f64; 2]; 8] = [
        [1.0, 0.0],
        [-1.0, 0.0],
        [0.0, 1.0],
        [0.0, -1.0],
        [0.7, 0.7],
        [-0.7, 0.7],
        [0.7, -0.7],
        [-0.7, -0.7],
    ];
    let mut radius = 512.0f64;
    while radius <= 4096.0 {
        for d in &directions {
            let probe = Pos {
                x: pos[0] + radius * d[0],
                y: pos[1] + radius * d[1],
                z: pos[2],
            };
            if let Some((ref_, snapped)) = mesh.find_nearest_poly(probe) {
                if ref_ != 0 {
                    return Some(snapped);
                }
            }
        }
        radius *= 2.0;
    }

    None
}

const SEED_PAIRS: &[(&str, [f64; 3], [f64; 3], bool, f64)] = &[
    (
        "short_village_eastgate",
        [46890.0, 51531.0, -2976.0],
        [47872.0, 52736.0, -3800.0],
        false,
        0.0,
    ),
    (
        "medium_guard_stairs",
        [46880.0, 50752.0, -2889.0],
        [47595.0, 51569.0, -2992.0],
        false,
        0.0,
    ),
    (
        "within_21_19_a",
        [45000.0, 48000.0, -3500.0],
        [52000.0, 56000.0, -3500.0],
        false,
        0.0,
    ),
    (
        "within_21_19_b",
        [44000.0, 60000.0, -3000.0],
        [55000.0, 44000.0, -3000.0],
        false,
        0.0,
    ),
    (
        "within_21_19_smooth",
        [46890.0, 51531.0, -2976.0],
        [47872.0, 52736.0, -3800.0],
        true,
        7.5,
    ),
    (
        "within_21_19_smooth_long",
        [45000.0, 48000.0, -3500.0],
        [52000.0, 56000.0, -3500.0],
        true,
        7.5,
    ),
    (
        "cross_to_21_20",
        [46890.0, 51531.0, -2976.0],
        [50000.0, 70000.0, -4000.0],
        false,
        0.0,
    ),
    (
        "cross_to_22_19",
        [46890.0, 51531.0, -2976.0],
        [75000.0, 55000.0, -3500.0],
        false,
        0.0,
    ),
    (
        "cross_long_21_18_to_22_20",
        [40000.0, 20000.0, -3000.0],
        [75000.0, 70000.0, -3500.0],
        false,
        0.0,
    ),
    (
        "cross_22_19_to_21_20",
        [70000.0, 40000.0, -3500.0],
        [45000.0, 72000.0, -4000.0],
        false,
        0.0,
    ),
    (
        "cross_smooth_22_19",
        [70000.0, 40000.0, -3500.0],
        [48000.0, 50000.0, -3500.0],
        true,
        7.5,
    ),
    (
        "within_22_19",
        [68000.0, 38000.0, -3500.0],
        [76000.0, 50000.0, -3500.0],
        false,
        0.0,
    ),
    (
        "within_21_20",
        [40000.0, 70000.0, -4000.0],
        [52000.0, 80000.0, -4000.0],
        false,
        0.0,
    ),
    (
        "far_diagonal_20_19_missing",
        [10000.0, 45000.0, -3500.0],
        [55000.0, 50000.0, -3500.0],
        false,
        0.0,
    ),
];

fn run_generate(mesh_dir: &str, out_path: &str) -> Result<(), String> {
    let mesh = Mesh::new(mesh_dir);
    mesh.set_cache_capacity(8);
    let mut specs: Vec<PairSpec> = Vec::new();
    for (name, start, end, smooth, clearance) in SEED_PAIRS {
        let start_pos = match bind_pos(&mesh, *start) {
            Some(pos) => pos,
            None => {
                println!("skip (start unbindable): {name}");

                continue;
            }
        };
        let end_pos = match bind_pos(&mesh, *end) {
            Some(pos) => pos,
            None => {
                println!("skip (end unbindable): {name}");

                continue;
            }
        };
        let spec = PairSpec {
            name: name.to_string(),
            start: *start,
            end: *end,
            smooth: *smooth,
            clearance: *clearance,
            bound_start: [start_pos.x, start_pos.y, start_pos.z],
            bound_end: [end_pos.x, end_pos.y, end_pos.z],
            expected: Expected::default(),
        };
        let route = match mesh.route(start_pos, end_pos, &filter_of(&spec)) {
            Ok(route) => route,
            Err(err) => {
                println!("skip (route error): {name}: {err}");

                continue;
            }
        };
        println!(
            "{}: found={} partial={} hier={} corridor={} wps={} length={:.0} explored={}",
            spec.name,
            route.found,
            route.partial,
            route.hierarchical,
            route.corridor.len(),
            route.waypoints.len(),
            route_length(&route.waypoints),
            route.explored
        );
        specs.push(PairSpec {
            expected: Expected {
                found: route.found,
                partial: route.partial,
                hierarchical: route.hierarchical,
                corridor: route.corridor.len() as i64,
                waypoints: route.waypoints.len(),
                length: route_length(&route.waypoints),
                explored: route.explored,
                pocket_escape: route.pocket_escape,
            },
            ..spec
        });
    }
    let data = serde_json::to_string_pretty(&specs).map_err(|e| e.to_string())?;
    std::fs::write(out_path, data).map_err(|e| e.to_string())?;

    Ok(())
}

fn load_pairs(path: &str) -> Result<Vec<PairSpec>, String> {
    let data = std::fs::read_to_string(path).map_err(|e| e.to_string())?;
    serde_json::from_str(&data).map_err(|e| e.to_string())
}

fn verify(spec: &PairSpec, route: &swarm_navmesh::Route, output: &mut BenchOutput) {
    if route.found != spec.expected.found
        || route.partial != spec.expected.partial
        || route.corridor.len() as i64 != spec.expected.corridor
        || route.waypoints.len() != spec.expected.waypoints
    {
        output.verify_errors += 1;
        println!(
            "VERIFY MISMATCH {}: got found={} partial={} corridor={} wps={}, want found={} partial={} corridor={} wps={}",
            spec.name,
            route.found,
            route.partial,
            route.corridor.len(),
            route.waypoints.len(),
            spec.expected.found,
            spec.expected.partial,
            spec.expected.corridor,
            spec.expected.waypoints
        );
    }
}

fn run_warm(mesh_dir: &str, pairs_path: &str, rounds: usize, out_path: &str) -> Result<(), String> {
    let mut specs = load_pairs(pairs_path)?;
    let mesh = Mesh::new(mesh_dir);
    mesh.set_cache_capacity(8);
    let mut output = BenchOutput {
        engine: "rust".to_string(),
        mode: "warm".to_string(),
        rounds,
        ..BenchOutput::default()
    };

    // The warmup pass.
    let warmup_began = std::time::Instant::now();
    for spec in specs.iter_mut() {
        match mesh.route(pos_of(spec.bound_start), pos_of(spec.bound_end), &filter_of(spec)) {
            Ok(route) => verify(spec, &route, &mut output),
            Err(err) => {
                spec.expected.corridor = -1;
                output.pairs.push(PairResult {
                    name: spec.name.clone(),
                    first_note: format!("error: {err}"),
                    ..PairResult::default()
                });
            }
        }
    }
    output.warmup_ns = warmup_began.elapsed().as_nanos() as u64;

    // The timed rounds.
    let mut results: Vec<PairResult> = specs
        .iter()
        .map(|spec| PairResult {
            name: spec.name.clone(),
            min_ns: u64::MAX,
            ..PairResult::default()
        })
        .collect();
    for _ in 0..rounds {
        for (i, spec) in specs.iter().enumerate() {
            if spec.expected.corridor < 0 {
                continue;
            }
            let began = std::time::Instant::now();
            let route = mesh.route(pos_of(spec.bound_start), pos_of(spec.bound_end), &filter_of(spec));
            let elapsed = began.elapsed().as_nanos() as u64;
            let res = &mut results[i];
            match route {
                Err(_) => {
                    res.mismatches += 1;
                }
                Ok(route) => {
                    res.rounds += 1;
                    res.total_ns += elapsed;
                    if elapsed < res.min_ns {
                        res.min_ns = elapsed;
                    }
                    if elapsed > res.max_ns {
                        res.max_ns = elapsed;
                    }
                    res.median_ns = 0; // filled after the sort below
                    res.samples.push(elapsed);
                    if route.found != spec.expected.found
                        || route.partial != spec.expected.partial
                        || route.corridor.len() as i64 != spec.expected.corridor
                        || route.waypoints.len() != spec.expected.waypoints
                    {
                        res.mismatches += 1;
                        output.verify_errors += 1;
                    }
                }
            }
        }
    }
    let mut total_timed = 0u64;
    for (i, spec) in specs.iter().enumerate() {
        let res = &mut results[i];
        if res.rounds == 0 {
            continue;
        }
        res.samples.sort_unstable();
        res.median_ns = res.samples[res.samples.len() / 2];
        res.found = spec.expected.found;
        res.partial = spec.expected.partial;
        res.hierarchical = spec.expected.hierarchical;
        res.corridor = spec.expected.corridor.max(0) as usize;
        res.waypoints = spec.expected.waypoints;
        res.length = spec.expected.length;
        res.explored = spec.expected.explored;
        total_timed += res.total_ns;
        res.samples.clear();
        output.pairs.push(res.clone());
    }
    output.total_timed_ns = total_timed;
    output.peak_rss_kb = peak_rss_kb();
    output.total_alloc_bytes = TOTAL_ALLOC.load(Ordering::Relaxed);
    output.mallocs = MALLOC_COUNT.load(Ordering::Relaxed);

    let data = serde_json::to_string_pretty(&output).map_err(|e| e.to_string())?;
    std::fs::write(out_path, data).map_err(|e| e.to_string())?;

    Ok(())
}

fn run_cold(mesh_dir: &str, pairs_path: &str, out_path: &str) -> Result<(), String> {
    let specs = load_pairs(pairs_path)?;
    let mut output = BenchOutput {
        engine: "rust".to_string(),
        mode: "cold".to_string(),
        rounds: 1,
        ..BenchOutput::default()
    };
    let mut total = 0u64;
    for spec in &specs {
        // A fresh mesh per pair: the cold path pays every decode and
        // every abstract build.
        let mesh = Mesh::new(mesh_dir);
        mesh.set_cache_capacity(8);
        let began = std::time::Instant::now();
        let result = mesh.route(pos_of(spec.bound_start), pos_of(spec.bound_end), &filter_of(spec));
        let elapsed = began.elapsed().as_nanos() as u64;
        output.pairs.push(PairResult {
            name: spec.name.clone(),
            total_ns: elapsed,
            min_ns: elapsed,
            median_ns: elapsed,
            max_ns: elapsed,
            rounds: 1,
            first_note: match &result {
                Ok(_) => String::new(),
                Err(err) => format!("error: {err}"),
            },
            ..PairResult::default()
        });
        total += elapsed;
    }
    output.total_timed_ns = total;
    output.peak_rss_kb = peak_rss_kb();

    let data = serde_json::to_string_pretty(&output).map_err(|e| e.to_string())?;
    std::fs::write(out_path, data).map_err(|e| e.to_string())?;

    Ok(())
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let mut mode = "warm".to_string();
    let mut mesh_dir = "data/navmesh".to_string();
    let mut pairs_path = "benchmarks/pathfind-go-vs-rust/pairs.json".to_string();
    let mut out_path = "rust_results.json".to_string();
    let mut rounds = 50usize;
    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "-mode" | "--mode" => {
                mode = args[i + 1].clone();
                i += 1;
            }
            "-mesh" | "--mesh" => {
                mesh_dir = args[i + 1].clone();
                i += 1;
            }
            "-pairs" | "--pairs" => {
                pairs_path = args[i + 1].clone();
                i += 1;
            }
            "-rounds" | "--rounds" => {
                rounds = args[i + 1].parse().unwrap_or(50);
                i += 1;
            }
            "-out" | "--out" => {
                out_path = args[i + 1].clone();
                i += 1;
            }
            other => {
                eprintln!("unknown argument: {other}");
                std::process::exit(2);
            }
        }
        i += 1;
    }

    let result = match mode.as_str() {
        "generate" => run_generate(&mesh_dir, &out_path),
        "warm" => run_warm(&mesh_dir, &pairs_path, rounds, &out_path),
        "cold" => run_cold(&mesh_dir, &pairs_path, &out_path),
        other => Err(format!("unknown mode {other}")),
    };
    if let Err(err) = result {
        eprintln!("Error: {err}");
        std::process::exit(1);
    }
}
