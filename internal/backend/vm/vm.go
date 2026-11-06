// Package vm is the backend that executes a T3XF in process with the k3
// virtual machine instead of starting k3r.
package vm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/nokia/ntt/internal/backend"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/runtime"
	k3vm "github.com/nokia/ntt/internal/vm"
)

// ErrClosed is returned when requests are made to a closed or terminated
// Runtime.
var ErrClosed = backend.ErrClosed

var _ backend.Runtime = (*Runtime)(nil)

// eventBuffer is the number of events that can be queued before the machine
// is slowed down by a slow consumer.
const eventBuffer = 256

// Runtime runs the test cases and control parts of a T3XF image on the k3
// virtual machine. Requests run sequentially, in the order they were made,
// on a single machine, so module variables persist between them like in k3r.
//
// The consumer of Events must keep draining the channel until it is closed.
type Runtime struct {
	machine *k3vm.Machine
	ctx     context.Context

	mu     sync.Mutex // protects queue and closed
	queue  []string
	closed bool
	wake   chan struct{}

	events chan k3log.Event
	done   chan struct{} // closed when the machine has stopped and all events are emitted
	err    error         // valid after done is closed
}

// ModuleParFunc returns the value of module parameter id for the test case or
// control part req.
type ModuleParFunc = backend.ModuleParFunc

type config struct {
	ctx    context.Context
	params ModuleParFunc
}

// Option configures a Runtime.
type Option func(*config)

// WithContext stops the runtime when ctx is done. The machine can't be
// interrupted, so the request that is running finishes first.
func WithContext(ctx context.Context) Option {
	return func(c *config) { c.ctx = ctx }
}

// WithModuleParameters sets the source of module parameter values. The
// machine can't evaluate values yet, so NewRuntime fails when f sets a
// module parameter of the image.
func WithModuleParameters(f ModuleParFunc) Option {
	return func(c *config) { c.params = f }
}

// NewRuntime loads the given T3XF file and starts a machine on it. The caller
// must call Close to release the resources.
func NewRuntime(t3xf string, opts ...Option) (*Runtime, error) {
	cfg := config{ctx: context.Background()}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.ctx == nil {
		cfg.ctx = context.Background()
	}

	img, err := k3vm.LoadFile(t3xf)
	if err != nil {
		return nil, fmt.Errorf("vm: %w", err)
	}
	if cfg.params != nil {
		for _, v := range img.ModuleParameters {
			if _, ok := cfg.params("", v.Name); ok {
				return nil, fmt.Errorf("vm: module parameter %s: setting module parameters is not supported", v.Name)
			}
		}
	}

	pr, pw := io.Pipe()
	m := k3vm.New(img)
	m.Out = pw

	r := &Runtime{
		machine: m,
		ctx:     cfg.ctx,
		wake:    make(chan struct{}, 1),
		events:  make(chan k3log.Event, eventBuffer),
		done:    make(chan struct{}),
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		r.readLog(pr)
	}()
	go func() {
		err := r.serve()
		pw.Close()
		<-readerDone

		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		r.err = err
		close(r.events)
		close(r.done)
	}()

	return r, nil
}

// Start requests the machine to run the test case or control part with the
// given fully qualified id, for example "module.Testcase" or
// "module.control". The call does not wait for it to finish. Requests queue
// up and run in order.
func (r *Runtime) Start(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	r.queue = append(r.queue, id)
	r.signal()
	return nil
}

// Events returns the channel of k3log events emitted by the machine. The
// channel is closed after the machine stopped and all events were emitted.
func (r *Runtime) Events() <-chan k3log.Event {
	return r.events
}

// Done returns a channel that is closed when the machine has stopped and all
// events were emitted.
func (r *Runtime) Done() <-chan struct{} {
	return r.done
}

// Err returns the error that stopped the machine. Err returns nil while the
// machine is running.
func (r *Runtime) Err() error {
	select {
	case <-r.done:
		return r.err
	default:
		return nil
	}
}

// CloseRequests tells the runtime that no more requests will follow. The
// machine finishes the queued requests and stops, after which Events is
// closed. Unlike Close it does not wait and keeps the events.
func (r *Runtime) CloseRequests() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.signal()
	return nil
}

// Close calls CloseRequests, waits for the machine to stop and returns its
// error.
//
// Close discards events that are not consumed anymore, so it can't block
// because of a missing consumer.
func (r *Runtime) Close() error {
	_ = r.CloseRequests()

	for range r.events {
	}
	<-r.done
	return r.err
}

// signal wakes serve. r.mu must be held.
func (r *Runtime) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// next returns the next request. ok is false when no more requests follow.
func (r *Runtime) next() (id string, ok bool, err error) {
	for {
		r.mu.Lock()
		if len(r.queue) > 0 {
			id, r.queue = r.queue[0], r.queue[1:]
			r.mu.Unlock()
			return id, true, nil
		}
		closed := r.closed
		r.mu.Unlock()
		if closed {
			return "", false, nil
		}
		select {
		case <-r.wake:
		case <-r.ctx.Done():
			return "", false, fmt.Errorf("vm: %w", r.ctx.Err())
		}
	}
}

// serve initialises the machine and runs the requests until no more follow.
func (r *Runtime) serve() error {
	if err := r.machine.Init(); err != nil {
		return fmt.Errorf("vm: %w", err)
	}
	for {
		id, ok, err := r.next()
		if !ok {
			return err
		}
		if err := r.ctx.Err(); err != nil {
			return fmt.Errorf("vm: %w", err)
		}
		if err := r.run(id); err != nil {
			return fmt.Errorf("vm: %s: %w", id, err)
		}
	}
}

// run executes the test case or control part id.
func (r *Runtime) run(id string) error {
	fn, _ := r.machine.Names[id].(*runtime.Function)
	switch {
	case fn != nil && fn.Flags&runtime.Testcase != 0:
		if len(fn.Parameters) > 0 {
			return fmt.Errorf("testcase has parameters")
		}
		_, err := r.machine.RunTestcase(fn)
		return err
	case fn != nil && slices.Contains(r.machine.Controls, fn):
		return r.machine.RunControl(fn)
	default:
		return fmt.Errorf("no such testcase or control part")
	}
}

// readLog parses the k3log read from rd and emits the events until EOF.
func (r *Runtime) readLog(rd io.Reader) {
	br := bufio.NewReaderSize(rd, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			if e, perr := k3log.ParseEvent(line); perr == nil {
				r.events <- e
			}
		}
		if err != nil {
			return
		}
	}
}
