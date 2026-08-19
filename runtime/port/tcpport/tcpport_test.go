package tcpport_test

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/tcpport"
	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse error: %v\n%s", tree, src)
	}
	return tree
}

func run(t *testing.T, tc, src string) (runtime.Verdict, string) {
	t.Helper()
	// Zero-value options => real clock + real concurrency: the live-SUT
	// execution mode, which is what a networked port must run under.
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, tc)
	if err != nil {
		t.Fatalf("RunTestcase(%s): %v", tc, err)
	}
	return v, reason
}

// startEchoServer spins up a local TCP server that echoes every byte
// (newlines included) straight back, standing in for a live SUT. It
// returns the dial address and a stop function that closes the listener
// and joins the connection goroutines.
func startEchoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				io.Copy(conn, conn) // echo until the client hangs up
			}()
		}
	}()
	return ln.Addr().String(), func() {
		ln.Close()
		wg.Wait()
	}
}

// TestTCPPort_SendReceiveOverRealSocket is the headline Phase 3 proof: a
// built-in TCP port, registered with a dial address and no user C code,
// carries `p.send` out over a real socket to a live endpoint and a
// `p.receive` back in — `map(self:p, system:sp)` reaches the network.
func TestTCPPort_SendReceiveOverRealSocket(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	tcpport.Register("P", addr)
	t.Cleanup(tcpport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send("ping");
			alt {
				[] p.receive("ping") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no echo from live SUT"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestTCPPort_MultipleFrames proves consecutive sends round-trip in order
// over the same live connection.
func TestTCPPort_MultipleFrames(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()

	tcpport.Register("P", addr)
	t.Cleanup(tcpport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send("one");
			p.send("two");
			alt {
				[] p.receive("one") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no first echo"); }
			}
			alt {
				[] p.receive("two") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no second echo"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestTCPPort_DialFailureIsHonest proves a map against a down SUT fails at
// map time (an Error verdict from the dial error) rather than silently
// receiving nothing — the engine never fabricates a pass.
func TestTCPPort_DialFailureIsHonest(t *testing.T) {
	// Reserve a port, then close it so the address is (almost certainly)
	// not listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	tcpport.Register("P", addr, tcpport.WithDialTimeout(500*time.Millisecond))
	t.Cleanup(tcpport.Reset)

	v, _ := run(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			map(self:p, system:p);
			setverdict(pass);
		}
	}`)
	if v == runtime.PassVerdict {
		t.Fatalf("verdict = pass, want a non-pass (error) verdict: a failed dial must not fabricate success")
	}
}
