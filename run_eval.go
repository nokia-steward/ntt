package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/nokia/ntt/commands/run"
	"github.com/nokia/ntt/internal/backend"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/project"
	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/runtime/tl"
)

// evalBackend is ntt run's eval backend: the suite runs on ntt's engine,
// the interpreter, which writes the structured line log the run reads as
// events.
var evalBackend = run.Backend{
	Build: func(conf *project.Config, analyze bool) error {
		files, err := runFiles(conf)
		if err != nil {
			return err
		}
		return buildSuite(files, analyze)
	},
	Runtime: func(ctx context.Context, conf *project.Config, _ backend.ModuleParFunc) (backend.Runtime, error) {
		files, err := runFiles(conf)
		if err != nil {
			return nil, err
		}
		d := newStaticDriver(files)
		d.modParam = moduleParameters(conf)
		d.live = runLive
		d.timeout = runTimeout
		return newEvalRuntime(ctx, d, os.Stderr), nil
	},
}

// evalRuntime runs the jobs started on it one after another on a
// staticDriver and emits the events of its line log.
type evalRuntime struct {
	ctx     context.Context
	driver  *staticDriver
	unknown io.Writer // where a line that is no event goes

	mu     sync.Mutex // guards queue and closed
	queue  []string
	closed bool
	wake   chan struct{}

	events chan k3log.Event
	done   chan struct{}
	err    error // valid after done is closed
}

var _ backend.Runtime = (*evalRuntime)(nil)

func newEvalRuntime(ctx context.Context, d *staticDriver, unknown io.Writer) *evalRuntime {
	if ctx == nil {
		ctx = context.Background()
	}
	r := &evalRuntime{
		ctx:     ctx,
		driver:  d,
		unknown: unknown,
		wake:    make(chan struct{}, 1),
		events:  make(chan k3log.Event, 256),
		done:    make(chan struct{}),
	}
	go r.loop()
	return r
}

// Start queues the testcase or control part id; one the suite does not
// have is an error.
func (r *evalRuntime) Start(id string) error {
	if !r.driver.hasJob(id) {
		return fmt.Errorf("%s: no such testcase or control part", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("start %s: %w", id, backend.ErrClosed)
	}
	r.queue = append(r.queue, id)
	r.signal()
	return nil
}

func (r *evalRuntime) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *evalRuntime) Events() <-chan k3log.Event { return r.events }
func (r *evalRuntime) Done() <-chan struct{}      { return r.done }

func (r *evalRuntime) Err() error {
	select {
	case <-r.done:
		return r.err
	default:
		return nil
	}
}

func (r *evalRuntime) CloseRequests() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.signal()
	return nil
}

// Close waits for the queued jobs to finish, discarding the events not
// consumed, and returns why the run ended, if it failed.
func (r *evalRuntime) Close() error {
	_ = r.CloseRequests()
	go func() {
		for range r.events {
		}
	}()
	<-r.done
	return r.err
}

// next is the next job to run, waiting for one; false when none will come.
func (r *evalRuntime) next() (string, bool) {
	for {
		r.mu.Lock()
		if len(r.queue) > 0 {
			id := r.queue[0]
			r.queue = r.queue[1:]
			r.mu.Unlock()
			return id, true
		}
		closed := r.closed
		r.mu.Unlock()
		if closed {
			return "", false
		}
		select {
		case <-r.wake:
		case <-r.ctx.Done():
			return "", false
		}
	}
}

func (r *evalRuntime) loop() {
	defer close(r.done)
	defer close(r.events)
	w := &eventWriter{out: r.events, unknown: r.unknown, finished: map[string]bool{}}
	lines := tl.NewLinesWriter(w)
	// Testcases run one after another on the virtual clock each start from
	// the wall clock; keep the log's timestamps from going backwards.
	r.driver.testLogger = &tl.Monotonic{Next: lines}
	// What fails a job and its log does not show: a control part's own
	// error, a testcase that ends in error before its end is logged.
	var unlogged []error
	for {
		id, ok := r.next()
		if !ok {
			break
		}
		v, reason, err := r.driver.Run(r.ctx, id)
		if err != nil {
			unlogged = append(unlogged, err)
			break
		}
		if v != rreport.Error {
			continue
		}
		control := strings.HasSuffix(id, ".control")
		if control && reason != "" || !control && !w.finished[id] {
			if reason == "" {
				reason = "error"
			}
			unlogged = append(unlogged, fmt.Errorf("%s: %s", id, reason))
		}
	}
	if err := lines.Close(); err != nil {
		unlogged = append(unlogged, fmt.Errorf("writing log: %w", err))
	}
	w.flush()
	r.err = errors.Join(unlogged...)
}

// eventWriter takes the lines of the structured line log and sends each as
// an event; a line that is none goes to unknown.
type eventWriter struct {
	out      chan<- k3log.Event
	unknown  io.Writer
	buf      []byte
	finished map[string]bool // testcases whose end (tcfi) was logged
}

func (w *eventWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.line(line)
	}
	return len(b), nil
}

// flush takes a last line without its newline.
func (w *eventWriter) flush() {
	if len(w.buf) > 0 {
		line := string(w.buf)
		w.buf = nil
		w.line(line)
	}
}

func (w *eventWriter) line(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	e, err := k3log.ParseEvent(line)
	if err != nil {
		fmt.Fprintf(w.unknown, "ntt run: unparsed log line: %s\n", line)
		return
	}
	if e.ID() == "tcfi" {
		name, _, _ := strings.Cut(e.Field(3), "(")
		w.finished[name] = true
	}
	w.out <- e
}
