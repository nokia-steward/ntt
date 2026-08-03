// Tests for the async PTC scheduling + alt-no-match wait path
// shipped to cover the async external-port scenario.
//
// The smoke test reproduces the contract a daemon-style server PTC depends on:
//
//   - `comp.start(f)` on an `alive` component runs f in its own
//     goroutine so the MTC continues immediately.
//   - The PTC's alt with only `port.receive(...)` clauses parks
//     on TestcaseExec.MessageReady instead of speculative-firing
//     the body with `req` left undefined.
//   - When the MTC later runs `d.stop`, the PTC goroutine wakes
//     and unwinds cleanly within the WaitPTCs() drain budget.
//
// We don't depend on a real C++ port here; instead the MTC just
// runs `T := 1.0; T.start; T.timeout; d.stop;` so the alt never
// receives a message and we observe (a) the body didn't fire and
// (b) the testcase completes in well under the 5 s WaitPTCs cap.
package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// fakeServerDriver is the bare-minimum PortDriver the async PTC
// smoke needs so the daemon's `srv.receive(...)` clause counts as an
// external-port guard. The driver does nothing on Send / Map; the
// test never expects the daemon to actually receive a message (the
// point is to assert the alt body did NOT fire on an empty queue).
type fakeServerDriver struct{}

func (fakeServerDriver) Send(payload runtime.Object, sender runtime.Object) error { return nil }
func (fakeServerDriver) Map(local, remote string) error                           { return nil }
func (fakeServerDriver) Unmap(local, remote string) error                         { return nil }

// TestAsyncPTC_InjectWakesAltAndBodyRuns is the inverse smoke:
// once an external producer enqueues a matching message via the
// runtime.inject() / EnqueueMessageFrom path, the parked alt
// scheduler wakes, the matching clause fires exactly once, the
// body's setverdict(pass) lands, and `d.stop` joins cleanly.
//
// The test installs the same fake driver as the empty-queue
// smoke so `srv.receive(...)` counts as an external-port guard,
// then spawns a tiny producer goroutine that pushes one
// SrvRequest into srv's queue after a short delay (i.e. after the
// PTC has had time to park).
func TestAsyncPTC_InjectWakesAltAndBodyRuns(t *testing.T) {
	prev := runtime.SetPortDriverProvider(func(typeName, instName string) runtime.PortDriver {
		if typeName == "MyServer_PT" {
			return fakeServerDriver{}
		}
		return nil
	})
	t.Cleanup(func() { runtime.SetPortDriverProvider(prev) })

	tree := parse(t, `module InjectWakeSmoke {
        type record SrvRequest { integer connectionId }
        type record SrvResponse { integer connectionId }
        type port MyServer_PT message {
            out SrvResponse;
            in  SrvRequest
        }
        type component MainCT {}
        type component DaemonCT {
            port MyServer_PT srv;
        }

        function f_daemon() runs on DaemonCT {
            map(self:srv, system:srv);
            var SrvRequest req;
            alt {
                [] srv.receive(SrvRequest:?) -> value req {
                    setverdict(pass);
                }
            }
        }

        testcase tc_InjectWakes() runs on MainCT system MainCT {
            var DaemonCT d := DaemonCT.create alive;
            d.start(f_daemon());
            timer T := 0.5; T.start; T.timeout;
            d.stop;
        }
    }`)

	// Spawn the producer after a short delay so the PTC is
	// already parked on its alt. Using the global
	// runtime.CurrentExec() handle is how the cabi/cgo bridge
	// dispatches inject() in production - we mimic that here so
	// the test exercises the same code path.
	go func() {
		time.Sleep(50 * time.Millisecond)
		// Spin briefly until the testcase has installed itself
		// as the current exec; without this the producer can
		// race past RunTestcaseWith's setup.
		for i := 0; i < 50; i++ {
			if e := runtime.CurrentExec(); e != nil {
				req := runtime.NewRecord()
				req.Set("connectionId", runtime.NewInt(42))
				e.EnqueueMessageFrom("srv", req, nil)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	start := time.Now()
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "InjectWakeSmoke.tc_InjectWakes")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	elapsed := time.Since(start)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s reason=%q, want pass (the injected msg should have woken the alt)", v, reason)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("testcase took %v; want < 4 s", elapsed)
	}
}

// TestAsyncPTC_AltOnEmptyQueueDoesNotFireBody is the gap #8
// reproducer reduced to a self-contained TTCN-3 program. The
// fixture mirrors GapEightSmoke from
// Integration scenario: a daemon PTC binds a
// port, alt-receives, and we assert the body never ran.
func TestAsyncPTC_AltOnEmptyQueueDoesNotFireBody(t *testing.T) {
	// The alt's "park on MessageReady" path only fires when the
	// receive port has an external driver bound. Register a noop
	// driver for MyServer_PT so `srv.receive(...)` counts as an
	// external-port guard. Restore the previous provider on exit
	// so the rest of the suite isn't poisoned.
	prev := runtime.SetPortDriverProvider(func(typeName, instName string) runtime.PortDriver {
		if typeName == "MyServer_PT" {
			return fakeServerDriver{}
		}
		return nil
	})
	t.Cleanup(func() { runtime.SetPortDriverProvider(prev) })

	tree := parse(t, `module GapEightSmoke {
        type record Bind { charstring host, integer tcpPort }
        type record SrvRequest { integer connectionId }
        type record SrvResponse { integer connectionId }
        type port MyServer_PT message {
            out SrvResponse, Bind;
            in  SrvRequest
        }
        type component MainCT {}
        type component DaemonCT {
            port MyServer_PT srv;
        }

        function f_waitOnly(charstring p_host, integer p_port) runs on DaemonCT {
            map(self:srv, system:srv);
            var SrvRequest req;
            alt {
                [] srv.receive(SrvRequest:?) -> value req {
                    var SrvResponse rsp := { connectionId := req.connectionId };
                    srv.send(rsp);
                    setverdict(fail, "alt body fired on empty queue");
                    repeat;
                }
            }
        }

        testcase tc_BlockingAlt() runs on MainCT system MainCT {
            var DaemonCT d := DaemonCT.create alive;
            d.start(f_waitOnly("0.0.0.0", 50059));
            timer T := 0.05; T.start; T.timeout;
            d.stop;
            setverdict(pass);
        }
    }`)

	start := time.Now()
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "GapEightSmoke.tc_BlockingAlt")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	elapsed := time.Since(start)

	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s reason=%q, want pass (the alt body must NOT have fired)", v, reason)
	}
	// 5 s is the WaitPTCs hard cap. A correct fix joins in
	// well under a second; anything north of 4 s means the
	// daemon goroutine missed the stop signal.
	if elapsed > 4*time.Second {
		t.Fatalf("testcase took %v; want < 4 s (alt should park on MessageReady, stop should wake)", elapsed)
	}
}
