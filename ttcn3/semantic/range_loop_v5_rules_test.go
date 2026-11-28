package semantic

import "testing"

// A V5 loop iterating into a variable declared outside it has a range of
// initialised elements; V4's may have uninitialised ones.
func TestRangeLoopV5UninitialisedElement(t *testing.T) {
	body := `{
		type component Comp {}
		testcase tc() runs on Comp {
			var charstring e;
			for (e in {"a", -, "c"}) {}
			for (var charstring x in {"a", -, "c"}) {}
		}
	}`
	if v4 := analyzeCodes(t, "module M "+body); hasCode(v4, "range-loop-uninitialised-element") {
		t.Errorf("V4: unexpected range-loop-uninitialised-element in %v", v4)
	}
	v5 := analyzeCodes(t, `module M language "TTCN-3:5" `+body)
	n := 0
	for _, c := range v5 {
		if c == "range-loop-uninitialised-element" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("V5: want one range-loop-uninitialised-element, got %v", v5)
	}
}
