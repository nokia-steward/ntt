package text_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/runtime/codec/text"
)

func TestTEXT_EncodeRecord(t *testing.T) {
	plan, _ := text.Codec{}.Compile(nil)
	rec := codec.NewRecord(
		"Host", codec.StringValue("example.com"),
		"Port", codec.IntValue(443),
	)
	wire, err := text.Codec{}.Encode(plan, rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	s := string(wire)
	if !strings.Contains(s, "Host: example.com") {
		t.Errorf("missing Host line: %q", s)
	}
	if !strings.Contains(s, "Port: 443") {
		t.Errorf("missing Port line: %q", s)
	}
}

func TestTEXT_RoundTripPrimitive(t *testing.T) {
	plan, _ := text.Codec{}.Compile(nil)
	in := codec.StringValue("hello")
	wire, _ := text.Codec{}.Encode(plan, in)
	out, err := text.Codec{}.Decode(plan, wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if sv, ok := out.(codec.StringValue); !ok || string(sv) != string(in) {
		t.Errorf("round trip: got %v, want %v", out, in)
	}
}
