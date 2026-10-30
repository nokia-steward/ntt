package tl_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/tl"
)

// linesOf writes events as lines, in UTC.
func linesOf(t *testing.T, events ...*tl.Event) []string {
	t.Helper()
	var b bytes.Buffer
	w := tl.NewLinesWriter(&b).In(time.UTC)
	for _, e := range events {
		w.Log(e)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
}

func wantLines(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const ts = 1791444258794614 // 2026-10-08T07:24:18.794614Z

var (
	mtc    = tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	worker = tl.ComponentID{Name: "worker", ID: "2", Type: "C"}
	system = tl.ComponentID{Null: true}
)

// TestLinesWriter: one event per line, `timestamp|event|identity=file:line
// |fields`: a testcase's start is the test system's, its MTC created and
// started where the testcase is declared; a setverdict line carries the
// verdict before it and the reason, which is left out when there is none;
// a timer is named after its owner, its duration in seconds.
func TestLinesWriter(t *testing.T) {
	got := linesOf(t,
		&tl.Event{Op: "tliTcExecute", Ts: ts, C: system, Args: []tl.Arg{
			{Name: "tcId", Val: tl.TestcaseID("M", "tc")}, {Name: "dur", Val: tl.Duration(5)}}},
		&tl.Event{Op: "tliTcStart", Ts: ts, Src: "M.ttcn3", Line: 9, C: mtc, Args: []tl.Arg{
			{Name: "tcId", Val: tl.TestcaseID("M", "tc")}}},
		&tl.Event{Op: "tliSEnter", Ts: ts, C: mtc, Args: []tl.Arg{
			{Name: "name", Val: tl.QualifiedName("M", "tc")}, {Name: "kind", Val: tl.String("testcase")}}},
		&tl.Event{Op: "tliSetVerdict", Ts: ts, Src: "M.ttcn3", Line: 12, C: mtc, Args: []tl.Arg{
			{Name: "verdict", Val: tl.String("pass")}, {Name: "reason", Val: tl.String("a b")}},
			Parts: []tl.Value{{Kind: "charstring", Text: "a"}, {Kind: "integer", Text: "1"}}},
		&tl.Event{Op: "tliSetVerdict", Ts: ts, Src: "M.ttcn3", Line: 13, C: mtc, Args: []tl.Arg{
			{Name: "verdict", Val: tl.String("inconc")}}},
		&tl.Event{Op: "tliANomatch", Ts: ts, C: mtc}, // no line of its own
		&tl.Event{Op: "tliTStart", Ts: ts, Src: "M.ttcn3", Line: 14, C: mtc, Member: true, Args: []tl.Arg{
			{Name: "timer", Val: tl.TimerID("t", "", "")}, {Name: "dur", Val: tl.Duration(0.5)}}},
		&tl.Event{Op: "tliTStart", Ts: ts, Src: "M.ttcn3", Line: 15, C: mtc, Args: []tl.Arg{
			{Name: "timer", Val: tl.TimerID("u", "", "")}, {Name: "dur", Val: tl.Duration(2)}}},
		&tl.Event{Op: "tliAWait", Ts: ts, Src: "M.ttcn3", Line: 16, C: mtc},
		&tl.Event{Op: "tliSLeave", Ts: ts, C: mtc, Args: []tl.Arg{
			{Name: "name", Val: tl.QualifiedName("M", "tc")}, {Name: "kind", Val: tl.String("testcase")}}},
		&tl.Event{Op: "tliTcTerminated", Ts: ts, C: mtc, Args: []tl.Arg{
			{Name: "tcId", Val: tl.TestcaseID("M", "tc")}, {Name: "verdict", Val: tl.String("inconc")}}},
	)
	wantLines(t, got,
		`20261008T072418.794614|tcst|ntt|M.tc|5.0`,
		`20261008T072418.794614|cocr|ntt|MTC|once`,
		`20261008T072418.794614|cost|MTC=M.ttcn3:9|M.tc`,
		`20261008T072418.794614|tcen|MTC|M.tc()`,
		`20261008T072418.794614|setv|MTC=M.ttcn3:12|none|pass|"a"1`,
		`20261008T072418.794614|bctr|MTC=M.ttcn3:13|:tc()`,
		`20261008T072418.794614|setv|MTC=M.ttcn3:13|pass|inconc`,
		`20261008T072418.794614|tmst|MTC=M.ttcn3:14|MTC.t|0.5s`,
		`20261008T072418.794614|tmst|MTC=M.ttcn3:15|M.tc.u|2.0s`,
		`20261008T072418.794614|alwt|MTC=M.ttcn3:16|20261008T072419.294614`,
		`20261008T072418.794614|tclv|MTC|M.tc()`,
		`20261008T072418.794614|cofi|MTC|inconc`,
		`20261008T072418.794614|tcfi|ntt|M.tc|inconc`,
	)
}

// TestLinesWriterCalls: a function is entered as called from its call
// site, with its in parameters, and left from there with its out
// parameters and its result; a PTC's behaviour starts where it is
// declared; a message is queued by its sender, and taken by a receive.
func TestLinesWriterCalls(t *testing.T) {
	pars := func(vals ...string) tl.Content {
		return tl.Params(
			tl.Param{Name: "x", Mode: "in", Val: tl.Value{Kind: "integer", Text: vals[0]}},
			tl.Param{Name: "y", Mode: "out", Val: tl.Value{Kind: "integer", Text: vals[1]}})
	}
	port := func(c tl.ComponentID, name string) tl.Content {
		return tl.PortID{Comp: c, Name: name, Index: -1}.Content()
	}
	got := linesOf(t,
		&tl.Event{Op: "tliTcStart", Ts: ts, Src: "M.ttcn3", Line: 19, C: mtc, Args: []tl.Arg{
			{Name: "tcId", Val: tl.TestcaseID("M", "tc")}}},
		&tl.Event{Op: "tliSEnter", Ts: ts, Src: "M.ttcn3", Line: 3, C: mtc, Site: tl.Pos{Src: "M.ttcn3", Line: 20},
			Args: []tl.Arg{{Name: "name", Val: tl.QualifiedName("M", "f")}, {Name: "tciPars", Val: pars("2", "0")},
				{Name: "kind", Val: tl.String("function")}}},
		&tl.Event{Op: "tliSLeave", Ts: ts, Src: "M.ttcn3", Line: 3, C: mtc,
			Args: []tl.Arg{{Name: "name", Val: tl.QualifiedName("M", "f")}, {Name: "tciPars", Val: pars("2", "3")},
				{Name: "returnValue", Val: tl.Value{Kind: "float", Text: "12.7"}.AsValue()},
				{Name: "kind", Val: tl.String("function")}}},
		&tl.Event{Op: "tliCStart", Ts: ts, Src: "M.ttcn3", Line: 21, C: mtc, Site: tl.Pos{Src: "M.ttcn3", Line: 5},
			Args: []tl.Arg{{Name: "comp", Val: worker.Content()}, {Name: "name", Val: tl.BehaviourID("M", "g")}}},
		&tl.Event{Op: "tliMSend_c", Ts: ts, Src: "M.ttcn3", Line: 22, C: mtc, Args: []tl.Arg{
			{Name: "at", Val: port(mtc, "p")}, {Name: "to", Val: port(worker, "q")},
			{Name: "msgValue", Val: tl.Value{Kind: "charstring", Text: `a"b`}.AsValue()}}},
		&tl.Event{Op: "tliMDetected_c", Ts: ts + 1500000, C: worker, Args: []tl.Arg{
			{Name: "at", Val: port(worker, "q")}, {Name: "from", Val: port(mtc, "p")},
			{Name: "msgValue", Val: tl.Value{Kind: "charstring", Text: `a"b`}.AsValue()}}},
		&tl.Event{Op: "tliMReceive_c", Ts: ts, Src: "M.ttcn3", Line: 6, C: worker, Args: []tl.Arg{
			{Name: "at", Val: port(worker, "q")},
			{Name: "msgValue", Val: tl.Value{Kind: "charstring", Text: `a"b`}.AsValue()},
			{Name: "msgTmpl", Val: tl.Value{Kind: "charstring", Match: &tl.Matching{Symbol: "any_value"}}.AsTemplate()}}},
	)
	wantLines(t, got[3:],
		`20261008T072418.794614|fnen|MTC=M.ttcn3:20|M.f(x=2,y=-)`,
		`20261008T072418.794614|fnlv|MTC=M.ttcn3:20|M.f(x=-,y=3)->12.6999999999999992895`,
		`20261008T072418.794614|cost|worker=M.ttcn3:5|M.g`,
		`20261008T072418.794614|ptsd|MTC=M.ttcn3:22|MTC.p|worker.q|charstring|"a""b"`,
		`20261008T072420.294614|ptqu|MTC=M.ttcn3:22|worker.q|message(value=charstring:"a""b",sender=C:MTC(running),timestamp=1.500000)`,
		`20261008T072418.794614|ptrx|worker=M.ttcn3:6|worker.q|value=charstring:?|ready(match=message(value=charstring:"a""b",sender=C:MTC(running),timestamp=1.500000))+consume`,
	)
}

// TestLineValues: values as the lines log writes them — compact, strings
// quoted with a quote doubled and a control character as char(...),
// floats in full precision, a set's and a union's fields named, a
// record's not, and templates with their attributes.
func TestLineValues(t *testing.T) {
	two := 2
	for _, c := range []struct {
		v    tl.Value
		want string
	}{
		{tl.Value{Kind: "integer", Text: "-200"}, `-200`},
		{tl.Value{Kind: "float", Text: "1"}, `1.0`},
		{tl.Value{Kind: "float", Text: "10000"}, `10000.0`},
		{tl.Value{Kind: "float", Text: "2.2"}, `2.20000000000000017764`},
		{tl.Value{Kind: "float", Text: "0.5"}, `0.5`},
		{tl.Value{Kind: "charstring", Text: "a\nb"}, `"a"&char(0,0,0,10)&"b"`},
		{tl.Value{Kind: "charstring", Text: `say "hi" | ok`}, `"say ""hi"" | ok"`},
		{tl.Value{Kind: "charstring", Text: ""}, `""`},
		{tl.Value{Kind: "bitstring", Text: "0101"}, `'0101'B`},
		{tl.Value{Kind: "octetstring", Text: "0aff"}, `'0AFF'O`},
		{tl.Value{Kind: "bitstring", Text: "01?1*"}, `"01?1*"`},
		{tl.Value{Kind: "charstring", Null: true}, `#`},
		{tl.Value{Omit: true}, `omit`},
		{tl.Value{Kind: "record", Elems: []tl.Value{{Name: "a", Kind: "integer", Text: "1"}, {Name: "b", Omit: true}}}, `{1,omit}`},
		{tl.Value{Kind: "set", Elems: []tl.Value{{Name: "a", Kind: "integer", Text: "1"}, {Name: "b", Kind: "boolean", Text: "true"}}}, `{a:=1,b:=true}`},
		{tl.Value{Kind: "union", Elems: []tl.Value{{Name: "i", Kind: "integer", Text: "1"}}}, `{i:=1}`},
		{tl.Value{Kind: "record_of", Elems: []tl.Value{{Kind: "integer", Text: "1"}, {Match: &tl.Matching{Symbol: "any_value_or_none"}}}}, `{1,*}`},
		{tl.Value{Match: &tl.Matching{Symbol: "list", List: []tl.Value{{Kind: "integer", Text: "1"},
			{Kind: "integer", Match: &tl.Matching{Symbol: "range", Lower: &tl.Value{Kind: "integer", Text: "6"}, Upper: &tl.Value{Kind: "integer", Text: "9"}}}}}}, `(1,(6..9))`},
		{tl.Value{Kind: "integer", Match: &tl.Matching{Symbol: "complement", List: []tl.Value{{Kind: "integer", Text: "1"}}}}, `complement(1)`},
		{tl.Value{Kind: "charstring", Match: &tl.Matching{Symbol: "any_value"}, Length: &tl.Length{Lower: 2, Upper: &two}}, `? length(2)`},
		{tl.Value{Kind: "charstring", Match: &tl.Matching{Symbol: "pattern", Pattern: `a\d+`}}, `"a\d+"`},
		{tl.Value{Kind: "integer", Text: "5", IfPresent: true}, `5 ifpresent`},
	} {
		got := linesOf(t, &tl.Event{Op: "tliLog", Ts: ts, C: system, Args: []tl.Arg{{Name: "log", Val: tl.String("x")}},
			Parts: []tl.Value{c.v}})
		if want := `20261008T072418.794614|ulog|ntt|` + c.want; got[0] != want {
			t.Errorf("got %s, want %s", got[0], want)
		}
	}
}

// TestLinesWriterStartsEachTestcaseAfresh: what one testcase left in a
// port's queue, or a timer running, is not the next testcase's.
func TestLinesWriterStartsEachTestcaseAfresh(t *testing.T) {
	port := tl.PortID{Comp: mtc, Name: "p", Index: -1}.Content()
	msg := func(v string) tl.Arg {
		return tl.Arg{Name: "msgValue", Val: tl.Value{Kind: "integer", Text: v}.AsValue()}
	}
	start := func(tc string) *tl.Event {
		return &tl.Event{Op: "tliTcStart", Ts: ts, C: mtc, Args: []tl.Arg{{Name: "tcId", Val: tl.TestcaseID("M", tc)}}}
	}
	got := linesOf(t,
		start("t1"),
		&tl.Event{Op: "tliMDetected_c", Ts: ts, C: mtc, Args: []tl.Arg{{Name: "at", Val: port}, msg("111")}},
		&tl.Event{Op: "tliTStart", Ts: ts, C: mtc, Member: true, Args: []tl.Arg{
			{Name: "timer", Val: tl.TimerID("t", "", "")}, {Name: "dur", Val: tl.Duration(1)}}},
		start("t2"),
		&tl.Event{Op: "tliMDetected_c", Ts: ts, C: mtc, Args: []tl.Arg{{Name: "at", Val: port}, msg("333")}},
		&tl.Event{Op: "tliMReceive_c", Ts: ts, C: mtc, Args: []tl.Arg{{Name: "at", Val: port}, msg("333")}},
		&tl.Event{Op: "tliTTimeoutMismatch", Ts: ts, C: mtc, Member: true, Args: []tl.Arg{
			{Name: "timer", Val: tl.TimerID("t", "", "")}}},
	)
	wantLines(t, got[len(got)-2:],
		`20261008T072418.794614|ptrx|MTC|MTC.p||ready(match=message(value=integer:333,sender=:null,timestamp=0.000000))+consume`,
		`20261008T072418.794614|tmto|MTC|MTC.t|never(inactive)`,
	)
}
