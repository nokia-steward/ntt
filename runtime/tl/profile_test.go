package tl_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/tl"
)

// TestProfile follows the live profiler's model over a logged run: each
// send answered by a receive on the same port, before the next send, is
// one latency sample.
func TestProfile(t *testing.T) {
	mtc := tl.ComponentID{Name: "mtc", ID: "1", Type: "C"}
	ptc := tl.ComponentID{Name: "echo", ID: "2", Type: "C"}
	p := tl.PortID{Comp: mtc, Name: "p", Index: -1}
	q := tl.PortID{Comp: ptc, Name: "p", Index: -1}
	val := tl.Value{Kind: "charstring", Text: "ping"}.AsValue()
	tcID := tl.Arg{Name: "tcId", Val: tl.TestcaseID("M", "tc")}
	send := func(ts int64, c tl.ComponentID, at tl.PortID) *tl.Event {
		return &tl.Event{Op: "tliMSend_c", Ts: ts, C: c, Args: []tl.Arg{{"at", at.Content()}, {"msgValue", val}}}
	}
	recv := func(ts int64, c tl.ComponentID, at tl.PortID) *tl.Event {
		return &tl.Event{Op: "tliMReceive_c", Ts: ts, C: c, Args: []tl.Arg{{"at", at.Content()}, {"msgValue", val}}}
	}
	evs := []*tl.Event{
		{Op: "tliTcStart", Ts: 1000, C: mtc, Args: []tl.Arg{tcID}},
		{Op: "tliCCreate", Ts: 1000, C: mtc, Args: []tl.Arg{{"comp", ptc.Content()}, {"name", tl.String("echo")}, {"alive", tl.Boolean(false)}}},
		send(1000, mtc, p), recv(1100, ptc, q), send(1100, ptc, q), recv(1250, mtc, p), // 250µs
		send(2000, mtc, p), send(2100, mtc, p), recv(2400, mtc, p), // the second send answered: 300µs
		recv(3000, mtc, p), // answers no send
		{Op: "tliTcTerminated", Ts: 5000, C: mtc, Args: []tl.Arg{tcID, {"verdict", tl.Verdict("pass")}, {"reason", tl.String("ok")}}},
	}
	for _, jsonl := range []bool{false, true} {
		got := logOf(t, jsonl, evs).Profile()
		want := []tl.TestcaseProfile{{
			Testcase: "M.tc", Module: "M", Name: "tc", Verdict: "pass", Reason: "ok", Duration: 4 * time.Millisecond,
			Ports: []tl.PortProfile{
				{Component: "echo", Port: "p", Sends: 1, Receives: 1},
				{Component: "mtc", Port: "p", Sends: 3, Receives: 3, Latencies: []time.Duration{250 * time.Microsecond, 300 * time.Microsecond}},
			},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("jsonl=%v:\n got %+v\nwant %+v", jsonl, got, want)
		}
	}
}
