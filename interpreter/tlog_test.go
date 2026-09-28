package interpreter_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3"
)

// pingPong exercises most of what the first TCI-TL slice logs: component
// creation and start, a connection, sends, detections, a mismatch and a
// match in an alt, a timer, a verdict, done, and the testcase frame.
const pingPong = `module M {
	type port P message { inout charstring }
	type component C { port P p }
	function echo() runs on C {
		alt {
			[] p.receive("ping") { p.send("pong"); }
		}
	}
	testcase tc() runs on C system C {
		var C e := C.create("echo");
		connect(self:p, e:p);
		e.start(echo());
		p.send("ping");
		timer t := 1.0; t.start;
		alt {
			[] p.receive("nope") { setverdict(fail); }
			[] p.receive("pong") { setverdict(pass, "echoed"); }
			[] t.timeout { setverdict(fail, "no echo"); }
		}
		e.done;
	}
}`

func runLogged(t *testing.T, src string, opts interpreter.TestcaseOptions) (*tl.Recorder, runtime.Verdict) {
	t.Helper()
	rec := &tl.Recorder{}
	opts.TestLogger = rec
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	return rec, v
}

// perComponent splits a log into each component's own sequence of
// operations, the order that is fixed by the test regardless of how the
// components interleave.
func perComponent(rec *tl.Recorder) map[string][]string {
	out := map[string][]string{}
	for _, e := range rec.Events {
		out[e.C.Name] = append(out[e.C.Name], e.Op)
	}
	return out
}

func TestTestLog_EventsAreValidAndInOrder(t *testing.T) {
	rec, _ := runLogged(t, pingPong, clocks[0].opts)
	for _, e := range rec.Events {
		if err := e.Validate(); err != nil {
			t.Errorf("%v", err)
		}
	}
	got := perComponent(rec)
	want := map[string][]string{
		"mtc": {
			"tliTcStart", "tliTcStarted", "tliCCreate", "tliPConnect", "tliCStart",
			"tliMSend_c", "tliTStart", "tliAEnter", "tliANomatch", "tliAWait",
			"tliMDetected_c", "tliMMismatch_c", "tliMReceive_c", "tliSetVerdict",
			"tliALeave", "tliCDone", "tliTcTerminated",
		},
		"echo": {
			"tliMDetected_c", "tliAEnter", "tliMReceive_c", "tliMSend_c", "tliALeave", "tliCTerminated",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("per-component operations:\n got %v\nwant %v", got, want)
	}
}

// TestTestLog_SameOperationsOnBothClocks is the property the log exists to
// check: a testcase performs the same operations on the virtual clock and
// on the real clock.
//
// Two things legitimately differ, and the comparison leaves them out.
// Components interleave differently in real time, so it is per component.
// And some events record when an arrival happened relative to what the
// component was doing, not what it did: on the real clock the echo PTC
// enters its alt before the request arrives and waits, on the virtual one
// the request is already queued. Those are the arrival (tliMDetected), an
// alt round that found nothing (tliANomatch, tliADefaults, tliAWait), and a
// mismatch, which depends on which message was at the head.
func TestTestLog_SameOperationsOnBothClocks(t *testing.T) {
	virtual, _ := runLogged(t, pingPong, clocks[0].opts)
	live, _ := runLogged(t, pingPong, clocks[1].opts)
	v, l := componentActions(virtual), componentActions(live)
	if !reflect.DeepEqual(v, l) {
		t.Fatalf("the clocks performed different operations:\nvirtual %v\n   live %v", v, l)
	}
}

// componentActions is perComponent without the events that depend on the
// timing of arrivals.
func componentActions(rec *tl.Recorder) map[string][]string {
	out := map[string][]string{}
	for c, ops := range perComponent(rec) {
		for _, op := range ops {
			switch {
			case strings.HasPrefix(op, "tliMDetected"), strings.HasPrefix(op, "tliMMismatch"),
				op == "tliANomatch", op == "tliADefaults", op == "tliAWait":
				continue
			}
			out[c] = append(out[c], op)
		}
	}
	return out
}

func TestTestLog_EventContent(t *testing.T) {
	rec, _ := runLogged(t, pingPong, clocks[0].opts)
	find := func(comp, op string) *tl.Event {
		for _, e := range rec.Events {
			if e.C.Name == comp && e.Op == op {
				return e
			}
		}
		t.Fatalf("no %s from %s", op, comp)
		return nil
	}
	if e := find("mtc", "tliMSend_c"); e.Line != 13 {
		t.Errorf("send at line %d, want 13", e.Line)
	}
	if e := find("mtc", "tliSetVerdict"); !hasArg(e, "reason") {
		t.Errorf("setverdict carries no reason: %+v", e)
	}
	if e := find("mtc", "tliCCreate"); e.C.Type != "C" {
		t.Errorf("creating component type = %q, want C", e.C.Type)
	}
	for _, e := range rec.Events {
		if e.Ts == 0 {
			t.Fatalf("%s has no timestamp", e.Op)
		}
	}
}

func hasArg(e *tl.Event, name string) bool {
	for _, a := range e.Args {
		if a.Name == name {
			return true
		}
	}
	return false
}

// TestTestLog_ValidatesAgainstAnnexB writes a real run as XML and validates
// it against the normative schemas; see runtime/tl/gen/README.md.
func TestTestLog_ValidatesAgainstAnnexB(t *testing.T) {
	dir := os.Getenv("NTT_TCI_TL_XSD")
	if dir == "" {
		t.Skip("NTT_TCI_TL_XSD not set; see runtime/tl/gen/README.md")
	}
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed")
	}
	var buf bytes.Buffer
	w := tl.NewXMLWriter(&buf)
	for _, k := range clocks {
		opts := k.opts
		opts.TestLogger = w
		if _, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, pingPong)}, "M.tc", opts); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "run.xml")
	if err := os.WriteFile(f, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(xmllint, "--noout", "--schema", filepath.Join(dir, "Log_v4_10_1.xsd"), f).CombinedOutput(); err != nil {
		t.Fatalf("log does not validate:\n%s", out)
	}
}

// logOps runs M.tc with a recording logger and returns the verdict and the
// logged operations.
func logOps(t *testing.T, src string, opts interpreter.TestcaseOptions) (runtime.Verdict, string, *tl.Recorder) {
	t.Helper()
	rec := &tl.Recorder{}
	opts.TestLogger = rec
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	return v, reason, rec
}

func count(rec *tl.Recorder, op string) int {
	n := 0
	for _, o := range rec.Ops() {
		if o == op {
			n++
		}
	}
	return n
}

// TestTestLog_DoesNotReevaluateClauses: logging a send's `to` and a
// receive's `from` must use the values the engine evaluated. Evaluating
// the clauses again ran their side effects once more with logging on than
// off. (The engine itself evaluates a `to` clause twice, logging or not;
// that is recorded as a separate defect.)
func TestTestLog_DoesNotReevaluateClauses(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p; var integer calls := 0 }
		function peer() runs on C return C { calls := calls + 1; return self; }
		testcase tc() runs on C system C {
			connect(self:p, self:p);
			p.send(1) to peer();
			timer t := 1.0; t.start;
			alt {
				[] p.receive(1) from peer() {}
				[] t.timeout {}
			}
			setverdict(pass, "calls ", calls);
		}
	}`
	for _, k := range clocks {
		_, off, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, on, _ := logOps(t, src, k.opts); on != off {
			t.Errorf("%s clock: logging off gives %q, on gives %q", k.name, off, on)
		}
	}
}

// TestTestLog_MismatchOncePerMessage: a mismatch is logged once per message
// and receiving operation, however often an alt re-checks it, and every
// head a trigger discards is logged.
func TestTestLog_MismatchOncePerMessage(t *testing.T) {
	t.Run("two clauses, polled", func(t *testing.T) {
		src := `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(3);
				timer t := 0.3; t.start;
				alt {
					[] p.receive(1) { setverdict(fail); }
					[] p.receive(2) { setverdict(fail); }
					[] t.timeout { setverdict(pass); }
				}
			}
		}`
		for _, k := range clocks {
			_, _, rec := logOps(t, src, k.opts)
			if n := count(rec, "tliMMismatch_c"); n != 2 {
				t.Errorf("%s clock: %d mismatches logged, want 2 (one per clause)", k.name, n)
			}
		}
	})
	t.Run("trigger discards", func(t *testing.T) {
		src := `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(1); p.send(1); p.send(2);
				timer t := 0.5; t.start;
				alt {
					[] p.trigger(2) { setverdict(pass); }
					[] t.timeout { setverdict(fail, "no 2"); }
				}
			}
		}`
		_, _, rec := logOps(t, src, clocks[0].opts)
		if n := count(rec, "tliMMismatch_c"); n != 2 {
			t.Errorf("%d mismatches logged, want 2: each discarded head is an event", n)
		}
	})
	t.Run("each testcase", func(t *testing.T) {
		src := `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(3);
				alt {
					[] p.receive(1) { setverdict(fail); }
					[else] { setverdict(pass); }
				}
			}
		}`
		rec := &tl.Recorder{}
		opts := clocks[0].opts
		opts.TestLogger = rec
		for i := 0; i < 2; i++ {
			if _, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts); err != nil {
				t.Fatal(err)
			}
		}
		if n := count(rec, "tliMMismatch_c"); n != 2 {
			t.Errorf("%d mismatches over two runs, want one each", n)
		}
	})
}

// TestTestLog_TerminatedCarriesTheFinalVerdict: a testcase cut off by its
// execute() timeout logs the stop and the error it ends with, not the pass
// it had set, and a timer the stop interrupted did not time out.
func TestTestLog_TerminatedCarriesTheFinalVerdict(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C system C {
			setverdict(pass);
			timer t := 3.0; t.start; t.timeout;
		}
		control { execute(tc(), 0.2); }
	}`
	rec := &tl.Recorder{}
	v, _, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", interpreter.TestcaseOptions{TestLogger: rec})
	if err != nil || v != runtime.ErrorVerdict {
		t.Fatalf("control verdict = %s, err %v; want error", v, err)
	}
	var term *tl.Event
	for _, e := range rec.Events {
		if e.Op == "tliTcTerminated" {
			term = e
		}
	}
	if term == nil || count(rec, "tliTcStop") != 1 {
		t.Fatalf("want one tliTcStop and a tliTcTerminated, got %v", rec.Ops())
	}
	for _, a := range term.Args {
		if a.Name == "verdict" && a.Val.Kids[0].Text != "error" {
			t.Errorf("tliTcTerminated verdict = %s, want error", a.Val.Kids[0].Text)
		}
	}
	if n := count(rec, "tliTTimeout"); n != 0 {
		t.Errorf("an interrupted timer logged %d timeouts", n)
	}
}

// TestTestLog_AnyPortAndPortArrays covers two receive and send paths the
// first version did not log fully: `any port.receive`, and a send on an
// element of a connected port array, whose peer is the same element.
func TestTestLog_AnyPortAndPortArrays(t *testing.T) {
	_, _, rec := logOps(t, `module M {
		type port P message { inout integer }
		type component C { port P p[2] }
		testcase tc() runs on C system C {
			var C a := C.create;
			connect(a:p[0], self:p[0]);
			a.start(sends());
			timer t := 1.0; t.start;
			alt {
				[] any port.receive { setverdict(pass); }
				[] t.timeout { setverdict(fail); }
			}
			a.done;
		}
		function sends() runs on C { p[0].send(7); }
	}`, clocks[0].opts)
	if count(rec, "tliMReceive_c") != 1 {
		t.Errorf("any port.receive not logged: %v", rec.Ops())
	}
	for _, e := range rec.Events {
		if e.Op == "tliMSend_c" && !hasArg(e, "to") {
			t.Errorf("send on a connected port-array element has no `to`")
		}
	}
}
