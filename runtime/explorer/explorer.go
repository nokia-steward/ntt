// Package explorer exposes the JSON shape that the VS Code test
// explorer extension (and any other IDE integration) consumes. The
// schema is intentionally stable: adding fields is allowed, renames
// must bump the SchemaVersion constant.
//
// Two top-level documents are defined:
//
//   - Tree     describes the project's static testcase / module
//              hierarchy. Returned by `ntt explorer list`.
//
//   - Event    is one entry in the streaming output of `ntt explorer
//              run`. Events carry start / pass / fail / log payloads
//              so the IDE can update its tree live.
//
// Both documents are encoded as JSON Lines so the IDE can consume
// them with a streaming reader.
package explorer

import (
	"encoding/json"
	"io"
	"time"
)

const SchemaVersion = "1"

// Tree is the static project description served to the IDE.
type Tree struct {
	Schema  string   `json:"schema"`
	Project string   `json:"project"`
	Modules []Module `json:"modules"`
}

// Module groups a set of testcases under one TTCN-3 module.
type Module struct {
	Name      string     `json:"name"`
	File      string     `json:"file,omitempty"`
	Testcases []Testcase `json:"testcases"`
}

// Testcase describes one runnable testcase.
type Testcase struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// EventKind enumerates the lifecycle events the executor streams to
// the IDE. Keeping them as strings means JSON consumers can match
// against them without recompiling.
type EventKind string

const (
	EventStarted   EventKind = "started"
	EventFinished  EventKind = "finished"
	EventLog       EventKind = "log"
	EventTestStart EventKind = "test_start"
	EventTestEnd   EventKind = "test_end"
)

// Event is one entry in the streaming run output.
type Event struct {
	Schema   string    `json:"schema"`
	Kind     EventKind `json:"kind"`
	Time     time.Time `json:"time"`
	Testcase string    `json:"testcase,omitempty"`
	Verdict  string    `json:"verdict,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// Stream wraps a writer in a small helper that emits one JSON line
// per Event. It's safe for sequential use from a single goroutine.
type Stream struct {
	enc *json.Encoder
}

func NewStream(w io.Writer) *Stream {
	enc := json.NewEncoder(w)
	return &Stream{enc: enc}
}

func (s *Stream) Emit(e Event) error {
	if e.Schema == "" {
		e.Schema = SchemaVersion
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	return s.enc.Encode(e)
}

// WriteTree serializes t to w as a single JSON document.
func WriteTree(w io.Writer, t Tree) error {
	if t.Schema == "" {
		t.Schema = SchemaVersion
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}
