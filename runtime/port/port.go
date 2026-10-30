// Package port implements TTCN-3 port instances per Core Language clauses
// 6.2.9 (port type), 14 (operations) and 22.5 (message vs procedure based
// communication).
//
// Each Port has a single in-queue (an unbounded FIFO channel) and a list
// of peers it is connected to via Connect / Map. Message-based ports
// deliver values directly; procedure-based ports add a Reply channel per
// call so the caller can wait for a reply via getreply.
//
// The implementation is deliberately Go-channel native: alt-style
// snapshot semantics live in `runtime/alt`, where they multiplex over
// the in-queue channels exported here. Test ports (the user-extension
// surface) plug in by calling Enqueue on a port from outside the runtime.
package port

import (
	"fmt"
	"sync"
)

// Kind classifies the port shape (clause 14.1).
type Kind int

const (
	Message Kind = iota
	Procedure
	Mixed
)

// String renders the kind in TTCN-3 keyword form.
func (k Kind) String() string {
	switch k {
	case Message:
		return "message"
	case Procedure:
		return "procedure"
	case Mixed:
		return "mixed"
	}
	return "unknown"
}

// Envelope wraps a payload plus the metadata needed to drive
// `from`/`sender` selectors and procedure-call replies. The type is
// called Envelope rather than Message because Message is also the name
// of the Kind constant (clauses 14.1 / 22.5).
type Envelope struct {
	Payload interface{}
	From    *Port

	// Reply is non-nil for procedure-based envelopes. The receiver
	// fulfils a call by sending exactly one value on Reply.
	Reply chan interface{}
}

// Port is the runtime representation of a TTCN-3 port instance. Test
// ports interact via Enqueue (incoming) and Snapshot (drain everything
// queued so far).
//
// The in-queue is a slice + wake channel rather than a buffered Go
// channel. We need true peek semantics (TTCN-3 `receive` with a
// non-matching template leaves the head in place) and a way to remove
// arbitrary entries (matching by selector), neither of which buffered
// channels support. The Wake channel is a 1-deep signal that fires on
// every Enqueue so alt schedulers can `select` until something arrives.
type Port struct {
	Name string
	Kind Kind

	mu     sync.Mutex
	queue  []*Envelope
	wake   chan struct{}
	peers  []*Port
	closed bool
}

// New returns a fresh, unconnected port. Capacity is accepted for API
// compatibility with the older channel-based implementation but is
// ignored: the slice-backed queue grows on demand.
func New(name string, kind Kind, _ int) *Port {
	return &Port{
		Name: name,
		Kind: kind,
		wake: make(chan struct{}, 1),
	}
}

// Connect wires two ports together so a `send` on one delivers to the
// other's in-queue. TTCN-3 `connect` is symmetric and idempotent.
func Connect(a, b *Port) {
	a.addPeer(b)
	b.addPeer(a)
}

// Disconnect removes the symmetric connection. It is a no-op if the
// ports weren't connected to begin with.
func Disconnect(a, b *Port) {
	a.removePeer(b)
	b.removePeer(a)
}

// Send delivers payload to every connected peer. The Kind of the value
// is opaque to the port: the codec layer turns user templates into
// payloads before they reach Send. Procedure calls go through Call,
// which is Send with a Reply channel attached.
func (p *Port) Send(payload interface{}) {
	p.mu.Lock()
	peers := append([]*Port{}, p.peers...)
	p.mu.Unlock()
	for _, peer := range peers {
		peer.deliver(&Envelope{Payload: payload, From: p})
	}
}

// Call is the procedure-based variant of Send. It returns a channel that
// resolves once a peer answers with Reply. If multiple peers are
// connected, the first reply wins; that matches Titan's semantics for
// `getreply` on procedure ports.
func (p *Port) Call(payload interface{}) <-chan interface{} {
	reply := make(chan interface{}, 1)
	p.mu.Lock()
	peers := append([]*Port{}, p.peers...)
	p.mu.Unlock()
	if len(peers) == 0 {
		// No peer means we'll never get a reply. Return the unbuffered
		// channel as-is; callers in an alt branch will simply not match.
		return reply
	}
	for _, peer := range peers {
		peer.deliver(&Envelope{Payload: payload, From: p, Reply: reply})
	}
	return reply
}

// Enqueue is the entry point for external test ports. It bypasses Send
// (which would loop back through peers) and lands the message directly
// in the local in-queue, just as if a peer had sent it.
func (p *Port) Enqueue(payload interface{}) {
	p.deliver(&Envelope{Payload: payload})
}

func (p *Port) deliver(msg *Envelope) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.queue = append(p.queue, msg)
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Receive pops the head of the queue, blocking until something arrives.
// It is the procedural counterpart of `receive` for code paths that
// don't go through the alt scheduler.
func (p *Port) Receive() *Envelope {
	for {
		if env, ok := p.Try(); ok {
			return env
		}
		<-p.wake
	}
}

// Wake exposes the wake-up signal channel for alt's select statement.
// Receiving from Wake means "something was enqueued"; the receiver
// must call Peek / Try to actually inspect the queue.
func (p *Port) Wake() <-chan struct{} { return p.wake }

// Try removes and returns the head of the queue, or (nil, false) if
// the queue is empty.
func (p *Port) Try() (*Envelope, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return nil, false
	}
	env := p.queue[0]
	p.queue = p.queue[1:]
	return env, true
}

// Peek returns the head of the queue without removing it. Returns
// (nil, false) when the queue is empty.
func (p *Port) Peek() (*Envelope, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return nil, false
	}
	return p.queue[0], true
}

// DropHead removes the head of the queue (after a successful Peek+match
// in an alt branch). It's a no-op if the queue is empty.
func (p *Port) DropHead() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) > 0 {
		p.queue = p.queue[1:]
	}
}

// Snapshot returns a copy of the current queue contents without
// modifying it. Used by debuggers and the LSP introspection layer.
func (p *Port) Snapshot() []*Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Envelope, len(p.queue))
	copy(out, p.queue)
	return out
}

// Clear discards all queued messages. Useful for test setup / teardown
// between testcases.
func (p *Port) Clear() {
	p.mu.Lock()
	p.queue = p.queue[:0]
	p.mu.Unlock()
}

// Len reports the current queue depth.
func (p *Port) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

// Close marks the port shut down. Subsequent deliveries are dropped;
// pending messages stay in the queue so consumers can drain them.
func (p *Port) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
}

// Peers returns a snapshot copy of the connected peers, for diagnostics.
func (p *Port) Peers() []*Port {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Port, len(p.peers))
	copy(out, p.peers)
	return out
}

func (p *Port) addPeer(other *Port) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, x := range p.peers {
		if x == other {
			return
		}
	}
	p.peers = append(p.peers, other)
}

func (p *Port) removePeer(other *Port) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, x := range p.peers {
		if x == other {
			p.peers = append(p.peers[:i], p.peers[i+1:]...)
			return
		}
	}
}

// String renders a port as `name (kind, N peers)` for log output.
func (p *Port) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("%s (%s, %d peers)", p.Name, p.Kind, len(p.peers))
}
