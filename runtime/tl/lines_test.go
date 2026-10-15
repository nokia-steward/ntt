package tl_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/tl"
)

// TestLinesWriter: one event per line, `timestamp|event|component=file:line
// |fields`, the fields as TTCN-3 writes them; a setverdict line carries the
// verdict before it, a testcase start the duration execute() gave it, and a
// `|` or a newline in a field is escaped.
func TestLinesWriter(t *testing.T) {
	var b bytes.Buffer
	w := tl.NewLinesWriter(&b)
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	const ts = 1791444258794614 // 2026-10-08T07:24:18.794614Z
	w.Log(&tl.Event{Op: "tliTcExecute", Ts: ts, C: tl.ComponentID{Null: true}, Args: []tl.Arg{
		{"tcId", tl.TestcaseID("M", "tc")}, {"dur", tl.Duration(5)}}})
	w.Log(&tl.Event{Op: "tliTcStart", Ts: ts, Src: "M.ttcn3", Line: 9, C: mtc, Args: []tl.Arg{
		{"tcId", tl.TestcaseID("M", "tc")}}})
	w.Log(&tl.Event{Op: "tliSetVerdict", Ts: ts, Src: "M.ttcn3", Line: 12, C: mtc, Args: []tl.Arg{
		{"verdict", tl.String("pass")}, {"reason", tl.String("a|b\nc")}}})
	w.Log(&tl.Event{Op: "tliSetVerdict", Ts: ts, Src: "M.ttcn3", Line: 13, C: mtc, Args: []tl.Arg{
		{"verdict", tl.String("inconc")}}})
	w.Log(&tl.Event{Op: "tliANomatch", Ts: ts, C: mtc}) // no line of its own
	w.Log(&tl.Event{Op: "tliTStart", Ts: ts, Src: "M.ttcn3", Line: 14, C: mtc, Args: []tl.Arg{
		{"timer", tl.TimerID("t", "", "")}, {"dur", tl.Duration(0.5)}}})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`20261008T072418.794614|tcst|ntt=M.ttcn3:9|M.tc|5`,
		`20261008T072418.794614|setv|mtc=M.ttcn3:12|none|pass|a\|b\nc`,
		`20261008T072418.794614|setv|mtc=M.ttcn3:13|pass|inconc|`,
		`20261008T072418.794614|tmst|mtc=M.ttcn3:14|t|0.5`,
	}
	got := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
