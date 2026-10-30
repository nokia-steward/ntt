// Package alt implements TTCN-3 alt-statement snapshot semantics per
// Core Language clause 20. An alt block selects the first matching
// branch from a set of guards over ports, timers and components.
//
// The scheduler models an alt as a list of Branches, each with an
// optional boolean Guard and an Event source. On each Run iteration
// the scheduler takes a snapshot of every event source (poll without
// blocking), picks the first branch whose guard is true and whose
// event has a ready value, and dispatches its Body. If no branch is
// ready, the scheduler blocks on a select over all event channels
// simultaneously until one fires, then re-snapshots and dispatches.
//
// `interleave` and `default` semantics are layered on top of this same
// primitive: see `runtime/alt/interleave.go` for the interleave
// rewriting helper and `Default` for default-altstep registration.
package alt

import (
	"reflect"

	"github.com/nokia/ntt/runtime/component"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/timer"
)

// Branch is one alternative inside an alt block.
type Branch struct {
	// Guard is the optional `[expr]` boolean; nil means "true".
	Guard func() bool

	// Source decides what kind of event the branch waits for. Exactly
	// one of the source fields must be set.
	Port      *port.Port
	Timer     *timer.Timer
	Component *component.Component

	// PortFilter, when non-nil, gates a port branch: the branch only
	// fires when filter returns true for the popped envelope. The
	// default (nil) accepts every envelope, which matches a bare
	// `receive` operation.
	PortFilter func(*port.Envelope) bool

	// ComponentEvent selects which lifecycle transition we wait for.
	ComponentEvent ComponentEvent

	// Body is invoked when the branch fires. The argument is one of:
	//   - *port.Envelope for port branches
	//   - nil           for timer branches
	//   - *component.Component for component branches
	Body func(interface{})
}

// ComponentEvent enumerates the component lifecycle transitions an alt
// branch can wait for.
type ComponentEvent int

const (
	OnDone ComponentEvent = iota
	OnKilled
	OnAny
)

// Verdict reports how the scheduler returned: which branch fired (or -1
// if Run returned because of a context-style stop), and the consumed
// value (envelope for ports, nil for timers, component pointer for
// component branches).
type Verdict struct {
	BranchIndex int
	Consumed    interface{}
}

// Run scans branches in order until one fires, then returns. The
// scheduler does NOT loop: TTCN-3 `alt` exits after one branch fires
// unless the user re-enters it (typical pattern: `alt { ... repeat; }`).
//
// A stop channel can be passed to abort the wait: if stop is non-nil
// and the channel signals before any branch fires, Run returns with
// BranchIndex == -1.
//
// Implementation note: ports / timers / components are all backed by
// Go channels. The naïve "select over the union" pattern would CONSUME
// the event we were waiting for, which then disappears before the next
// snapshot pass. We work around this by, after select wakes us,
// requeuing the consumed value (Envelope back into the port inbox,
// timeout signal re-armed on the timer); closed component channels
// stay closed so no requeuing is needed.
func Run(branches []Branch, stop <-chan struct{}) Verdict {
	if v, ok := snapshot(branches); ok {
		return v
	}
	for {
		cases, kinds := buildSelectCases(branches, stop)
		if len(cases) == 0 {
			return Verdict{BranchIndex: -1}
		}
		chosen, val, _ := reflect.Select(cases)
		if stop != nil && chosen == len(cases)-1 {
			return Verdict{BranchIndex: -1}
		}
		k := kinds[chosen]
		requeue(branches[k.branchIdx], k.sourceKind, val)
		if v, ok := snapshot(branches); ok {
			return v
		}
	}
}

// snapshot is the non-blocking pass: try every branch in order, pick
// the first one whose guard is true and whose source has a value
// ready. Port branches use Peek so a non-matching head stays in the
// queue (TTCN-3 receive-and-leave semantics).
func snapshot(branches []Branch) (Verdict, bool) {
	for i, br := range branches {
		if br.Guard != nil && !br.Guard() {
			continue
		}
		switch {
		case br.Port != nil:
			env, ok := br.Port.Peek()
			if !ok {
				continue
			}
			if br.PortFilter != nil && !br.PortFilter(env) {
				continue
			}
			br.Port.DropHead()
			if br.Body != nil {
				br.Body(env)
			}
			return Verdict{BranchIndex: i, Consumed: env}, true
		case br.Timer != nil:
			if !br.Timer.Drain() {
				continue
			}
			if br.Body != nil {
				br.Body(nil)
			}
			return Verdict{BranchIndex: i, Consumed: br.Timer}, true
		case br.Component != nil:
			if !componentReady(br.Component, br.ComponentEvent) {
				continue
			}
			if br.Body != nil {
				br.Body(br.Component)
			}
			return Verdict{BranchIndex: i, Consumed: br.Component}, true
		}
	}
	return Verdict{}, false
}

// componentReady checks whether the lifecycle event we care about has
// fired. We rely on Done() closing for both Done and Killed transitions
// because the registry sets state before closing the channel.
func componentReady(c *component.Component, event ComponentEvent) bool {
	select {
	case <-c.Done():
	default:
		return false
	}
	state := c.State()
	switch event {
	case OnDone:
		return state == component.Done
	case OnKilled:
		return state == component.Killed
	case OnAny:
		return state == component.Done || state == component.Killed
	}
	return false
}

// sourceKind tells requeue how to handle a value read out from a
// select case so it stays observable for the follow-up snapshot pass.
type sourceKind int

const (
	srcPort sourceKind = iota
	srcTimer
	srcComponent
)

// caseKind couples a select case to the branch it came from plus the
// source kind we need to requeue.
type caseKind struct {
	branchIdx  int
	sourceKind sourceKind
}

// buildSelectCases packs every branch's event source into a
// reflect.SelectCase and returns a parallel slice describing what each
// case represents. The final case (if stop is non-nil) is the cancel
// signal.
//
// All cases are receive-only on signal channels (Port.Wake,
// Timer.Timeout, Component.Done). None of these reads CONSUME state
// the snapshot pass relies on: Wake is just a "something arrived"
// notification, Timeout is buffered and Reassert restores it, and
// Done is closed so it never drains.
func buildSelectCases(branches []Branch, stop <-chan struct{}) ([]reflect.SelectCase, []caseKind) {
	cases := make([]reflect.SelectCase, 0, len(branches)+1)
	kinds := make([]caseKind, 0, len(branches)+1)
	for i, br := range branches {
		switch {
		case br.Port != nil:
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(br.Port.Wake()),
			})
			kinds = append(kinds, caseKind{branchIdx: i, sourceKind: srcPort})
		case br.Timer != nil:
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(br.Timer.Timeout()),
			})
			kinds = append(kinds, caseKind{branchIdx: i, sourceKind: srcTimer})
		case br.Component != nil:
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(br.Component.Done()),
			})
			kinds = append(kinds, caseKind{branchIdx: i, sourceKind: srcComponent})
		}
	}
	if stop != nil {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(stop),
		})
		kinds = append(kinds, caseKind{branchIdx: -1, sourceKind: srcPort})
	}
	return cases, kinds
}

// requeue restores any state we accidentally consumed in the select
// wake-up so the next snapshot pass can observe the same event. Port
// wake signals don't carry data and don't need requeuing. Timer
// timeouts must be re-asserted because reading from a buffered channel
// removes the signal. Component done channels are closed, so reads
// don't drain anything.
func requeue(br Branch, kind sourceKind, val reflect.Value) {
	_ = val
	switch kind {
	case srcPort:
		// Wake is a notification only; no requeue needed.
	case srcTimer:
		if br.Timer == nil {
			return
		}
		_ = br.Timer.Reassert()
	case srcComponent:
		// Closed-channel reads are non-consuming.
	}
}
