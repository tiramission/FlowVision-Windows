package main

import "sync"

// thumbTask is one thumbnail load waiting for a worker. priority is the
// distance from the viewport center in DIP; lower values run first.
type thumbTask struct {
	key      string
	item     Item
	size     int
	priority float64
	frame    uint64
	seq      uint64
}

// thumbQueue is a small scheduling layer for thumbnail work. The grid updates
// each visible task every frame; tasks that were not visible in the latest
// frame are dropped before they start. Only a few dozen items are pending, so
// a map plus a linear best-task scan is cheaper and simpler than a heap.
type thumbQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending map[string]*thumbTask
	running map[string]bool
	seq     uint64
}

func newThumbQueue() *thumbQueue {
	q := &thumbQueue{
		pending: map[string]*thumbTask{},
		running: map[string]bool{},
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// submit adds a task or refreshes the priority and frame of one already
// waiting. A task already running is left alone.
func (q *thumbQueue) submit(t *thumbTask) {
	q.mu.Lock()
	if old, ok := q.pending[t.key]; ok {
		old.priority = t.priority
		old.frame = t.frame
		q.cond.Broadcast()
		q.mu.Unlock()
		return
	}
	if q.running[t.key] {
		q.mu.Unlock()
		return
	}
	q.seq++
	t.seq = q.seq
	q.pending[t.key] = t
	q.cond.Broadcast()
	q.mu.Unlock()
}

// touch updates a waiting task without creating a new one. It reports whether
// the task was still waiting.
func (q *thumbQueue) touch(key string, priority float64, frame uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.pending[key]
	if !ok {
		return false
	}
	t.priority = priority
	t.frame = frame
	return true
}

// retain drops waiting tasks that the latest grid frame did not touch. A
// running task cannot be interrupted, but it keeps its cache result.
func (q *thumbQueue) retain(frame uint64) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var dropped []string
	for key, t := range q.pending {
		if t.frame < frame {
			delete(q.pending, key)
			dropped = append(dropped, key)
		}
	}
	return dropped
}

// next waits for the closest waiting task. Workers block here instead of
// starting one goroutine per thumbnail.
func (q *thumbQueue) next() *thumbTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if best := q.popLocked(); best != nil {
			return best
		}
		q.cond.Wait()
	}
}

// popNow takes the closest waiting task, or returns nil. Tests use it too.
func (q *thumbQueue) popNow() *thumbTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.popLocked()
}

func (q *thumbQueue) popLocked() *thumbTask {
	var best *thumbTask
	for _, t := range q.pending {
		if best == nil || t.priority < best.priority || (t.priority == best.priority && t.seq < best.seq) {
			best = t
		}
	}
	if best == nil {
		return nil
	}
	delete(q.pending, best.key)
	q.running[best.key] = true
	return best
}

func (q *thumbQueue) done(key string) {
	q.mu.Lock()
	delete(q.running, key)
	q.mu.Unlock()
}

func (q *thumbQueue) stats() (pending, running int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending), len(q.running)
}

func (q *thumbQueue) reset() {
	q.mu.Lock()
	q.pending = map[string]*thumbTask{}
	q.running = map[string]bool{}
	q.cond.Broadcast()
	q.mu.Unlock()
}
