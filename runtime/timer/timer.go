// Package timer implements TTCN-3 timer values per Core Language clause
// 22. A Timer is an opaque object the test author can start, stop, read,
// or wait for; the runtime listens for Timeouts via the channel returned
// by Timeout().
//
// The package is intentionally tiny: timers are state machines (Idle ->
// Running -> Expired) with a single goroutine per running timer. The
// scheduler (`runtime/alt`) treats Timeout() as just another channel,
// so timer expiry plugs into snapshot semantics without special-casing.
package timer

import (
	"fmt"
	"sync"
	"time"
)

// State is the lifecycle of a single timer.
type State int

const (
	Idle State = iota
	Running
	Expired
)

// Timer is the runtime representation of a TTCN-3 timer.
type Timer struct {
	Name string

	mu        sync.Mutex
	state     State
	duration  time.Duration
	startedAt time.Time
	timeout   chan struct{}
	stop      chan struct{}
}

// New returns a fresh Timer in the Idle state. Name is used for
// diagnostics only; the runtime tracks timer identity by pointer.
func New(name string) *Timer {
	return &Timer{
		Name:    name,
		timeout: make(chan struct{}, 1),
		stop:    make(chan struct{}, 1),
	}
}

// Start (re)starts the timer with the given duration. Calling Start on
// a Running timer first stops it. A duration <= 0 fires immediately.
func (t *Timer) Start(d time.Duration) {
	t.mu.Lock()
	if t.state == Running {
		t.stopLocked()
	}
	t.duration = d
	t.startedAt = time.Now()
	t.state = Running
	// Drain stale signals so a fresh start always starts from zero.
	select {
	case <-t.timeout:
	default:
	}
	select {
	case <-t.stop:
	default:
	}
	t.mu.Unlock()

	// We spawn a goroutine per running timer. This is dirt simple and
	// works fine for the typical "tens of timers in flight" load. Hot
	// runtimes can later swap this out for a min-heap scheduler.
	go func() {
		select {
		case <-time.After(d):
			t.mu.Lock()
			if t.state == Running {
				t.state = Expired
				select {
				case t.timeout <- struct{}{}:
				default:
				}
			}
			t.mu.Unlock()
		case <-t.stop:
			return
		}
	}()
}

// Stop returns the timer to the Idle state. Stopping an Idle or Expired
// timer is a no-op (matches Titan / Annex behaviour).
func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopLocked()
}

func (t *Timer) stopLocked() {
	if t.state == Running {
		select {
		case t.stop <- struct{}{}:
		default:
		}
	}
	t.state = Idle
	// Clear any timeout signal that landed between fire and Stop.
	select {
	case <-t.timeout:
	default:
	}
}

// Read returns the elapsed time since Start in seconds. For a stopped
// timer Read returns the duration accumulated while it was running.
// TTCN-3 expects a float "in seconds", so the caller is free to
// reinterpret the duration as a float value.
func (t *Timer) Read() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Running {
		return time.Since(t.startedAt)
	}
	return t.duration
}

// Running reports whether the timer is currently counting down. Expired
// timers do NOT count as running (per TTCN-3 semantics: timeout
// transitions the timer back to Idle from the user's perspective once
// the alt branch fires).
func (t *Timer) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state == Running
}

// State returns the current lifecycle state.
func (t *Timer) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Timeout returns a receive-only channel that signals when the timer
// expires. The channel is buffered with capacity 1 so a missed receive
// doesn't deadlock the runtime; consuming the signal (via Drain or a
// successful receive) is what officially acknowledges the timeout.
func (t *Timer) Timeout() <-chan struct{} { return t.timeout }

// Drain consumes any pending timeout signal and transitions an Expired
// timer back to Idle. The boolean return reports whether the call did
// any work (signal consumed OR state moved out of Expired). The alt
// scheduler calls Drain after the user branch fires to make subsequent
// `running` checks return false.
func (t *Timer) Drain() bool {
	consumed := false
	select {
	case <-t.timeout:
		consumed = true
	default:
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Expired {
		t.state = Idle
		return true
	}
	return consumed
}

// Reassert re-emits a timeout signal for an Expired timer, which the
// alt scheduler uses after consuming the channel during a select wake-
// up that did not actually fire the timer's branch. Returns true if
// the timer was Expired (and so the reassertion happened).
func (t *Timer) Reassert() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != Expired {
		return false
	}
	select {
	case t.timeout <- struct{}{}:
	default:
	}
	return true
}

// String renders the timer for diagnostics, e.g. "T_guard (Running, 0.42s)".
func (t *Timer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fmt.Sprintf("%s (%s, %s)", t.Name, stateName(t.state), t.duration)
}

func stateName(s State) string {
	switch s {
	case Idle:
		return "Idle"
	case Running:
		return "Running"
	case Expired:
		return "Expired"
	default:
		return "Unknown"
	}
}
