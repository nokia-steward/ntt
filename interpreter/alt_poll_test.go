package interpreter

import (
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestAltNeedsPoll: an alt waiting on the real clock polls only for a
// guard that can come true with nothing to wake it — a message for the
// component, a timer the wait is bounded by, a stop. Idle components
// waiting for messages and their own timers used to poll every 2 ms,
// and so slowed every other component down.
func TestAltNeedsPoll(t *testing.T) {
	for _, tt := range []struct {
		alt  string
		want bool
	}{
		{`alt { [] p.receive {} [] T.timeout {} }`, false},
		{`alt { [] p.receive(integer:?) -> value x {} [] any port.receive {} }`, false},
		{`alt { [x > 1] p.receive {} [] any timer.timeout {} }`, false},
		{`alt { [] p.getreply {} [] p.catch {} }`, false},
		{`alt { [] p.receive {} [else] {} }`, false},
		{`alt { [f()] p.receive {} }`, true},
		{`alt { [T.read > 1.0] p.receive {} }`, true},
		{`alt { [c.alive] p.receive {} }`, true},
		{`alt { [] c.done {} }`, true},
		{`alt { [] any component.killed {} }`, true},
		{`alt { [] p.catch(timeout) {} }`, true},
		{`alt { [] Ta[1].timeout {} }`, true},
		{`alt { [] U.timeout {} }`, true},
		{`alt { [] as() {} }`, true},
	} {
		tree := ttcn3.Parse("module M { function g() { " + tt.alt + " } }")
		if tree.Err != nil {
			t.Fatalf("%s: %v", tt.alt, tree.Err)
		}
		var alt *syntax.AltStmt
		tree.Inspect(func(n syntax.Node) bool {
			if a, ok := n.(*syntax.AltStmt); ok && alt == nil {
				alt = a
			}
			return alt == nil
		})
		env := runtime.NewEnv(nil)
		env.Set("T", &runtime.TimerHandle{})
		env.Set("x", runtime.NewInt(2))
		if got := altNeedsPoll(alt, env); got != tt.want {
			t.Errorf("altNeedsPoll(%s) = %v, want %v", tt.alt, got, tt.want)
		}
	}
}

// TestAllTimerWaitsForTheLastTimer: `all timer.timeout` comes true when
// the last running timer is due, so the alt waits that long — not, with
// one timer due and another running, not at all, which spun.
func TestAllTimerWaitsForTheLastTimer(t *testing.T) {
	tree := ttcn3.Parse("module M { function g() { alt { [] all timer.timeout {} } } }")
	var alt *syntax.AltStmt
	tree.Inspect(func(n syntax.Node) bool {
		if a, ok := n.(*syntax.AltStmt); ok {
			alt = a
		}
		return alt == nil
	})
	now := time.Now()
	env := runtime.NewEnv(nil)
	env.Set("t1", &runtime.TimerHandle{Running: true, StartedAt: now.Add(-time.Second), Duration: 0.5})
	env.Set("t2", &runtime.TimerHandle{Running: true, StartedAt: now, Duration: 2})
	d, ok := nextAltTimerDeadlineLenient(alt, env)
	if !ok || d < time.Second {
		t.Fatalf("nextAltTimerDeadlineLenient = %v, %v; want about 2s, true", d, ok)
	}
	env.Set("t2", &runtime.TimerHandle{Running: true, StartedAt: now.Add(-time.Second), Duration: 0.5})
	if d, ok := nextAltTimerDeadlineLenient(alt, env); !ok || d != 0 {
		t.Fatalf("with all timers due: %v, %v; want 0, true", d, ok)
	}
}
