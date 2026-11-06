package ber_test

import (
	"bytes"
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	_ "github.com/nokia/ntt/runtime/codec/ber"
)

// roundTrip encodes v with the BER codec, decodes the result, and
// asserts the two values compare equal via Inspect (which gives us a
// canonical string form for any codec.Value).
func roundTrip(t *testing.T, name string, v codec.Value) codec.Value {
	t.Helper()
	c := codec.Lookup(name)
	if c == nil {
		t.Fatalf("%s codec not registered", name)
	}
	plan, err := c.Compile(nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	wire, err := c.Encode(plan, v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := c.Decode(plan, wire)
	if err != nil {
		t.Fatalf("decode of %x: %v", wire, err)
	}
	if got.Inspect() != v.Inspect() {
		t.Errorf("round-trip mismatch:\n  encoded:  %x\n  expected: %s\n  actual:   %s", wire, v.Inspect(), got.Inspect())
	}
	return got
}

func TestBER_IntegerEdgeCases(t *testing.T) {
	cases := []int64{0, 1, -1, 127, 128, -128, -129, 32767, -32768, 1<<31, -(1 << 31), 1<<62, -(1 << 62)}
	for _, c := range cases {
		roundTrip(t, "BER", codec.IntValue(c))
	}
}

// TestBER_IntegerWireFormat checks the encoder against X.690 examples.
// X.690 Annex C gives `INTEGER 256` as 02 02 01 00; 0 as 02 01 00;
// 127 as 02 01 7F; 128 as 02 02 00 80; -128 as 02 01 80; -129 as
// 02 02 FF 7F.
func TestBER_IntegerWireFormat(t *testing.T) {
	c := codec.Lookup("BER")
	plan, _ := c.Compile(nil)
	cases := []struct {
		n    int64
		want []byte
	}{
		{0, []byte{0x02, 0x01, 0x00}},
		{127, []byte{0x02, 0x01, 0x7F}},
		{128, []byte{0x02, 0x02, 0x00, 0x80}},
		{256, []byte{0x02, 0x02, 0x01, 0x00}},
		{-128, []byte{0x02, 0x01, 0x80}},
		{-129, []byte{0x02, 0x02, 0xFF, 0x7F}},
	}
	for _, tc := range cases {
		got, err := c.Encode(plan, codec.IntValue(tc.n))
		if err != nil {
			t.Fatalf("encode %d: %v", tc.n, err)
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("encode(%d) = %x, want %x", tc.n, got, tc.want)
		}
	}
}

func TestBER_Boolean(t *testing.T) {
	roundTrip(t, "BER", codec.BoolValue(true))
	roundTrip(t, "BER", codec.BoolValue(false))
	// Wire form: TRUE = 01 01 FF, FALSE = 01 01 00.
	c := codec.Lookup("BER")
	plan, _ := c.Compile(nil)
	for _, tc := range []struct {
		val  codec.BoolValue
		want []byte
	}{
		{true, []byte{0x01, 0x01, 0xFF}},
		{false, []byte{0x01, 0x01, 0x00}},
	} {
		got, err := c.Encode(plan, tc.val)
		if err != nil {
			t.Fatalf("encode %v: %v", tc.val, err)
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("encode(%v) = %x, want %x", tc.val, got, tc.want)
		}
	}
}

func TestBER_OctetString(t *testing.T) {
	roundTrip(t, "BER", codec.BytesValue(nil))
	roundTrip(t, "BER", codec.BytesValue([]byte{0x00}))
	roundTrip(t, "BER", codec.BytesValue([]byte("hello, world")))
	roundTrip(t, "BER", codec.BytesValue(make([]byte, 200))) // exercises long-form length.
}

func TestBER_UTF8String(t *testing.T) {
	roundTrip(t, "BER", codec.StringValue("ascii"))
	roundTrip(t, "BER", codec.StringValue("π ≈ 3.14159"))
	roundTrip(t, "BER", codec.StringValue(""))
}

func TestBER_Sequence(t *testing.T) {
	// SEQUENCE OF { INTEGER, OCTET STRING, BOOLEAN }
	v := codec.ListValue{
		Elements: []codec.Value{
			codec.IntValue(42),
			codec.BytesValue([]byte{0xDE, 0xAD, 0xBE, 0xEF}),
			codec.BoolValue(true),
		},
	}
	roundTrip(t, "BER", v)
}

func TestBER_DERAndCERAreRegistered(t *testing.T) {
	for _, name := range []string{"BER", "DER", "CER"} {
		if codec.Lookup(name) == nil {
			t.Errorf("%s codec missing", name)
		}
	}
}

func TestBER_DecodeRejectsTrailingGarbage(t *testing.T) {
	c := codec.Lookup("BER")
	plan, _ := c.Compile(nil)
	wire, _ := c.Encode(plan, codec.IntValue(1))
	wire = append(wire, 0xFF)
	if _, err := c.Decode(plan, wire); err == nil {
		t.Errorf("decode should reject trailing garbage")
	}
}

func TestBER_DecodeRejectsIndefiniteLength(t *testing.T) {
	c := codec.Lookup("BER")
	plan, _ := c.Compile(nil)
	// 30 80 = constructed SEQUENCE with indefinite length.
	if _, err := c.Decode(plan, []byte{0x30, 0x80, 0x00, 0x00}); err == nil {
		t.Errorf("decode should reject indefinite length")
	}
}
