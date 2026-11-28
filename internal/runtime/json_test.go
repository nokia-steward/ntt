package runtime

import (
	"math"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	color := EnumeratedType{Body: []*Var{
		{Name: "red", Value: EnumValue{Name: "red", Val: 0}},
		{Name: "green", Value: EnumValue{Name: "green", Val: 1}},
	}}
	union := UnionType{Fields: []*Var{
		{Name: "i", Type: IntegerType, Index: 0},
		{Name: "s", Type: CharstringType, Index: 3},
	}}
	rec := &NamedType{Name: "R", Type: RecordType{Fields: []*Var{
		{Name: "n", Type: IntegerType, Index: 0},
		{Name: "opt", Type: BooleanType, Index: 1, Flags: Optional},
		{Name: "l", Type: RecordOfType{ElementType: FloatType}, Index: 2},
		{Name: "c", Type: color, Index: 3},
		{Name: "u", Type: union, Index: 4},
		{Name: "o", Type: OctetstringType, Index: 5},
		{Name: "name", Type: UniversalCharstringType, Index: 6},
	}}, Attributes: Attributes{Encode: Attribute{Value: "JSON"}}}

	v := []any{
		7, OmitVal, []any{1.5, math.Inf(-1)}, EnumValue{Name: "green", Val: 1},
		[]any{Undefined{}, Undefined{}, Undefined{}, "x"}, Binary{Unit: 'O', Digits: "CAFE"}, "Zoë",
	}
	const want = `{"n":7,"l":[1.5,"-infinity"],"c":"green","u":{"s":"x"},"o":"CAFE","name":"Zoë"}`
	got, err := EncodeJSON(v, rec)
	if err != nil || got != want {
		t.Fatalf("EncodeJSON = %s, %v; want %s", got, err, want)
	}
	enc, err := EncValue(v, rec)
	if err != nil {
		t.Fatal(err)
	}
	var back any
	if code, _ := DecValue(enc, rec, &back); code != 0 {
		t.Fatalf("DecValue = %d", code)
	}
	if again, err := EncodeJSON(back, rec); err != nil || again != want {
		t.Errorf("round trip = %s, %v", again, err)
	}

	for _, bad := range []string{
		`{"l":[],"c":"red","u":{"i":1},"o":"","name":""}`,             // n missing
		`{"n":1,"n":2,"l":[],"c":"red","u":{"i":1},"o":"","name":""}`, // n twice
		`{"n":1,"l":[],"c":"blue","u":{"i":1},"o":"","name":""}`,      // no such enumerated value
		`{"n":1,"l":[],"c":"red","u":{"i":1},"o":"","name":""} 1`,     // two values
	} {
		if _, err := DecodeJSON(bad, rec); err == nil {
			t.Errorf("DecodeJSON(%s) accepted", bad)
		}
	}
	if got, err := DecodeJSON(`{"n":1,"x":{"y":[]},"l":[],"c":"red","u":{"i":1},"o":"ab","name":""}`, rec); err != nil {
		t.Errorf("unknown member: %v", err)
	} else if r := got.([]any); r[1] != OmitVal || r[5] != (Binary{Unit: 'O', Digits: "AB"}) {
		t.Errorf("decoded %v", r)
	}
}
