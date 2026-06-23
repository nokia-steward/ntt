// Tests: PTC start barrier on first
// map(...) call. Two `d.start(...)` calls in immediate succession
// must dispatch their PTCs' on_map hooks in start-order, otherwise
// a later `ds[i].stop` -> on_unmap pop-front (the cabi/cgo C++
// port shape the external test-port suite uses) would close the
// wrong listen socket.
package interpreter_test

import (
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// mapOrderRecorder remembers every Map invocation so the test
// can assert that they fired in start-order. We also delay the
// FIRST Map by a configurable duration so that, without the
// start-barrier, the second PTC's Map would race ahead of the
// first and land first in the recorder.
type mapOrderRecorder struct {
	mu        sync.Mutex
	mapped    []string
	mapDelays map[int]time.Duration // 1-based index -> sleep
	mapSeen   int                   // number of Map calls observed so far
}

func (r *mapOrderRecorder) Send(payload runtime.Object, sender runtime.Object) error {
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
	r.mu.Lock()
	r.mapped = append(r.mapped, "send-"+itoa(p.Int64()))
	r.mu.Unlock()
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (r *mapOrderRecorder) Map(local, remote string) error {
	r.mu.Lock()
	r.mapSeen++
	idx := r.mapSeen
	delay := r.mapDelays[idx]
	r.mapped = append(r.mapped, "map-"+itoa(int64(idx)))
	r.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

func (r *mapOrderRecorder) Unmap(local, remote string) error {
	r.mu.Lock()
	r.mapped = append(r.mapped, "unmap")
	r.mu.Unlock()
	return nil
}

func (r *mapOrderRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.mapped))
	copy(out, r.mapped)
	return out
}

// TestPTCStartBarrier_MapHappensInStartOrder is the language-level
// reproducer for the start-barrier behaviour. Three
// `d.start(...)` calls fire in sequence; the first PTC's Map hook
// sleeps for 30 ms so the second and third PTCs' Map calls would
// land first if `comp.start` returned without waiting. With the
// barrier in place the parent parks on each PTC's MapChan before
// the next `d.start` fires, so map-1 always precedes map-2 which
// always precedes map-3 in the recorded event stream.
func TestPTCStartBarrier_MapHappensInStartOrder(t *testing.T) {
	rec := &mapOrderRecorder{
		mapDelays: map[int]time.Duration{1: 30 * time.Millisecond},
	}
	prev := runtime.SetPortDriverProvider(func(typeName, instName string) runtime.PortDriver {
		if typeName == "MyServer_PT" {
			return rec
		}
		return nil
	})
	t.Cleanup(func() { runtime.SetPortDriverProvider(prev) })

	tree := parse(t, `module StartBarrier {
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

        function f_bindOnly(charstring p_host, integer p_port) runs on DaemonCT {
            map(self:srv, system:srv);
            var Bind b := { host := p_host, tcpPort := p_port };
            srv.send(b);
            var SrvRequest req;
            alt { [] srv.receive(SrvRequest:?) -> value req { setverdict(pass); } }
        }

        testcase tc_StartOrder() runs on MainCT system MainCT {
            var DaemonCT d1 := DaemonCT.create alive;
            d1.start(f_bindOnly("0.0.0.0", 60001));
            var DaemonCT d2 := DaemonCT.create alive;
            d2.start(f_bindOnly("0.0.0.0", 60002));
            var DaemonCT d3 := DaemonCT.create alive;
            d3.start(f_bindOnly("0.0.0.0", 60003));
            timer T := 0.2; T.start; T.timeout;
            d1.stop; d2.stop; d3.stop;
            setverdict(pass);
        }
    }`)

	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "StartBarrier.tc_StartOrder")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s, want pass", v)
	}

	events := rec.Events()
	// Collect just the map indices to compare against expected
	// start-order. The post-map sends and final unmaps are
	// allowed to interleave; they happen on the per-PTC
	// goroutine after the map released the barrier.
	var maps []string
	for _, e := range events {
		if len(e) >= 4 && e[:4] == "map-" {
			maps = append(maps, e)
		}
	}
	want := []string{"map-1", "map-2", "map-3"}
	if len(maps) != len(want) {
		t.Fatalf("got %d map events, want %d (events=%v)", len(maps), len(want), events)
	}
	for i := range want {
		if maps[i] != want[i] {
			t.Errorf("map[%d] = %q, want %q (full events=%v)",
				i, maps[i], want[i], events)
		}
	}
}
