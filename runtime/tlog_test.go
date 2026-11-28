package runtime_test

import (
	"math"
	"math/big"
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

// TestTLValueNotation: values in TTCN-3 notation, the special floats and
// template digits included.
func TestTLValueNotation(t *testing.T) {
	for _, tc := range []struct {
		v    runtime.Object
		want string
	}{
		{runtime.Float(math.Inf(1)), "infinity"},
		{runtime.Float(math.Inf(-1)), "-infinity"},
		{runtime.Float(math.NaN()), "not_a_number"},
		{runtime.Float(1.5), "1.5"},
		{&runtime.Binarystring{String: "'1 0?'B", Unit: runtime.Bit, Value: big.NewInt(-1)}, "10?"},
		{&runtime.Binarystring{String: "'ab*'O", Unit: runtime.Octet, Value: big.NewInt(-1)}, "AB*"},
	} {
		if got := runtime.TLValue(tc.v).Text; got != tc.want {
			t.Errorf("TLValue(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
	if got := tl.Duration(math.Inf(1)).Text; got != "INF" {
		t.Errorf("an infinite duration is %q, want the xsd:float INF", got)
	}
	p := runtime.TLValue(&runtime.String{Value: []rune("ä*"), IsPattern: true})
	if p.Kind != "universal_charstring" {
		t.Errorf("a pattern of non-ASCII characters is a %s", p.Kind)
	}
}

// TestTLScanRepeatsPairsEntryAndLeaving: a scan after waiting leaves out
// what it repeats, and a behaviour's leaving with its entry: logged when
// the entry was, even after the scan, and not when the entry was not.
func TestTLScanRepeatsPairsEntryAndLeaving(t *testing.T) {
	exec := runtime.NewTestcaseExec("M.tc")
	enter := &tl.Event{Op: "tliSEnter", Args: []tl.Arg{{Name: "name", Val: tl.QualifiedName("M", "guard")}}}
	leave := &tl.Event{Op: "tliSLeave", Args: []tl.Arg{{Name: "name", Val: tl.QualifiedName("M", "guard")}}}
	setv := &tl.Event{Op: "tliSetVerdict", Args: []tl.Arg{{Name: "verdict", Val: tl.String("pass")}}}

	exec.TLScanBegin(1, false)
	if exec.TLScanRepeats(1, enter) || exec.TLScanRepeats(1, leave) {
		t.Fatal("the first scan repeats nothing")
	}
	exec.TLScanEnd(1)

	// The scan after waiting enters the default again: neither its entry
	// nor its leaving is logged, what it does anew is.
	exec.TLScanBegin(1, true)
	if !exec.TLScanRepeats(1, enter) {
		t.Fatal("repeated entry logged")
	}
	if exec.TLScanRepeats(1, setv) {
		t.Fatal("a new operation inside not logged")
	}
	exec.TLScanEnd(1)
	if !exec.TLScanRepeats(1, leave) {
		t.Fatal("leaving logged without its entry")
	}

	// An entry logged in a scan is left logged after it.
	exec.TLAltBump(1)
	exec.TLScanBegin(1, true)
	if exec.TLScanRepeats(1, enter) {
		t.Fatal("entry of a new alt round taken for a repeat")
	}
	exec.TLScanEnd(1)
	if exec.TLScanRepeats(1, leave) {
		t.Fatal("leaving of a logged entry left out")
	}
}
