// Temporary decode probe (debugging the v3 grid parity).
use std::io::Read;

fn main() {
    let dir = "data/navmesh";
    let mut names: Vec<String> = std::fs::read_dir(dir)
        .unwrap()
        .flatten()
        .map(|e| e.file_name().to_string_lossy().to_string())
        .filter(|n| n.ends_with(".nm"))
        .collect();
    names.sort();
    for name in names {
        probe_tile(&format!("{dir}/{name}"));
    }
}

fn probe_tile(path: &str) {
    let mut data = std::fs::read(path).unwrap();
    if data.len() >= 4 && data[0] == 0x28 && data[1] == 0xB5 {
        let mut out = Vec::new();
        zstd::stream::decode_all(std::io::Cursor::new(&data)).unwrap();
        let mut dec = zstd::stream::Decoder::new(std::io::Cursor::new(&data)).unwrap();
        dec.read_to_end(&mut out).unwrap();
        data = out;
    }
    let tile = swarm_navmesh::decode_tile(&data).unwrap();
    let g = tile.grid.as_ref().unwrap();
    let mut sum: u64 = 0;
    let mut max: u32 = 0;
    for e in &g.entries {
        sum += *e as u64;
        if *e > max {
            max = *e;
        }
    }
    println!(
        "{}: polys={} entries={} sum={} max={} rawLen={}",
        path,
        tile.polys.len(),
        g.entries.len(),
        sum,
        max,
        data.len()
    );
}
