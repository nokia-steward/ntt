// Tests: PTC arg `arr[i]` (loop-variable index)
// must be evaluated at d.start time, not lazily inside the PTC
// goroutine where the parent's `i` may have already advanced.
package interpreter_test

import (
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// portRecorderDriver remembers every Send payload so the test can
// assert which port number the daemon actually saw at .start time.
type portRecorderDriver struct {
	mu  sync.Mutex
	got []int64
}

func (d *portRecorderDriver) Send(payload runtime.Object, sender runtime.Object) error {
	rec, ok := payload.(*runtime.Record)
	if !ok || rec == nil {
		return nil
	}
	raw, ok := rec.Get("tcpPort")
	if !ok {
		return nil
	}
	p, ok := raw.(runtime.Int)
	if !ok {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.got = append(d.got, p.Int64())
	return nil
}
func (d *portRecorderDriver) Map(local, remote string) error   { return nil }
func (d *portRecorderDriver) Unmap(local, remote string) error { return nil }

func (d *portRecorderDriver) Ports() []int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]int64, len(d.got))
	copy(out, d.got)
	return out
}

// TestPTCArgSnapshot_LoopIndexNotCapturedByReference is the
// language-level reproducer. The MTC walks a list
// of ports inside a `while (i < lengthof(ports))` loop and starts
// one PTC per iteration with `ports[i]`. Without the fix the PTC
// re-reads the now-advanced `i`, lands out-of-bounds, and Send
// emits tcpPort=0; with the fix each Send carries the
// per-iteration snapshot.
func TestPTCArgSnapshot_LoopIndexNotCapturedByReference(t *testing.T) {
	rec := &portRecorderDriver{}
	prev := runtime.SetPortDriverProvider(func(typeName, instName string) runtime.PortDriver {
		if typeName == "MyServer_PT" {
			return rec
		}
		return nil
	})
	t.Cleanup(func() { runtime.SetPortDriverProvider(prev) })

	tree := parse(t, `module PtcArgSnap {
        type record Bind { charstring host, integer tcpPort }
        type record SrvRequest { integer connectionId }
        type record SrvResponse { integer connectionId }
        type port MyServer_PT message {
            out Bind, SrvResponse;
            in SrvRequest
        }
        type component MainCT {}
        type component DaemonCT {
            port MyServer_PT srv;
        }
        type record of integer IL;

        function f_bindOnly(charstring p_host, integer p_port) runs on DaemonCT {
            map(self:srv, system:srv);
            var Bind b := { host := p_host, tcpPort := p_port };
            srv.send(b);
            var SrvRequest req;
            alt { [] srv.receive(SrvRequest:?) -> value req { setverdict(pass); } }
        }

        testcase tc_LoopIndex() runs on MainCT system MainCT {
            var IL ports := { 50061, 50062, 50063 };
            var integer i := 0;
            var DaemonCT d1, d2, d3;
            while (i < lengthof(ports)) {
                var DaemonCT d := DaemonCT.create alive;
                if (i == 0) { d1 := d; }
                if (i == 1) { d2 := d; }
                if (i == 2) { d3 := d; }
                d.start(f_bindOnly("0.0.0.0", ports[i]));
                i := i + 1;
            }
            timer T := 0.2; T.start; T.timeout;
            d1.stop;
            d2.stop;
            d3.stop;
            setverdict(pass);
        }
    }`)

	start := time.Now()
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "PtcArgSnap.tc_LoopIndex")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s, want pass", v)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("testcase took %v; want < 4 s", elapsed)
	}
	got := rec.Ports()
	want := map[int64]bool{50061: true, 50062: true, 50063: true}
	if len(got) != 3 {
		t.Fatalf("got %d Bind sends, want 3 (one per loop iteration); ports=%v", len(got), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected port %d (PTC arg captured by reference?); want one of 50061/50062/50063", p)
		}
		delete(want, p)
	}
	if len(want) != 0 {
		t.Errorf("missing ports: %v", want)
	}
}
