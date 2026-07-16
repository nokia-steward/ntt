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
