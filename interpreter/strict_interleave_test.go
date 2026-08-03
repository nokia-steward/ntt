package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestStrictSched_GetcallSenderGuardNotCorrupted covers C1: a server whose
// getcall is gated on a `[v == null]` boolean guard and binds the caller via
// `-> sender v` must still accept the call under the scheduler. The
// approximate pre-populate heuristic used to bind `v` to the latest PTC ref
// on the round-0 no-match, flipping the guard false so the server could
// never accept — the server then timed out (Sem_220303_ReplyOperation_001).
// Under the coop scheduler that heuristic is suppressed, so `v` binds only
// from the real matched call.
func TestStrictSched_GetcallSenderGuardNotCorrupted(t *testing.T) {
	src := `module m {
		signature Sig(in integer x);
		type port P procedure { inout Sig }
		type component C { port P p; var C v_client := null }
		function srv() runs on C {
			timer t := 10.0;
			t.start;
			alt {
				[v_client == null] p.getcall(Sig:?) -> sender v_client { setverdict(pass); }
				[] t.timeout { setverdict(fail, "server never accepted: [v==null] guard corrupted"); }
			}
		}
		function cli() runs on C { p.call(Sig:{x:=1}, nowait); }
		testcase tc() runs on C system C {
			var C server := C.create;
			var C client := C.create;
			connect(server:p, client:p);
			server.start(srv());
			client.start(cli());
			all component.done;
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (getcall sender-guard must not be corrupted)", v, reason)
	}
}

// TestStrictSched_CallTimeoutCatchFires covers C3: the timeout duration D of
// a blocking `p.call(S, D){ ... }` is a virtual timer, so a `catch(timeout)`
// guard in the response block fires when D elapses with no matching reply.
// The server never replies, so the call-body alt parks on D's deadline, the
// deterministic clock advances to it, and catch(timeout) wins. Without the
// synthetic timer catch(timeout) could never match (it probed for a
// MsgException that never arrives) and the call block hung. Mirrors
// Sem_220302_GetcallOperation_004 (a client whose call is never answered).
func TestStrictSched_CallTimeoutCatchFires(t *testing.T) {
	src := `module m {
		signature Sig(in integer x) return integer;
		type port P procedure { inout Sig }
		type component C { port P p }
		function srv() runs on C {
			timer t := 10.0; t.start;
			alt { [] t.timeout {} }
		}
		function cli() runs on C {
			p.call(Sig:{x:=1}, 1.0) {
				[] p.getreply { setverdict(fail, "unexpected reply"); }
				[] p.catch(timeout) { setverdict(pass); }
			}
		}
		testcase tc() runs on C system C {
			var C server := C.create;
			var C client := C.create;
			connect(server:p, client:p);
			server.start(srv());
			client.start(cli());
			all component.done;
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (catch(timeout) must fire when the call is unanswered)", v, reason)
	}
}

// TestStrictSched_CallReplyBeatsTimeout is C3's ordering guard: when a
// matching reply IS produced, the getreply clause must win over
// catch(timeout) — the timeout must not fire prematurely. The server accepts
// the call and replies with return value 42; the client's getreply(value 42)
// matches on the first snapshot, before the virtual clock ever advances to
// the call's 5.0 timeout. Mirrors Sem_220302_GetcallOperation_005 (the reply
// arrives and the value-qualified getreply wins).
func TestStrictSched_CallReplyBeatsTimeout(t *testing.T) {
	src := `module m {
		signature Sig(in integer x) return integer;
		type port P procedure { inout Sig }
		type component C { port P p }
		function srv() runs on C {
			timer t := 10.0; t.start;
			alt {
				[] p.getcall(Sig:?) { p.reply(Sig:{x:=1} value 42); }
				[] t.timeout {}
			}
		}
		function cli() runs on C {
			p.call(Sig:{x:=1}, 5.0) {
				[] p.getreply(Sig:{x:=1} value 42) { setverdict(pass); }
				[] p.catch(timeout) { setverdict(fail, "timed out despite a matching reply"); }
			}
		}
		testcase tc() runs on C system C {
			var C server := C.create;
			var C client := C.create;
			connect(server:p, client:p);
			server.start(srv());
			client.start(cli());
			all component.done;
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (a matching reply must beat catch(timeout))", v, reason)
	}
}

// TestStrictSched_MulticastCallTargetsOnly covers non-blocking multicast
// `p.call(S:{}) to (a, c)`: the call must reach ONLY the addressed
// components, and an UNaddressed bare `getcall; reply` responder must NOT
// reply spuriously. Three servers are started; the call targets two of them,
// so exactly two replies arrive, both from an addressed component. Regression
// guard for Sem_220301_CallOperation_015: the fix routes the multicast to the
// targeted peers (A1) and replays a bare responder only when a call is
// actually queued on its own ports (A2 selective deferred replay), rather
// than forking it and letting its non-blocking getcall fall through to a
// spurious reply.
func TestStrictSched_MulticastCallTargetsOnly(t *testing.T) {
	src := `module m {
		signature S() noblock;
		type port P procedure { inout S }
		type component C { port P p }
		function srv() runs on C { p.getcall(S:?); p.reply(S:{}); }
		testcase tc() runs on C system C {
			var C c1 := C.create, c2 := C.create, c3 := C.create, v_from;
			var integer n := 0;
			connect(self:p, c1:p); connect(self:p, c2:p); connect(self:p, c3:p);
			c1.start(srv()); c2.start(srv()); c3.start(srv());
			p.call(S:{}) to (c1, c3);
			timer g := 10.0; g.start;
			alt {
				[] p.getreply(S:?) -> sender v_from {
					if (v_from == c1 or v_from == c3) {
						n := n + 1;
						if (n < 2) { repeat; }
						else { setverdict(pass); }
					} else { setverdict(fail, "reply from an unaddressed component"); }
				}
				[] g.timeout { setverdict(fail, "did not receive both addressed replies"); }
			}
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (multicast must target only c1/c3, no spurious reply)", v, reason)
	}
}

// TestStrictSched_AnyTimerInAltstepResolvesCallerTimer covers C4(c): a
// directly-called `runs on` altstep runs on the strict evaluator (C4(b)), and
// `any timer.timeout` inside it must resolve the CALLER component's running
// timer — a testcase-body-local timer the altstep's lexical scope cannot
// reach (ETSI 23.7). The exec/component timer registry makes it visible, so
// the timer fires (virtual 0.1s), the altstep sets its out-param, and the
// write-back reaches the caller variable. Mirrors
// Sem_050402_actual_parameters_212. A per-goroutine mechanism was unsound
// under the scheduler (goroutine-id reuse); this uses the exec registry.
func TestStrictSched_AnyTimerInAltstepResolvesCallerTimer(t *testing.T) {
	src := `module m {
		type component C {}
		altstep a(out integer p_val) {
			[] any timer.timeout { p_val := 9; }
		}
		testcase tc() runs on C system C {
			var integer v_val := 5;
			timer t := 0.1;
			t.start;
			a(v_val);
			if (v_val == 9) { setverdict(pass); }
			else { setverdict(fail, "any timer.timeout did not resolve the caller-scope timer"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (any timer in a direct altstep must see the caller's timer)", v, reason)
	}
}

// TestStrictSched_TimerRunningUsesVirtualClock covers `T.running` under the
// virtual clock: three timers start at 1.0/2.0/3.0 and `t_medium.timeout`
// advances the clock to 2.0. At that point t_short (1.0) must read NOT
// running (its deadline passed in virtual time) and t_long (3.0) must read
// still running. tickTimer only checks the wall clock — which never advances
// here — so it wrongly reported t_short as still running. Mirrors
// Sem_2306_timer_timeout_007 (timeouts fire shortest-to-longest).
func TestStrictSched_TimerRunningUsesVirtualClock(t *testing.T) {
	src := `module m {
		type component C { timer t_short, t_medium, t_long }
		testcase tc() runs on C system C {
			t_long.start(3.0);
			t_medium.start(2.0);
			t_short.start(1.0);
			t_medium.timeout;
			if (t_short.running) { setverdict(fail, "short timer still running at t=2.0"); stop; }
			if (not t_long.running) { setverdict(fail, "long timer already expired at t=2.0"); stop; }
			setverdict(pass);
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (T.running must track the virtual clock)", v, reason)
	}
}

// TestStrictSched_StandaloneCheckInvokesDefault covers a standalone
// `p.check(...)` that must NOT match: it honours the inner `from self` filter
// (the reply is from the PTC, not self), so on no-match it invokes the
// activated default, whose getreply matches and re-asserts pass then stops —
// the trailing `setverdict(fail)` must never run. The crux is that
// runDefaults detects the fired default by an actual guard MATCH, not a
// verdict change: the PTC already set pass, so the default re-asserting pass
// changes nothing (the old verdict-change heuristic missed it and the fail
// line ran). Mirrors Sem_2204_the_check_operation_059.
func TestStrictSched_StandaloneCheckInvokesDefault(t *testing.T) {
	src := `module m {
		signature S(out integer p_par1) return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function f() runs on C {
			p.getcall;
			setverdict(pass, "Call received");
			p.reply(S:{ p_par1 := 1} value 5);
		}
		altstep a() runs on C {
			[] p.getreply {
				setverdict(pass, "default matched: check correctly did not match");
				stop;
			}
		}
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			activate(a());
			connect(self:p, v_ptc:p);
			p.call(S:{ p_par1 := - }, nowait);
			v_ptc.start(f());
			p.check(getreply(S:? value ?) from self);
			setverdict(fail, "check matched a reply that is not from self");
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (standalone check must invoke the default on no-match)", v, reason)
	}
}

// TestStrictSched_RepeatingDefaultStillLoops guards the runDefaults change
// against the default-mechanism regression: a default that `repeat`s must
// still run its whole altstep loop (consuming every queued message), not a
// single branch. Two messages are sent; the default counts each and repeats,
// so the counter reaches 2. Mirrors Sem_200501_the_default_mechanism_006.
func TestStrictSched_RepeatingDefaultStillLoops(t *testing.T) {
	src := `module m {
		type port P message { inout integer }
		type component C { var integer vc := 0; port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) { vc := vc + 1; repeat; }
		}
		testcase tc() runs on C system C {
			activate(a());
			p.send(integer:5);
			p.send(integer:1);
			alt {
				[] p.receive(integer:1) { vc := vc + 1; setverdict(pass); }
			}
			if (vc == 2) { setverdict(pass); }
			else { setverdict(fail, "repeating default did not consume both messages"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (repeating default must loop over both messages)", v, reason)
	}
}

// TestStrictSched_AnyPortGetcallBindsRedirect covers `any port.getcall(tmpl)
// -> param(...)` under the scheduler: the redirect form fans out over the
// CURRENT component's own ports (as bare names, so the per-port receive
// re-qualifies back to the component-private storage key rather than double-
// qualifying), matches the queued call, and binds the `-> param` targets so
// the reply computed from them carries the right value. Regression guard for
// the per-component port-qualification bug that made
// Sem_220302_GetcallOperation_005 time out under strict.
func TestStrictSched_AnyPortGetcallBindsRedirect(t *testing.T) {
	src := `module m {
		signature Sig(in integer x, out integer y, inout integer z) return integer;
		type port P procedure { inout Sig }
		type component C { port P p }
		function srv() runs on C {
			var integer a; var integer c;
			timer t := 10.0; t.start;
			alt {
				[] any port.getcall(Sig:{x:=?,y:=?,z:=?}) -> param(a, -, c) {
					p.reply(Sig:{x:=-,y:=a+c,z:=a+c+1} value a);
				}
				[] t.timeout { setverdict(fail, "server never accepted via any port"); }
			}
		}
		function cli() runs on C {
			p.call(Sig:{x:=1,y:=-,z:=3}, 5.0) {
				[] p.getreply(Sig:{x:=-,y:=4,z:=5} value 1) { setverdict(pass); }
				[] p.catch(timeout) { setverdict(fail, "no reply via any port"); }
			}
		}
		testcase tc() runs on C system C {
			var C server := C.create;
			var C client := C.create;
			connect(server:p, client:p);
			server.start(srv());
			client.start(cli());
			all component.done;
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (any port.getcall must bind its redirect)", v, reason)
	}
}

// TestStrictSched_ForkMessagePeers covers the coop fork model: a sender
// PTC and a receiver PTC (message comm) must BOTH fork so they interleave
// under the scheduler — running either inline would block the single-runner
// turn. It also exercises positional record-template matching against the
// map-based payload record (the template is coerced to named fields so the
// match is order-independent). Mirrors Sem_13_declaring_msg_003 / SendOp.
func TestStrictSched_ForkMessagePeers(t *testing.T) {
	src := `module M {
		type record R { integer i, charstring s }
		type port P message { inout R }
		type component C { port P p }
		function snd() runs on C { p.send(R:{1, "hi"}); }
		function rcv() runs on C {
			timer t := 3.0;
			t.start;
			alt {
				[] p.receive(R:{1, "hi"}) { setverdict(pass); }
				[] t.timeout { setverdict(fail, "receiver got no message"); }
			}
		}
		testcase tc() runs on C system C {
			var C a := C.create, b := C.create;
			connect(a:p, b:p);
			a.start(snd());
			b.start(rcv());
			a.done;
			b.done;
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (both peers must fork and the record template match)", v, reason)
	}
}

// TestStrictSched_CompDoneParks covers the cooperative scheduler: a
// standalone `comp.done` must PARK the MTC (release the single-runner token)
// so the forked PTC is granted a turn and runs to completion. Without the
// park the MTC races to the end holding the token, the PTC never runs
// (starves), and the run deadlocks. The PTC is `create alive` so it forks
// under the scheduler; its body sets pass, which the testcase verdict
// inherits once it is done.
func TestStrictSched_CompDoneParks(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { setverdict(pass); }
		testcase tc() runs on C system C {
			var C ptc := C.create alive;
			ptc.start(f());
			ptc.done;
			if (ptc.running) { setverdict(fail, "ptc still running after done"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (comp.done must park so the PTC runs)", v, reason)
	}
}

// TestStrictComp_ModeledDoneUsesVirtualClock covers a skipped finite-timer
// PTC's `.done` state under the deterministic clock: the MTC's observation
// window (`g.timeout`) advances VIRTUAL time, so completion of the modelled
// body must be measured against the virtual clock. A real-time measure would
// read ~0 (no wall time passed) and the PTC would never be seen as done.
// Mirrors Sem_210307_done_operation_00x.
func TestStrictComp_ModeledDoneUsesVirtualClock(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { timer t := 1.0; t.start; t.timeout; }
		testcase tc() runs on C system C {
			var C p := C.create;
			timer g := 2.0;
			p.start(f());
			g.start;
			g.timeout;
			if (p.done) { setverdict(pass); }
			else { setverdict(fail, "modelled PTC not done after the virtual observation window"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (modelled done must use the virtual clock)", v, reason)
	}
}

// TestStrictAlt_AltstepGuardTimerConcludes covers a timer guard nested
// INSIDE an altstep-call alternative (`alt { [] a() }` where `a` has
// `[] t.timeout {}`). The strict block step must look THROUGH the altstep
// call to find t's deadline (nextAltTimerVirtualDeadline recurses into
// altstep guards); without that no deadline is found and the alt blocks
// forever. Mirrors Sem_1101_ValueVars_001 / Sem_160201_invoking_altsteps_004.
// A context bounds the run so a regression surfaces as a non-pass rather
// than a hang.
func TestStrictAlt_AltstepGuardTimerConcludes(t *testing.T) {
	src := `module M {
		type component C { timer t }
		altstep a() runs on C {
			[] t.timeout { setverdict(pass); }
		}
		testcase tc() runs on C system C {
			t.start(0.05);
			alt {
				[] a();
			}
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass via the altstep's nested timer guard", v, reason)
	}
}

// TestStrictInterleave_TakesEachBranchOnce covers the core interleave
// semantics (ETSI ES 201 873-1 §20.4): EVERY alternative is taken exactly
// once, unlike a plain alt which takes only one. Both messages are queued
// (loopback self-send), so a correct interleave consumes both and the
// counter reaches 11; the best-effort "take one branch" model would leave
// it at 1 or 10.
func TestStrictInterleave_TakesEachBranchOnce(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var integer c := 0;
			p.send(integer:1);
			p.send(integer:2);
			interleave {
				[] p.receive(integer:1) { c := c + 1; }
				[] p.receive(integer:2) { c := c + 10; }
			}
			if (c == 11) { setverdict(pass); }
			else { setverdict(fail, "interleave did not take both branches"); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (interleave must take every branch once)", v, reason)
	}
}

// TestStrictInterleave_ActiveDefaultsStillTakeEachBranch covers an
// interleave that has activated defaults but no `@nodefault`. That
// combination used to fall back to the best-effort evaluator, which takes
// only ONE alternative; the strict snapshot must take each branch exactly
// once regardless of whether defaults are active. Both messages are queued
// before the interleave, so no branch ever has to block and the default
// never gets a chance to fire — the counter must reach 11, not 1 or 10.
func TestStrictInterleave_ActiveDefaultsStillTakeEachBranch(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:99) { setverdict(fail, "default consumed a branch message"); }
		}
		testcase tc() runs on C system C {
			var integer c := 0;
			activate(a());
			p.send(integer:1);
			p.send(integer:2);
			interleave {
				[] p.receive(integer:1) { c := c + 1; }
				[] p.receive(integer:2) { c := c + 10; }
			}
			if (c == 11) { setverdict(pass); }
			else { setverdict(fail, "interleave with active defaults did not take both branches"); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (active defaults must not reduce interleave to one branch)", v, reason)
	}
}

// TestStrictInterleave_FiredDefaultLeavesInterleave covers the other half of
// the same removal: with no `@nodefault`, an activated default is appended
// after the remaining alternatives (ETSI 20.5) and one that FIRES leaves the
// interleave. The first branch matches the queued message; the second can
// never match, so the interleave would otherwise block forever. The default
// matches instead, sets pass and stops. The signal that makes this work is
// runDefaults reporting a default that actually took a branch rather than
// one that changed the verdict — a signal only available under the
// deterministic scheduler, which is the default engine configuration. The
// default here deliberately sets no verdict, so the older verdict-change
// heuristic cannot see it fire and the interleave would block instead.
func TestStrictInterleave_FiredDefaultLeavesInterleave(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { var integer vc := 0; port P p }
		altstep a() runs on C {
			[] p.receive(integer:7) { vc := vc + 1; }
		}
		testcase tc() runs on C system C {
			activate(a());
			p.send(integer:1);
			p.send(integer:7);
			interleave {
				[] p.receive(integer:1) { }
				[] p.receive(integer:5) { setverdict(fail, "matched a message that was never sent"); }
			}
			if (vc == 1) { setverdict(pass); }
			else { setverdict(fail, "the activated default did not fire to leave the interleave"); }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicScheduler: true, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (a fired default must leave the interleave)", v, reason)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %s: the interleave blocked instead of leaving when the default fired", d)
	}
}

// TestStrictAlt_NoDefaultSuppressesDefaults covers `alt @nodefault`, which
// the strict alt evaluator previously ignored: it ran the activated defaults
// unconditionally. The alt's own alternative cannot match, so a default that
// is wrongly consulted sets fail; with @nodefault honoured the timer guard
// fires instead.
func TestStrictAlt_NoDefaultSuppressesDefaults(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) { setverdict(fail, "default invoked despite @nodefault"); }
		}
		testcase tc() runs on C system C {
			timer t := 3.0;
			activate(a());
			t.start;
			p.send(integer:1);
			alt @nodefault {
				[] p.receive(integer:5) { setverdict(fail, "matched a message that was never sent"); }
				[] t.timeout { setverdict(pass); }
			}
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true, Context: ctx})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (@nodefault must suppress defaults on a plain alt)", v, reason)
	}
}

// TestStrictInterleave_NoDefaultSuppressesDefaults covers `interleave
// @nodefault`: an activated default must NOT be invoked while the interleave
// blocks, so the timer alternative fires instead of the default's fail.
// Mirrors Sem_2004_InterleaveStatement_013. The deterministic clock lets the
// 3s timer fire instantly.
func TestStrictInterleave_NoDefaultSuppressesDefaults(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a() runs on C {
			[] p.receive(integer:?) { setverdict(fail, "default invoked despite @nodefault"); }
		}
		testcase tc() runs on C system C {
			timer t := 3.0;
			activate(a());
			t.start;
			p.send(integer:1);
			interleave @nodefault {
				[] p.receive(integer:5) { setverdict(fail, "matched a message that was never sent"); }
				[] t.timeout { setverdict(pass); break; }
			}
		}
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (@nodefault must suppress the default)", v, reason)
	}
}
