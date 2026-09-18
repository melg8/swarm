// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The external pathfinding benchmark bridge (docs/fastpath_research.md
//! section 7): the flat little endian dump `cmd/navmesh-export` writes
//! loads into the native substrates of the raasta navigation engine
//! and the condor navmesh family (ChannelSearch, TA*, prepared TRA*),
//! and the same owner query times every engine. One algorithm per
//! process: the engine name rides argv[1].
//!
//! Usage:
//!
//!     l2bridge <raasta|channel|tastar|trastar> <dump.bin> [query-times]
//!
//! Every printed line carries the BUILD or QUERY marker the runner
//! script greps into the measured tables.

use std::collections::HashMap;
use std::io::Read;
use std::time::{Duration, Instant};

const DUMP_MAGIC: u32 = 0x424D_324C; // 'L2MB' little endian

struct Dump {
    polys: Vec<[f32; 4]>,            // x0, y0, x1, y1 world rect
    areas: Vec<u8>,                  // 0 ground, 1 water
    links: Vec<(u32, u32, [f32; 4])>, // from, to, portal ax ay bx by
    start: [f64; 2],
    goal: [f64; 2],
}

fn read_dump(path: &str) -> Dump {
    let mut file = std::fs::File::open(path)
        .unwrap_or_else(|err| panic!("open {path}: {err}"));
    let mut data = Vec::new();
    file.read_to_end(&mut data)
        .unwrap_or_else(|err| panic!("read {path}: {err}"));

    let u32_at = |off: usize| u32::from_le_bytes(data[off..off + 4].try_into().unwrap());
    let f32_at = |off: usize| f32::from_le_bytes(data[off..off + 4].try_into().unwrap());
    let f64_at = |off: usize| f64::from_le_bytes(data[off..off + 8].try_into().unwrap());

    if u32_at(0) != DUMP_MAGIC {
        panic!("not a swarm dump: magic {:#x}", u32_at(0));
    }
    let poly_count = u32_at(8) as usize;
    let link_count = u32_at(12) as usize;
    let region_count = u32_at(68) as usize;

    let mut off = 72 + region_count * 8;
    let mut polys = Vec::with_capacity(poly_count);
    let mut areas = Vec::with_capacity(poly_count);
    for _ in 0..poly_count {
        polys.push([
            f32_at(off),
            f32_at(off + 4),
            f32_at(off + 8),
            f32_at(off + 12),
        ]);
        areas.push(data[off + 16]);
        off += 17;
    }
    let mut links = Vec::with_capacity(link_count);
    for _ in 0..link_count {
        let from = u32_at(off);
        let to = u32_at(off + 4);
        links.push((
            from,
            to,
            [
                f32_at(off + 8),
                f32_at(off + 12),
                f32_at(off + 16),
                f32_at(off + 20),
            ],
        ));
        off += 24;
    }

    Dump {
        polys,
        areas,
        links,
        start: [f64_at(20), f64_at(28)],
        goal: [f64_at(44), f64_at(52)],
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum CondorEngine {
    Channel,
    TaStar,
    TraStar,
}

fn run_raasta(dump: &Dump, times: usize) {
    use raasta::{NavMesh, NavPoly, NavPolyId, Vec2};

    let poly_count = dump.polys.len();
    let began = Instant::now();

    // The undirected adjacency: the dedupe of the directed link dump
    // (neighbor lists stay small, the linear contains is the cheap
    // side of the build).
    let mut adj: Vec<Vec<u32>> = vec![Vec::new(); poly_count];
    for (from, to, _) in &dump.links {
        let (from, to) = (*from as usize, *to as usize);
        let (f, t) = (from.min(to), from.max(to));
        let (f, t) = (f as u32, t as u32);
        if !adj[f as usize].contains(&t) {
            adj[f as usize].push(t);
        }
        if !adj[t as usize].contains(&f) {
            adj[t as usize].push(f);
        }
    }

    let mut mesh = NavMesh::new();
    for (i, rect) in dump.polys.iter().enumerate() {
        let (x0, y0, x1, y1) = (rect[0], rect[1], rect[2], rect[3]);
        mesh.add_poly(NavPoly {
            id: NavPolyId(i as u32),
            vertices: vec![
                Vec2::new(x0, y0),
                Vec2::new(x1, y0),
                Vec2::new(x1, y1),
                Vec2::new(x0, y1),
            ],
            neighbors: adj[i].iter().map(|&n| NavPolyId(n)).collect(),
            // The production swim pricing: the water polys cost 3x
            // (the NavPoly cost field, the plain find_path applies it).
            cost: if dump.areas[i] == 1 { 3.0 } else { 1.0 },
            layer: 0,
        });
    }
    let build = began.elapsed();
    println!("build raasta: {} polys, {:.3?} BUILD", poly_count, build);

    let start = Vec2::new(dump.start[0] as f32, dump.start[1] as f32);
    let goal = Vec2::new(dump.goal[0] as f32, dump.goal[1] as f32);
    let mut best = Duration::MAX;
    for round in 1..=times {
        let began = Instant::now();
        let path = mesh.find_path(start, goal);
        let took = began.elapsed();
        best = best.min(took);
        match path {
            Some(path) => println!(
                "query raasta round {round}: found, {} polys, {:.3?} QUERY",
                path.len(),
                took
            ),
            None => println!(
                "query raasta round {round}: no path, {:.3?} QUERY",
                took
            ),
        }
    }
    println!("query raasta best: {:.3?} QUERY", best);
}

fn run_condor(dump: &Dump, times: usize, engine: CondorEngine) {
    use condor_navmesh::navmesh::{
        Navmesh, NavmeshCell, NavmeshPathfinder, NavmeshPortal, NavmeshQuery,
    };
    use condor_navmesh::{Point2, SearchOutcome};

    let poly_count = dump.polys.len();
    let began = Instant::now();

    let cells: Vec<NavmeshCell> = dump
        .polys
        .iter()
        .enumerate()
        .map(|(i, rect)| {
            let (x0, y0, x1, y1) =
                (rect[0] as f64, rect[1] as f64, rect[2] as f64, rect[3] as f64);
            NavmeshCell::new(
                i.to_string(),
                vec![
                    Point2::new(x0, y0),
                    Point2::new(x1, y0),
                    Point2::new(x1, y1),
                    Point2::new(x0, y1),
                ],
            )
        })
        .collect();

    // The portals: the undirected dedupe of the directed link dump
    // (the same cell pair with the same segment once; the distinct
    // spans of one pair stay distinct portals).
    let mut portals: Vec<NavmeshPortal> = Vec::new();
    let mut seen: HashMap<(u32, u32, [u32; 4]), ()> = HashMap::new();
    for (from, to, seg) in &dump.links {
        let key = (
            (*from).min(*to),
            (*from).max(*to),
            [
                seg[0].to_bits(),
                seg[1].to_bits(),
                seg[2].to_bits(),
                seg[3].to_bits(),
            ],
        );
        if seen.insert(key, ()).is_some() {
            continue;
        }
        portals.push(NavmeshPortal {
            left_cell: *from as usize,
            right_cell: *to as usize,
            start: Point2::new(seg[0] as f64, seg[1] as f64),
            end: Point2::new(seg[2] as f64, seg[3] as f64),
        });
    }

    let navmesh = Navmesh::new(cells, portals);
    navmesh
        .validate()
        .unwrap_or_else(|err| panic!("navmesh validate: {err:?}"));
    let build = began.elapsed();
    println!(
        "build condor: {} cells, {} portals, {:.3?} BUILD",
        poly_count,
        navmesh.portals().len(),
        build
    );

    let start = Point2::new(dump.start[0], dump.start[1]);
    let goal = Point2::new(dump.goal[0], dump.goal[1]);

    // The prepared TRA*: the preprocess is the algorithm's own build
    // cost; the queries time against the prepared map.
    let run_search = |label: &str, answer: condor_navmesh::navmesh::NavmeshSearchResult, took: Duration, best: &mut Duration| {
        *best = (*best).min(took);
        match answer {
            Ok(outcome) => match outcome {
                SearchOutcome::Found { path, stats } => println!(
                    "query {label} round: found, {} points, {} visited, {:.3?} QUERY",
                    path.points().len(),
                    stats.visited_nodes,
                    took
                ),
                SearchOutcome::NoPath { stats } => println!(
                    "query {label} round: no path, {} visited, {:.3?} QUERY",
                    stats.visited_nodes,
                    took
                ),
                _ => println!(
                    "query {label} round: unknown outcome, {:.3?} QUERY",
                    took
                ),
            },
            Err(err) => {
                println!("query {label} round: error {err:?}, {:.3?} QUERY", took);
            }
        }
    };

    if engine == CondorEngine::TraStar {
        use condor_navmesh::{PreparedNavmeshBuilder, TRAStarBuilder};

        let began = Instant::now();
        let prepared_map = TRAStarBuilder.preprocess(&navmesh);
        let preprocess = began.elapsed();
        match prepared_map {
            Ok(map) => {
                println!(
                    "build trastar preprocess: {:.3?} BUILD",
                    preprocess
                );
                let mut best = Duration::MAX;
                for round in 1..=times {
                    let began = Instant::now();
                    let answer = map.search(NavmeshQuery::new(start, goal));
                    let took = began.elapsed();
                    run_search("trastar", answer, took, &mut best);
                    let _ = round;
                }
                println!("query trastar best: {:.3?} QUERY", best);
            }
            Err(err) => {
                println!(
                    "build trastar preprocess: error {err:?} after {:.3?} BUILD",
                    preprocess
                );
            }
        }

        return;
    }

    if engine == CondorEngine::TaStar {
        let pf = condor_navmesh::TAStar;
        let mut best = Duration::MAX;
        for round in 1..=times {
            let began = Instant::now();
            let answer = pf.search(&navmesh, NavmeshQuery::new(start, goal));
            let took = began.elapsed();
            run_search("tastar", answer, took, &mut best);
            let _ = round;
        }
        println!("query tastar best: {:.3?} QUERY", best);

        return;
    }

    let pf = condor_navmesh::ChannelSearch;
    let mut best = Duration::MAX;
    for round in 1..=times {
        let began = Instant::now();
        let answer = pf.search(&navmesh, NavmeshQuery::new(start, goal));
        let took = began.elapsed();
        run_search("channel", answer, took, &mut best);
        let _ = round;
    }
    println!("query channel best: {:.3?} QUERY", best);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 {
        eprintln!("usage: l2bridge <raasta|channel|tastar|trastar> <dump.bin> [times]");
        std::process::exit(2);
    }
    let engine = args[1].as_str();
    let dump = read_dump(&args[2]);
    let times: usize = if args.len() > 3 {
        args[3].parse().unwrap_or(3)
    } else {
        3
    };
    println!(
        "dump: {} polys, {} links, start ({}, {}) goal ({}, {})",
        dump.polys.len(),
        dump.links.len(),
        dump.start[0],
        dump.start[1],
        dump.goal[0],
        dump.goal[1]
    );
    match engine {
        "raasta" => run_raasta(&dump, times),
        "channel" => run_condor(&dump, times, CondorEngine::Channel),
        "tastar" => run_condor(&dump, times, CondorEngine::TaStar),
        "trastar" => run_condor(&dump, times, CondorEngine::TraStar),
        other => {
            eprintln!("unknown engine {other}");
            std::process::exit(2);
        }
    }
}
