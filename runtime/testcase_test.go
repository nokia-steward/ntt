package runtime_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

func TestTestcaseExec_VerdictAggregation(t *testing.T) {
	cases := []struct {
		name    string
		seq     []runtime.Verdict
		reasons []string
		want    runtime.Verdict
		reason  string
	}{
		{"none-stays-none", nil, nil, runtime.NoneVerdict, ""},
		{"pass-only", []runtime.Verdict{runtime.PassVerdict}, []string{""}, runtime.PassVerdict, ""},
		{"pass-then-fail", []runtime.Verdict{runtime.PassVerdict, runtime.FailVerdict}, []string{"", "broken"}, runtime.FailVerdict, "broken"},
		{"fail-then-pass-stays-fail", []runtime.Verdict{runtime.FailVerdict, runtime.PassVerdict}, []string{"x", ""}, runtime.FailVerdict, "x"},
		{"inconc-then-fail", []runtime.Verdict{runtime.InconcVerdict, runtime.FailVerdict}, []string{"a", "b"}, runtime.FailVerdict, "b"},
		{"error-wins-everything", []runtime.Verdict{runtime.PassVerdict, runtime.FailVerdict, runtime.ErrorVerdict, runtime.PassVerdict}, []string{"", "x", "boom", ""}, runtime.ErrorVerdict, "boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := runtime.NewTestcaseExec("tc")
			for i, v := range tc.seq {
				ex.SetVerdict(v, tc.reasons[i])
			}
			if got := ex.GetVerdict(); got != tc.want {
				t.Errorf("verdict = %s, want %s", got, tc.want)
			}
			if got := ex.Reason(); got != tc.reason {
				t.Errorf("reason = %q, want %q", got, tc.reason)
			}
		})
	}
}

func TestTestcaseExec_Log(t *testing.T) {
	ex := runtime.NewTestcaseExec("tc")
	ex.Log("hello")
	ex.Log("world")
	got := ex.Logs()
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("Logs = %v", got)
	}
}

func TestTestcaseExec_StopFlag(t *testing.T) {
	ex := runtime.NewTestcaseExec("tc")
	if ex.Stopped() {
		t.Fatal("fresh exec is stopped")
	}
	ex.Stop()
	if !ex.Stopped() {
		t.Fatal("Stop did not flip Stopped")
	}
}

func TestTestcaseExec_ProcedureEnvelopeIsolation(t *testing.T) {
	ex := runtime.NewTestcaseExec("tc")

	// A queued procedure call must be invisible to message receive
	// and to HasPendingCalls must report it.
	if ex.HasPendingCalls() {
		t.Fatal("fresh exec reports pending calls")
	}
	ex.EnqueueEnvelope("p", runtime.PortMessage{Kind: runtime.MsgCall, Signature: "S"})
	ex.EnqueueMessageFrom("p", runtime.NewInt(7), nil)
	if !ex.HasPendingCalls() {
		t.Fatal("HasPendingCalls did not see the queued call")
	}

	// Message receive steps over the call and returns the message.
	got, ok := ex.DequeueMessage("p")
	if !ok {
		t.Fatal("DequeueMessage found nothing")
	}
	if iv, ok := got.(runtime.Int); !ok || iv.Int64() != 7 {
		t.Fatalf("DequeueMessage = %v, want Int(7)", got)
	}

	// The call is still queued (message receive did not consume it).
	if _, ok := ex.PeekKind("p", runtime.MsgCall); !ok {
		t.Fatal("call envelope was consumed by message receive")
	}

	// DequeueKind pulls the call; afterwards no call is pending.
	call, ok := ex.DequeueKind("p", runtime.MsgCall)
	if !ok || call.Kind != runtime.MsgCall || call.Signature != "S" {
		t.Fatalf("DequeueKind(call) = %+v,%v", call, ok)
	}
	if ex.HasPendingCalls() {
		t.Fatal("HasPendingCalls still true after draining the call")
	}
	if _, ok := ex.DequeueKind("p", runtime.MsgReply); ok {
		t.Fatal("DequeueKind(reply) matched on an empty queue")
	}
}

func TestTestcaseExec_ReplyCarriesParamsAndReturn(t *testing.T) {
	ex := runtime.NewTestcaseExec("tc")

	// A reply envelope carries the signature parameter record in
	// Payload and the procedure return value in RetValue; both must
	// survive the round-trip through the kind-tagged queue so the
	// getreply `-> param` / `-> value` redirects can bind them.
	params := &runtime.Record{Fields: map[string]runtime.Object{"p_par": runtime.NewInt(2)}}
	ex.EnqueueEnvelope("p[1]", runtime.PortMessage{
		Kind:     runtime.MsgReply,
		Payload:  params,
		RetValue: runtime.NewInt(5),
	})

	// A reply must not be visible to message receive.
	if _, ok := ex.DequeueMessage("p[1]"); ok {
		t.Fatal("reply envelope leaked into message receive")
	}

	got, ok := ex.DequeueKind("p[1]", runtime.MsgReply)
	if !ok {
		t.Fatal("DequeueKind(reply) found nothing")
	}
	if iv, ok := got.RetValue.(runtime.Int); !ok || iv.Int64() != 5 {
		t.Fatalf("reply RetValue = %v, want Int(5)", got.RetValue)
	}
	rec, ok := got.Payload.(*runtime.Record)
	if !ok {
		t.Fatalf("reply Payload = %T, want *Record", got.Payload)
	}
	if pv, ok := rec.Fields["p_par"].(runtime.Int); !ok || pv.Int64() != 2 {
		t.Fatalf("reply Payload.p_par = %v, want Int(2)", rec.Fields["p_par"])
	}
}

func TestParseVerdict(t *testing.T) {
	cases := map[string]runtime.Verdict{
		"pass":  runtime.PassVerdict,
		"PASS":  runtime.PassVerdict,
		"fail":  runtime.FailVerdict,
		"error": runtime.ErrorVerdict,
		"none":  runtime.NoneVerdict,
		"inconc": runtime.InconcVerdict,
	}
	for in, want := range cases {
		got, ok := runtime.ParseVerdict(in)
		if !ok || got != want {
			t.Errorf("ParseVerdict(%q) = %v,%v want %v,true", in, got, ok, want)
		}
	}
	if _, ok := runtime.ParseVerdict("nonsense"); ok {
		t.Errorf("ParseVerdict accepted nonsense")
	}
}
