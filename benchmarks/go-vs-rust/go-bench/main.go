// Go vs Rust migration benchmark suite for the melg8/swarm transfer analysis.
// Mirrors the Rust implementation in ../rust-bench workload-for-workload.
// Subcommands: json | cpu | conc | spawn | http
package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "sync/atomic"
    "time"
)

// AgentMessage mimics one LLM/agent orchestration message of the swarm domain.
type AgentMessage struct {
    Role        string     `json:"role"`
    Model       string     `json:"model"`
    Content     string     `json:"content"`
    Temperature float64    `json:"temperature"`
    Tokens      int        `json:"tokens"`
    Stop        []string   `json:"stop"`
    ToolCalls   []ToolCall `json:"tool_calls"`
}

type ToolCall struct {
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

func sampleMessage(i int) AgentMessage {
    return AgentMessage{
        Role:        "assistant",
        Model:       "l2j-c1-swarm",
        Content:     fmt.Sprintf("engage target %d at (%d, %d, %d); loot; return to spot", i, i%300, i%250, -3000),
        Temperature: 0.7,
        Tokens:      512 + i%64,
        Stop:        []string{"<|eot|>", "STOP"},
        ToolCalls: []ToolCall{
            {Name: "attack", Arguments: map[string]interface{}{
                "target_id": i % 10000, "skill": "power_strike", "force": i%2 == 0}},
            {Name: "move", Arguments: map[string]interface{}{
                "x": i % 300, "y": i % 250, "z": -3000}},
        },
    }
}

func rssHWM() int {
    data, err := os.ReadFile("/proc/self/status")
    if err != nil {
        return 0
    }
    for _, line := range strings.Split(string(data), "\n") {
        if strings.HasPrefix(line, "VmHWM:") {
            fields := strings.Fields(line)
            v, _ := strconv.Atoi(fields[1])
            return v
        }
    }
    return 0
}

func benchJSON() {
    msg := sampleMessage(42)
    payload, _ := json.Marshal(msg)
    best := 0.0
    for round := 0; round < 5; round++ {
        var ops int64
        deadline := time.Now().Add(2 * time.Second)
        for time.Now().Before(deadline) {
            b, err := json.Marshal(msg)
            if err != nil {
                panic(err)
            }
            var m AgentMessage
            if err := json.Unmarshal(b, &m); err != nil {
                panic(err)
            }
            ops++
        }
        rate := float64(ops) / 2.0
        if rate > best {
            best = rate
        }
    }
    fmt.Printf("json payload_bytes=%d ops_per_sec=%.0f rss_hwm_kb=%d\n", len(payload), best, rssHWM())
}

func fib(n int) int {
    if n < 2 {
        return n
    }
    return fib(n-1) + fib(n-2)
}

func benchConc() {
    workers := runtime.NumCPU()
    jobs := make(chan int, 1024)
    var done int64
    var wg sync.WaitGroup
    for w := 0; w < workers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := range jobs {
                _ = fib(15 + j%6)
                atomic.AddInt64(&done, 1)
            }
        }()
    }
    const total = 200000
    start := time.Now()
    for i := 0; i < total; i++ {
        jobs <- i
    }
    close(jobs)
    wg.Wait()
    elapsed := time.Since(start)
    fmt.Printf("conc workers=%d jobs=%d jobs_per_sec=%.0f total_ms=%d rss_hwm_kb=%d\n",
        workers, total, float64(total)/elapsed.Seconds(), elapsed.Milliseconds(), rssHWM())
}

func benchSpawn() {
    const n = 200000
    var started int64
    quit := make(chan struct{})
    start := time.Now()
    for i := 0; i < n; i++ {
        go func() {
            atomic.AddInt64(&started, 1)
            <-quit
        }()
    }
    for atomic.LoadInt64(&started) < n {
        time.Sleep(time.Millisecond)
    }
    spawned := time.Since(start)
    close(quit)
    fmt.Printf("spawn goroutines=%d spawn_and_schedule_ms=%d rss_hwm_kb=%d\n",
        n, spawned.Milliseconds(), rssHWM())
}

func benchCPU() {
    buf := bytes.Repeat([]byte("swarm"), 205) // 1025 bytes
    best := 0.0
    for round := 0; round < 5; round++ {
        var ops int64
        deadline := time.Now().Add(2 * time.Second)
        for time.Now().Before(deadline) {
            h := sha256.New()
            h.Write(buf)
            _ = h.Sum(nil)
            ops++
        }
        rate := float64(ops) * float64(len(buf)) / 2.0 / 1e6
        if rate > best {
            best = rate
        }
    }
    fmt.Printf("cpu sha256_1kb_mb_per_sec=%.1f rss_hwm_kb=%d\n", best, rssHWM())
}

func benchHTTP() {
    mux := http.NewServeMux()
    mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/json")
        w.Write(body)
    })
    srv := &http.Server{Addr: "127.0.0.1:18081", Handler: mux}
    go srv.ListenAndServe()
    time.Sleep(200 * time.Millisecond)

    payload, _ := json.Marshal(sampleMessage(7))
    url := "http://127.0.0.1:18081/echo"
    var count, nanos int64
    var wg sync.WaitGroup
    clients := 64
    client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: clients}}
    deadline := time.Now().Add(3 * time.Second)
    for c := 0; c < clients; c++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for time.Now().Before(deadline) {
                start := time.Now()
                resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
                if err != nil {
                    return
                }
                io.Copy(io.Discard, resp.Body)
                resp.Body.Close()
                atomic.AddInt64(&count, 1)
                atomic.AddInt64(&nanos, int64(time.Since(start)))
            }
        }()
    }
    time.Sleep(3500 * time.Millisecond)
    srv.Close()
    wg.Wait()
    c := atomic.LoadInt64(&count)
    ns := atomic.LoadInt64(&nanos)
    var mean time.Duration
    if c > 0 {
        mean = time.Duration(ns / c)
    }
    fmt.Printf("http clients=%d req_per_sec=%.0f mean_latency_ms=%.2f count=%d rss_hwm_kb=%d\n",
        clients, float64(c)/3.0, float64(mean.Nanoseconds())/1e6, c, rssHWM())
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "usage: go-bench <json|cpu|conc|spawn|http>")
        os.Exit(2)
    }
    switch os.Args[1] {
    case "json":
        benchJSON()
    case "cpu":
        benchCPU()
    case "conc":
        benchConc()
    case "spawn":
        benchSpawn()
    case "http":
        benchHTTP()
    default:
        fmt.Fprintln(os.Stderr, "unknown subcommand")
        os.Exit(2)
    }
}
