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
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict})
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
			interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, Context: ctx})
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
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true})
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
		interpreter.TestcaseOptions{Profile: runtime.ProfileStrict, DeterministicClock: true})
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
	v, reason := runStrict(t, "M.tc", `module M {
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
