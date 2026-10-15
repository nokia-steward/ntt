// Package mctr implements the master controller: it accepts host
// controller connections, dispatches Run requests, and aggregates the
// Verdicts that come back. The master is intentionally protocol-only
// at this stage: scheduling policy (which case goes to which host),
// failover, and live logging streaming are layered on top of the small
// primitives here.
package mctr

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/runtime/wire"
)

// Host is the master's view of one connected host controller.
type Host struct {
	Name    string
	Cases   []string
	enc     *wire.Encoder
	dec     *wire.Decoder
	results chan wire.Verdict
}

// Master holds the suite-wide state across connected hosts.
type Master struct {
	mu       sync.Mutex
	hosts    []*Host
	listener net.Listener
}

// NewMaster constructs an empty master. Call Listen to accept hosts,
// then Run to dispatch testcases.
func NewMaster() *Master { return &Master{} }

// Listen binds addr and starts accepting host connections in the
// background. Returns the actual address the listener bound to so
// tests can use ":0" and discover the chosen port.
func (m *Master) Listen(addr string) (string, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	m.listener = l
	go m.acceptLoop()
	return l.Addr().String(), nil
}

func (m *Master) acceptLoop() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		go m.handleConn(conn)
	}
}

func (m *Master) handleConn(c net.Conn) {
	enc := wire.NewEncoder(c)
	dec := wire.NewDecoder(c)
	first, err := dec.Decode()
	if err != nil || first.Kind != wire.HelloKind {
		c.Close()
		return
	}
	var hello wire.Hello
	if err := first.UnmarshalBody(&hello); err != nil {
		c.Close()
		return
	}
	h := &Host{
		Name:    hello.Host,
		Cases:   hello.Cases,
		enc:     enc,
		dec:     dec,
		results: make(chan wire.Verdict, 16),
	}
	m.mu.Lock()
	m.hosts = append(m.hosts, h)
	m.mu.Unlock()

	_ = enc.EncodeBody(wire.ReadyKind, "", wire.Ready{Session: "ntt"})

	for {
		msg, err := dec.Decode()
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		switch msg.Kind {
		case wire.VerdictKind:
			var v wire.Verdict
			if msg.UnmarshalBody(&v) == nil {
				h.results <- v
			}
		case wire.GoodbyeKind:
			return
		}
	}
}

// Run dispatches one testcase to the first host that advertises it
// and waits up to timeout for the Verdict to come back. If no host
// advertises the case, Run returns Error.
func (m *Master) Run(ctx context.Context, caseName string, timeout time.Duration) report.Verdict {
	m.mu.Lock()
	var target *Host
	for _, h := range m.hosts {
		for _, c := range h.Cases {
			if c == caseName {
				target = h
				break
			}
		}
		if target != nil {
			break
		}
	}
	m.mu.Unlock()

	if target == nil {
		return report.Error
	}

	if err := target.enc.EncodeBody(wire.RunKind, "", wire.Run{Case: caseName}); err != nil {
		return report.Error
	}
	select {
	case v := <-target.results:
		verdict, err := report.VerdictFromString(v.Verdict)
		if err != nil {
			return report.Error
		}
		return verdict
	case <-time.After(timeout):
		return report.Error
	case <-ctx.Done():
		return report.Error
	}
}

// Hosts returns the list of connected hosts. Safe to call concurrently.
func (m *Master) Hosts() []*Host {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Host, len(m.hosts))
	copy(out, m.hosts)
	return out
}

// Close shuts the listener down. Connected hosts learn about it via
// their pending Decode calls returning EOF.
func (m *Master) Close() error {
	if m.listener == nil {
		return nil
	}
	return m.listener.Close()
}

// WaitForHosts blocks until at least n host controllers have
// connected or ctx is cancelled. Useful in tests and in single-shot
// CLI invocations where the operator knows how many hosts to expect.
func (m *Master) WaitForHosts(ctx context.Context, n int) error {
	for {
		m.mu.Lock()
		got := len(m.hosts)
		m.mu.Unlock()
		if got >= n {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("only %d/%d hosts connected", got, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
