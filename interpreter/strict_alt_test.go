package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func runStrict(t *testing.T, qname, src string) (runtime.Verdict, string) {
	t.Helper()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, qname,
		interpreter.TestcaseOptions{})
	if err != nil {
		t.Fatalf("RunTestcaseWith(%s): %v", qname, err)
	}
	return v, reason
}

// runStrictDet runs a logical alt/interleave test under the deterministic
// scheduler + virtual clock, so a timer guard (e.g. `t := 0.05`) fires on
// the virtual clock — deterministically — instead of racing a real 50ms
// deadline on a slow/loaded CI runner. Use it for tests that only care
// about WHICH clause fires, not how long the wait took; use runStrict for
// tests that assert real-clock timing (e.g. TestStrictAlt_TimerGuardFires).
func runStrictDet(t *testing.T, qname, src string) (runtime.Verdict, string) {
	t.Helper()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, qname,
		interpreter.TestcaseOptions{DeterministicScheduler: true, DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith(%s): %v", qname, err)
	}
	return v, reason
}

// TestStrictAlt_ConnectedPeerReceive covers the ProfileStrict snapshot
// alt evaluator + connection-topology routing: a connected peer sends,
// and the MTC's alt receives it on the connected port. Under the strict
// profile a send on a connected port must reach the PEER's queue (not
// the sender's own), and the alt must match by real guard evaluation —
// not the verdict-preferring heuristic. Mirrors
// Sem_2002_TheAltStatement_016.
func TestStrictAlt_ConnectedPeerReceive(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		function fsend() runs on C { p.send("ping"); }
		testcase tc() runs on C system C {
			var C peer := C.create;
			connect(self:p, peer:p);
			peer.start(fsend());
			alt {
				[] p.receive(charstring:"ping") { setverdict(pass); }
				[] p.receive { setverdict(fail, "wrong message"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (connected send must reach the peer's queue)", v, reason)
	}
}

// TestStrictAlt_TimerGuardFires covers honest snapshot blocking: the
// receive guard never matches (nothing is queued), so the alt blocks and
// the timer guard fires for real — instead of the approximate model
// fabricating a verdict. Uses a short real timer so the test is fast.
func TestStrictAlt_TimerGuardFires(t *testing.T) {
	start := time.Now()
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer t := 0.05;
			t.start;
			alt {
				[] p.receive(charstring:"never") { setverdict(fail, "unexpected message"); }
				[] t.timeout { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass via the timer guard", v, reason)
	}
	// The timer really elapses (~50ms); the approximate heuristic would
	// return in ~0ms by faking the branch.
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("alt returned in %v; the strict timer guard should have waited ~50ms", elapsed)
	}
}

// TestStrictAlt_ContextCancelsBlockedAlt covers cancellable execution: a
// strict alt with only a receive guard that never matches blocks
// forever; a deadline context must stop the run so the goroutine
// terminates (no leak / hang) rather than spinning on the backstop.
func TestStrictAlt_ContextCancelsBlockedAlt(t *testing.T) {
	src := `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			alt {
				[] p.receive(charstring:"never") { setverdict(pass); }
			}
		}
	}`
	tree := parse(t, src)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_, _, _ = interpreter.RunTestcaseWith([]*ttcn3.Tree{tree}, "M.tc",
			interpreter.TestcaseOptions{Context: ctx})
		close(done)
	}()

	select {
	case <-done:
		// good: the blocked alt was cancelled and the run returned.
	case <-time.After(3 * time.Second):
		t.Fatal("blocked strict alt was not cancelled by context (leak/hang)")
	}
}

// TestStrictAlt_DeterministicClockFiresTimerInstantly covers the
// deterministic clock: a 30s timer guard fires virtually-instantly
// (virtual-clock advance, no real sleep) with the correct verdict — the
// property that removes real-timer "pass->timeout" artifacts from the
// strict differential.
func TestStrictAlt_DeterministicClockFiresTimerInstantly(t *testing.T) {
	src := `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer t := 30.0;
			t.start;
			alt {
				[] p.receive(charstring:"never") { setverdict(fail, "unexpected message"); }
				[] t.timeout { setverdict(pass); }
			}
		}
	}`
	start := time.Now()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass via the timer guard", v, reason)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("deterministic 30s timer took %v; it must fire instantly (virtual clock)", elapsed)
	}
}

// TestStrictAlt_DeterministicClockSoonestTimerWins guards deadline
// ordering: the block step advances to the SOONEST timer deadline, so a
// 5s timer wins over a 30s one even though the 30s clause is listed
// first — not naive source order.
func TestStrictAlt_DeterministicClockSoonestTimerWins(t *testing.T) {
	src := `module M {
		type component C { }
		testcase tc() runs on C system C {
			timer t_long := 30.0;
			timer t_short := 5.0;
			t_long.start;
			t_short.start;
			alt {
				[] t_long.timeout { setverdict(fail, "long timer fired first"); }
				[] t_short.timeout { setverdict(pass); }
			}
		}
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (soonest timer must win)", v, reason)
	}
}

// TestStrictProc_ConnectedCallReply covers strict connection routing for
// procedure-based communication: a caller's `call` must reach the
// connected server's queue, and the server's `reply` must reach the
// caller's queue. Without proc routing under strict the call/reply land
// on the sender's own queue and the caller's getreply never matches.
func TestStrictProc_ConnectedCallReply(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function server() runs on C {
			p.getcall(S:?);
			p.reply(S:{} value 42);
		}
		testcase tc() runs on C system C {
			var C peer := C.create;
			connect(self:p, peer:p);
			p.call(S:{}, nowait);
			peer.start(server());
			peer.done;
			alt {
				[] p.getreply(S:?) { setverdict(pass); }
				[else] { setverdict(fail, "no reply reached the caller"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (connected call/reply routing)", v, reason)
	}
}

// TestStrictProc_CheckHonoursTemplate covers strict procedure-payload
// matching: `check(getreply(S:{p:=(100..200)} value ?))` must NOT match a
// reply whose parameter is out of range (here p_par1=1), so the fail
// branch does not fire and the bare-getreply fallback passes. Under the
// lenient (approximate) match the check wrongly fires. Mirrors
// Sem_2204_the_check_operation_057.
func TestStrictProc_CheckHonoursTemplate(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S(out integer p_par1) return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function f() runs on C {
			p.getcall;
			p.reply(S:{ p_par1 := 1 } value 5);
		}
		testcase tc() runs on C system C {
			var C peer := C.create;
			connect(self:p, peer:p);
			p.call(S:{ p_par1 := - }, nowait);
			peer.start(f());
			alt {
				[] p.check(getreply(S:{ p_par1 := (100..200) } value ?)) { setverdict(fail, "check wrongly matched"); }
				[] p.getreply { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (check must honour the param template)", v, reason)
	}
}

// TestStrictAlt_BooleanGuardGatesClause covers boolean-guard evaluation
// (ETSI 20.2): a clause `[expr] op {...}` is eligible only when `expr`
// holds. Both clauses below fire on the same timer, but the first is
// gated by a false boolean guard, so the strict snapshot must skip it and
// take the second (true-guarded) clause. Without guard evaluation the
// first clause would win by source order and the verdict would be fail.
func TestStrictAlt_BooleanGuardGatesClause(t *testing.T) {
	// Deterministic clock: the 0.05s timer is only the event that lets the
	// alt conclude; the test is about WHICH guarded clause fires, not the
	// wait. Firing it virtually makes the outcome deterministic instead of
	// racing a real 50ms deadline on a loaded runner (a Windows CI flake).
	v, reason := runStrictDet(t, "M.tc", `module M {
		type component C { }
		testcase tc() runs on C system C {
			var integer x := 0;
			timer t := 0.05;
			t.start;
			alt {
				[x > 0] t.timeout { setverdict(fail, "false-guarded clause fired"); }
				[x == 0] t.timeout { setverdict(pass); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (false boolean guard must skip its clause)", v, reason)
	}
}

// TestStrictProc_TwoPTCBlockingCall covers the concurrent two-PTC
// blocking-`call` shape (Sem_220301_CallOperation): a non-alive `server`
// PTC forks and blocks in `getcall` BEFORE any call exists, then a
// non-alive `client` PTC issues a blocking `call` whose reply the server
// produces. Both are `create`d (not `create alive`), so the strict
// forkStrict path must run their blocking bodies on real goroutines; the
// deterministic clock falls back to the real clock while they are live,
// so the server's safety timer does not fire before the client's call
// arrives. The MTC coordinates via component.done.
func TestStrictProc_TwoPTCBlockingCall(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function server() runs on C {
			timer t := 30.0; t.start;
			alt {
				[] p.getcall(S:?) { p.reply(S:{} value 42); }
				[] t.timeout { setverdict(fail, "server timed out"); }
			}
		}
		function client() runs on C {
			p.call(S:{}, 5.0) {
				[] p.getreply(S:? value 42) { setverdict(pass); }
				[] p.getreply { setverdict(fail, "wrong reply"); }
				[] p.catch(timeout) { setverdict(fail, "call timed out"); }
			}
		}
		testcase tc() runs on C system C {
			var C srv := C.create;
			var C cli := C.create;
			connect(srv:p, cli:p);
			srv.start(server());
			cli.start(client());
			all component.done;
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (two-PTC blocking call/reply)", v, reason)
	}
}

// TestStrictProc_GetcallPositionalParamRedirect covers two procedure
// redirect fixes: (1) a `getcall(...) -> param(...)` redirect is applied
// at all (getcall was missing from the RedirectExpr evaluator), and (2)
// the POSITIONAL form `param(v1, -, v3)` binds each target to the
// parameter record's field at that position (in signature order) rather
// than the whole payload. The server echoes the bound params back as the
// reply's return value; a wrong binding yields the wrong value and the
// client's value-qualified getreply would not match. Mirrors
// Sem_220301_CallOperation_001.
func TestStrictProc_GetcallPositionalParamRedirect(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S(in integer p1, out integer p2, inout integer p3) return integer;
		template S s_call := { p1 := 4, p2 := -, p3 := 5 };
		type port P procedure { inout S }
		type component C { port P p }
		function server() runs on C {
			var integer v1, v3;
			p.getcall(S:?) -> param(v1, -, v3);
			p.reply(S:{ p1 := -, p2 := v1 + v3, p3 := - } value v1 + v3);
		}
		testcase tc() runs on C system C {
			var C peer := C.create;
			connect(self:p, peer:p);
			p.call(S:s_call, nowait);
			peer.start(server());
			peer.done;
			alt {
				[] p.getreply(S:? value 8) { setverdict(fail, "wrong return value"); }
				[] p.getreply(S:{ p1 := -, p2 := 9, p3 := ? } value 9) { setverdict(pass); }
				[] p.getreply { setverdict(fail, "no positional bind"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (positional param redirect: v1=4,v3=5 -> value 9)", v, reason)
	}
}

// TestStrictProc_MultiClientBroadcast covers the multi-connect broadcast
// shape (Sem_220303_ReplyOperation): two non-alive client PTCs each issue
// a blocking `call`, and a server replies `to all component`. Both
// clients must run and receive the reply. The bug: the global
// HasPendingCalls flag caused the SECOND client to be skipped once the
// first had a queued call, so it never finished and `all component.done`
// hung. A client (blocking call) must always fork regardless of pending
// calls.
func TestStrictProc_MultiClientBroadcast(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function server() runs on C {
			timer t := 30.0; t.start;
			alt {
				[] p.getcall(S:?) { }
				[] t.timeout { setverdict(fail, "server timeout"); }
			}
			p.reply(S:{} value 7) to all component;
		}
		function client() runs on C {
			p.call(S:{}, 5.0) {
				[] p.getreply(S:? value 7) { setverdict(pass); }
				[] p.catch(timeout) { setverdict(fail, "client got no reply"); }
			}
		}
		testcase tc() runs on C system C {
			var C srv := C.create;
			var C a := C.create;
			var C b := C.create;
			connect(srv:p, a:p);
			connect(srv:p, b:p);
			srv.start(server());
			a.start(client());
			b.start(client());
			all component.done;
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (both clients must run and get the broadcast reply)", v, reason)
	}
}

// TestStrictProc_PortArrayConnectedRouting covers strict routing for
// port ARRAY elements: connect(self:p[i], v:p[i]) records the endpoint
// under the base name "p" (portRefName drops the index) while comm uses
// "p[i]", so strict routing must fall back to the base and re-apply the
// element index. Mirrors Sem_220304_getreply_operation_002.
func TestStrictProc_PortArrayConnectedRouting(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S();
		type port P procedure { inout S }
		type component C { port P p[2] }
		function f() runs on C {
			for (var integer i := 0; i < 2; i := i + 1) {
				p[i].getcall;
				p[i].reply(S:{});
			}
		}
		testcase tc() runs on C system C {
			var C v := C.create;
			for (var integer i := 0; i < 2; i := i + 1) {
				connect(self:p[i], v:p[i]);
				p[i].call(S:{}, nowait);
			}
			v.start(f());
			alt {
				[] any from p.getreply { setverdict(pass); }
				[else] { setverdict(fail, "port-array reply not routed to caller"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (port-array connected routing)", v, reason)
	}
}

// TestStrictProc_BareGetreplyRespectsSnapshot guards the ETSI 20.2 snapshot
// boundary on the BARE procedure-guard path. A specific
// `getreply(S:? value 42)` clause followed by a catch-all `getreply` must
// never lose the reply to the catch-all: the templated and message receive
// paths honour the round's frozen boundary, and this path used to peek the
// LIVE queue instead, so a reply arriving mid-round was invisible to the
// earlier clause and consumed by the later one. It surfaced as an
// intermittent "wrong reply" on loaded CI runners and reproduced under
// -race; run with -count to exercise the window.
func TestStrictProc_BareGetreplyRespectsSnapshot(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function server() runs on C {
			timer t := 30.0; t.start;
			alt {
				[] p.getcall(S:?) { p.reply(S:{} value 42); }
				[] t.timeout { setverdict(fail, "server timed out"); }
			}
		}
		function client() runs on C {
			p.call(S:{}, 5.0) {
				[] p.getreply(S:? value 42) { setverdict(pass); }
				[] p.getreply { setverdict(fail, "catch-all took the reply from the specific clause"); }
				[] p.catch(timeout) { setverdict(fail, "call timed out"); }
			}
		}
		testcase tc() runs on C system C {
			var C srv := C.create;
			var C cli := C.create;
			connect(srv:p, cli:p);
			srv.start(server());
			cli.start(client());
			all component.done;
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (a bare guard must respect the alt snapshot)", v, reason)
	}
}

// TestLiveClock_PTCBodyWithTimerRuns guards the real-clock fork predicate.
// A started PTC gets a goroutine only when a predicate says its body needs
// one. Under the cooperative scheduler that predicate is broad; on the real
// clock it used to be the narrow getcall-only one, on the reasoning that
// forking half a pair whose counterpart is not forked leaves it waiting for
// traffic that never comes. That reasoning does not transfer: with no
// scheduler there is no modelling, so an unforked body does not run at all.
//
// The body below sends, waits on a timer, then sends again. Before the fix
// NEITHER send arrived under the real clock — the testcase died on its own
// guard timer with no indication the PTC had never started — while the same
// source passed on the virtual clock. "Wait, then act" is an ordinary shape
// for pacing a live SUT, so this asserts both sends arrive.
func TestLiveClock_PTCBodyWithTimerRuns(t *testing.T) {
	src := `module M {
		type port P message { inout charstring }
		type component C { port P p }
		function two_phase() runs on C {
			p.send("before");
			timer d := 0.05; d.start; d.timeout;
			p.send("after");
		}
		testcase tc() runs on C system C {
			timer g := 10.0;
			var C q := C.create;
			connect(self:p, q:p);
			q.start(two_phase());
			g.start;
			alt {
				[] p.receive("before") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "PTC never ran at all"); stop; }
			}
			alt {
				[] p.receive("after") { setverdict(pass); }
				[] g.timeout { setverdict(fail, "PTC ran but its timer never released it"); }
			}
		}
	}`
	// Real clock: both options off, which is what --live selects.
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("real clock: verdict = %s (%s), want pass", v, reason)
	}
	// Same source, virtual clock: the verdict must agree. This pair is the
	// clock-substitution equivalence claim in miniature.
	v2, reason2, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{DeterministicScheduler: true, DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith (virtual): %v", err)
	}
	if v2 != v {
		t.Fatalf("clock substitution changed the verdict: real=%s (%s) virtual=%s (%s)",
			v, reason, v2, reason2)
	}
}

// TestLiveClock_DoneBlocksAndPreservesPTCVerdict covers ETSI 21.3.7 on the
// real clock, and the hollow pass that followed from it not holding.
//
// `comp.done` was gated on the cooperative scheduler at its call site, so a
// live run answered a non-blocking snapshot. The MTC then ran to the end of
// its body and teardown stopped the PTC — which had not yet reached its
// setverdict — so the testcase reported a pass nothing had earned. That is
// the failure class this engine exists not to produce, on the one path the
// conformance corpus cannot cover, since the corpus runs virtual.
//
// The PTC below sends immediately before its setverdict, so the probe
// distinguishes "ran to completion" from "was cut off" rather than
// inferring it from elapsed time — teardown's own grace period makes wall
// time a misleading proxy here.
func TestLiveClock_DoneBlocksAndPreservesPTCVerdict(t *testing.T) {
	src := `module M {
		type port P message { inout charstring }
		type component C { port P p }
		function late_fail() runs on C {
			p.send("early");
			timer d := 0.2; d.start; d.timeout;
			p.send("reached");
			setverdict(fail, "PTC reached setverdict");
		}
		testcase tc() runs on C system C {
			var C q := C.create;
			connect(self:p, q:p);
			q.start(late_fail());
			q.done;
			setverdict(pass);
		}
	}`
	for _, k := range []struct {
		name string
		opts interpreter.TestcaseOptions
	}{
		{"virtual", interpreter.TestcaseOptions{DeterministicScheduler: true, DeterministicClock: true}},
		{"live", interpreter.TestcaseOptions{}},
	} {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil {
			t.Fatalf("%s: RunTestcaseWith: %v", k.name, err)
		}
		// The PTC sets fail and the testcase must inherit it: `.done` has
		// to wait long enough for the PTC to get there.
		if v != runtime.FailVerdict {
			t.Fatalf("%s: verdict = %s (%s), want fail — the PTC's verdict must survive `.done`",
				k.name, v, reason)
		}
	}
}
