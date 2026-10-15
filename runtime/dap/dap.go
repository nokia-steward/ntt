// Package dap implements a minimal Debug Adapter Protocol server for
// the ntt interpreter. The current scope (M9 in the ntt-Titan plan)
// is enough to let a DAP-speaking IDE attach, list testcases as
// threads, launch one, and receive `output` + `terminated` events.
// Stepping / breakpoints are stubbed - they wire into interpreter
// hooks that the interpreter rewire (post-M9) will fill in.
//
// The wire format is the textbook DAP framing:
//
//	Content-Length: <n>\r\n
//	\r\n
//	<n bytes of UTF-8 JSON>
//
// We accept and emit one Message per frame.
package dap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Message is the common envelope for DAP requests, responses and
// events. The discriminator is the Type field.
type Message struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`              // "request" | "response" | "event"
	Command string          `json:"command,omitempty"` // request/response
	Event   string          `json:"event,omitempty"`   // event
	Request int             `json:"request_seq,omitempty"`
	Success bool            `json:"success,omitempty"`
	Message string          `json:"message,omitempty"` // response error message
	Body    json.RawMessage `json:"body,omitempty"`
	Args    json.RawMessage `json:"arguments,omitempty"`
}

// Reader reads framed DAP messages from r.
type Reader struct {
	br *bufio.Reader
}

func NewReader(r io.Reader) *Reader { return &Reader{br: bufio.NewReader(r)} }

func (r *Reader) Next() (*Message, error) {
	contentLength := -1
	for {
		line, err := r.br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(line[len("Content-Length:"):])
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %q", v)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r.br, buf); err != nil {
		return nil, err
	}
	m := &Message{}
	if err := json.Unmarshal(buf, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Writer emits framed DAP messages to w.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
	n  int
}

func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

func (w *Writer) Write(m *Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.n++
	if m.Seq == 0 {
		m.Seq = w.n
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	hdr := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w.w, hdr); err != nil {
		return err
	}
	_, err = w.w.Write(data)
	return err
}

// Handler is the integration point for the interpreter. ListTests
// returns the testcase identifiers as DAP "threads", Launch runs one
// of them and returns its terminal verdict. Both can be replaced for
// tests.
type Handler interface {
	ListTests() []string
	Launch(name string, emit func(string)) (verdict, reason string, err error)
}

// Serve reads requests from r, dispatches them through h, and writes
// responses + events to w until r returns io.EOF or the client sends
// a `disconnect` request.
func Serve(r io.Reader, w io.Writer, h Handler) error {
	in := NewReader(r)
	out := NewWriter(w)
	for {
		msg, err := in.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if msg.Type != "request" {
			continue
		}
		if err := dispatch(msg, h, out); err != nil {
			return err
		}
		if msg.Command == "disconnect" {
			return nil
		}
	}
}

func dispatch(req *Message, h Handler, out *Writer) error {
	switch req.Command {
	case "initialize":
		body, _ := json.Marshal(map[string]any{
			"supportsConfigurationDoneRequest": true,
			"supportsTerminateRequest":         true,
		})
		if err := respond(out, req, true, "", body); err != nil {
			return err
		}
		return out.Write(&Message{Type: "event", Event: "initialized"})
	case "launch":
		var args struct {
			Test string `json:"test"`
		}
		_ = json.Unmarshal(req.Args, &args)
		if args.Test == "" {
			return respond(out, req, false, "missing 'test' argument", nil)
		}
		if err := respond(out, req, true, "", nil); err != nil {
			return err
		}
		go func() {
			emit := func(line string) {
				body, _ := json.Marshal(map[string]any{
					"category": "stdout",
					"output":   line + "\n",
				})
				_ = out.Write(&Message{Type: "event", Event: "output", Body: body})
			}
			verdict, reason, err := h.Launch(args.Test, emit)
			if err != nil {
				emit("ntt-dap: launch error: " + err.Error())
			}
			body, _ := json.Marshal(map[string]any{
				"verdict": verdict,
				"reason":  reason,
			})
			_ = out.Write(&Message{Type: "event", Event: "terminated", Body: body})
		}()
		return nil
	case "threads":
		tests := h.ListTests()
		threads := make([]map[string]any, 0, len(tests))
		for i, name := range tests {
			threads = append(threads, map[string]any{"id": i + 1, "name": name})
		}
		body, _ := json.Marshal(map[string]any{"threads": threads})
		return respond(out, req, true, "", body)
	case "disconnect", "terminate", "configurationDone":
		return respond(out, req, true, "", nil)
	default:
		return respond(out, req, false, "unsupported command "+req.Command, nil)
	}
}

func respond(out *Writer, req *Message, ok bool, msg string, body json.RawMessage) error {
	resp := &Message{
		Type:    "response",
		Command: req.Command,
		Request: req.Seq,
		Success: ok,
		Message: msg,
		Body:    body,
	}
	return out.Write(resp)
}

// pipeBuffer is a simple in-memory io.ReadWriter, useful for tests
// that loop a Serve through itself without a real socket.
type pipeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (p *pipeBuffer) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Read(b)
}
func (p *pipeBuffer) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(b)
}
