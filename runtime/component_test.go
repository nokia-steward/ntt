package runtime_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

func newComponent(id int64, alive bool) *runtime.ComponentRef {
	c := &runtime.ComponentRef{ID: id, AliveModifier: alive}
	c.SetAlive(true)
	return c
}

// A component's state follows its behaviour: inactive before one is
// started, running while it runs, done after; killed once it is no longer
// alive (ETSI ES 201 873-1 21.3.5-21.3.8).
func TestComponentState(t *testing.T) {
	exec := runtime.NewTestcaseExec("tc")
	exec.SetMTCID(1)
	mtc, ptc := newComponent(1, false), newComponent(2, false)
	want := func(name string, got, want runtime.ComponentState) {
		t.Helper()
		if got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
	want("an inactive PTC", exec.ComponentState(ptc, mtc), runtime.ComponentState{Alive: true})
	want("the MTC", exec.ComponentState(mtc, ptc), runtime.ComponentState{Alive: true, Running: true})
	want("a PTC asking about itself", exec.ComponentState(ptc, ptc), runtime.ComponentState{Alive: true, Running: true})
	ptc.SetStarted()
	want("a running PTC", exec.ComponentState(ptc, mtc), runtime.ComponentState{Alive: true, Running: true})
	ptc.Terminate(false)
	want("a stopped PTC", exec.ComponentState(ptc, mtc), runtime.ComponentState{Done: true, Killed: true})
	want("no component", exec.ComponentState(nil, mtc), runtime.ComponentState{Done: true, Killed: true})
	var none *runtime.TestcaseExec
	want("outside a testcase", none.ComponentState(newComponent(3, false), nil), runtime.ComponentState{Alive: true})
}

// A stopped alive component stays alive, to be started again; a killed
// one does not (21.3.3, 21.3.4). Stopping the MTC stops the testcase.
func TestStopComponent(t *testing.T) {
	exec := runtime.NewTestcaseExec("tc")
	exec.SetMTCID(1)
	alive := newComponent(2, true)
	alive.SetStarted()
	exec.StopComponent(alive, false)
	if s := exec.ComponentState(alive, nil); !s.Alive || !s.Done || s.Killed {
		t.Errorf("a stopped alive component: %+v, want alive and done", s)
	}
	exec.StopComponent(alive, true)
	if s := exec.ComponentState(alive, nil); s.Alive || !s.Killed {
		t.Errorf("a killed alive component: %+v, want killed", s)
	}
	if exec.Stopped() {
		t.Fatalf("stopping a PTC stopped the testcase")
	}
	exec.StopComponent(newComponent(1, false), false)
	if !exec.Stopped() {
		t.Errorf("stopping the MTC did not stop the testcase")
	}
}

// A verdict set on a component is the testcase's and the component's.
func TestSetVerdictOn(t *testing.T) {
	exec := runtime.NewTestcaseExec("tc")
	ptc := newComponent(2, false)
	exec.SetVerdictOn(ptc, runtime.FailVerdict, "why")
	exec.SetVerdictOn(nil, runtime.PassVerdict, "")
	if v := exec.GetVerdict(); v != runtime.FailVerdict {
		t.Errorf("testcase verdict %s, want fail", v)
	}
	if v := ptc.GetVerdict(); v != runtime.FailVerdict {
		t.Errorf("component verdict %s, want fail", v)
	}
}
