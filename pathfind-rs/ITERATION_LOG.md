# Rust Port Iteration Log (honest compile-fix accounting)

Every `cargo check` cycle during the port is logged with its error
count and the fix category. Nothing is cherry-picked.

| Iteration | Errors | Fix categories |
|---|---|---|
| 1 | 36 | `abstract` is a reserved keyword in Rust (module rename); `RegionKey` never defined; `SIDE_*` vs `POLY_SIDE_*` naming drift between modules |
| 2 | 27 | zstd 0.13 `decode_all` API arity; `hop_cache::HopKey` private import path; missing lifetime on the wall-span cache; borrow conflicts (`&mut route` + `&route.corridor`); `CoarseNode.entry_comp` missing field; `RegionKey/Pos: Default` derives |
| 3 | 26 | (partial compile) same categories, converged |
| 4 | 5 | `region_of_world` returns a struct (Go returns two values) — three destructure sites; `i16` vs `i32` span arithmetic; `find_nearest_poly` 2-tuple vs 3 |
| 5 | 0 | library compiles; warnings cleaned |

## Fix category summary (the real porting friction)

1. **Borrow checker vs Go's aliasing (the expected one).** Go passes
   `*Tile`/`*Poly` pointers freely and mutates `route` while reading
   `route.corridor`. Rust forced: `Arc<Tile>` sharing + polygon
   indexes instead of pointers, corridor clones at the waypoint
   assembly call sites, explicit pool `Box`ing. ~40% of the errors.
2. **Go tuple/multiple-return mindset.** Three sites destructured
   `RegionKey` as a pair; Go returns two values, Rust returns a
   struct. Mechanical.
3. **Naming drift across 12 files.** Constants renamed mid-port
   (`SIDE_*` → `POLY_SIDE_*`) left stale imports; the compiler caught
   every one. Mechanical.
4. **Reserved keyword collision.** Go package `navmesh/abstract.go` →
   Rust module `abstract_graph.rs` (`abstract` is reserved).
5. **Crate API drift.** zstd 0.13's `decode_all` arity. Mechanical.
