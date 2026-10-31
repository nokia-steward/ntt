package ttcn3

import "encoding/json"

type Source struct {
	Filename string
	Events   []NodeEvent `json:"events"`
}

// NodeEvent is one syntax event in a compiled source file.
//
// MarshalJSON emits exactly one of:
//
//	{"kind": "open", "node": "Module", "end": 5}
//	{"kind": "add", "text": "module", "offs": 0}
//	{"kind": "close"}
type NodeEvent struct {
	// Kind is "open", "add", or "close".
	Kind string

	// Node is the syntax type name of an open event, such as "Module" or "Root".
	Node string

	// End is the index of the matching close event. Set only on open events.
	End int

	// Text is the text of an add event. Add events cover every token of the
	// source file in order, including comments, commas, semicolons and
	// preprocessor directives, but excluding EOF.
	Text string

	// Offs is the byte offset of an add event.
	Offs int
}

// MarshalJSON emits the compile-command event schema.
func (e NodeEvent) MarshalJSON() ([]byte, error) {
	switch e.Kind {
	case "open":
		return json.Marshal(struct {
			Kind string `json:"kind"`
			Node string `json:"node"`
			End  int    `json:"end"`
		}{e.Kind, e.Node, e.End})
	case "add":
		return json.Marshal(struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
			Offs int    `json:"offs"`
		}{e.Kind, e.Text, e.Offs})
	default:
		return json.Marshal(struct {
			Kind string `json:"kind"`
		}{e.Kind})
	}
}
