package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/tl"
)

func TestWriteLogDiff(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	log := func(verdict string) *tl.Log {
		var buf bytes.Buffer
		w := tl.NewJSONLWriter(&buf)
		tc := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
		w.Log(&tl.Event{Op: "tliTcStart", C: mtc, Args: []tl.Arg{tc}})
		w.Log(&tl.Event{Op: "tliSetVerdict", Line: 3, C: mtc, Args: []tl.Arg{{Name: "verdict", Val: tl.Verdict(verdict)}}})
		w.Log(&tl.Event{Op: "tliTcTerminated", C: mtc, Args: []tl.Arg{tc, {Name: "verdict", Val: tl.Verdict(verdict)}}})
		l, err := tl.ReadLog(&buf)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	var out bytes.Buffer
	if !writeLogDiff(&out, "a", "b", tl.Compare(log("pass"), log("pass"))) {
		t.Errorf("identical runs reported different:\n%s", out.String())
	}
	out.Reset()
	if writeLogDiff(&out, "a", "b", tl.Compare(log("pass"), log("fail"))) {
		t.Errorf("different verdicts reported the same")
	}
	for _, want := range []string{"M.tc: differs", "mtc, action 2:", "a: tliSetVerdict [mtc, line 3] verdict=pass", "b: tliSetVerdict [mtc, line 3] verdict=fail", "1 testcases: 0 same, 1 differ"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
