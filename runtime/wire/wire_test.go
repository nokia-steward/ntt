package wire_test

import (
	"bytes"
	"testing"

	"github.com/nokia/ntt/runtime/wire"
)

func TestEncodeDecodeHello(t *testing.T) {
	var buf bytes.Buffer
	enc := wire.NewEncoder(&buf)
	if err := enc.EncodeBody(wire.HelloKind, "1", wire.Hello{Host: "h1", Cases: []string{"M.a"}, Version: "0.1"}); err != nil {
		t.Fatalf("EncodeBody: %v", err)
	}
	dec := wire.NewDecoder(&buf)
	m, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Kind != wire.HelloKind {
		t.Errorf("Kind = %v, want hello", m.Kind)
	}
	var h wire.Hello
	if err := m.UnmarshalBody(&h); err != nil {
		t.Fatalf("UnmarshalBody: %v", err)
	}
	if h.Host != "h1" || len(h.Cases) != 1 || h.Cases[0] != "M.a" {
		t.Errorf("body mismatch: %+v", h)
	}
}

func TestEncodeRoundTripAllKinds(t *testing.T) {
	cases := []struct {
		kind wire.MessageKind
		body interface{}
	}{
		{wire.HelloKind, wire.Hello{Host: "h"}},
		{wire.ReadyKind, wire.Ready{Session: "s"}},
		{wire.RunKind, wire.Run{Case: "M.tc"}},
		{wire.VerdictKind, wire.Verdict{Case: "M.tc", Verdict: "pass", Duration: 0.1}},
		{wire.LogKind, wire.Log{Level: "info", Text: "hi"}},
		{wire.StopKind, wire.Stop{Reason: "done"}},
		{wire.GoodbyeKind, wire.Goodbye{}},
	}
	var buf bytes.Buffer
	enc := wire.NewEncoder(&buf)
	for _, c := range cases {
		if err := enc.EncodeBody(c.kind, "id", c.body); err != nil {
			t.Fatalf("encode %v: %v", c.kind, err)
		}
	}
	dec := wire.NewDecoder(&buf)
	for _, c := range cases {
		m, err := dec.Decode()
		if err != nil {
			t.Fatalf("decode %v: %v", c.kind, err)
		}
		if m.Kind != c.kind {
			t.Errorf("Kind = %v, want %v", m.Kind, c.kind)
		}
	}
}

func TestUnmarshalBody_EmptyError(t *testing.T) {
	m := wire.Message{Kind: wire.HelloKind}
	var h wire.Hello
	if err := m.UnmarshalBody(&h); err == nil {
		t.Error("expected error on empty body")
	}
}
