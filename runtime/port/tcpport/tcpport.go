// Package tcpport is a built-in TCP test port: it binds a TTCN-3 message
// port to a live TCP endpoint with no user C code and no cgo. Registering
// a port type with a dial address is enough to drive a real system under
// test —
//
//	tcpport.Register("MyPort_PT", "127.0.0.1:9000")
//	// ... run a testcase whose ports are of type MyPort_PT.
//
// then `map(self:p, system:sp)` opens a TCP connection to that endpoint,
// `p.send(v)` writes v as a frame, and each inbound frame the SUT sends
// back is delivered to `p.receive`. This is the article's "drive a real
// microservice over the network" scenario, realised on the strict engine's
// real-clock mode.
//
// It is a thin layer over runtime/port/goport (which installs the global
// port-driver provider and pushes inbound traffic into the running
// testcase via Inject); tcpport.Reset restores the previous provider.
//
// Two framings are supported (WithFraming):
//
//   - FramingNewline (default) — one text record per line; payloads are
//     charstring, written as UTF-8 + '\n' and delivered as charstring with
//     the trailing newline stripped. Broadly compatible with line / JSON /
//     text protocols. A payload containing a newline is split across
//     frames, so use length-prefix framing for binary.
//   - FramingLengthPrefix — a 4-byte big-endian length followed by that
//     many raw bytes; payloads are octetstring, so arbitrary binary
//     (newlines and NULs included) round-trips intact.
//
// A charstring may also be sent under length-prefix framing (its UTF-8
// bytes are the frame) and an octetstring under newline framing (its raw
// octets, then '\n'); the delivered type follows the framing.
package tcpport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
	"github.com/nokia/ntt/runtime/port/goport"
)

// defaultDialTimeout bounds the OnMap dial so a `map` against a
// not-yet-listening SUT fails fast instead of hanging the testcase.
const defaultDialTimeout = 10 * time.Second

// maxFrameLen caps a length-prefixed inbound frame so a hostile or
// desynchronised peer can't make the read loop allocate unbounded memory.
const maxFrameLen = 64 << 20 // 64 MiB

// Framing selects how messages are delimited on the wire.
type Framing int

const (
	// FramingNewline delimits text records with '\n' (charstring payloads).
	FramingNewline Framing = iota
	// FramingLengthPrefix frames each message with a 4-byte big-endian
	// length (octetstring payloads), so binary round-trips intact.
	FramingLengthPrefix
)

// Rule binds one component selector to a dial address and framing for a
// port. At map time the port picks the rule that best matches the mapping
// component (see resolveRule), so different components can drive different
// SUTs through the same port name.
type Rule struct {
	// Component selects which components this rule applies to: "*" (any),
	// "mtc" (the main test component), a component name (from
	// MyComp.create("name")), or a component type name.
	Component   string
	Addr        string        // "host:port" to dial
	Framing     Framing       // wire framing (FramingNewline if zero)
	DialTimeout time.Duration // OnMap dial timeout (defaultDialTimeout if <= 0)
}

// config holds the tunables an Option can set.
type config struct {
	dialTimeout time.Duration
	framing     Framing
}

// Option configures a registered TCP test port.
type Option func(*config)

// WithDialTimeout overrides the OnMap dial timeout (default 10s).
func WithDialTimeout(d time.Duration) Option {
	return func(c *config) { c.dialTimeout = d }
}

// WithFraming selects the wire framing (default FramingNewline).
func WithFraming(f Framing) Option {
	return func(c *config) { c.framing = f }
}

// Register binds a TTCN-3 message port TYPE name (e.g. "MyPort_PT") to a
// built-in TCP test port that dials addr ("host:port") when a port of
// that type is mapped, for any component. Each mapped instance opens its
// own connection. Call once per type at startup; the first Register
// installs the global port-driver provider. Defer Reset in tests to
// restore the previous provider.
func Register(portTypeName, addr string, opts ...Option) {
	cfg := config{dialTimeout: defaultDialTimeout, framing: FramingNewline}
	for _, o := range opts {
		o(&cfg)
	}
	RegisterRules(portTypeName, []Rule{{
		Component:   "*",
		Addr:        addr,
		Framing:     cfg.framing,
		DialTimeout: cfg.dialTimeout,
	}})
}

// RegisterRules binds a port name to component-selective address rules.
// When a port of this name is mapped, the port dials the address of the
// rule whose Component best matches the mapping component — its name, then
// "mtc" for the MTC, then its component type, then "*" — so one port name
// can reach different SUTs on different components. Defer Reset in tests.
func RegisterRules(portName string, rules []Rule) {
	norm := make([]Rule, len(rules))
	for i, r := range rules {
		if r.DialTimeout <= 0 {
			r.DialTimeout = defaultDialTimeout
		}
		norm[i] = r
	}
	goport.Register(portName, func(inst string) api.TestPort {
		return newPort(inst, norm)
	})
}

// Reset clears every registration and restores the port-driver provider
// that was installed before the first Register. Intended for tests.
func Reset() { goport.Reset() }

// tcpPort is a single TCP-backed test-port instance. The runtime calls
// OnMap/Send/OnUnmap/OnStop from the port's owning goroutine (one at a
// time); the read loop runs on its own goroutine and hands inbound frames
// back through goport.Inject, the sanctioned cross-goroutine path.
type tcpPort struct {
	api.Base
	inst  string
	rules []Rule

	mu      sync.Mutex
	conn    net.Conn
	framing Framing       // resolved from the matched rule at OnMap
	done    chan struct{} // closed when the read loop exits
	closed  bool
}

func newPort(inst string, rules []Rule) *tcpPort {
	return &tcpPort{
		Base:  api.Base{PortName: "tcp:" + inst},
		inst:  inst,
		rules: rules,
	}
}

// resolveRule picks the rule that best matches the component doing the
// map, in precedence order: the component's name, "mtc" (for the MTC),
// the component's type name, then "*". OnMap runs on the mapping
// component's goroutine, so CurrentComponent identifies it.
func (p *tcpPort) resolveRule() (Rule, bool) {
	var name, typeName string
	isMTC := true // a nil CurrentComponent is the MTC's main body
	if exec := runtime.CurrentExec(); exec != nil {
		if c := exec.CurrentComponent(); c != nil {
			name, typeName = c.Name, c.TypeName
			isMTC = c.ID == exec.MTCID()
		}
	}
	find := func(sel string) (Rule, bool) {
		if sel == "" {
			return Rule{}, false
		}
		for _, r := range p.rules {
			if r.Component == sel {
				return r, true
			}
		}
		return Rule{}, false
	}
	if r, ok := find(name); ok {
		return r, true
	}
	if isMTC {
		if r, ok := find("mtc"); ok {
			return r, true
		}
	}
	if r, ok := find(typeName); ok {
		return r, true
	}
	return find("*")
}

// OnMap resolves the address for the mapping component, dials it, and
// starts the read loop. A dial failure (or no matching rule) surfaces as a
// map error, so a testcase against a down/misconfigured SUT fails honestly
// at map time rather than silently receiving nothing.
func (p *tcpPort) OnMap(context.Context) error {
	rule, ok := p.resolveRule()
	if !ok {
		return fmt.Errorf("tcpport %s: no address rule matches the mapping component", p.inst)
	}
	conn, err := net.DialTimeout("tcp", rule.Addr, rule.DialTimeout)
	if err != nil {
		return fmt.Errorf("tcpport %s: dial %s: %w", p.inst, rule.Addr, err)
	}
	p.mu.Lock()
	p.conn = conn
	p.closed = false
	p.framing = rule.Framing
	p.done = make(chan struct{})
	done := p.done
	framing := p.framing
	p.mu.Unlock()
	go p.readLoop(conn, done, framing)
	return nil
}

// readLoop turns each inbound frame from the SUT into a TTCN-3 value and
// injects it into the running testcase. It exits when the connection is
// closed (by the SUT or by OnUnmap/OnStop) or on any read error, closing
// done so a concurrent close can join it.
func (p *tcpPort) readLoop(conn net.Conn, done chan struct{}, framing Framing) {
	defer close(done)
	r := bufio.NewReader(conn)
	if framing == FramingLengthPrefix {
		p.readLengthPrefixed(r)
		return
	}
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			frame := bytes.TrimRight(line, "\r\n")
			goport.Inject(p.inst, runtime.NewCharstring(string(frame)))
		}
		if err != nil {
			return
		}
	}
}

// readLengthPrefixed reads 4-byte-BE-length-delimited frames and injects
// each as an octetstring. A length over maxFrameLen (a desynchronised or
// hostile peer) ends the loop rather than allocating unbounded memory.
func (p *tcpPort) readLengthPrefixed(r *bufio.Reader) {
	var hdr [4]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n > maxFrameLen {
			return
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		goport.Inject(p.inst, bytesToOctetstring(buf))
	}
}

// Send writes the payload as one frame in the configured framing.
func (p *tcpPort) Send(_ context.Context, env *port.Envelope) error {
	b, err := encode(env.Payload)
	if err != nil {
		return err
	}
	p.mu.Lock()
	conn := p.conn
	framing := p.framing
	p.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("tcpport %s: send before map", p.inst)
	}
	var frame []byte
	if framing == FramingLengthPrefix {
		frame = make([]byte, 4+len(b))
		binary.BigEndian.PutUint32(frame[:4], uint32(len(b)))
		copy(frame[4:], b)
	} else {
		frame = append(b, '\n')
	}
	if _, err := conn.Write(frame); err != nil {
		return fmt.Errorf("tcpport %s: write: %w", p.inst, err)
	}
	return nil
}

// OnUnmap closes the connection (and joins the read loop).
func (p *tcpPort) OnUnmap(context.Context) error { return p.shutdown() }

// OnStop closes the connection on `p.stop`, mirroring OnUnmap.
func (p *tcpPort) OnStop(context.Context) error { return p.shutdown() }

// shutdown closes the connection and waits for the read loop to exit. It
// is idempotent: a second call (e.g. OnStop then teardown OnUnmap) is a
// no-op.
func (p *tcpPort) shutdown() error {
	p.mu.Lock()
	if p.closed || p.conn == nil {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	conn := p.conn
	done := p.done
	p.conn = nil
	p.mu.Unlock()

	err := conn.Close()
	if done != nil {
		<-done // read loop unblocks on the closed conn and exits
	}
	return err
}

// encode renders an outgoing TTCN-3 value as the frame's bytes: a
// charstring as its UTF-8 bytes, an octetstring as its raw octets.
func encode(payload interface{}) ([]byte, error) {
	obj, ok := payload.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("tcpport: non-runtime payload %T", payload)
	}
	switch v := obj.(type) {
	case *runtime.String:
		return []byte(string(v.Value)), nil
	case *runtime.Binarystring:
		if v.Unit != runtime.Octet {
			return nil, fmt.Errorf("tcpport: %s payload not supported (want charstring or octetstring)", obj.Type())
		}
		return octetstringToBytes(v), nil
	}
	return nil, fmt.Errorf("tcpport: unsupported payload type %T (want charstring or octetstring)", obj)
}

// octetstringToBytes returns the exact octet sequence of an octetstring,
// left-padded to its declared length (Value.Bytes() drops leading zero
// octets, e.g. '00FF'O -> [0xFF], so a raw copy would lose them).
func octetstringToBytes(b *runtime.Binarystring) []byte {
	out := make([]byte, b.Length)
	if b.Value == nil {
		return out
	}
	raw := b.Value.Bytes()
	if len(raw) > len(out) {
		// Defensive: value wider than its declared length — send it whole.
		return raw
	}
	copy(out[len(out)-len(raw):], raw)
	return out
}

// bytesToOctetstring builds an octetstring whose octet sequence is data.
func bytesToOctetstring(data []byte) *runtime.Binarystring {
	return &runtime.Binarystring{
		Value:  new(big.Int).SetBytes(data),
		Unit:   runtime.Octet,
		Length: len(data),
	}
}
