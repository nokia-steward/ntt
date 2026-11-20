package runtime_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// receiveConfig is an MTC whose port p holds the messages 1, 2 and 3,
// in that order, behind a call.
func receiveConfig() *runtime.TestcaseExec {
	exec := runtime.NewTestcaseExec("tc")
	exec.SetMTCID(1)
	exec.PushComponent(newComponent(1, false))
	exec.EnqueueEnvelope("p", runtime.PortMessage{Kind: runtime.MsgCall, Signature: "S"})
	for _, v := range []string{"1", "2", "3"} {
		exec.EnqueueMessageFrom("p", runtime.NewInt(v), nil)
	}
	return exec
}

func payloadIs(v string) func(runtime.PortMessage) bool {
	return func(m runtime.PortMessage) bool { return m.Payload != nil && m.Payload.Inspect() == v }
}

// A receiving operation matches the message at the head of its queue: a
// receive removes it when it matches, a check does not, and a head that
// does not match stays (ETSI ES 201 873-1 22.2.2, 22.4). A trigger removes
// the heads that do not match and takes the first that does (22.2.3),
// among those the alt's snapshot sees.
func TestReceive(t *testing.T) {
	for _, c := range []struct {
		name    string
		op      runtime.ReceiveOp
		want    runtime.ReceiveResult
		payload string // of the message matched
		left    int    // messages left on p, the call aside
	}{
		{"a receive of the head", runtime.ReceiveOp{Consume: true, Match: payloadIs("1")}, runtime.ReceiveMatched, "1", 2},
		{"a check of the head", runtime.ReceiveOp{Match: payloadIs("1")}, runtime.ReceiveMatched, "1", 3},
		{"a receive of another", runtime.ReceiveOp{Consume: true, Match: payloadIs("2")}, runtime.ReceiveMismatch, "", 3},
		{"a trigger", runtime.ReceiveOp{Consume: true, Trigger: true, Match: payloadIs("3")}, runtime.ReceiveMatched, "3", 0},
		{"a trigger matching none", runtime.ReceiveOp{Consume: true, Trigger: true, Match: payloadIs("4")}, runtime.ReceiveEmpty, "", 0},
		{"a trigger in a snapshot of three entries, the call one", runtime.ReceiveOp{Limit: 3, Consume: true, Trigger: true, Match: payloadIs("3")}, runtime.ReceiveEmpty, "", 1},
	} {
		exec := receiveConfig()
		op := c.op
		op.Port, op.Kind = "p", runtime.MsgMessage
		if op.Limit == 0 {
			op.Limit = -1
		}
		m, res := exec.Receive(op)
		got := ""
		if res == runtime.ReceiveMatched {
			got = m.Payload.Inspect()
		}
		if res != c.want || got != c.payload || exec.QueueLen("p")-1 != c.left {
			t.Errorf("%s: %v %q, %d left; want %v %q, %d left", c.name, res, got, exec.QueueLen("p")-1, c.want, c.payload, c.left)
		}
	}
}

// A check with no receiving operation sees the call at the head; a
// procedure receive takes only envelopes of its kind.
func TestReceiveKinds(t *testing.T) {
	exec := receiveConfig()
	anyMsg := func(m runtime.PortMessage) bool { return true }
	if m, res := exec.Receive(runtime.ReceiveOp{Port: "p", AnyKind: true, Limit: -1, Match: anyMsg}); res != runtime.ReceiveMatched || m.Kind != runtime.MsgCall {
		t.Errorf("a check of any kind: %v %v; want the call", res, m.Kind)
	}
	if _, res := exec.Receive(runtime.ReceiveOp{Port: "p", Kind: runtime.MsgReply, Limit: -1, Consume: true, Match: anyMsg}); res != runtime.ReceiveEmpty {
		t.Errorf("a getreply: %v; want nothing to take", res)
	}
	if m, res := exec.Receive(runtime.ReceiveOp{Port: "p", Kind: runtime.MsgCall, Limit: -1, Consume: true, Match: anyMsg}); res != runtime.ReceiveMatched || m.Signature != "S" || exec.QueueLen("p") != 3 {
		t.Errorf("a getcall: %v %q, %d left; want the call, 3 left", res, m.Signature, exec.QueueLen("p"))
	}
}
