// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//! The abstract sidecar format: the persistent coarse layer of one
//! region. The pack build serializes the cluster graph next to the
//! tile (the X_Y.ab sidecar); the runtime loads it instead of
//! re-scanning the decoded tile.
//!
//! This port carries the runtime half (the decode); the encoder lives
//! with the Go navbuild tool that produces the sidecars.

use crate::abstract_graph::{
    AbstractClassKey, AbstractEdge, AbstractNode, ClusterId, ClusterKey, RegionAbstract,
};
use crate::tile::RegionKey;
use crate::NavError;

const ABSTRACT_MAGIC: u32 = 0x3142_4153; // 'SAB1' little endian
const ABSTRACT_VERSION: u32 = 2;
const ABSTRACT_VERSION_V1: u32 = 1;
const ABSTRACT_HEADER_SIZE: usize = 56;
const ABSTRACT_EDGE_WIRE_SIZE: usize = 64;

fn err_bad(msg: String) -> NavError {
    NavError(format!("not a navmesh tile: {msg}"))
}

fn u32_le(data: &[u8], off: usize) -> u32 {
    u32::from_le_bytes([data[off], data[off + 1], data[off + 2], data[off + 3]])
}

fn u16_le(data: &[u8], off: usize) -> u16 {
    u16::from_le_bytes([data[off], data[off + 1]])
}

fn f64_le(data: &[u8], off: usize) -> f64 {
    f64::from_le_bytes([
        data[off],
        data[off + 1],
        data[off + 2],
        data[off + 3],
        data[off + 4],
        data[off + 5],
        data[off + 6],
        data[off + 7],
    ])
}

/// Parses one abstract sidecar.
pub fn decode_abstract(data: &[u8]) -> Result<RegionAbstract, NavError> {
    if data.len() < ABSTRACT_HEADER_SIZE {
        return Err(err_bad(format!(
            "the abstract {} bytes is too short",
            data.len()
        )));
    }
    if u32_le(data, 0) != ABSTRACT_MAGIC {
        return Err(err_bad(format!("the abstract magic 0x{:x}", u32_le(data, 0))));
    }
    let version = u32_le(data, 4);
    if version != ABSTRACT_VERSION && version != ABSTRACT_VERSION_V1 {
        return Err(err_bad(format!("the abstract version {version}")));
    }
    let checksums = version == ABSTRACT_VERSION;
    let col = u32_le(data, 8) as i16;
    let row = u32_le(data, 12) as i16;
    let polys = u32_le(data, 16) as usize;
    let node_count = u32_le(data, 20) as usize;
    let edge_count = u32_le(data, 24) as usize;
    let run_count = u32_le(data, 28) as usize;
    let tile_size = u64::from_le_bytes(data[32..40].try_into().unwrap()) as i64;
    let tile_mod_time_unix_nano =
        u64::from_le_bytes(data[40..48].try_into().unwrap()) as i64;

    let mut abstract_ = RegionAbstract {
        key: RegionKey { col, row },
        index: std::collections::HashMap::with_capacity(node_count),
        edges: Vec::with_capacity(edge_count),
        comps: vec![0u32; polys],
        nodes: Vec::new(),
        polys,
        tile_size,
        tile_mod_time_unix_nano,
        tile_checksums: checksums,
        tile_head_crc: u32_le(data, 48),
        tile_tail_crc: u32_le(data, 52),
    };

    let mut offset = ABSTRACT_HEADER_SIZE;
    for _ in 0..node_count {
        if offset + 8 > data.len() {
            return Err(err_bad("the abstract nodes truncate".to_string()));
        }
        let id: ClusterId = data[offset];
        let edge_n = u16_le(data, offset + 2) as usize;
        offset += 8;
        if offset + edge_n * 4 > data.len() {
            return Err(err_bad("the abstract node edges truncate".to_string()));
        }
        let mut node = AbstractNode {
            id,
            edges: Vec::with_capacity(edge_n),
            classes: std::collections::HashMap::with_capacity(edge_n),
        };
        for _ in 0..edge_n {
            let idx = u32_le(data, offset) as i32;
            offset += 4;
            if idx < 0 || (idx as usize) >= edge_count {
                return Err(err_bad(format!("the abstract edge index {idx}")));
            }
            node.edges.push(idx);
        }
        abstract_.index.insert(id, abstract_.nodes.len() as i32);
        abstract_.nodes.push(node);
    }
    if offset + edge_count * ABSTRACT_EDGE_WIRE_SIZE > data.len() {
        return Err(err_bad("the abstract edges truncate".to_string()));
    }
    for i in 0..edge_count {
        let base = offset + i * ABSTRACT_EDGE_WIRE_SIZE;
        let edge = AbstractEdge {
            to: ClusterKey {
                col: u16_le(data, base) as i16,
                row: u16_le(data, base + 2) as i16,
                id: data[base + 4],
            },
            to_ref: u64::from_le_bytes(data[base + 8..base + 16].try_into().unwrap()),
            link: u32_le(data, base + 16) as i32,
            mid: crate::query::Pos {
                x: f64_le(data, base + 20),
                y: f64_le(data, base + 28),
                z: f64_le(data, base + 36),
            },
            src_comp: u32_le(data, base + 44),
            dst_comp: u32_le(data, base + 48),
            ext_col: u16_le(data, base + 52) as i16,
            ext_row: u16_le(data, base + 54) as i16,
            ext_poly: u32_le(data, base + 56),
            src_area: data[base + 60],
            dst_area: data[base + 61],
        };
        abstract_.edges.push(edge);
    }
    offset += edge_count * ABSTRACT_EDGE_WIRE_SIZE;
    if offset + run_count * 8 > data.len() {
        return Err(err_bad("the abstract components truncate".to_string()));
    }
    let mut cursor = 0usize;
    for i in 0..run_count {
        let base = offset + i * 8;
        let value = u32_le(data, base);
        let count = u32_le(data, base + 4) as usize;
        for _ in 0..count {
            if cursor >= polys {
                return Err(err_bad("the abstract components overflow".to_string()));
            }
            abstract_.comps[cursor] = value;
            cursor += 1;
        }
    }

    // The class dedupe rebuilds from the stored edges: the build kept
    // one edge per class, the first mapping per stored edge
    // reconstructs the map.
    for node in abstract_.nodes.iter_mut() {
        for &edge_idx in &node.edges {
            let edge = &abstract_.edges[edge_idx as usize];
            let key = AbstractClassKey {
                to: edge.to,
                src_area: edge.src_area,
                dst_area: edge.dst_area,
                src_comp: edge.src_comp,
            };
            node.classes.insert(key, edge_idx);
        }
    }

    Ok(abstract_)
}
