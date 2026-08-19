package main

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/port/tcpport"
	rreport "github.com/nokia/ntt/runtime/report"
)

// startEchoServer spins up a local TCP echo server (bytes back verbatim,
// newlines included) standing in for a live SUT, and returns its address
// plus a stop func that closes the listener and joins its goroutines.
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
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().String(), func() {
		ln.Close()
		wg.Wait()
	}
}

// writeTC writes a TTCN-3 source file to a temp dir and returns its path.
func writeTC(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "m.ttcn")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestExecDeterministic covers the `ntt exec --deterministic` wiring: the
// staticDriver runs the testcase on the strict discrete-event scheduler +
// virtual clock, so a 30s timer fires instantly (virtual time) and the
// verdict is produced without any real-clock wait.
func TestExecDeterministic(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		testcase tc() runs on C system C {
			timer t := 30.0;
			t.start;
			t.timeout;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})

	start := time.Now()
	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != rreport.Pass {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("30s timer took %v; the deterministic virtual clock should fire it instantly", elapsed)
	}
}

// TestExecDeterministicForkedPTC covers concurrent execution under the
// scheduler: an alive PTC forks, runs, and the MTC's blocking comp.done
// parks so the PTC is scheduled — a case the retired approximate engine
// modelled only by skipping the body.
func TestExecDeterministicForkedPTC(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		function f() runs on C { setverdict(pass); }
		testcase tc() runs on C system C {
			var C p := C.create alive;
			p.start(f());
			p.done;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})

	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != rreport.Pass {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestExecLiveUsesRealClock covers the `ntt exec --live` wiring: with
// live=true the staticDriver leaves both deterministic knobs off, so the
// same strict engine runs on the REAL clock — a 0.4s timer takes real wall
// time to fire, whereas the virtual-clock default fires it instantly. This
// is the mode that paces real I/O against a live SUT.
func TestExecLiveUsesRealClock(t *testing.T) {
	src := `module m {
		type component C {}
		testcase tc() runs on C system C {
			timer t := 0.4;
			t.start;
			t.timeout;
			setverdict(pass);
		}
	}`
	path := writeTC(t, src)

	// Virtual-clock default: fires instantly.
	dv := newStaticDriver([]string{path})
	startV := time.Now()
	if v, reason, err := dv.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("virtual Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	if elapsed := time.Since(startV); elapsed > 200*time.Millisecond {
		t.Fatalf("virtual clock took %v for a 0.4s timer; should fire instantly", elapsed)
	}

	// --live: real clock paces the 0.4s timer for real.
	dl := newStaticDriver([]string{path})
	dl.live = true
	startL := time.Now()
	if v, reason, err := dl.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("live Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	if elapsed := time.Since(startL); elapsed < 350*time.Millisecond {
		t.Fatalf("live clock took only %v for a 0.4s timer; the real clock should pace it", elapsed)
	}
}

// TestExecProfileCapturesMetrics covers `ntt exec --profile`: a
// request/response loop against a live TCP SUT yields a per-port profile
// with matching send/receive counts, one latency sample per round trip,
// and a positive throughput. Proves the whole stack — interpreter capture,
// driver aggregation, LastMetrics — end to end over a real socket.
func TestExecProfileCapturesMetrics(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()
	tcpport.Register("P", addr)
	t.Cleanup(tcpport.Reset)

	path := writeTC(t, `module m {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			var integer i := 0;
			while (i < 5) {
				g.start;
				p.send("ping");
				alt {
					[] p.receive("ping") { }
					[] g.timeout { setverdict(fail, "no echo"); }
				}
				i := i + 1;
			}
			setverdict(pass);
			unmap(self:p, system:p);
		}
	}`)
	d := newStaticDriver([]string{path})
	d.profiling = true
	d.live = true

	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	m := d.LastMetrics()
	if m == nil || len(m.Ports) != 1 {
		t.Fatalf("metrics = %+v, want one port", m)
	}
	p := m.Ports[0]
	if p.Port != "p" {
		t.Fatalf("port name = %q, want %q", p.Port, "p")
	}
	if p.Sends != 5 || p.Receives != 5 {
		t.Fatalf("sends=%d receives=%d, want 5/5", p.Sends, p.Receives)
	}
	if p.Latency.Count != 5 {
		t.Fatalf("latency samples = %d, want 5", p.Latency.Count)
	}
	if p.Latency.Min <= 0 || p.Latency.Max < p.Latency.Min {
		t.Fatalf("latency min/max = %s/%s, want a real positive range", p.Latency.Min, p.Latency.Max)
	}
	if p.Throughput <= 0 {
		t.Fatalf("throughput = %f, want > 0", p.Throughput)
	}
}

// TestExecNoProfileByDefault confirms a plain run carries no metrics, so
// the functional path is untouched.
func TestExecNoProfileByDefault(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		testcase tc() runs on C system C { setverdict(pass); }
	}`)
	d := newStaticDriver([]string{path})
	if _, _, err := d.Run(context.Background(), "m.tc"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m := d.LastMetrics(); m != nil {
		t.Fatalf("LastMetrics = %+v, want nil without --profile", m)
	}
}
