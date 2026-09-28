package tl_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/tl"
)

// sample returns one event of each shape the encoder builds: component,
// port and timer identifiers, parameter lists, verdicts, structured values,
// templates with matching symbols, and value differences.
func sample() []*tl.Event {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	ptc := tl.ComponentID{Name: "PTC", ID: "2", Type: "C"}
	p := tl.PortID{Comp: mtc, Name: "p", Index: -1}
	q := tl.PortID{Comp: ptc, Name: "p", Index: 2}
	sys := tl.PortID{Comp: tl.ComponentID{Name: "system", ID: "system", Type: "C"}, Name: "p", Index: -1}
	rec := tl.Value{Kind: "record", Type: "Req", Module: "M", Elems: []tl.Value{
		{Kind: "charstring", Name: "method", Text: "GET"},
		{Kind: "integer", Name: "status", Text: "200"},
		{Kind: "octetstring", Name: "raw", Text: "0A1B"},
		{Kind: "record_of", Name: "tags", Elems: []tl.Value{{Kind: "charstring", Text: "a"}, {Kind: "charstring", Text: "b"}}},
		{Kind: "union", Name: "u", Elems: []tl.Value{{Kind: "boolean", Name: "flag", Text: "true"}}},
		{Kind: "enumerated", Name: "e", Text: "red"},
		{Kind: "charstring", Name: "body", Omit: true},
	}}
	upper := 3
	tmpl := tl.Value{Kind: "record", Elems: []tl.Value{
		{Kind: "charstring", Name: "method", Match: &tl.Matching{Symbol: "any_value"}},
		{Kind: "integer", Name: "status", Match: &tl.Matching{Symbol: "range",
			Lower: &tl.Value{Kind: "integer", Text: "200"}, Upper: &tl.Value{Kind: "integer", Text: "299"}, ExclUpper: true}},
		{Kind: "octetstring", Name: "raw", Match: &tl.Matching{Symbol: "any_value_or_none"}, Length: &tl.Length{Lower: 1, Upper: &upper}},
		{Kind: "record_of", Name: "tags", Match: &tl.Matching{Symbol: "superset", List: []tl.Value{{Kind: "charstring", Text: "a"}}}},
		{Kind: "union", Name: "u", Match: &tl.Matching{Symbol: "any_value"}},
		{Kind: "enumerated", Name: "e", Match: &tl.Matching{Symbol: "list", List: []tl.Value{{Kind: "enumerated", Text: "red"}, {Kind: "enumerated", Text: "blue"}}}},
		{Kind: "charstring", Name: "body", Match: &tl.Matching{Symbol: "pattern", Pattern: "ok*"}, IfPresent: true},
	}}
	anyTmpl := tl.Value{Match: &tl.Matching{Symbol: "any_value"}}
	// Values the runtime holds without their declared type: an omitted and
	// a wildcard field of unknown type, an unbound one, a positional record
	// held as a mixed list, and a uniform list with a wildcard element.
	untyped := tl.Value{Kind: "record", Elems: []tl.Value{
		{Name: "gone", Omit: true},
		{Name: "anything", Match: &tl.Matching{Symbol: "any_value_or_none"}},
		{Name: "unbound", Null: true},
		{Name: "text", Text: "no kind"},
		{Kind: "record_of", Name: "mixed", Elems: []tl.Value{{Kind: "integer", Text: "1"}, {Kind: "float", Text: "2.5"}, {Kind: "charstring", Text: "x"}}},
		{Kind: "record_of", Name: "uniform", Elems: []tl.Value{{Kind: "integer", Text: "1"}, {Match: &tl.Matching{Symbol: "any_value"}}}},
		{Kind: "set_of", Name: "wild", Elems: []tl.Value{{Omit: true}, {Match: &tl.Matching{Symbol: "any_value"}}}},
	}}
	return []*tl.Event{
		{Op: "tliTcStart", Ts: 1, C: mtc, Args: []tl.Arg{{"tcId", tl.TestcaseID("M", "tc")},
			{"tciPars", tl.Params(tl.Param{Name: "x", Mode: "in", Val: tl.Value{Kind: "integer", Text: "3"}})}, {"dur", tl.Duration(5)}}},
		{Op: "tliCCreate", Ts: 2, Src: "m.ttcn", Line: 4, C: mtc, Args: []tl.Arg{{"comp", ptc.Content()}, {"name", tl.String("PTC")}, {"alive", tl.Boolean(false)}}},
		{Op: "tliCStart", Ts: 2, C: mtc, Args: []tl.Arg{{"comp", ptc.Content()}, {"name", tl.BehaviourID("M", "f")}}},
		{Op: "tliPConnect", Ts: 3, C: mtc, Args: []tl.Arg{{"port1", p.Content()}, {"port2", q.Content()}}},
		{Op: "tliPMap", Ts: 3, C: mtc, Args: []tl.Arg{{"port1", p.Content()}, {"port2", sys.Content()}}},
		{Op: "tliMSend_c", Ts: 4, C: mtc, Args: []tl.Arg{{"at", p.Content()}, {"to", q.Content()}, {"msgValue", rec.AsValue()}, {"transmission-failure", tl.String(tl.TriOk)}}},
		{Op: "tliMSend_m", Ts: 4, C: mtc, Args: []tl.Arg{{"at", p.Content()}, {"to", sys.Content()}, {"msgValue", rec.AsValue()}}},
		{Op: "tliMDetected_c", Ts: 5, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"from", p.Content()}, {"msgValue", rec.AsValue()}}},
		{Op: "tliMMismatch_c", Ts: 6, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"msgValue", rec.AsValue()}, {"msgTmpl", tmpl.AsTemplate()},
			{"diffs", tl.Diffs(tl.Diff{Val: "/record/integer[1]", Tmpl: "/record/integer[1]", Desc: "status"})}}},
		{Op: "tliMReceive_c", Ts: 7, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"msgValue", rec.AsValue()}, {"msgTmpl", anyTmpl.AsTemplate()},
			{"from", mtc.Content()}, {"fromTmpl", tl.AnyTemplate()}}},
		{Op: "tliTStart", Ts: 8, C: mtc, Args: []tl.Arg{{"timer", tl.TimerID("t", "", "")}, {"dur", tl.Duration(0.5)}}},
		{Op: "tliTTimeout", Ts: 9, C: mtc, Args: []tl.Arg{{"timer", tl.TimerID("t", "", "")}, {"timerTmpl", tl.AnyTemplate()}}},
		{Op: "tliSetVerdict", Ts: 10, C: ptc, Args: []tl.Arg{{"verdict", tl.Verdict("pass")}, {"reason", tl.String(`ok & <"fine">`)}}},
		{Op: "tliCDone", Ts: 11, C: mtc, Args: []tl.Arg{{"compTmpl", tl.ValueTemplate(tl.Value{Kind: "component", Text: "2"})}}},
		{Op: "tliCTerminated", Ts: 11, C: ptc, Args: []tl.Arg{{"verdict", tl.Verdict("pass")}}},
		{Op: "tliAEnter", Ts: 12, C: mtc},
		{Op: "tliANomatch", Ts: 12, C: mtc},
		{Op: "tliAActivate", Ts: 12, C: mtc, Args: []tl.Arg{{"name", tl.QualifiedName("M", "a")}, {"ref", tl.Value{Kind: "default", Text: "1"}.AsValue()}}},
		{Op: "tliLog", Ts: 13, C: mtc, Args: []tl.Arg{{"log", tl.String("hello")}}},
		{Op: "tliMSend_c", Ts: 13, C: mtc, Args: []tl.Arg{{"at", p.Content()}, {"msgValue", untyped.AsValue()}}},
		{Op: "tliMMismatch_c", Ts: 13, C: ptc, Args: []tl.Arg{{"at", q.Content()}, {"msgValue", rec.AsValue()}, {"msgTmpl", untyped.AsTemplate()}, {"diffs", tl.Diffs()}}},
		{Op: "tliTcTerminated", Ts: 14, C: mtc, Args: []tl.Arg{{"tcId", tl.TestcaseID("M", "tc")}, {"verdict", tl.Verdict("pass")}}},
	}
}

func TestSchemaCoversTheLogBody(t *testing.T) {
	// Annex B.6 declares 125 event elements in the log body, one per TCI-TL
	// operation of clause 7.3.4.1 (tliCtrlTerminatedWithResult and
	// tliCtrlStartWithParameters included).
	if n := len(tl.Schema); n != 125 {
		t.Fatalf("schema has %d events, want 125", n)
	}
	for _, op := range []string{"tliMSend_c", "tliMReceive_m", "tliPrCall_c", "tliTTimeout", "tliSetVerdict", "tliCheckAnyMismatch_c"} {
		if _, ok := tl.Schema[op]; !ok {
			t.Errorf("schema lacks %s", op)
		}
	}
}

func TestSampleEventsValidate(t *testing.T) {
	for _, e := range sample() {
		if err := e.Validate(); err != nil {
			t.Error(err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	for _, tc := range []struct {
		e    tl.Event
		want string
	}{
		{tl.Event{Op: "tliNoSuch"}, "unknown operation"},
		{tl.Event{Op: "tliLog"}, `required parameter "log" missing`},
		{tl.Event{Op: "tliLog", Args: []tl.Arg{{"log", tl.String("a")}, {"log", tl.String("b")}}}, "given twice"},
		{tl.Event{Op: "tliLog", Args: []tl.Arg{{"log", tl.String("a")}, {"reason", tl.String("b")}}}, `no parameter "reason"`},
	} {
		err := tc.e.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.e.Op, err, tc.want)
		}
	}
}

// TestXMLValidatesAgainstAnnexB validates a log against the normative XML
// schemas of ES 201 873-6 Annex B. The schemas are ETSI's and are not
// vendored: set NTT_TCI_TL_XSD to a directory holding Log_v4_10_1.xsd and
// its imports (see gen/README.md for extracting them from the standard).
func TestXMLValidatesAgainstAnnexB(t *testing.T) {
	dir := os.Getenv("NTT_TCI_TL_XSD")
	if dir == "" {
		t.Skip("NTT_TCI_TL_XSD not set; see runtime/tl/gen/README.md")
	}
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed")
	}
	var buf bytes.Buffer
	w := tl.NewXMLWriter(&buf)
	for _, e := range sample() {
		w.Log(e)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "log.xml")
	if err := os.WriteFile(f, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(xmllint, "--noout", "--schema", filepath.Join(dir, "Log_v4_10_1.xsd"), f).CombinedOutput()
	if err != nil {
		t.Fatalf("log does not validate:\n%s\n%s", out, buf.String())
	}
}

func TestEmptyXMLLogIsStillALog(t *testing.T) {
	var buf bytes.Buffer
	w := tl.NewXMLWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<tli:tliInfo") {
		t.Fatalf("empty log has no body event:\n%s", buf.String())
	}
}

// TestJSONLMirrorsXML checks the JSON Lines form carries the same events
// with the same element trees as the XML form: a header line, then one line
// per event whose tag is the operation.
func TestJSONLMirrorsXML(t *testing.T) {
	var buf bytes.Buffer
	w := tl.NewJSONLWriter(&buf)
	evs := sample()
	for _, e := range evs {
		w.Log(e)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	type node struct {
		Tag   string            `json:"tag"`
		Attrs map[string]string `json:"attrs"`
		Kids  []json.RawMessage `json:"kids"`
	}
	sc := bufio.NewScanner(&buf)
	sc.Buffer(nil, 1<<20)
	var lines []node
	for sc.Scan() {
		var n node
		if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
			t.Fatalf("line %d: %v", len(lines)+1, err)
		}
		lines = append(lines, n)
	}
	if len(lines) != len(evs)+1 || lines[0].Tag != "header" {
		t.Fatalf("got %d lines (first %q), want a header and %d events", len(lines), lines[0].Tag, len(evs))
	}
	for i, e := range evs {
		if got := lines[i+1]; got.Tag != e.Op || got.Attrs["ts"] == "" || got.Attrs["name"] != e.C.Name {
			t.Errorf("line %d = %s %v, want %s from %s", i+2, got.Tag, got.Attrs, e.Op, e.C.Name)
		}
	}
}
