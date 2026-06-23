// Package component implements TTCN-3 component instances per Core
// Language clause 21. A Component is the unit of concurrency in
// TTCN-3: each test case runs on an MTC, which can create PTCs that
// execute behaviours concurrently and exchange messages over ports.
//
// The runtime models each component as a goroutine whose lifecycle
// state (Initial -> Running -> Done | Killed) is observable via
// channels so the alt scheduler can plug component branches
// (`done`/`killed` on a port set) into the same select primitive it
// uses for port and timer events.
package component

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Role distinguishes the special components from PTCs.
type Role int

const (
	PTC Role = iota
	MTC
	System
)

// String renders the role in TTCN-3 keyword form.
func (r Role) String() string {
	switch r {
	case MTC:
		return "mtc"
	case System:
		return "system"
	case PTC:
		return "ptc"
	}
	return "unknown"
}

// State is the lifecycle state of a component.
type State int

const (
	Initial State = iota
	Running
	Done
	Killed
)

func (s State) String() string {
	switch s {
	case Initial:
		return "initial"
	case Running:
		return "running"
	case Done:
		return "done"
	case Killed:
		return "killed"
	}
	return "unknown"
}

// Behaviour is the function a component runs. It receives the
// component itself (so it can spawn children, read parameters, ...) and
// returns when the testcase / function body completes.
type Behaviour func(*Component) error

// Component is the runtime handle on a TTCN-3 component instance. Every
// component carries its own unique numeric ID (assigned by the
// Registry), its role, its current state, and channels used by the alt
// scheduler to wait for completion.
type Component struct {
	ID   uint64
	Name string
	Role Role

	mu    sync.Mutex
	state State
	err   error
	done  chan struct{}
	kill  chan struct{}

	registry *Registry
}

// Registry tracks all components alive in a test execution. The runtime
// uses it to enumerate PTCs for `all component` operations and to
// allocate fresh IDs.
type Registry struct {
	mu    sync.Mutex
	next  uint64
	all   map[uint64]*Component
	mtc   *Component
	system *Component
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{all: map[uint64]*Component{}}
}

// New creates a fresh component in the Initial state and registers it.
// It does NOT start the behaviour - callers invoke Start once they are
// ready to dispatch.
func (r *Registry) New(name string, role Role) *Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := atomic.AddUint64(&r.next, 1)
	c := &Component{
		ID:       id,
		Name:     name,
		Role:     role,
		state:    Initial,
		done:     make(chan struct{}),
		kill:     make(chan struct{}, 1),
		registry: r,
	}
	r.all[id] = c
	switch role {
	case MTC:
		r.mtc = c
	case System:
		r.system = c
	}
	return c
}

// MTC returns the registered MTC, or nil if none was created.
func (r *Registry) MTC() *Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mtc
}

// System returns the registered system component, or nil if none.
func (r *Registry) System() *Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.system
}

// All returns a snapshot copy of every registered component.
func (r *Registry) All() []*Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Component, 0, len(r.all))
	for _, c := range r.all {
		out = append(out, c)
	}
	return out
}

// PTCs returns only the non-MTC / non-System components. The runtime
// uses this for `all component.done` semantics.
func (r *Registry) PTCs() []*Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Component, 0, len(r.all))
	for _, c := range r.all {
		if c.Role == PTC {
			out = append(out, c)
		}
	}
	return out
}

// Start launches the behaviour on its own goroutine and transitions the
// component to Running. Calling Start twice is a programming error and
// panics; the runtime ensures it never does.
func (c *Component) Start(b Behaviour) {
	c.mu.Lock()
	if c.state != Initial {
		c.mu.Unlock()
		panic(fmt.Sprintf("component %s: Start called in state %s", c.Name, c.state))
	}
	c.state = Running
	c.mu.Unlock()

	go func() {
		var err error
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(error); ok {
					err = e
				} else {
					err = fmt.Errorf("%v", r)
				}
			}
			c.finish(err)
		}()

		// Bail out early if Kill was called between Start returning
		// and this goroutine actually running.
		select {
		case <-c.kill:
			err = ErrKilled
			return
		default:
		}

		err = b(c)
		// If a kill arrived during the behaviour, prefer reporting it.
		select {
		case <-c.kill:
			if err == nil {
				err = ErrKilled
			}
		default:
		}
	}()
}

// ErrKilled is the sentinel error reported by a component whose
// behaviour was interrupted by Kill().
var ErrKilled = fmt.Errorf("component killed")

// Kill marks the component for termination. The Behaviour is expected
// to check Kill() (or the Killed channel) at convenient points; the
// runtime does NOT preemptively interrupt goroutines.
func (c *Component) Kill() {
	select {
	case c.kill <- struct{}{}:
	default:
	}
}

// Killed returns a channel that closes when Kill has been called. Long-
// running behaviours integrate it into their own select / alt loops.
func (c *Component) Killed() <-chan struct{} { return c.kill }

// Done returns a channel that closes when the behaviour terminates,
// regardless of whether it returned naturally, errored, or was killed.
func (c *Component) Done() <-chan struct{} { return c.done }

// State returns the current lifecycle state.
func (c *Component) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Err returns the behaviour's terminating error (if any). Returns nil
// while the component is still Running, and after a clean completion.
func (c *Component) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Alive reports whether the component is still in Initial or Running.
// Matches the TTCN-3 `alive` operation.
func (c *Component) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == Initial || c.state == Running
}

func (c *Component) finish(err error) {
	c.mu.Lock()
	c.err = err
	if err == ErrKilled {
		c.state = Killed
	} else {
		c.state = Done
	}
	close(c.done)
	c.mu.Unlock()
}
