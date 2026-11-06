package oer_test

import (
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	_ "github.com/nokia/ntt/runtime/codec/oer"
)

func roundTrip(t *testing.T, v codec.Value) {
	t.Helper()
	c := codec.Lookup("OER")
	if c == nil {
		t.Fatal("OER codec not registered")
	}
	plan, _ := c.Compile(nil)
	wire, err := c.Encode(plan, v)
	if err != nil {
		t.Fatalf("encode %v: %v", v.Inspect(), err)
	}
	got, err := c.Decode(plan, wire)
	if err != nil {
		t.Fatalf("decode %x: %v", wire, err)
	}
	if got.Inspect() != v.Inspect() {
		t.Errorf("round-trip mismatch:\n  wire: %x\n  in:   %s\n  out:  %s", wire, v.Inspect(), got.Inspect())
	}
}

func TestOER_Integers(t *testing.T) {
	for _, n := range []int64{0, 1, -1, 127, 128, -128, -129, 0x7FFF, -0x8000, 1<<31, -(1 << 31), 1<<62, -(1 << 62)} {
		roundTrip(t, codec.IntValue(n))
	}
}

func TestOER_Boolean(t *testing.T) {
	roundTrip(t, codec.BoolValue(true))
	roundTrip(t, codec.BoolValue(false))
}

func TestOER_OctetString(t *testing.T) {
	roundTrip(t, codec.BytesValue(nil))
	roundTrip(t, codec.BytesValue([]byte{0x00}))
	roundTrip(t, codec.BytesValue([]byte("hello, world")))
	roundTrip(t, codec.BytesValue(make([]byte, 300))) // exercises long-form length
}

func TestOER_String(t *testing.T) {
	roundTrip(t, codec.StringValue(""))
	roundTrip(t, codec.StringValue("ascii"))
	roundTrip(t, codec.StringValue("π ≈ 3.14159"))
}

func TestOER_List(t *testing.T) {
	roundTrip(t, codec.ListValue{Elements: []codec.Value{
		codec.IntValue(1),
		codec.IntValue(2),
		codec.IntValue(3),
	}})
	roundTrip(t, codec.ListValue{Elements: []codec.Value{
		codec.StringValue("a"),
		codec.BoolValue(true),
		codec.BytesValue([]byte{0xDE, 0xAD, 0xBE, 0xEF}),
	}})
	roundTrip(t, codec.ListValue{})
}

func TestOER_NestedList(t *testing.T) {
	roundTrip(t, codec.ListValue{Elements: []codec.Value{
		codec.ListValue{Elements: []codec.Value{codec.IntValue(1), codec.IntValue(2)}},
		codec.ListValue{Elements: []codec.Value{codec.IntValue(3)}},
		codec.ListValue{},
	}})
}

func TestOER_TrailingGarbageRejected(t *testing.T) {
	c := codec.Lookup("OER")
	plan, _ := c.Compile(nil)
	wire, _ := c.Encode(plan, codec.IntValue(7))
	wire = append(wire, 0xFF)
	if _, err := c.Decode(plan, wire); err == nil {
		t.Errorf("expected error for trailing garbage")
	}
}
