// Package backend defines the interface between k3 run and the runtime that
// executes the test cases and control parts of a T3XF.
package backend

import (
	"errors"

	"github.com/nokia/ntt/internal/k3log"
)

// ErrClosed is returned when requests are made to a closed or terminated
// Runtime.
var ErrClosed = errors.New("runtime closed")

// ModuleParFunc returns the value of module parameter id, for example
// "Mod.X", for the test case or control part req, for example "Mod.TC". The
// value is in TTCN-3 value notation. ok is false when the declared default
// applies.
type ModuleParFunc func(req, id string) (value string, ok bool)

// ParameterMap returns a ModuleParFunc that looks up module parameters in m,
// regardless of the request.
func ParameterMap(m map[string]string) ModuleParFunc {
	return func(_, id string) (string, bool) {
		v, ok := m[id]
		return v, ok
	}
}

// Runtime is a running test executor. Tests and control parts are started
// with Start and run sequentially in the order they were requested. The
// runtime emits the k3log events they produce on Events.
//
// The consumer of Events must keep draining the channel until it is closed.
type Runtime interface {
	// Start requests the runtime to run the test case or control part with
	// the given fully qualified id, for example "module.Testcase" or
	// "module.control". The call does not wait for it to finish. Requests
	// queue up and run in order. Watch Events for the k3log events (tcst/tcfi
	// for tests, cpen/cplv for control parts). Start returns an error
	// wrapping ErrClosed once the runtime does not accept requests anymore.
	Start(id string) error

	// Events returns the channel of k3log events emitted by the runtime. The
	// channel is closed after the runtime exited and all events were emitted.
	Events() <-chan k3log.Event

	// Done returns a channel that is closed when the runtime has exited and
	// all events were emitted.
	Done() <-chan struct{}

	// Err returns the exit error of the runtime. Err returns nil while the
	// runtime is running.
	Err() error

	// CloseRequests tells the runtime that no more requests will follow. The
	// runtime finishes the queued requests and exits, after which Events is
	// closed. Unlike Close it does not wait and keeps the events.
	CloseRequests() error

	// Close calls CloseRequests, waits for the runtime to exit and returns
	// its exit error.
	//
	// Close discards events that are not consumed anymore, so it can't block
	// because of a missing consumer.
	Close() error
}
