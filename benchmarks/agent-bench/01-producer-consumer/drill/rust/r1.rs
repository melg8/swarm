//! Multi-threaded producer-consumer queue.
//!
//! Bounded queue built on Arc + Mutex + Condvar (std only):
//! - 2 producers send 25 items each
//! - 3 consumers receive until the queue is drained and closed
//! - main verifies that the total number of processed items matches

use std::collections::VecDeque;
use std::sync::{Arc, Condvar, Mutex};
use std::thread;
use std::time::{Duration, Instant};

const QUEUE_CAP: usize = 8;
const ITEMS_PER_PRODUCER: usize = 25;
const PRODUCERS: usize = 2;
const CONSUMERS: usize = 3;

/// Shared state protected by the Mutex.
struct QueueState {
    items: VecDeque<usize>,
    closed: bool,
}

/// Bounded blocking queue: producers block while full, consumers block while empty.
struct BoundedQueue {
    state: Mutex<QueueState>,
    not_full: Condvar,
    not_empty: Condvar,
    capacity: usize,
}

impl BoundedQueue {
    fn new(capacity: usize) -> Self {
        Self {
            state: Mutex::new(QueueState {
                items: VecDeque::with_capacity(capacity),
                closed: false,
            }),
            not_full: Condvar::new(),
            not_empty: Condvar::new(),
            capacity,
        }
    }

    /// Push an item, blocking while the queue is full.
    /// Returns Err(item) if the queue was closed before the item could be sent.
    fn send(&self, item: usize) -> Result<(), usize> {
        let mut st = self.state.lock().unwrap();
        while st.items.len() == self.capacity && !st.closed {
            st = self.not_full.wait(st).unwrap();
        }
        if st.closed {
            return Err(item);
        }
        st.items.push_back(item);
        self.not_empty.notify_one();
        Ok(())
    }

    /// Pop an item, blocking while the queue is empty.
    /// Returns None only when the queue is closed AND drained.
    fn recv(&self) -> Option<usize> {
        let mut st = self.state.lock().unwrap();
        while st.items.is_empty() && !st.closed {
            st = self.not_empty.wait(st).unwrap();
        }
        let item = st.items.pop_front();
        if item.is_some() {
            self.not_full.notify_one();
        }
        item
    }

    /// Close the queue and wake every blocked thread.
    fn close(&self) {
        let mut st = self.state.lock().unwrap();
        st.closed = true;
        self.not_full.notify_all();
        self.not_empty.notify_all();
    }
}

fn main() {
    let started = Instant::now();
    let queue = Arc::new(BoundedQueue::new(QUEUE_CAP));
    let mut handles: Vec<thread::JoinHandle<(usize, usize)>> = Vec::new();

    // Producers
    for p in 0..PRODUCERS {
        let q = Arc::clone(&queue); // FIXED
        handles.push(thread::spawn(move || {
            for i in 0..ITEMS_PER_PRODUCER {
                let item = p * 1_000 + i;
                if q.send(item).is_err() {
                    break; // queue closed underneath us
                }
                if i % 5 == 0 {
                    thread::sleep(Duration::from_millis(1));
                }
            }
            (p, 0)
        }));
    }

    // Consumers: count how many items each one processed
    for c in 0..CONSUMERS {
        let q = Arc::clone(&queue); // FIXED
        handles.push(thread::spawn(move || {
            let mut processed = 0usize;
            while q.recv().is_some() {
                processed += 1;
            }
            (c, processed)
        }));
    }

    // Wait for the producers (first PRODUCERS handles), then close the queue.
    let producer_handles: Vec<_> = handles.drain(..PRODUCERS).collect();
    for h in producer_handles {
        h.join().unwrap();
    }
    queue.close();

    // Collect consumer results.
    let mut total = 0usize;
    for h in handles {
        let (c, processed) = h.join().unwrap();
        println!("consumer {c}: processed {processed} items");
        total += processed;
    }

    let expected = PRODUCERS * ITEMS_PER_PRODUCER;
    let verdict = if total == expected { "OK" } else { "MISMATCH" };
    println!(
        "total processed: {total}/{expected} [{verdict}] in {:?}",
        started.elapsed()
    );
}
