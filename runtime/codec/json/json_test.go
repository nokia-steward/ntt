package json_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/runtime/codec/json"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func parseAttrs(t *testing.T, src string) *attr.AttributeSet {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree.Err != nil {
		t.Fatalf("parse: %v", tree.Err)
	}
	var first *syntax.WithSpec
	tree.Root.Inspect(func(n syntax.Node) bool {
		if w, ok := n.(*syntax.WithSpec); ok && first == nil {
			first = w
			return false
		}
		return true
	})
	if first == nil {
		t.Fatalf("no WithSpec in source")
	}
	set, _ := attr.Parse(first)
	return set
}

func TestJSON_EncodeRecordCompact(t *testing.T) {
	set := parseAttrs(t, `module M {
		type record R { integer i, charstring s } with { encode "JSON"; variant "JSON(compact)" }
	}`)
	plan, err := json.Codec{}.Compile(set)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	rec := codec.NewRecord(
		"i", codec.IntValue(42),
		"s", codec.StringValue("hello"),
	)
	wire, err := json.Codec{}.Encode(plan, rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := []byte(`{"i":42,"s":"hello"}`)
	if !bytes.Equal(wire, want) {
		t.Fatalf("wire = %s, want %s", wire, want)
	}
}

func TestJSON_RoundTripPrimitives(t *testing.T) {
	plan, _ := json.Codec{}.Compile(nil)
	c := json.Codec{}
	cases := []codec.Value{
		codec.IntValue(7),
		codec.BoolValue(true),
		codec.StringValue("xy"),
		codec.FloatValue(1.5),
	}
	for _, in := range cases {
		wire, err := c.Encode(plan, in)
		if err != nil {
			t.Fatalf("Encode %v: %v", in, err)
		}
		got, err := c.Decode(plan, wire)
		if err != nil {
			t.Fatalf("Decode %v: %v", wire, err)
		}
		// Float decoded back may differ in representation but value equal.
		if !equalValues(in, got) {
			t.Errorf("round-trip mismatch: %v -> %s -> %v", in, wire, got)
		}
	}
}

func TestJSON_AliasOnEncode(t *testing.T) {
	set := parseAttrs(t, `module M {
		type record R { integer payload } with { encode "JSON"; variant (payload) "name as ""data""" }
	}`)
	plan, err := json.Codec{}.Compile(set)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	rec := codec.NewRecord("payload", codec.IntValue(9))
	wire, err := json.Codec{}.Encode(plan, rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Contains(wire, []byte(`"data": 9`)) && !bytes.Contains(wire, []byte(`"data":9`)) {
		t.Errorf("alias was not applied, wire = %s", wire)
	}
}

func TestJSON_DecodeArrayAndObject(t *testing.T) {
	plan, _ := json.Codec{}.Compile(nil)
	wire := []byte(`[{"a":1},{"a":2}]`)
	got, err := json.Codec{}.Decode(plan, wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	list, ok := got.(codec.ListValue)
	if !ok {
		t.Fatalf("Decode = %T, want ListValue", got)
	}
	if len(list.Elements) != 2 {
		t.Fatalf("len = %d, want 2", len(list.Elements))
	}
	for i, e := range list.Elements {
		rec, ok := e.(codec.RecordValue)
		if !ok {
			t.Fatalf("element %d = %T", i, e)
		}
		if iv, ok := rec.Fields["a"].(codec.IntValue); !ok || int64(iv) != int64(i+1) {
			t.Errorf("element %d.a = %v, want %d", i, rec.Fields["a"], i+1)
		}
	}
}

func equalValues(a, b codec.Value) bool {
	switch x := a.(type) {
	case codec.IntValue:
		y, ok := b.(codec.IntValue)
		return ok && x == y
	case codec.BoolValue:
		y, ok := b.(codec.BoolValue)
		return ok && x == y
	case codec.StringValue:
		y, ok := b.(codec.StringValue)
		return ok && x == y
	case codec.FloatValue:
		y, ok := b.(codec.FloatValue)
		return ok && x == y
	}
	return reflect.DeepEqual(a, b)
}
