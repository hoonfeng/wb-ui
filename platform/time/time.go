// Package wbtime provides time-related primitives for wb-ui, mirroring
// WebCore::Timer and WTF::MonotonicTime / WallTime from WebKit.
//
// It wraps Go's standard time package with a timer abstraction that matches
// the WebCore::Timer interface (single-shot / repeating, start / stop /
// isActive) and monotonic / wall clock types for animation and frame
// scheduling.
//
// Usage:
//
//	t := wbtime.NewTimer(func() { fmt.Println("fired") })
//	t.Start(16*time.Millisecond, true) // 60fps repeating
//	defer t.Stop()
//
//	start := wbtime.Now()
//	elapsed := start.Elapsed()
package wbtime

import (
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// MonotonicTime
// ---------------------------------------------------------------------------

// MonotonicTime wraps a monotonic clock reading, suitable for measuring
// elapsed time, animation timing, and frame scheduling. Unlike WallTime,
// MonotonicTime is not affected by system time adjustments (NTP, DST).
type MonotonicTime struct {
	wall time.Time // Go's Time carries both wall and monotonic components
}

// Now returns the current monotonic time.
func Now() MonotonicTime {
	return MonotonicTime{wall: time.Now()}
}

// IsZero reports whether t is the zero value.
func (t MonotonicTime) IsZero() bool { return t.wall.IsZero() }

// Sub returns the duration from t to u (t - u). Use this to measure elapsed
// time: d := Now().Sub(start).
func (t MonotonicTime) Sub(u MonotonicTime) time.Duration {
	return t.wall.Sub(u.wall)
}

// Add returns t + d.
func (t MonotonicTime) Add(d time.Duration) MonotonicTime {
	return MonotonicTime{wall: t.wall.Add(d)}
}

// Elapsed returns the duration since t.
func (t MonotonicTime) Elapsed() time.Duration {
	return time.Since(t.wall)
}

// After reports whether t is after u.
func (t MonotonicTime) After(u MonotonicTime) bool {
	return t.wall.After(u.wall)
}

// Before reports whether t is before u.
func (t MonotonicTime) Before(u MonotonicTime) bool {
	return t.wall.Before(u.wall)
}

// Seconds returns t as a Unix-like timestamp (seconds since epoch).
// For monotonic measurements prefer Elapsed or Sub.
func (t MonotonicTime) Seconds() float64 {
	return float64(t.wall.UnixNano()) / 1e9
}

// ---------------------------------------------------------------------------
// WallTime
// ---------------------------------------------------------------------------

// WallTime wraps a wall-clock time, suitable for timestamps displayed to
// users or stored persistently. It corresponds to WebKit's WallTime.
type WallTime struct {
	t time.Time
}

// NowWall returns the current wall-clock time.
func NowWall() WallTime {
	return WallTime{t: time.Now()}
}

// Time returns the underlying Go time.Time.
func (wt WallTime) Time() time.Time { return wt.t }

// Sub returns wt - u as a duration.
func (wt WallTime) Sub(u WallTime) time.Duration { return wt.t.Sub(u.t) }

// Add returns wt + d.
func (wt WallTime) Add(d time.Duration) WallTime { return WallTime{t: wt.t.Add(d)} }

// After reports whether wt is after u.
func (wt WallTime) After(u WallTime) bool { return wt.t.After(u.t) }

// Before reports whether wt is before u.
func (wt WallTime) Before(u WallTime) bool { return wt.t.Before(u.t) }

// Unix returns the Unix timestamp (seconds since 1970-01-01).
func (wt WallTime) Unix() int64 { return wt.t.Unix() }

// Format formats the wall time according to the given layout (same as
// time.Time.Format).
func (wt WallTime) Format(layout string) string { return wt.t.Format(layout) }

// ---------------------------------------------------------------------------
// Timer
// ---------------------------------------------------------------------------

// Timer is the Go translation of WebCore::Timer. It fires a callback after
// a specified interval, either once or repeatedly. Unlike Go's
// time.Timer, WebCore::Timer has explicit start/stop/isActive semantics
// and supports single-shot and repeating modes.
type Timer struct {
	mu       sync.Mutex
	callback func()
	interval time.Duration
	repeat   bool
	active   bool
	timer    *time.Timer
	stopCh   chan struct{}
}

// NewTimer creates a new Timer with the given callback. The timer is
// initially inactive; call Start to begin.
func NewTimer(callback func()) *Timer {
	return &Timer{
		callback: callback,
		stopCh:   make(chan struct{}),
	}
}

// Start activates the timer. If repeat is true, the timer fires every
// interval; otherwise it fires once. If the timer is already active,
// it is restarted with the new parameters.
func (t *Timer) Start(interval time.Duration, repeat bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.active {
		t.stop()
	}
	t.interval = interval
	t.repeat = repeat
	t.active = true

	if repeat {
		t.timer = time.NewTimer(interval)
		go t.repeatLoop()
	} else {
		t.timer = time.NewTimer(interval)
		go t.fireOnce()
	}
}

// Stop deactivates the timer. It is safe to call on an inactive timer.
func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active {
		t.stop()
	}
}

// stopLocked stops the timer without acquiring the lock (caller must hold mu).
func (t *Timer) stop() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.active = false
	// Signal the fire goroutine to exit
	select {
	case t.stopCh <- struct{}{}:
	default:
	}
}

// IsActive reports whether the timer is currently active.
func (t *Timer) IsActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// Interval returns the timer's interval.
func (t *Timer) Interval() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.interval
}

// IsRepeating reports whether the timer is repeating.
func (t *Timer) IsRepeating() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.repeat
}

// Fire immediately invokes the timer callback (regardless of whether the
// timer is active). This is a testing aid.
func (t *Timer) Fire() {
	t.callback()
}

func (t *Timer) fireOnce() {
	defer t.stopIfActive()

	t.mu.Lock()
	timer := t.timer
	t.mu.Unlock()

	if timer == nil {
		return
	}

	select {
	case <-timer.C:
		t.callback()
	case <-t.stopCh:
	}
}

func (t *Timer) repeatLoop() {
	for {
		t.mu.Lock()
		timer := t.timer
		t.mu.Unlock()

		if timer == nil {
			return
		}

		select {
		case <-timer.C:
			t.callback()
			// Reset for next fire
			t.mu.Lock()
			if t.active && t.repeat {
				t.timer.Reset(t.interval)
			}
			t.mu.Unlock()
		case <-t.stopCh:
			return
		}
	}
}

func (t *Timer) stopIfActive() {
	t.mu.Lock()
	defer t.mu.Unlock()
	// If the timer fired naturally (not stopped), deactivate for single-shot
	if !t.repeat {
		t.active = false
	}
}

// OneShot is a convenience for creating and starting a single-shot timer.
func OneShot(callback func(), delay time.Duration) *Timer {
	t := NewTimer(callback)
	t.Start(delay, false)
	return t
}

// Every is a convenience for creating and starting a repeating timer.
func Every(callback func(), interval time.Duration) *Timer {
	t := NewTimer(callback)
	t.Start(interval, true)
	return t
}

// Sleep is a cancellable sleep. Returns true if the full duration elapsed,
// false if cancelled via the channel.
func Sleep(d time.Duration, cancel <-chan struct{}) bool {
	select {
	case <-time.After(d):
		return true
	case <-cancel:
		return false
	}
}
