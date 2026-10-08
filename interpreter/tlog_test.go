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
			"tliTcStart", "tliTcStarted", "tliSEnter", "tliCCreate", "tliPConnect", "tliCStart",
			"tliMSend_c", "tliTStart", "tliAEnter", "tliTTimeoutMismatch", "tliANomatch", "tliAWait",
			"tliMDetected_c", "tliMMismatch_c", "tliMReceive_c", "tliSetVerdict",
			"tliALeave", "tliCDone", "tliSLeave", "tliTcTerminated",
		},
		"echo": {
			"tliMDetected_c", "tliSEnter", "tliAEnter", "tliMReceive_c", "tliMSend_c", "tliALeave", "tliSLeave", "tliCTerminated",
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
			case strings.Contains(op, "Detected"), strings.Contains(op, "Mismatch"),
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
		for _, src := range []string{pingPong, procedureExchange, everyOperation} {
			if _, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, _, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, `module M {
		type component C {}
		testcase a() runs on C system C { setverdict(pass); }
		control { execute(a(), 2.0); }
	}`)}, "M", interpreter.TestcaseOptions{TestLogger: w}); err != nil {
		t.Fatal(err)
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

// procedureExchange is a server answering calls with a reply or an
// exception, and a caller using both the blocking and the nowait form.
const procedureExchange = `module M {
	signature Add(in integer a, in integer b) return integer exception (charstring);
	type port PP procedure { inout Add }
	type component C { port PP p }
	function server() runs on C {
		var integer x, y;
		alt {
			[] p.getcall(Add:{a := ?, b := ?}) -> param (x, y) {
				if (y == 0) { p.raise(Add, "b is zero"); } else { p.reply(Add:{a := x, b := y} value x + y); }
				repeat;
			}
		}
	}
	testcase tc() runs on C system C {
		var C s := C.create("server");
		connect(self:p, s:p);
		s.start(server());
		p.call(Add:{a := 2, b := 3}, 1.0) {
			[] p.getreply(Add:{a := ?, b := ?} value 5) { setverdict(pass); }
			[] p.catch(timeout) { setverdict(fail, "timeout"); }
		}
		p.call(Add:{a := 1, b := 0}, nowait);
		timer t := 1.0; t.start;
		alt {
			[] p.catch(Add, "b is zero") {}
			[] t.timeout { setverdict(fail, "no exception"); }
		}
		s.stop;
	}
}`

// TestTestLog_ProcedureCommunication covers the tliPr operations: each
// call, reply and raise, its arrival, and the getcall, getreply or catch
// that took it, in cause-and-effect order, the same on both clocks.
func TestTestLog_ProcedureCommunication(t *testing.T) {
	want := map[string][]string{
		"mtc":    {"tliPrCall_c", "tliPrGetReply_c", "tliPrCall_c", "tliPrCatch_c"},
		"server": {"tliPrGetCall_c", "tliPrReply_c", "tliPrGetCall_c", "tliPrRaise_c"},
	}
	for _, k := range clocks {
		rec, _ := runLogged(t, procedureExchange, k.opts)
		got := map[string][]string{}
		for _, e := range rec.Events {
			if err := e.Validate(); err != nil {
				t.Errorf("%s clock: %v", k.name, err)
			}
			if strings.HasPrefix(e.Op, "tliPr") && !strings.Contains(e.Op, "Detected") && !strings.Contains(e.Op, "Mismatch") {
				got[e.C.Name] = append(got[e.C.Name], e.Op)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s clock: procedure operations\n got %v\nwant %v", k.name, got, want)
		}
		// Each call is logged before its arrival at the server.
		call, arrived := -1, -1
		for i, e := range rec.Events {
			if e.Op == "tliPrCall_c" && call < 0 {
				call = i
			}
			if e.Op == "tliPrGetCallDetected_c" && arrived < 0 {
				arrived = i
			}
		}
		if call < 0 || arrived < 0 || call > arrived {
			t.Errorf("%s clock: call at %d, its arrival at %d", k.name, call, arrived)
		}
	}
}

func TestTestLog_CatchTimeout(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		signature S();
		type port PP procedure { inout S }
		type component C { port PP p }
		testcase tc() runs on C system C {
			connect(self:p, self:p);
			p.call(S:{}, 0.2) {
				[] p.getreply(S:{}) { setverdict(fail, "no one replies"); }
				[] p.catch(timeout) { setverdict(pass); }
			}
		}
	}`, clocks[0].opts)
	if count(rec, "tliPrCatchTimeout") != 1 {
		t.Fatalf("no tliPrCatchTimeout in %v", rec.Ops())
	}
}

// TestTestLog_ControlPart covers a control part: its start and end frame
// the testcases it executes, each announced by tliTcExecute with its
// timeout when execute() gives one.
func TestTestLog_ControlPart(t *testing.T) {
	rec := &tl.Recorder{}
	_, _, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, `module M {
		type component C {}
		testcase a() runs on C system C { setverdict(pass); }
		testcase b(integer n) runs on C system C { setverdict(pass); }
		control {
			execute(a());
			execute(b(3), 5.0);
		}
	}`)}, "M", interpreter.TestcaseOptions{TestLogger: rec})
	if err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, e := range rec.Events {
		if err := e.Validate(); err != nil {
			t.Error(err)
		}
		switch e.Op {
		case "tliCtrlStart", "tliCtrlTerminated", "tliTcExecute", "tliTcStart", "tliTcTerminated":
			ops = append(ops, e.Op)
		}
	}
	want := []string{"tliCtrlStart", "tliTcExecute", "tliTcStart", "tliTcTerminated", "tliTcExecute", "tliTcStart", "tliTcTerminated", "tliCtrlTerminated"}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("control part:\n got %v\nwant %v", ops, want)
	}
	var second *tl.Event
	for _, e := range rec.Events {
		if e.Op == "tliTcExecute" {
			second = e
		}
	}
	if !hasArg(second, "dur") {
		t.Errorf("execute(b(3), 5.0) logged no duration")
	}
}

// panickingLogger fails on every event.
type panickingLogger struct{}

func (panickingLogger) Log(*tl.Event) { panic("logger failed") }

// TestTestLog_NeverChangesTheVerdict: whatever goes wrong in the logging —
// here, a logger that fails on every event — the test runs as it does with
// logging off. A failure used to surface as an interpreter panic, turning
// the testcase into an error.
func TestTestLog_NeverChangesTheVerdict(t *testing.T) {
	for _, src := range []string{pingPong, procedureExchange, everyOperation} {
		for _, k := range clocks {
			off, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil {
				t.Fatal(err)
			}
			opts := k.opts
			opts.TestLogger = panickingLogger{}
			on, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
			if err != nil || on != off {
				t.Errorf("%s clock: verdict %s (%s) with a failing logger, %s without", k.name, on, reason, off)
			}
		}
	}
}

// TestTestLog_ReceiveWithoutTemplate: a getcall or getreply with no
// template (`p.getcall;`) has no call node, which the position lookup must
// treat as absent rather than dereference.
func TestTestLog_ReceiveWithoutTemplate(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p }
		function f() runs on C { p.getcall; p.reply(S:{}); }
		testcase tc() runs on C system C {
			var C c := C.create;
			connect(self:p, c:p);
			p.call(S:{}, nowait);
			c.start(f());
			c.done;
			alt {
				[] p.getreply { setverdict(pass); }
				[else] { setverdict(fail, "no reply"); }
			}
		}
	}`, clocks[0].opts)
	for _, e := range rec.Events {
		if e.Op == "tliInfo" {
			t.Errorf("logging failed: %+v", e.Args)
		}
	}
	if count(rec, "tliPrGetCall_c") != 1 || count(rec, "tliPrGetReply_c") != 1 {
		t.Errorf("template-less getcall / getreply not logged: %v", rec.Ops())
	}
}

// TestTestLog_ProcedureForms covers procedure paths beyond a plain
// call and getcall: a port mapped to the system, `any from` over a port
// array, `any port`, and a port connected to itself.
func TestTestLog_ProcedureForms(t *testing.T) {
	t.Run("mapped port", func(t *testing.T) {
		rec, _ := runLogged(t, `module M {
			signature S();
			type port P procedure { inout S }
			type component C { port P p }
			testcase tc() runs on C system C {
				map(self:p, system:p);
				p.call(S:{}, nowait);
				setverdict(pass);
			}
		}`, clocks[0].opts)
		if count(rec, "tliPrCall_m") != 1 || count(rec, "tliPrCall_c") != 0 {
			t.Errorf("a call on a mapped port is not logged as tliPrCall_m: %v", rec.Ops())
		}
	})
	t.Run("any from a port array", func(t *testing.T) {
		rec, _ := runLogged(t, `module M {
			signature S() return integer;
			type port P procedure { inout S }
			type component C { port P p[2] }
			function f() runs on C { p[1].getcall; p[1].reply(S:{} value 1); }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p[0], c:p[0]);
				connect(self:p[1], c:p[1]);
				p[1].call(S:{}, nowait);
				c.start(f());
				c.done;
				alt {
					[] any from p.getreply(S:{}) { setverdict(pass); }
					[else] { setverdict(fail, "no reply"); }
				}
			}
		}`, clocks[0].opts)
		if n := count(rec, "tliPrGetReply_c"); n != 1 {
			t.Errorf("any from p.getreply logged %d receives: %v", n, rec.Ops())
		}
	})
	t.Run("connected to itself", func(t *testing.T) {
		rec, _ := runLogged(t, `module M {
			signature S();
			type port P procedure { inout S }
			type component C { port P q }
			testcase tc() runs on C system C {
				connect(self:q, self:q);
				q.call(S:{}, nowait);
				q.getcall(S:{});
				setverdict(pass);
			}
		}`, clocks[0].opts)
		if n := count(rec, "tliPrGetCall_c"); n != 1 {
			t.Errorf("getcall logged %d receives: %v", n, rec.Ops())
		}
		for _, e := range rec.Events {
			if e.Op == "tliPrCall_c" && !hasArg(e, "to") {
				t.Errorf("a call on a port connected to itself names no destination")
			}
		}
	})
}

// TestTestLog_TimestampsNeverGoBack: under the virtual clock a testcase's
// events run ahead of the wall clock by the virtual time it used. The next
// testcase, and the control part around them, must not appear to happen
// before it.
func TestTestLog_TimestampsNeverGoBack(t *testing.T) {
	rec := &tl.Recorder{}
	opts := clocks[0].opts
	opts.TestLogger = rec
	if _, _, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, `module M {
		type component C {}
		testcase a() runs on C system C { timer w := 5.0; w.start; w.timeout; setverdict(pass); }
		control { execute(a()); execute(a()); }
	}`)}, "M", opts); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(rec.Events); i++ {
		if rec.Events[i].Ts < rec.Events[i-1].Ts {
			t.Fatalf("%s at %d comes after %s at %d", rec.Events[i].Op, rec.Events[i].Ts, rec.Events[i-1].Op, rec.Events[i-1].Ts)
		}
	}
}

// TestTestLog_ClocksCompareTheSame is the comparison the logs are for: the
// same testcases run on the virtual and the real clock, logged in the two
// formats, read back and compared, do the same.
func TestTestLog_ClocksCompareTheSame(t *testing.T) {
	logs := make([]*tl.Log, 2)
	for i, k := range clocks {
		var buf bytes.Buffer
		w := tl.NewJSONLWriter(&buf)
		if i == 1 {
			w = tl.NewXMLWriter(&buf)
		}
		opts := k.opts
		opts.TestLogger = w
		for _, src := range []string{pingPong, procedureExchange, everyOperation} {
			if _, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		l, err := tl.ReadLog(&buf)
		if err != nil {
			t.Fatal(err)
		}
		logs[i] = l
	}
	res := tl.Compare(logs[0], logs[1])
	if len(res) != 3 {
		t.Fatalf("%d testcases compared, want 3", len(res))
	}
	for _, r := range res {
		if !r.Same() {
			for _, d := range r.Differences {
				t.Errorf("%s: %s differs:\n virtual %s\n    live %s", r.Testcase, d.Component, summary(d.A), summary(d.B))
			}
		}
		if r.Compared < 10 {
			t.Errorf("%s: only %d events compared", r.Testcase, r.Compared)
		}
	}
}

func summary(n *tl.Node) string {
	if n == nil {
		return "(none)"
	}
	return tl.Summary(n)
}

// everyOperation performs, once each, the operations the second TCI-TL
// slice logs: function entry and exit with parameters and a result, a
// @lazy parameter evaluated, assignments, a module parameter read, rnd,
// match both ways, a codec round trip, component and port state, the
// Mismatch forms of done and timeout, a check with no receiving operation
// and one whose from clause fails, and a multicast to system addresses.
const everyOperation = `module M {
	modulepar integer LIMIT := 3;
	type port P message { inout integer, charstring } with { extension "address" }
	type component C { port P p; port P q; timer t_never := 100.0 }
	type record R { integer a, integer b }
	function twice(in integer x, @lazy integer y) return integer {
		return x + y + y;
	}
	function idle() runs on C { timer w := 0.5; w.start; w.timeout; }
	testcase tc() runs on C system C {
		var integer v := 0;
		v := twice(LIMIT, 2);
		var R r := { a := 1, b := 2 };
		r.b := 5;
		var float f := rnd(1.5);
		if (not match(r, R:{ a := 1, b := 5 })) { setverdict(fail, "match"); }
		if (match(v, (0..2))) { setverdict(fail, "range"); }
		var bitstring enc := encvalue(r);
		var R back;
		if (decvalue(enc, back) != 0) { setverdict(fail, "decode"); }
		var C c := C.create("idler");
		if (c.running or not c.alive) { setverdict(fail, "fresh ptc"); }
		c.start(idle());
		connect(self:q, self:q);
		q.stop;
		q.start;
		q.send(1);
		q.clear;
		q.halt;
		q.start;
		q.send(2);
		t_never.start;
		alt {
			[] c.done { setverdict(fail, "not yet"); }
			[] t_never.timeout { setverdict(fail, "never"); }
			[] q.check(from c) { setverdict(fail, "from mtc"); }
			[] q.check { setverdict(pass); }
		}
		c.done;
		timer t := 0.1; t.start; t.timeout;
		map(self:p, system:p);
		p.send(7) to (1, 2);
	}
}`

func TestTestLog_EveryOperation(t *testing.T) {
	want := []string{
		"tliSEnter", "tliSLeave", "tliEvaluate", "tliVar", "tliModulePar", "tliRnd",
		"tliMatch", "tliMatchMismatch", "tliEncode", "tliDecode", "tliCRunning", "tliCAlive",
		"tliPStart", "tliPStop", "tliPHalt", "tliPClear", "tliCDoneMismatch",
		"tliTTimeoutMismatch", "tliCheckAnyMismatch_c", "tliCheckedAny_c",
		"tliTTimeoutDetected", "tliTTimeout", "tliMSend_m_MC",
	}
	for _, k := range clocks {
		rec, _ := runLogged(t, everyOperation, k.opts)
		for _, e := range rec.Events {
			if err := e.Validate(); err != nil {
				t.Errorf("%s clock: %v", k.name, err)
			}
			if e.Op == "tliInfo" {
				t.Errorf("%s clock: %s", k.name, e.Summary())
			}
		}
		for _, op := range want {
			if count(rec, op) == 0 {
				t.Errorf("%s clock: no %s in %v", k.name, op, rec.Ops())
			}
		}
	}
}

// TestTestLog_DestinationsInOneOrder: the connection graph is a map; a
// broadcast's and a multicast's destinations are still logged in one
// order, so two runs of a testcase log them alike.
func TestTestLog_DestinationsInOneOrder(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function sink() runs on C { timer w := 1.0; w.start; alt { [] p.receive { repeat; } [] w.timeout {} } }
		testcase tc() runs on C system C {
			var C a := C.create("a"), b := C.create("b"), c := C.create("c"), d := C.create("d");
			connect(self:p, a:p); connect(self:p, b:p); connect(self:p, c:p); connect(self:p, d:p);
			p.send(1) to all component;
			p.send(2) to (a, b, c);
			setverdict(pass);
		}
	}`
	var first []string
	for i := 0; i < 8; i++ {
		_, _, rec := logOps(t, src, clocks[0].opts)
		var got []string
		for _, e := range rec.Events {
			if strings.HasPrefix(e.Op, "tliMSend_c_") {
				got = append(got, e.Summary())
			}
		}
		if len(got) != 2 {
			t.Fatalf("sends: %v", rec.Ops())
		}
		if first == nil {
			first = got
		} else if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d logged\n%v\nfirst\n%v", i, got, first)
		}
	}
}

// TestTestLog_ComponentAndPortOperations: the component and port fixes
// behave alike with logging on.
func TestTestLog_ComponentAndPortOperations(t *testing.T) {
	for name, src := range componentAndPortOps {
		for _, k := range clocks {
			opts := k.opts
			opts.TestLogger = &tl.Recorder{}
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestTestLog_ImportedDefinitions: a function and module parameters are
// logged with the module that declares them, not the one importing them.
func TestTestLog_ImportedDefinitions(t *testing.T) {
	a := parse(t, `module A {
		modulepar integer mpA := 7;
		modulepar { integer mpG := 1 }
		function getA() return integer { return mpA; }
	}`)
	m := parse(t, `module M {
		import from A all;
		type component C {}
		testcase tc() runs on C {
			var integer v := mpG + getA();
			if (v == 8) { setverdict(pass) }
		}
	}`)
	rec := &tl.Recorder{}
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{m, a}, "M.tc", interpreter.TestcaseOptions{TestLogger: rec})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("%s (%s) %v", v, reason, err)
	}
	var got []string
	for _, e := range rec.Events {
		if e.Op != "tliModulePar" && e.Op != "tliSEnter" {
			continue
		}
		for _, a := range e.Args {
			if a.Name == "name" {
				got = append(got, e.Op+" "+a.Val.Attrs[0].Value+"."+a.Val.Attrs[1].Value)
			}
		}
	}
	want := []string{"tliSEnter M.tc", "tliModulePar A.mpG", "tliSEnter A.getA", "tliModulePar A.mpA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

// TestTestLog_WaitingAltLogsOnce: an alt that waits scans its guards and
// defaults again and again — on the real clock every few milliseconds. What
// a scan repeats is logged once per alt round, so both clocks log the same.
func TestTestLog_WaitingAltLogsOnce(t *testing.T) {
	src := `module M {
		type component C { timer tg := 5.0 }
		modulepar boolean ON := true;
		function late() runs on C { timer t := 0.3; t.start; t.timeout; }
		altstep guardA() runs on C { [] tg.timeout { setverdict(fail, "guard") } }
		testcase tc() runs on C system C {
			var C w := C.create("w");
			w.start(late());
			tg.start;
			var default d := activate(guardA());
			alt { [ON] w.done { setverdict(pass) } }
		}
	}`
	var per [2]map[string][]string
	for i, k := range clocks {
		rec, _ := runLogged(t, src, k.opts)
		if n := len(filterOps(rec, "mtc", "tliSEnter")); n != 2 {
			t.Errorf("%s clock: the MTC's tliSEnter %d times, want the testcase's and the default's once: %v", k.name, n, rec.Ops())
		}
		if n := count(rec, "tliModulePar"); n != 1 {
			t.Errorf("%s clock: the guard's module parameter read logged %d times", k.name, n)
		}
		per[i] = componentActions(rec)
	}
	if !reflect.DeepEqual(per[0], per[1]) {
		t.Errorf("the clocks logged different operations:\nvirtual %v\n   live %v", per[0], per[1])
	}
}

// filterOps returns the operations op that component comp logged.
func filterOps(rec *tl.Recorder, comp, op string) []*tl.Event {
	var out []*tl.Event
	for _, e := range rec.Events {
		if e.C.Name == comp && e.Op == op {
			out = append(out, e)
		}
	}
	return out
}
