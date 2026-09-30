package tl_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/tl"
)

// run builds the events of one testcase in which the MTC sends v to a PTC
// with id ptcID, which receives it; arrivals are logged too.
func run(tc, v, ptcID string, detectedFirst bool) []*tl.Event {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	ptc := tl.ComponentID{Name: "echo", ID: ptcID, Type: "C"}
	p := tl.PortID{Comp: mtc, Name: "p", Index: -1}
	q := tl.PortID{Comp: ptc, Name: "p", Index: -1}
	val := tl.Value{Kind: "charstring", Text: v}
	tcID := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", tc)}
	detected := &tl.Event{Op: "tliMDetected_c", Ts: 3, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"from", p.Content()}, {"msgValue", val.AsValue()}}}
	enter := &tl.Event{Op: "tliAEnter", Ts: 3, Line: 5, C: ptc}
	nomatch := &tl.Event{Op: "tliANomatch", Ts: 3, Line: 5, C: ptc}
	evs := []*tl.Event{
		{Op: "tliTcStart", Ts: 1, C: mtc, Args: []tl.Arg{tcID}},
		{Op: "tliCCreate", Ts: 1, Line: 9, C: mtc, Args: []tl.Arg{{"comp", ptc.Content()}, {"name", tl.String("echo")}, {"alive", tl.Boolean(false)}}},
		{Op: "tliMSend_c", Ts: 2, Line: 10, C: mtc, Args: []tl.Arg{{"at", p.Content()}, {"to", q.Content()}, {"msgValue", val.AsValue()}}},
	}
	// The two orders a correct run can log: the message already queued
	// when the PTC reaches its alt, or the PTC waiting for it.
	if detectedFirst {
		evs = append(evs, detected, enter)
	} else {
		evs = append(evs, enter, nomatch, detected)
	}
	evs = append(evs,
		&tl.Event{Op: "tliMReceive_c", Ts: 4, Line: 6, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"msgValue", val.AsValue()},
			{"from", mtc.Content()}}},
		&tl.Event{Op: "tliSetVerdict", Ts: 5, Line: 6, C: ptc, Args: []tl.Arg{{"verdict", tl.Verdict("pass")}}},
		&tl.Event{Op: "tliCDone", Ts: 6, Line: 11, C: mtc, Args: []tl.Arg{{"compTmpl", tl.ValueTemplate(tl.Value{Kind: "component", Text: ptcID})}}},
		&tl.Event{Op: "tliTcTerminated", Ts: 7, C: mtc, Args: []tl.Arg{tcID, {"verdict", tl.Verdict("pass")}}},
	)
	return evs
}

func logOf(t *testing.T, jsonl bool, runs ...[]*tl.Event) *tl.Log {
	t.Helper()
	var buf bytes.Buffer
	w := tl.NewXMLWriter(&buf)
	if jsonl {
		w = tl.NewJSONLWriter(&buf)
	}
	for _, r := range runs {
		for _, e := range r {
			w.Log(e)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	l, err := tl.ReadLog(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestReadLogReadsBothFormatsAlike(t *testing.T) {
	x, j := logOf(t, false, run("tc", "ping", "2", true)), logOf(t, true, run("tc", "ping", "2", true))
	if len(x.Events) != len(j.Events) || x.Header == nil || j.Header == nil {
		t.Fatalf("XML has %d events, JSON Lines %d", len(x.Events), len(j.Events))
	}
	for _, r := range tl.Compare(x, j) {
		if !r.Same() {
			t.Errorf("%s differs between the formats: %+v", r.Testcase, r.Differences)
		}
	}
}

func TestCompareIgnoresArrivalTimingAndIDs(t *testing.T) {
	a := logOf(t, true, run("tc", "ping", "2", true))
	b := logOf(t, false, run("tc", "ping", "7", false))
	res := tl.Compare(a, b)
	if len(res) != 1 || !res[0].Same() {
		t.Fatalf("equivalent runs reported different: %+v", res)
	}
	if res[0].Compared == 0 {
		t.Fatalf("nothing was compared")
	}
}

func TestCompareFindsDifferences(t *testing.T) {
	base := run("tc", "ping", "2", true)
	other := run("tc", "pong", "2", true)
	res := tl.Compare(logOf(t, true, base), logOf(t, true, other))
	if len(res) != 1 || res[0].Same() {
		t.Fatalf("a different message was not found: %+v", res)
	}
	var comps []string
	for _, d := range res[0].Differences {
		comps = append(comps, d.Component)
		if d.A == nil || d.B == nil || !strings.Contains(tl.Summary(d.A), `"ping"`) || !strings.Contains(tl.Summary(d.B), `"pong"`) {
			t.Errorf("%s: difference not located at the message: %s / %s", d.Component, tl.Summary(d.A), tl.Summary(d.B))
		}
	}
	if strings.Join(comps, ",") != "echo,mtc" {
		t.Errorf("differing components = %v, want echo and mtc", comps)
	}

	// A component that stops early, and a testcase only one run has.
	short := run("tc", "ping", "2", true)
	short = append(short[:len(short)-3:len(short)-3], short[len(short)-2:]...) // no setverdict from echo
	res = tl.Compare(logOf(t, true, base, run("only", "x", "2", true)), logOf(t, true, short))
	if len(res) != 2 || res[0].Same() || res[1].Missing != "b" {
		t.Fatalf("results = %+v", res)
	}
	if d := res[0].Differences[0]; d.Component != "echo" || d.A == nil || d.B != nil {
		t.Errorf("missing action not reported: %+v", d)
	}
}

func TestSummaryIsReadable(t *testing.T) {
	l := logOf(t, true, run("tc", "ping", "2", true))
	var got []string
	for _, e := range l.Events {
		got = append(got, tl.Summary(e))
	}
	for _, want := range []string{
		`tliMSend_c [mtc, line 10] at=mtc:p to=echo:p msgValue="ping"`,
		`tliSetVerdict [echo, line 6] verdict=pass`,
		`tliTcTerminated [mtc] tcId=M.tc verdict=pass`,
	} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("no summary %q among\n%s", want, strings.Join(got, "\n"))
		}
	}
}

// twoWorkers is a testcase with two components created with the same name
// "w", ids 2 and 3, each setting a verdict; order says which of them goes
// first in the log.
func twoWorkers(v2, v3 string, w3First bool) []*tl.Event {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	w2 := tl.ComponentID{Name: "w", ID: "2", Type: "C"}
	w3 := tl.ComponentID{Name: "w", ID: "3", Type: "C"}
	tc := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
	create := func(c tl.ComponentID) *tl.Event {
		return &tl.Event{Op: "tliCCreate", Line: 5, C: mtc, Args: []tl.Arg{{"comp", c.Content()}, {"name", tl.String("w")}, {"alive", tl.Boolean(false)}}}
	}
	verdict := func(c tl.ComponentID, v string) *tl.Event {
		return &tl.Event{Op: "tliSetVerdict", Line: 3, C: c, Args: []tl.Arg{{"verdict", tl.Verdict(v)}}}
	}
	a, b := verdict(w2, v2), verdict(w3, v3)
	if w3First {
		a, b = b, a
	}
	return []*tl.Event{
		{Op: "tliTcStart", C: mtc, Args: []tl.Arg{tc}},
		create(w2), create(w3), a, b,
		{Op: "tliTcTerminated", C: mtc, Args: []tl.Arg{tc, {"verdict", tl.Verdict("fail")}}},
	}
}

// TestCompareTellsSameNamedComponentsApart: components created with the
// same name are compared by creation order, not merged into one stream.
// Merged, two workers that swapped roles compared the same, and two that
// merely interleaved differently compared different.
func TestCompareTellsSameNamedComponentsApart(t *testing.T) {
	if r := tl.Compare(logOf(t, true, twoWorkers("pass", "fail", false)), logOf(t, true, twoWorkers("pass", "fail", true))); !r[0].Same() {
		t.Errorf("the same workers, interleaved differently, compared different: %+v", r[0].Differences)
	}
	r := tl.Compare(logOf(t, true, twoWorkers("pass", "fail", false)), logOf(t, true, twoWorkers("fail", "pass", false)))
	if r[0].Same() {
		t.Fatalf("workers that swapped verdicts compared the same")
	}
	if got := r[0].Differences[0].Component; got != "w" {
		t.Errorf("first differing component = %q, want w (the first created)", got)
	}
}

// TestCompareKeepsCreatedNamesApartFromRoles: a component created with the
// name of a role (mtc) or of another component's id is still its own.
func TestCompareKeepsCreatedNamesApartFromRoles(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	fake := tl.ComponentID{Name: "mtc", ID: "2", Type: "C"}
	tc := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
	run := func(first, second tl.ComponentID) []*tl.Event {
		return []*tl.Event{
			{Op: "tliTcStart", C: mtc, Args: []tl.Arg{tc}},
			{Op: "tliCCreate", C: mtc, Args: []tl.Arg{{"comp", fake.Content()}, {"name", tl.String("mtc")}, {"alive", tl.Boolean(false)}}},
			{Op: "tliLog", Line: 1, C: first, Args: []tl.Arg{{"log", tl.String("one")}}},
			{Op: "tliLog", Line: 2, C: second, Args: []tl.Arg{{"log", tl.String("two")}}},
			{Op: "tliTcTerminated", C: mtc, Args: []tl.Arg{tc, {"verdict", tl.Verdict("none")}}},
		}
	}
	if r := tl.Compare(logOf(t, true, run(mtc, fake)), logOf(t, true, run(fake, mtc))); r[0].Same() {
		t.Errorf("the MTC and a PTC created as \"mtc\" were merged into one component")
	}
}

// TestCompareFilesLateEventsOutsideTestcases: an event a component logged
// after its testcase ended is compared outside any testcase, wherever it
// falls in the log.
func TestCompareFilesLateEventsOutsideTestcases(t *testing.T) {
	late := &tl.Event{Op: "tliInfo", C: tl.ComponentID{Name: "2", ID: tl.LateID}, Args: []tl.Arg{
		{"level", tl.Integer(1)}, {"info", tl.String("after testcase M.tc ended: tliCTerminated [2]")}}}
	first, second := run("tc", "ping", "2", true), run("tc2", "ping", "2", true)
	a := logOf(t, true, first, []*tl.Event{late}, second)                 // before tc2 starts
	b := logOf(t, true, first, second[:2], []*tl.Event{late}, second[2:]) // inside tc2
	for _, r := range tl.Compare(a, b) {
		if !r.Same() {
			t.Errorf("%s: a late event's position made the runs differ: %+v", r.Testcase, r.Differences)
		}
	}
}

// TestCompareXMLAgainstJSONL: text XML cannot hold compares as XML holds
// it, so a run compares the same with its own XML and JSON Lines logs.
func TestCompareXMLAgainstJSONL(t *testing.T) {
	odd := run("tc", "a\x01b", "2", true)
	for _, r := range tl.Compare(logOf(t, false, odd), logOf(t, true, odd)) {
		if !r.Same() {
			t.Errorf("%s: XML and JSON Lines of one run differ: %+v", r.Testcase, r.Differences)
		}
	}
}

// TestCompareSeesTheSourceFile: the same operation at the same line of a
// different file is a different operation.
func TestCompareSeesTheSourceFile(t *testing.T) {
	a, b := run("tc", "ping", "2", true), run("tc", "ping", "2", true)
	a[2].Src, b[2].Src = "/x/one.ttcn", "/y/two.ttcn"
	if r := tl.Compare(logOf(t, true, a), logOf(t, true, b)); r[0].Same() {
		t.Errorf("a send in another file compared the same")
	}
	b[2].Src = "/elsewhere/one.ttcn"
	if r := tl.Compare(logOf(t, true, a), logOf(t, true, b)); !r[0].Same() {
		t.Errorf("the same file at another path compared different: %+v", r[0].Differences)
	}
}

func TestReadLogRejectsWhatIsNotALog(t *testing.T) {
	for name, in := range map[string]string{
		"not TCI-TL":        "{\"a\":1}\n{\"b\":2}\n",
		"no header":         "{\"tag\":\"tliLog\"}\n",
		"null element":      "{\"tag\":\"header\"}\n{\"tag\":\"x\",\"kids\":[null]}\n{\"tag\":\"y\"}\n",
		"bad line mid-file": "{\"tag\":\"header\"}\n{oops\n{\"tag\":\"tliLog\"}\n",
		"after logfile":     "<logfile><header/><body/></logfile><logfile/>",
	} {
		if _, err := tl.ReadLog(strings.NewReader(in)); err == nil {
			t.Errorf("%s: read without error", name)
		}
	}
}

func TestReadLogToleratesWhatARunLeaves(t *testing.T) {
	full := func(jsonl bool) string {
		var buf bytes.Buffer
		w := tl.NewXMLWriter(&buf)
		if jsonl {
			w = tl.NewJSONLWriter(&buf)
		}
		for _, e := range run("tc", "ping", "2", true) {
			w.Log(e)
		}
		_ = w.Close()
		return buf.String()
	}
	x, j := full(false), full(true)
	cut := func(s string) string { return s[:len(s)-len(s)/5] }
	for name, in := range map[string]string{
		"byte order mark": "\xEF\xBB\xBF" + j,
		"killed, XML":     cut(x),
		"killed, JSONL":   cut(j),
	} {
		l, err := tl.ReadLog(strings.NewReader(in))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.HasPrefix(name, "killed") != l.Truncated || len(l.Events) == 0 {
			t.Errorf("%s: truncated=%v, %d events", name, l.Truncated, len(l.Events))
		}
	}
}

// TestSummaryShowsWhatTellsValuesApart: the parts of a value a reader
// needs to see two values differ.
func TestSummaryShowsWhatTellsValuesApart(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1"}
	up := 3
	ev := &tl.Event{Op: "tliMReceive_c", C: mtc, Args: []tl.Arg{
		{"at", tl.PortID{Comp: mtc, Name: "pa", Index: 1}.Content()},
		{"msgValue", tl.Value{Kind: "octetstring", Text: "AB"}.AsValue()},
		{"msgTmpl", tl.Value{Kind: "record", Type: "R1", Elems: []tl.Value{
			{Kind: "integer", Name: "n", Match: &tl.Matching{Symbol: "range", ExclLower: true,
				Lower: &tl.Value{Kind: "integer", Text: "1"}, Upper: &tl.Value{Kind: "integer", Text: "5"}}},
			{Kind: "float", Name: "f", Text: "1"},
			{Kind: "charstring", Name: "s", Match: &tl.Matching{Symbol: "any_value"}, IfPresent: true, Length: &tl.Length{Lower: 1, Upper: &up}},
		}}.AsTemplate()},
	}}
	got := ev.Summary()
	for _, want := range []string{"at=mtc:pa[1]", "msgValue='AB'O", "R1: {", "n := (!1 .. 5)", "f := 1.0", "s := ? ifpresent length(1 .. 3)"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
}

// TestReadLogToleratesAnyCut: a killed run's XML log may end anywhere,
// inside a multi-byte character included.
func TestReadLogToleratesAnyCut(t *testing.T) {
	var buf bytes.Buffer
	w := tl.NewXMLWriter(&buf)
	for _, e := range run("tc", "grüße — ☃", "2", true) {
		w.Log(e)
	}
	_ = w.Close()
	full := buf.String()
	// From inside the first event: before it, the log has no content.
	start := strings.Index(full, "tliTcStart")
	if start < 0 {
		t.Fatal("no first event")
	}
	for i := start + 1; i < len(full); i++ {
		if _, err := tl.ReadLog(strings.NewReader(full[:i])); err != nil {
			t.Fatalf("cut at %d of %d: %v", i, len(full), err)
		}
	}
}

func TestReadLogReadsLongLines(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 65 MB value")
	}
	mtc := tl.ComponentID{Name: "mtc", ID: "1"}
	var buf bytes.Buffer
	w := tl.NewJSONLWriter(&buf)
	w.Log(&tl.Event{Op: "tliLog", C: mtc, Args: []tl.Arg{{"log", tl.String(strings.Repeat("x", 65<<20))}}})
	_ = w.Close()
	l, err := tl.ReadLog(&buf)
	if err != nil || len(l.Events) != 1 {
		t.Fatalf("read %v, err %v", l, err)
	}
}

func TestArrivalTimingIsNotAMatch(t *testing.T) {
	for op, want := range map[string]bool{
		"tliMMismatch_c": true, "tliPrGetReplyDetected_m": true, "tliCDoneMismatch": true,
		"tliTTimeoutMismatch": true, "tliCheckAnyMismatch_m": true, "tliAWait": true,
		"tliMatchMismatch": false, "tliMatch": false, "tliMReceive_c": false, "tliTTimeout": false,
	} {
		if tl.ArrivalTiming(op) != want {
			t.Errorf("ArrivalTiming(%s) = %v", op, !want)
		}
	}
}

// TestCompareKeysComponentsByCreator: two PTCs each create an unnamed
// helper; which creates first differs between correct runs.
func TestCompareKeysComponentsByCreator(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	a := tl.ComponentID{Name: "a", ID: "2", Type: "C"}
	b := tl.ComponentID{Name: "b", ID: "3", Type: "C"}
	tcID := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
	create := func(by, c tl.ComponentID, name string) *tl.Event {
		return &tl.Event{Op: "tliCCreate", C: by, Args: []tl.Arg{{"comp", c.Content()}, {"name", tl.String(name)}, {"alive", tl.Boolean(false)}}}
	}
	logf := func(v string) *tl.Event {
		return &tl.Event{Op: "tliLog", Args: []tl.Arg{{"log", tl.String(v)}}}
	}
	build := func(aFirst bool) []*tl.Event {
		ha, hb := tl.ComponentID{ID: "4"}, tl.ComponentID{ID: "5"}
		if !aFirst {
			ha.ID, hb.ID = "5", "4"
		}
		ea, eb := logf("A"), logf("B")
		ea.C, eb.C = ha, hb
		evs := []*tl.Event{{Op: "tliTcStart", C: mtc, Args: []tl.Arg{tcID}}, create(mtc, a, "a"), create(mtc, b, "b")}
		if aFirst {
			evs = append(evs, create(a, ha, ""), create(b, hb, ""))
		} else {
			evs = append(evs, create(b, hb, ""), create(a, ha, ""))
		}
		return append(evs, ea, eb, &tl.Event{Op: "tliTcTerminated", C: mtc, Args: []tl.Arg{tcID, {"verdict", tl.Verdict("pass")}}})
	}
	for _, r := range tl.Compare(logOf(t, true, build(true)), logOf(t, true, build(false))) {
		if !r.Same() {
			t.Errorf("%s differs: %+v", r.Testcase, r.Differences)
		}
	}
}

// TestCompareTakesDestinationsAsASet: a broadcast's destinations may be
// listed in any order.
func TestCompareTakesDestinationsAsASet(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	tcID := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
	ports := []tl.PortID{{Comp: tl.ComponentID{Name: "x", ID: "2"}, Name: "p", Index: -1}, {Comp: tl.ComponentID{Name: "y", ID: "3"}, Name: "p", Index: -1}}
	build := func(to ...tl.PortID) []*tl.Event {
		return []*tl.Event{{Op: "tliTcStart", C: mtc, Args: []tl.Arg{tcID}},
			{Op: "tliMSend_c_BC", C: mtc, Args: []tl.Arg{{"at", tl.PortID{Comp: mtc, Name: "p", Index: -1}.Content()},
				{"to", tl.PortIDList(to...)}, {"msgValue", tl.Value{Kind: "integer", Text: "1"}.AsValue()}}},
			{Op: "tliTcTerminated", C: mtc, Args: []tl.Arg{tcID, {"verdict", tl.Verdict("pass")}}}}
	}
	for _, r := range tl.Compare(logOf(t, true, build(ports[0], ports[1])), logOf(t, true, build(ports[1], ports[0]))) {
		if !r.Same() {
			t.Errorf("%s differs: %+v", r.Testcase, r.Differences)
		}
	}
}

func TestSummaryOfAParameterWithoutValue(t *testing.T) {
	n := &tl.Node{Tag: "tliSEnter", Kids: []*tl.Node{{Tag: "tciPars", Kids: []*tl.Node{{Tag: "par", Attrs: []tl.Attr{{Name: "name", Value: "x"}}}}}}}
	if got := tl.Summary(n); !strings.Contains(got, "x := -") {
		t.Errorf("summary: %s", got)
	}
}
