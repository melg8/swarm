// Go vs Rust migration benchmark suite for the melg8/swarm transfer analysis.
// Mirrors the Go implementation in ../go-bench workload-for-workload.
// Subcommands: json | cpu | conc | spawn | http
use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

#[derive(Clone, Serialize, Deserialize)]
struct ToolCall {
    name: String,
    arguments: serde_json::Value,
}

#[derive(Clone, Serialize, Deserialize)]
struct AgentMessage {
    role: String,
    model: String,
    content: String,
    temperature: f64,
    tokens: i64,
    stop: Vec<String>,
    tool_calls: Vec<ToolCall>,
}

fn sample_message(i: usize) -> AgentMessage {
    AgentMessage {
        role: "assistant".into(),
        model: "l2j-c1-swarm".into(),
        content: format!(
            "engage target {} at ({}, {}, {}); loot; return to spot",
            i, i % 300, i % 250, -3000
        ),
        temperature: 0.7,
        tokens: 512 + (i % 64) as i64,
        stop: vec!["<|eot|>".into(), "STOP".into()],
        tool_calls: vec![
            ToolCall {
                name: "attack".into(),
                arguments: serde_json::json!({
                    "target_id": i % 10000,
                    "skill": "power_strike",
                    "force": i % 2 == 0
                }),
            },
            ToolCall {
                name: "move".into(),
                arguments: serde_json::json!({
                    "x": i % 300, "y": i % 250, "z": -3000
                }),
            },
        ],
    }
}

fn rss_hwm() -> u64 {
    if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
        for line in status.lines() {
            if let Some(rest) = line.strip_prefix("VmHWM:") {
                let kb: u64 = rest.split_whitespace().next().unwrap_or("0").parse().unwrap_or(0);
                return kb;
            }
        }
    }
    0
}

fn bench_json() {
    let msg = sample_message(42);
    let payload = serde_json::to_vec(&msg).unwrap();
    let mut best = 0.0f64;
    for _round in 0..5 {
        let mut ops: u64 = 0;
        let deadline = Instant::now() + Duration::from_secs(2);
        while Instant::now() < deadline {
            let b = serde_json::to_vec(&msg).unwrap();
            let _m: AgentMessage = serde_json::from_slice(&b).unwrap();
            ops += 1;
        }
        let rate = ops as f64 / 2.0;
        if rate > best {
            best = rate;
        }
    }
    println!(
        "json payload_bytes={} ops_per_sec={:.0} rss_hwm_kb={}",
        payload.len(),
        best,
        rss_hwm()
    );
}

fn fib(n: u64) -> u64 {
    if n < 2 {
        return n;
    }
    fib(n - 1) + fib(n - 2)
}

fn bench_cpu() {
    let buf: Vec<u8> = b"swarm".repeat(205); // 1025 bytes
    use sha2::{Digest, Sha256};
    let mut best = 0.0f64;
    for _round in 0..5 {
        let mut ops: u64 = 0;
        let deadline = Instant::now() + Duration::from_secs(2);
        while Instant::now() < deadline {
            let mut h = Sha256::new();
            h.update(&buf);
            let _ = h.finalize();
            ops += 1;
        }
        let rate = ops as f64 * buf.len() as f64 / 2.0 / 1e6;
        if rate > best {
            best = rate;
        }
    }
    println!("cpu sha256_1kb_mb_per_sec={:.1} rss_hwm_kb={}", best, rss_hwm());
}

async fn bench_conc(workers: usize) {
    let total: u64 = 200_000;
    let (tx, rx) = tokio::sync::mpsc::channel::<u64>(1024);
    let rx = Arc::new(tokio::sync::Mutex::new(rx));
    let done = Arc::new(AtomicU64::new(0));
    let mut handles = Vec::new();
    for _w in 0..workers {
        let rx = rx.clone();
        let done = done.clone();
        handles.push(tokio::spawn(async move {
            loop {
                let job = {
                    let mut guard = rx.lock().await;
                    guard.recv().await
                };
                match job {
                    Some(j) => {
                        let _ = fib(15 + j % 6);
                        done.fetch_add(1, Ordering::Relaxed);
                    }
                    None => break,
                }
            }
        }));
    }
    let start = Instant::now();
    for i in 0..total {
        if tx.send(i).await.is_err() {
            break;
        }
    }
    drop(tx);
    while done.load(Ordering::Relaxed) < total {
        tokio::time::sleep(Duration::from_millis(1)).await;
    }
    let elapsed = start.elapsed();
    for h in handles {
        let _ = h.await;
    }
    println!(
        "conc workers={} jobs={} jobs_per_sec={:.0} total_ms={} rss_hwm_kb={}",
        workers,
        total,
        total as f64 / elapsed.as_secs_f64(),
        elapsed.as_millis(),
        rss_hwm()
    );
}

async fn bench_spawn() {
    const N: usize = 200_000;
    let started = Arc::new(AtomicU64::new(0));
    let begin = Instant::now();
    for _i in 0..N {
        let started = started.clone();
        tokio::spawn(async move {
            started.fetch_add(1, Ordering::Relaxed);
            tokio::time::sleep(Duration::from_secs(3600)).await;
        });
    }
    while started.load(Ordering::Relaxed) < N as u64 {
        tokio::time::sleep(Duration::from_millis(1)).await;
    }
    let spawned = begin.elapsed();
    println!(
        "spawn tasks={} spawn_and_schedule_ms={} rss_hwm_kb={}",
        N,
        spawned.as_millis(),
        rss_hwm()
    );
    std::process::exit(0);
}

async fn bench_http() {
    async fn echo(body: axum::body::Bytes) -> axum::body::Bytes {
        body
    }
    let app = axum::Router::new().route("/echo", axum::routing::post(echo));
    let listener = tokio::net::TcpListener::bind("127.0.0.1:18082").await.unwrap();
    let server = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });
    tokio::time::sleep(Duration::from_millis(200)).await;

    let client = reqwest::Client::builder()
        .pool_max_idle_per_host(64)
        .build()
        .unwrap();
    let payload = serde_json::to_vec(&sample_message(7)).unwrap();
    let url = "http://127.0.0.1:18082/echo".to_string();
    let count = Arc::new(AtomicU64::new(0));
    let nanos = Arc::new(AtomicU64::new(0));
    let deadline = Instant::now() + Duration::from_secs(3);
    let mut handles = Vec::new();
    for _c in 0..64 {
        let client = client.clone();
        let payload = payload.clone();
        let url = url.clone();
        let count = count.clone();
        let nanos = nanos.clone();
        handles.push(tokio::spawn(async move {
            while Instant::now() < deadline {
                let start = Instant::now();
                match client
                    .post(&url)
                    .header("content-type", "application/json")
                    .body(payload.clone())
                    .send()
                    .await
                {
                    Ok(resp) => {
                        let _ = resp.bytes().await;
                        count.fetch_add(1, Ordering::Relaxed);
                        nanos.fetch_add(start.elapsed().as_nanos() as u64, Ordering::Relaxed);
                    }
                    Err(_) => break,
                }
            }
        }));
    }
    for h in handles {
        let _ = h.await;
    }
    server.abort();
    let c = count.load(Ordering::Relaxed);
    let ns = nanos.load(Ordering::Relaxed);
    let mean_ms = if c > 0 { ns as f64 / c as f64 / 1e6 } else { 0.0 };
    println!(
        "http clients=64 req_per_sec={:.0} mean_latency_ms={:.2} count={} rss_hwm_kb={}",
        c as f64 / 3.0,
        mean_ms,
        c,
        rss_hwm()
    );
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 2 {
        eprintln!("usage: rust-bench <json|cpu|conc|spawn|http>");
        std::process::exit(2);
    }
    let workers = std::thread::available_parallelism().map(|n| n.get()).unwrap_or(2);
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(workers)
        .enable_all()
        .build()
        .unwrap();
    match args[1].as_str() {
        "json" => bench_json(),
        "cpu" => bench_cpu(),
        "conc" => runtime.block_on(bench_conc(workers)),
        "spawn" => runtime.block_on(bench_spawn()),
        "http" => runtime.block_on(bench_http()),
        other => {
            eprintln!("unknown subcommand: {}", other);
            std::process::exit(2);
        }
    }
}
