//go:build drill1

// Each drill file is a standalone main of the same stage: the build
// tag keeps them out of the normal package build (and out of go build
// ./... and the lint typecheck, which one package of three mains
// breaks). Run one with:
//   go run -tags drill1 ./benchmarks/agent-bench/01-producer-consumer/drill/go

// Multi-threaded producer-consumer queue.
//
// Bounded queue built on sync.Mutex + sync.Cond (mirrors Arc<Mutex>+Condvar):
// - 2 producers send 25 items each
// - 3 consumers receive until the queue is drained and closed
// - main verifies that the total number of processed items matches
package main

import (
    "errors"
    "fmt"
    "sync"
    "time"
)

const (
    queueCap         = 8
    itemsPerProducer = 25
    producers        = 2
    consumers        = 3
)

// BoundedQueue is a bounded blocking queue: producers block while full,
// consumers block while empty.
type BoundedQueue struct {
    mu       sync.Mutex
    notFull  *sync.Cond
    notEmpty *sync.Cond
    items    []int
    capacity int
    closed   bool
}

// NewBoundedQueue creates a queue with the given capacity.
func NewBoundedQueue(capacity int) *BoundedQueue {
    q := &BoundedQueue{
        items:    make([]int, 0, capacity),
        capacity: capacity,
    }
    q.notFull = sync.NewCond(&q.mu)
    q.notEmpty = sync.NewCond(&q.mu)
    return q
}

// Send pushes an item, blocking while the queue is full.
// It returns an error if the queue was closed before the item could be sent.
func (q *BoundedQueue) Send(item int) error {
    q.mu.Lock()
    defer q.mu.Unlock()
    for len(q.items) == q.capacity && !q.closed {
        q.notFull.Wait()
    }
    if q.closed {
        return errors.New("queue closed")
    }
    q.items = append(q.items, item)
    q.notEmpty.Signal()
    return nil
}

// Recv pops an item, blocking while the queue is empty.
// The second return is false only when the queue is closed AND drained.
func (q *BoundedQueue) Recv() (int, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()
    for len(q.items) == 0 && !q.closed {
        q.notEmpty.Wait()
    }
    if len(q.items) == 0 {
        return 0, false
    }
    item := q.items[0]
    q.items = q.items[1:]
    q.notFull.Signal()
    return item, true
}

// Close shuts the queue down and wakes every blocked thread.
func (q *BoundedQueue) Close() {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.closed = true
    q.notFull.Broadcast()
    q.notEmpty.Broadcast()
}

func main() {
    q := NewBoundedQueue(queueCap)

    // Producers.
    var producerWG sync.WaitGroup
    for p := 0; p < producers; p++ {
        producerWG.Add(1)
        go func() {
            defer producerWG.Done()
            for i := 0; i < itemsPerProducer; i++ {
                item := p*1000 + i
                if err := q.Send(item); err != nil {
                    break // queue closed underneath us
                }
                if i%5 == 0 {
                    time.Sleep(time.Millisecond)
                }
            }
        }()
    }

    // Consumers: count how many items each one processed.
    consumerCounts := make([]int, consumers)
    var consumerWG sync.WaitGroup
    for c := 0; c < consumers; c++ {
        consumerWG.Add(1)
        go func(c int) {
            defer consumerWG.Done()
            for {
                if _, ok := q.Recv(); !ok {
                    break
                }
                consumerCounts[c]++
            }
        }(c)
    }

    // Wait for the producers, then close the queue and wait for consumers.
    producerWG.Wait()
    q.Close()
    consumerWG.Wait()

    total := 0
    for c, n := range consumerCounts {
        fmt.Printf("consumer %d: processed %d items\n", c, n)
        total += n
    }
    expected := producers * itemsPerProducer
    verdict := "OK"
    if total != expected {
        verdict = "MISMATCH"
    }
    fmt.Printf("total processed: %d/%d [%s]\n", total, expected, verdict)
}
