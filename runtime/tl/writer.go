package tl

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is written to the log header. It names the standard the log
// follows and the unit of every ts, which the standard leaves to the tool.
const Version = "ETSI ES 201 873-6 V4.12.1 TCI-TL; ntt; ts=microseconds since 1970-01-01T00:00:00Z"

// Writer streams events to an io.Writer as a TCI-TL log. It is safe for
// concurrent use. Close completes the log; it does not close the
// underlying writer.
type Writer struct {
	mu     sync.Mutex
	w      *bufio.Writer
	jsonl  bool
	events int
	err    error
	closed bool
}

// NewXMLWriter starts a log in the Annex B XML format: a `logfile`
// (Log schema, B.6) whose body holds one element per event.
func NewXMLWriter(w io.Writer) *Writer {
	lw := &Writer{w: bufio.NewWriter(w)}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<tli:logfile")
	for _, ns := range namespaces {
		b.WriteString(` xmlns:` + ns.Prefix + `="` + ns.URI + `"`)
	}
	b.WriteString(">\n")
	lw.write(b.String())
	lw.writeNode(lw.header())
	lw.write("\n<tli:body>\n")
	return lw
}

// NewJSONLWriter starts a log in JSON Lines: the header, then one event per
// line, each the same element tree the XML format would carry.
func NewJSONLWriter(w io.Writer) *Writer {
	lw := &Writer{w: bufio.NewWriter(w), jsonl: true}
	lw.writeNode(lw.header())
	return lw
}

func (lw *Writer) header() *Node {
	return el(NSLog, "header",
		text(NSLog, "version", Version),
		text(NSLog, "ts", strconv.FormatInt(time.Now().UnixMicro(), 10)))
}

// Log writes e. Events are written in the order Log is called.
func (lw *Writer) Log(e *Event) {
	n := e.node()
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.closed {
		return
	}
	lw.events++
	lw.writeNodeLocked(n)
	if lw.err == nil {
		lw.err = lw.w.Flush()
	}
}

// Close completes the log and returns the first write error. The body of
// an XML log must hold at least one event, so an empty log gets a tliInfo
// saying so.
func (lw *Writer) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.closed {
		return lw.err
	}
	if !lw.jsonl {
		if lw.events == 0 {
			lw.writeNodeLocked((&Event{
				Op:   "tliInfo",
				Ts:   time.Now().UnixMicro(),
				C:    ComponentID{Null: true},
				Args: []Arg{{"level", Integer(0)}, {"info", String("no events were logged")}},
			}).node())
		}
		lw.write("</tli:body>\n</tli:logfile>\n")
	}
	lw.closed = true
	if lw.err == nil {
		lw.err = lw.w.Flush()
	}
	return lw.err
}

func (lw *Writer) writeNode(n *Node) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.writeNodeLocked(n)
}

func (lw *Writer) writeNodeLocked(n *Node) {
	if lw.err != nil {
		return
	}
	if lw.jsonl {
		b, err := json.Marshal(n.toJSON())
		if err != nil {
			lw.err = err
			return
		}
		lw.write(string(b) + "\n")
		return
	}
	if err := n.writeXML(lw.w); err != nil {
		lw.err = err
		return
	}
	lw.write("\n")
}

func (lw *Writer) write(s string) {
	if lw.err != nil {
		return
	}
	_, lw.err = lw.w.WriteString(s)
}

// Recorder keeps events in memory, for tests.
type Recorder struct {
	mu     sync.Mutex
	Events []*Event
}

func (r *Recorder) Log(e *Event) {
	r.mu.Lock()
	r.Events = append(r.Events, e)
	r.mu.Unlock()
}

// Ops returns the operation names logged so far, in order.
func (r *Recorder) Ops() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.Events))
	for i, e := range r.Events {
		out[i] = e.Op
	}
	return out
}

// Validator checks every event against the schema before passing it on to
// Next (which may be nil), and keeps the first few violations. For tests
// and for checking an executor's output.
type Validator struct {
	Next Logger

	mu     sync.Mutex
	events int
	errs   []error
}

func (v *Validator) Log(e *Event) {
	if err := e.Validate(); err != nil {
		v.mu.Lock()
		if len(v.errs) < 20 {
			v.errs = append(v.errs, err)
		}
		v.mu.Unlock()
	}
	v.mu.Lock()
	v.events++
	v.mu.Unlock()
	if v.Next != nil {
		v.Next.Log(e)
	}
}

// Result returns the number of events seen and the violations kept.
func (v *Validator) Result() (int, []error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.events, append([]error(nil), v.errs...)
}

// Monotonic passes events on to Next and keeps a log's timestamps from
// going backwards. Under the virtual clock a testcase's events run ahead of
// the wall clock by the virtual time it used, so the next event taken from
// the wall clock — the next testcase's, or its control part's — would
// otherwise be earlier than the last. Now reports the time to give such an
// event: the wall clock, or the latest timestamp logged if that is later.
//
// On the real clock, components time their events concurrently and the log
// takes them one at a time, so an event can reach the log after a later
// one; it is given the later one's timestamp. Events pass to Next in the
// order they are stamped.
type Monotonic struct {
	Next Logger

	mu   sync.Mutex
	last int64
}

func (m *Monotonic) Log(e *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.Ts < m.last {
		e.Ts = m.last
	}
	m.last = e.Ts
	m.Next.Log(e)
}

// Now is the timestamp, in microseconds since the Unix epoch, for an event
// that is not timed by a testcase's clock.
func (m *Monotonic) Now() int64 {
	now := time.Now().UnixMicro()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last > now {
		return m.last
	}
	return now
}
