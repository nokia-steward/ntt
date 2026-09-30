package tl

import (
	"fmt"
	"strconv"
)

// Event is one TCI-TL logging operation (ES 201 873-6 clause 7.3.4.1): Op
// names it (for example "tliMSend_c"), Am, Ts, Src, Line and C are the
// parameters every operation has, and Args the operation's own.
//
// Ts is in microseconds since 1970-01-01T00:00:00Z. The standard leaves the
// unit to the tool; the log header records the choice. Under the virtual
// clock it is the testcase's start time plus the virtual time elapsed, so
// the ordering and spacing of events reflect the clock the test ran on.
type Event struct {
	Op   string
	Am   string
	Ts   int64
	Src  string
	Line int
	C    ComponentID
	Args []Arg
}

// Arg is one operation-specific parameter, named as in the XML schema.
type Arg struct {
	Name string
	Val  Content
}

// Logger receives events. Implementations must be safe for concurrent use:
// components log from their own goroutines.
type Logger interface {
	Log(*Event)
}

// Validate checks e against the Annex B event it names: the operation must
// exist, every parameter must be one the schema declares for it and appear
// at most once, and every parameter the schema requires must be present.
func (e *Event) Validate() error {
	fields, ok := Schema[e.Op]
	if !ok {
		return fmt.Errorf("tl: unknown operation %q", e.Op)
	}
	seen := map[string]bool{}
	for _, a := range e.Args {
		if seen[a.Name] {
			return fmt.Errorf("tl: %s: parameter %q given twice", e.Op, a.Name)
		}
		seen[a.Name] = true
		known := false
		for _, f := range fields {
			if f.Name == a.Name {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("tl: %s has no parameter %q", e.Op, a.Name)
		}
	}
	for _, f := range fields {
		if !f.Optional && !seen[f.Name] {
			return fmt.Errorf("tl: %s: required parameter %q missing", e.Op, f.Name)
		}
	}
	return nil
}

// node returns e as the element the Log schema's Body declares for it, its
// parameters in schema order.
func (e *Event) node() *Node {
	n := &Node{NS: NSLog, Tag: e.Op}
	n.Attrs = append(n.Attrs, Attr{"ts", strconv.FormatInt(e.Ts, 10)})
	if e.Src != "" {
		n.Attrs = append(n.Attrs, Attr{"src", e.Src})
	}
	if e.Line > 0 {
		n.Attrs = append(n.Attrs, Attr{"line", strconv.Itoa(e.Line)})
	}
	// The producing component, flattened into the Event type's required
	// name / id / type attributes.
	n.Attrs = append(n.Attrs, Attr{"name", e.C.Name}, Attr{"id", e.C.ID}, Attr{"type", e.C.Type})
	n.Kids = append(n.Kids, text(NSEvents, "am", e.Am))
	for _, f := range Schema[e.Op] {
		for _, a := range e.Args {
			if a.Name == f.Name {
				n.Kids = append(n.Kids, a.Val.in(NSEvents, f.Name))
			}
		}
	}
	return n
}

// LateID is the component id of an event a component logged after its
// testcase had ended (see runtime.TestcaseExec.TLogFrom). Comparisons file
// such events outside any testcase, wherever they fall in the log.
const LateID = "late"

// Summary renders e in one line, as Summary does its element.
func (e *Event) Summary() string { return Summary(e.node()) }
