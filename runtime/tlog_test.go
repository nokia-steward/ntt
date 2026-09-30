package runtime_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
)

// TestTLogAfterTheTestcaseEnded: a component whose behaviour outlives its
// testcase must not appear to act in the next one, so what it logs after
// its testcase ended is recorded as a tliInfo naming the operation, the
// component and its testcase.
func TestTLogAfterTheTestcaseEnded(t *testing.T) {
	exec := runtime.NewTestcaseExec("M.tc")
	rec := &tl.Recorder{}
	exec.SetTestLogger(rec)
	ptc := tl.ComponentID{Name: "PTC", ID: "2"}
	exec.TLogFrom(rec, ptc, "tliSetVerdict", "", 0, tl.Arg{Name: "verdict", Val: tl.Verdict("pass")})
	exec.TLEnd()
	exec.TLogFrom(rec, ptc, "tliCTerminated", "", 0, tl.Arg{Name: "verdict", Val: tl.Verdict("pass")})
	if got := rec.Ops(); strings.Join(got, ",") != "tliSetVerdict,tliInfo" {
		t.Fatalf("ops = %v", got)
	}
	late := rec.Events[1]
	if err := late.Validate(); err != nil {
		t.Fatal(err)
	}
	info := late.Args[1].Val.Text
	for _, want := range []string{"tliCTerminated", "PTC", "M.tc"} {
		if !strings.Contains(info, want) {
			t.Errorf("late event %q does not name %s", info, want)
		}
	}
}

// TestTLValueOfAMapIsInKeyOrder: a map's pairs are logged in key order, so
// the same map logs the same however Go iterates its storage.
func TestTLValueOfAMapIsInKeyOrder(t *testing.T) {
	m := runtime.NewMap()
	for _, k := range []string{"c", "a", "e", "b", "d", "f"} {
		m.Set(runtime.NewCharstring(k), runtime.NewCharstring("v"+k))
	}
	v := runtime.TLValue(m)
	var keys []string
	for _, e := range v.Elems {
		keys = append(keys, e.Elems[0].Text)
	}
	if strings.Join(keys, "") != "abcdef" || v.Type != "map" {
		t.Fatalf("map logged as %s with keys %v", v.Type, keys)
	}
	if got := m.Inspect(); !strings.HasPrefix(got, `{["a"] := "va", ["b"]`) {
		t.Errorf("map text not in key order: %s", got)
	}
}
