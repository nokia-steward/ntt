package main

import (
	"bufio"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/cfg"
	"github.com/nokia/ntt/runtime/port/httpport"
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

// TestExecConfiguredTCPPort covers config-driven test-port wiring: a
// [TESTPORT_PARAMETERS] block with transport=tcp is enough for a plain
// `ntt exec` to register the built-in TCP port and drive a live SUT — no
// user Go code. It exercises the .cfg -> registerConfiguredTestPorts ->
// live round-trip path over a real socket.
func TestExecConfiguredTCPPort(t *testing.T) {
	addr, stop := startEchoServer(t)
	defer stop()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "t.cfg")
	cfgSrc := fmt.Sprintf("[TESTPORT_PARAMETERS]\n*.p.transport := \"tcp\"\n*.p.host := %q\n*.p.port := %q\n", host, port)
	if err := os.WriteFile(cfgPath, []byte(cfgSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _, err := cfg.Load(cfgPath)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	if n := registerConfiguredTestPorts(f); n != 1 {
		t.Fatalf("registered %d ports, want 1", n)
	}
	t.Cleanup(tcpport.Reset)

	path := writeTC(t, `module m {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send("hi");
			alt {
				[] p.receive("hi") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no reply from configured SUT"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	d := newStaticDriver([]string{path})
	d.live = true // an external transport needs the real clock (as runExec sets)

	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
}

// TestRegisterConfiguredTestPorts_NonTCPIgnored confirms a non-tcp (or
// address-less) transport registers nothing, so unrelated
// [TESTPORT_PARAMETERS] entries are inert.
func TestRegisterConfiguredTestPorts_NonTCPIgnored(t *testing.T) {
	f, _ := cfg.Parse(strings.NewReader("[TESTPORT_PARAMETERS]\n*.p.transport := \"udp\"\n*.q.host := \"h\"\n"))
	t.Cleanup(tcpport.Reset)
	if n := registerConfiguredTestPorts(f); n != 0 {
		t.Fatalf("registered %d ports, want 0 (udp + address-less should be ignored)", n)
	}
}

// startTagServer is a TCP server that replies a fixed tag line to every
// inbound line, so a test can tell which of several SUTs a port reached.
func startTagServer(t *testing.T, tag string) (addr string, hits *int64, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hits = new(int64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(hits, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					if _, err := conn.Write([]byte(tag + "\n")); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), hits, func() { ln.Close(); wg.Wait() }
}

// TestExecPerComponentAddress_MTC covers a per-component override for the
// MTC: `mtc.p.address` wins over the `*.p.address` default, so the MTC's
// port reaches the mtc-specific SUT.
func TestExecPerComponentAddress_MTC(t *testing.T) {
	addrA, hitsA, stopA := startTagServer(t, "A")
	defer stopA()
	addrB, hitsB, stopB := startTagServer(t, "B")
	defer stopB()

	f := loadCfg(t, fmt.Sprintf("[TESTPORT_PARAMETERS]\n*.p.transport := \"tcp\"\n*.p.address := %q\nmtc.p.address := %q\n", addrA, addrB))
	if n := registerConfiguredTestPorts(f); n != 1 {
		t.Fatalf("registered %d ports, want 1", n)
	}
	t.Cleanup(tcpport.Reset)

	// The MTC's rule ("mtc") must win over "*": it reaches server B and the
	// echoed tag it receives is "B".
	path := writeTC(t, `module m {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send("hi");
			alt {
				[] p.receive("B") { setverdict(pass); }
				[] p.receive(charstring:?) -> value v { setverdict(fail, "reached the * SUT, not mtc's"); }
				[] g.timeout { setverdict(fail, "no reply"); }
			}
		}
	}`)
	d := newStaticDriver([]string{path})
	d.live = true
	if v, reason, err := d.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass (mtc rule -> server B)", v, reason, err)
	}
	if atomic.LoadInt64(hitsB) == 0 || atomic.LoadInt64(hitsA) != 0 {
		t.Fatalf("connections: A=%d B=%d, want A=0 B>0 (mtc rule points at B)", *hitsA, *hitsB)
	}
}

// TestExecPerComponentAddress_PTCType covers a per-component override by
// component TYPE: a PTC of type Worker reaches the Worker-specific SUT
// while the `*` default points elsewhere. The proof is which server took
// the connection (verdict propagation from a forked PTC is orthogonal).
func TestExecPerComponentAddress_PTCType(t *testing.T) {
	addrA, hitsA, stopA := startTagServer(t, "A")
	defer stopA()
	addrB, hitsB, stopB := startTagServer(t, "B")
	defer stopB()

	f := loadCfg(t, fmt.Sprintf("[TESTPORT_PARAMETERS]\n*.p.transport := \"tcp\"\n*.p.address := %q\nWorker.p.address := %q\n", addrA, addrB))
	if n := registerConfiguredTestPorts(f); n != 1 {
		t.Fatalf("registered %d ports, want 1", n)
	}
	t.Cleanup(tcpport.Reset)

	path := writeTC(t, `module m {
		type port P message { inout charstring }
		type component C { port P p }
		type component Worker { port P p }
		function work() runs on Worker {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send("hi");
			alt {
				[] p.receive { }
				[] g.timeout { }
			}
			unmap(self:p, system:p);
		}
		testcase tc() runs on C system C {
			var Worker w := Worker.create alive;
			w.start(work());
			w.done;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})
	d.live = true
	if v, reason, err := d.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	// The Worker-typed PTC must dial server B (its type rule), not the *
	// default A.
	if atomic.LoadInt64(hitsB) == 0 || atomic.LoadInt64(hitsA) != 0 {
		t.Fatalf("connections: A=%d B=%d, want A=0 B>0 (Worker rule points at B)", *hitsA, *hitsB)
	}
}

// loadCfg writes src to a temp .cfg and loads it.
func loadCfg(t *testing.T, src string) *cfg.File {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.cfg")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _, err := cfg.Load(p)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	return f
}

// TestExecAllComponentDoneMergesPTCVerdict covers ETSI 21.3.7: `all
// component.done` blocks like the singular `.done`, so under the
// cooperative scheduler each started PTC is granted the token and its body
// actually runs — a failing PTC's verdict must survive, not be lost to the
// MTC racing past a non-blocking snapshot. Runs on the default (virtual /
// coop) path, where the bug lived.
func TestExecAllComponentDoneMergesPTCVerdict(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		function ok() runs on C { setverdict(pass); }
		function bad() runs on C { setverdict(fail, "ptc must be heard"); }
		testcase tc() runs on C system C {
			var C a := C.create alive;
			var C b := C.create alive;
			a.start(ok());
			b.start(bad());
			all component.done;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})
	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != rreport.Fail {
		t.Fatalf("verdict=%s (%s), want fail: a failing PTC's verdict must survive all component.done", v, reason)
	}
}

// TestExampleLiveTestingRuns keeps examples/live-testing/ honest: it runs
// the example's real .ttcn source against an in-process echo SUT and
// checks the shipped .cfg still parses into a TCP port registration. A
// docs example that no longer compiles or passes is worse than none, and
// this catches that in CI without needing an external server or the
// example's fixed port.
func TestExampleLiveTestingRuns(t *testing.T) {
	const src = "examples/live-testing/app.ttcn"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("example not present: %v", err)
	}

	// The shipped cfg must still describe a TCP test port.
	f, _, err := cfg.Load("examples/live-testing/app.cfg")
	if err != nil {
		t.Fatalf("load example cfg: %v", err)
	}
	var sawTCP bool
	for _, p := range f.TestPortParameters() {
		if p.Param == "transport" && strings.EqualFold(p.Value, "tcp") {
			sawTCP = true
		}
	}
	if !sawTCP {
		t.Fatal(`example app.cfg no longer declares transport := "tcp"`)
	}

	// Run the example's own source against an ephemeral echo SUT, so the
	// test never contends for the example's fixed port.
	addr, stop := startEchoServer(t)
	defer stop()
	tcpport.Register("p", addr)
	t.Cleanup(tcpport.Reset)

	d := newStaticDriver([]string{src})
	d.profiling = true
	d.live = true

	for _, tc := range []string{"app.tc_echo", "app.tc_latency_budget"} {
		v, reason, err := d.Run(context.Background(), tc)
		if err != nil || v != rreport.Pass {
			t.Fatalf("%s: verdict=%s reason=%q err=%v, want pass", tc, v, reason, err)
		}
		m := d.LastMetrics()
		if m == nil || len(m.Ports) == 0 || m.Ports[0].Latency.Count == 0 {
			t.Fatalf("%s: expected per-port latency samples, got %+v", tc, m)
		}
	}
}

// TestExecExitStatusReflectsVerdict covers the CI contract: `ntt exec`
// must report a failing suite through a non-zero exit status (runExec
// returning an error), not just in the printed report — otherwise a CI
// pipeline treats a red suite as green. A passing suite must stay silent.
func TestExecExitStatusReflectsVerdict(t *testing.T) {
	// runExec reads package-level flag vars; save and restore them so this
	// test can't leak state into its neighbours.
	oldFormat, oldOut, oldCfg, oldPatterns := execFormat, execOutDir, execCfgPath, execPatterns
	t.Cleanup(func() {
		execFormat, execOutDir, execCfgPath, execPatterns = oldFormat, oldOut, oldCfg, oldPatterns
	})
	execFormat, execOutDir, execCfgPath, execPatterns = "text", "", "", nil

	tc := func(body string) string {
		return `module m {
			type component C {}
			testcase tc() runs on C system C { ` + body + ` }
		}`
	}
	for _, k := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"pass", "setverdict(pass);", false},
		{"none", "", false},
		{"fail", `setverdict(fail, "boom");`, true},
		{"inconc", "setverdict(inconc);", true},
	} {
		t.Run(k.name, func(t *testing.T) {
			execOutDir = t.TempDir() // keep the report out of the test log
			err := runExec(nil, []string{writeTC(t, tc(k.body))})
			if k.wantErr && err == nil {
				t.Fatalf("%s suite: runExec returned nil; want an error so the CLI exits non-zero", k.name)
			}
			if !k.wantErr && err != nil {
				t.Fatalf("%s suite: runExec returned %v; want nil", k.name, err)
			}
		})
	}
}

// TestExecConfiguredHTTPPort covers config-driven HTTP wiring: a
// [TESTPORT_PARAMETERS] block with transport=http is enough for a plain
// `ntt exec` to drive a REST service — no user Go code.
func TestExecConfiguredHTTPPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	f, _ := cfg.Parse(strings.NewReader(fmt.Sprintf(
		"[TESTPORT_PARAMETERS]\n*.p.transport := \"http\"\n*.p.host := %q\n*.p.port := %q\n",
		u.Hostname(), u.Port())))
	if n := registerConfiguredTestPorts(f); n != 1 {
		t.Fatalf("registered %d ports, want 1", n)
	}
	t.Cleanup(httpport.Reset)

	path := writeTC(t, `module m {
		type record HttpRequest  { charstring method, charstring path, charstring body }
		type record HttpResponse { integer status, charstring body }
		type port P message { out HttpRequest; in HttpResponse }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/api/v1/health", body := "" });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := ? }) { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no response from the configured SUT"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	d := newStaticDriver([]string{path})
	d.live = true
	if v, reason, err := d.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
}

// TestExecConfiguredHTTPSPort covers TLS wiring from a .cfg: scheme=https
// plus a ca_cert is enough to verify a real TLS endpoint with no Go code.
func TestExecConfiguredHTTPSPort(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"secure":true}`)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	f, _ := cfg.Parse(strings.NewReader(fmt.Sprintf(
		"[TESTPORT_PARAMETERS]\n*.p.transport := \"http\"\n*.p.scheme := \"https\"\n"+
			"*.p.host := %q\n*.p.port := %q\n*.p.ca_cert := %q\n*.p.server_name := \"example.com\"\n",
		u.Hostname(), u.Port(), ca)))
	if n := registerConfiguredTestPorts(f); n != 1 {
		t.Fatalf("registered %d ports, want 1", n)
	}
	t.Cleanup(httpport.Reset)

	path := writeTC(t, `module m {
		type record HttpRequest  { charstring method, charstring path, charstring body }
		type record HttpResponse { integer status, charstring body }
		type port P message { out HttpRequest; in HttpResponse }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/secure", body := "" });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := ? }) { setverdict(pass); }
				[] p.receive(HttpResponse:{ status := 0, body := ? }) { setverdict(fail, "TLS failed"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	d := newStaticDriver([]string{path})
	d.live = true
	if v, reason, err := d.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
}
