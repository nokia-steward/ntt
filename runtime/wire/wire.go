// Package wire defines the JSON-line wire protocol that the master
// controller (`cmd/ntt-mctr`) and host controllers (`cmd/ntt-hc`)
// speak to each other in a distributed test run. The wire format is
// deliberately not Titan-binary-compatible at this stage: it's
// transport-agnostic and human-debuggable, and a Titan-bridge can be
// layered on top by re-marshalling individual messages.
//
// A Frame is a length-prefixed JSON document. Each Frame carries one
// Message, which is a discriminated union over the message types in
// MessageKind. The protocol is request/response with optional
// fire-and-forget notifications.
package wire

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// MessageKind enumerates the verbs the protocol understands. The set
// is intentionally small at M5; future iterations add component
// placement, port mapping, codec dispatch, etc.
type MessageKind string

const (
	HelloKind     MessageKind = "hello"
	ReadyKind     MessageKind = "ready"
	RunKind       MessageKind = "run"
	VerdictKind   MessageKind = "verdict"
	LogKind       MessageKind = "log"
	StopKind      MessageKind = "stop"
	GoodbyeKind   MessageKind = "goodbye"
)

// Message is the discriminated union of all protocol messages.
type Message struct {
	Kind MessageKind     `json:"kind"`
	ID   string          `json:"id,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

// Hello is the first message a host controller sends after connecting
// to the master. It advertises the host's name and the list of
// testcases it can run.
type Hello struct {
	Host    string   `json:"host"`
	Cases   []string `json:"cases"`
	Version string   `json:"version"`
}

// Ready is sent by the master after Hello to confirm the host's
// registration. It carries any session-wide configuration the host
// will need (cfg path, log directory, ...).
type Ready struct {
	Session  string `json:"session"`
	LogDir   string `json:"log_dir,omitempty"`
	CfgInline string `json:"cfg_inline,omitempty"`
}

// Run is a request to execute a testcase. The host responds with a
// Verdict once execution completes.
type Run struct {
	Case   string            `json:"case"`
	Params map[string]string `json:"params,omitempty"`
}

// Verdict is the host's report on a single Run.
type Verdict struct {
	Case     string  `json:"case"`
	Verdict  string  `json:"verdict"`
	Reason   string  `json:"reason,omitempty"`
	Duration float64 `json:"duration_seconds"`
}

// Log is a streamed log line. Hosts can send any number of Log
// messages between a Run and its corresponding Verdict.
type Log struct {
	Case  string `json:"case,omitempty"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// Stop is a master->host request to halt all execution and disconnect.
type Stop struct {
	Reason string `json:"reason,omitempty"`
}

// Goodbye is the last message a host sends before closing the channel.
type Goodbye struct{}

// Encoder writes Messages to an io.Writer. Concurrent calls are safe
// only if the caller serialises them.
type Encoder struct {
	w *json.Encoder
}

// NewEncoder wraps w. Each Encode call emits exactly one JSON line.
func NewEncoder(w io.Writer) *Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &Encoder{w: enc}
}

// Encode writes one Message.
func (e *Encoder) Encode(m Message) error { return e.w.Encode(m) }

// EncodeBody is a convenience that marshals body to JSON and wraps it
// in a Message with the given kind.
func (e *Encoder) EncodeBody(kind MessageKind, id string, body interface{}) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return e.Encode(Message{Kind: kind, ID: id, Body: raw})
}

// Decoder reads Messages from an io.Reader.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder wraps r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Decode reads one Message.
func (d *Decoder) Decode() (Message, error) {
	line, err := d.r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return Message{}, fmt.Errorf("wire: %w (line=%q)", err, line)
	}
	return m, nil
}

// UnmarshalBody decodes m.Body into v. Returns an error if Body is
// empty or v isn't a pointer to a JSON-decodable type.
func (m Message) UnmarshalBody(v interface{}) error {
	if len(m.Body) == 0 {
		return fmt.Errorf("wire: %s has empty body", m.Kind)
	}
	return json.Unmarshal(m.Body, v)
}

// marshalMessage is the shared encoder both the JSON-line and the
// binary frame writers use to produce the per-message payload.
func marshalMessage(m Message) ([]byte, error) {
	return json.Marshal(m)
}

// unmarshalMessage is the inverse of marshalMessage.
func unmarshalMessage(b []byte) (Message, error) {
	var m Message
	if err := json.Unmarshal(b, &m); err != nil {
		return Message{}, err
	}
	return m, nil
}
