// Package tcpport is a built-in TCP test port: it binds a TTCN-3 message
// port to a live TCP endpoint with no user C code and no cgo. Registering
// a port type with a dial address is enough to drive a real system under
// test —
//
//	tcpport.Register("MyPort_PT", "127.0.0.1:9000")
//	// ... run a testcase whose ports are of type MyPort_PT.
//
// then `map(self:p, system:sp)` opens a TCP connection to that endpoint,
// `p.send(cs)` writes the charstring as a newline-terminated frame, and
// each inbound line the SUT sends back is delivered to `p.receive` as a
// charstring. This is the article's "drive a real microservice over the
// network" scenario, realised on the strict engine's real-clock mode.
//
// It is a thin layer over runtime/port/goport (which installs the global
// port-driver provider and pushes inbound traffic into the running
// testcase via Inject); tcpport.Reset restores the previous provider.
//
// Framing is newline-delimited (one charstring per line), the broadly
// compatible default for line/JSON/text protocols and trivially testable
// with a local echo server. A payload that itself contains a newline is
// split across frames — binary/length-prefixed framing and octetstring
// payloads are a deliberate follow-on, not modelled here.
package tcpport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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

// config holds the tunables an Option can set.
type config struct {
	dialTimeout time.Duration
}

// Option configures a registered TCP test port.
type Option func(*config)

// WithDialTimeout overrides the OnMap dial timeout (default 10s).
func WithDialTimeout(d time.Duration) Option {
	return func(c *config) { c.dialTimeout = d }
}

// Register binds a TTCN-3 message port TYPE name (e.g. "MyPort_PT") to a
// built-in TCP test port that dials addr ("host:port") when a port of
// that type is mapped. Each mapped instance opens its own connection.
// Call once per type at startup; the first Register installs the global
// port-driver provider. Defer Reset in tests to restore the previous
// provider.
func Register(portTypeName, addr string, opts ...Option) {
	cfg := config{dialTimeout: defaultDialTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	goport.Register(portTypeName, func(inst string) api.TestPort {
		return newPort(inst, addr, cfg)
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
	inst        string
	addr        string
	dialTimeout time.Duration

	mu     sync.Mutex
	conn   net.Conn
	done   chan struct{} // closed when the read loop exits
	closed bool
}

func newPort(inst, addr string, cfg config) *tcpPort {
	return &tcpPort{
		Base:        api.Base{PortName: "tcp:" + inst},
		inst:        inst,
		addr:        addr,
		dialTimeout: cfg.dialTimeout,
	}
}

// OnMap dials the endpoint and starts the read loop. A dial failure
// surfaces as a map error, so a testcase against a down SUT fails
// honestly at map time rather than silently receiving nothing.
func (p *tcpPort) OnMap(context.Context) error {
	conn, err := net.DialTimeout("tcp", p.addr, p.dialTimeout)
	if err != nil {
		return fmt.Errorf("tcpport %s: dial %s: %w", p.inst, p.addr, err)
	}
	p.mu.Lock()
	p.conn = conn
	p.closed = false
	p.done = make(chan struct{})
	done := p.done
	p.mu.Unlock()
	go p.readLoop(conn, done)
	return nil
}

// readLoop turns each newline-terminated frame from the SUT into a
// charstring and injects it into the running testcase. It exits when the
// connection is closed (by the SUT or by OnUnmap/OnStop) or on any read
// error, closing done so a concurrent close can join it.
func (p *tcpPort) readLoop(conn net.Conn, done chan struct{}) {
	defer close(done)
	r := bufio.NewReader(conn)
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

// Send writes the charstring payload as a newline-terminated frame.
func (p *tcpPort) Send(_ context.Context, env *port.Envelope) error {
	b, err := encode(env.Payload)
	if err != nil {
		return err
	}
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("tcpport %s: send before map", p.inst)
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
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

// encode renders an outgoing TTCN-3 value as bytes. Only charstring is
// modelled for now (octetstring / binary framing is a follow-on).
func encode(payload interface{}) ([]byte, error) {
	obj, ok := payload.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("tcpport: non-runtime payload %T", payload)
	}
	if s, ok := obj.(*runtime.String); ok {
		return []byte(string(s.Value)), nil
	}
	return nil, fmt.Errorf("tcpport: unsupported payload type %T (want charstring)", obj)
}
