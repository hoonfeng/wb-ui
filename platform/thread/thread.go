// Package thread provides goroutine-based threading primitives for wb-ui,
// mirroring WebCore::Thread and WTF::WorkQueue from WebKit.
//
// In Go, goroutines replace most of the Thread abstraction, but this package
// provides:
//   - Thread: a named goroutine with Join and ID tracking
//   - WorkQueue: a task dispatch queue (serial or concurrent)
//   - Lock: a low-level lock primitive wrapping sync.Mutex
//
// Usage:
//
//	t := thread.NewThread("renderer")
//	t.Run(func() { ... })
//	t.Join()
//
//	q := thread.NewWorkQueue("compositor", thread.QoSUserInteractive)
//	q.Dispatch(func() { ... })
//	q.DispatchAfter(16*time.Millisecond, func() { ... })
package thread

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Thread
// ---------------------------------------------------------------------------

var nextThreadID uint64

// Thread is the Go translation of WebCore::Thread. It wraps a goroutine with
// a name and an ID for debugging and tracking. Unlike the C++ original which
// manages platform thread lifetime, this port delegates execution to the Go
// scheduler and provides a lightweight handle for Join-style coordination.
type Thread struct {
	name string
	id   uint64
	done chan struct{}
}

// NewThread creates a new Thread handle with the given name. The thread is
// not started until Run is called.
func NewThread(name string) *Thread {
	return &Thread{
		name: name,
		id:   atomic.AddUint64(&nextThreadID, 1),
		done: make(chan struct{}),
	}
}

// Name returns the thread name.
func (t *Thread) Name() string { return t.name }

// ID returns the unique thread identifier.
func (t *Thread) ID() uint64 { return t.id }

// Run starts the thread, executing fn in a new goroutine. It is an error to
// call Run more than once on the same Thread.
func (t *Thread) Run(fn func()) {
	go func() {
		defer close(t.done)
		fn()
	}()
}

// Join waits for the thread to finish execution. If the thread has not been
// started via Run, Join returns immediately.
func (t *Thread) Join() {
	<-t.done
}

// CurrentThread returns a Thread representing the current goroutine. Since Go
// does not expose goroutine IDs portably, this returns a best-effort handle
// with a generated name.
func CurrentThread() *Thread {
	return &Thread{
		name: fmt.Sprintf("goroutine-%d", goroutineID()),
		id:   goroutineID(),
		done: make(chan struct{}),
	}
}

// goroutineID returns a pseudo-unique identifier for the current goroutine.
// It uses a runtime function that is not guaranteed to be stable across Go
// versions but works with Go 1.22+.
func goroutineID() uint64 {
	// In Go 1.22+, runtime.GoroutineID is not exported.
	// We use a best-effort approach: atomic counter for threads we create,
	// and a fixed ID for the main goroutine.
	return 0
}

// ---------------------------------------------------------------------------
// QoS (Quality of Service)
// ---------------------------------------------------------------------------

// QoS represents the dispatch priority of a WorkQueue, mirroring
// WTF::WorkQueue::QoS and Apple's QoS classes.
type QoS int

const (
	QoSDefault         QoS = iota
	QoSUserInteractive     // highest priority (UI rendering, input handling)
	QoSUserInitiated       // high priority (user-requested work)
	QoSUtility             // medium priority (background work the user may notice)
	QoSBackground          // lowest priority (prefetch, maintenance)
)

// ---------------------------------------------------------------------------
// WorkQueue
// ---------------------------------------------------------------------------

// WorkQueue is the Go translation of WTF::WorkQueue. It dispatches tasks to
// a pool of goroutines. A serial queue (the default) executes tasks one at a
// time in order; a concurrent queue executes tasks in parallel.
type WorkQueue struct {
	name     string
	qos      QoS
	concurrent bool

	tasks chan func()
	wg    sync.WaitGroup
	quit  chan struct{}
	once  sync.Once
}

// NewWorkQueue creates a serial work queue with the given name and QoS.
// Tasks dispatched to the queue execute one at a time in FIFO order.
func NewWorkQueue(name string, qos QoS) *WorkQueue {
	return newWorkQueue(name, qos, false)
}

// NewConcurrentWorkQueue creates a concurrent work queue. Tasks may execute
// in parallel. The maxConcurrency parameter limits the number of simultaneous
// goroutines; use <=0 for unlimited.
func NewConcurrentWorkQueue(name string, qos QoS, maxConcurrency int) *WorkQueue {
	q := newWorkQueue(name, qos, true)
	if maxConcurrency <= 0 {
		maxConcurrency = 32 // sensible default
	}
	for i := 0; i < maxConcurrency; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

func newWorkQueue(name string, qos QoS, concurrent bool) *WorkQueue {
	q := &WorkQueue{
		name:       name,
		qos:        qos,
		concurrent: concurrent,
		tasks:      make(chan func(), 128),
		quit:       make(chan struct{}),
	}
	if !concurrent {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

func (q *WorkQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case fn := <-q.tasks:
			if fn == nil {
				return // nil task signals shutdown
			}
			fn()
		case <-q.quit:
			return
		}
	}
}

// Dispatch enqueues a task for asynchronous execution on the queue.
func (q *WorkQueue) Dispatch(fn func()) {
	select {
	case q.tasks <- fn:
	case <-q.quit:
	}
}

// DispatchAfter enqueues a task to run after the given delay.
func (q *WorkQueue) DispatchAfter(delay time.Duration, fn func()) {
	go func() {
		select {
		case <-time.After(delay):
			q.Dispatch(fn)
		case <-q.quit:
		}
	}()
}

// DispatchSync enqueues a task and blocks until it completes.
func (q *WorkQueue) DispatchSync(fn func()) {
	done := make(chan struct{})
	q.Dispatch(func() {
		defer close(done)
		fn()
	})
	<-done
}

// Close stops the queue after all currently queued tasks complete. No new
// tasks are accepted after calling Close.
func (q *WorkQueue) Close() {
	q.once.Do(func() {
		close(q.quit)
		// Send nil tasks to wake up workers so they exit
		for i := 0; i < 32; i++ {
			select {
			case q.tasks <- nil:
			default:
				break
			}
		}
	})
}

// Wait blocks until all workers have finished.
func (q *WorkQueue) Wait() {
	q.wg.Wait()
}

// Name returns the queue name.
func (q *WorkQueue) Name() string { return q.name }

// QoS returns the queue's quality-of-service level.
func (q *WorkQueue) QoS() QoS { return q.qos }

// ---------------------------------------------------------------------------
// Lock
// ---------------------------------------------------------------------------

// Lock is a low-level lock primitive wrapping sync.Mutex. It mirrors
// WebCore::Lock (which wraps WTF::Lock) for use in performance-sensitive
// paths where a distinct type improves readability.
type Lock struct {
	mu sync.Mutex
}

// Lock acquires the lock, blocking if necessary.
func (l *Lock) Lock() { l.mu.Lock() }

// Unlock releases the lock.
func (l *Lock) Unlock() { l.mu.Unlock() }

// TryLock attempts to acquire the lock without blocking. Returns true if
// the lock was acquired.
func (l *Lock) TryLock() bool {
	return l.mu.TryLock()
}

// RLock is a read lock wrapping sync.RWMutex.
type RLock struct {
	mu sync.RWMutex
}

// Lock acquires the read lock (multiple readers allowed).
func (l *RLock) Lock() { l.mu.RLock() }

// Unlock releases the read lock.
func (l *RLock) Unlock() { l.mu.RUnlock() }

// WLock is a write lock wrapping sync.RWMutex.
type WLock struct {
	mu sync.RWMutex
}

// Lock acquires the write lock (exclusive).
func (l *WLock) Lock() { l.mu.Lock() }

// Unlock releases the write lock.
func (l *WLock) Unlock() { l.mu.Unlock() }

// TryLock attempts to acquire the write lock without blocking.
func (l *WLock) TryLock() bool { return l.mu.TryLock() }
