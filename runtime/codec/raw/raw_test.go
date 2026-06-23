package raw_test

import (
	"bytes"
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/runtime/codec/raw"
	"github.com/nokia/ntt/ttcn3/attr"
)

func newPlan(t *testing.T, attrs *attr.AttributeSet) codec.Plan {
	t.Helper()
	p, err := raw.Codec{}.Compile(attrs)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return p
}

func TestRAW_EncodeDecodeUint32(t *testing.T) {
	p := newPlan(t, nil)
	enc, err := raw.Codec{}.Encode(p, codec.IntValue(0xDEADBEEF))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(enc, want) {
		t.Fatalf("Encode = %x, want %x", enc, want)
	}
}

func TestRAW_ByteOrder(t *testing.T) {
	src := `module M {
		type integer T with { encode "RAW"; variant "BYTEORDER(last)" }
	}`
	set := parseAttrs(t, src)
	p := newPlan(t, set)
	enc, _ := raw.Codec{}.Encode(p, codec.IntValue(0x11223344))
	want := []byte{0x44, 0x33, 0x22, 0x11}
	if !bytes.Equal(enc, want) {
		t.Fatalf("LittleEndian Encode = %x, want %x", enc, want)
	}
	got, err := raw.Codec{}.Decode(p, want)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if iv, ok := got.(codec.BytesValue); ok {
		// scalar plan with no field length defaults to bytes; this is
		// the documented fallback, so make sure we got the bytes back.
		if !bytes.Equal([]byte(iv), want) {
			t.Errorf("decode bytes = %x, want %x", iv, want)
		}
	}
}

func TestRAW_RecordRoundTrip(t *testing.T) {
	src := `module M {
		type record R {
			integer hdr,
			integer body
		} with {
			encode "RAW";
			variant (hdr) "FIELDLENGTH(8)";
			variant (body) "FIELDLENGTH(16)";
		}
	}`
	set := parseAttrs(t, src)
	p, err := raw.Codec{}.Compile(set)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	original := codec.NewRecord(
		"hdr", codec.IntValue(0x12),
		"body", codec.IntValue(0x3456),
	)
	wire, err := raw.Codec{}.Encode(p, original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := []byte{0x12, 0x34, 0x56}
	if !bytes.Equal(wire, want) {
		t.Fatalf("wire = %x, want %x", wire, want)
	}

	round, err := raw.Codec{}.Decode(p, wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rec, ok := round.(codec.RecordValue)
	if !ok {
		t.Fatalf("Decode returned %T, want RecordValue", round)
	}
	if got, want := int64(rec.Fields["hdr"].(codec.IntValue)), int64(0x12); got != want {
		t.Errorf("hdr = %d, want %d", got, want)
	}
	if got, want := int64(rec.Fields["body"].(codec.IntValue)), int64(0x3456); got != want {
		t.Errorf("body = %d, want %d", got, want)
	}
}

func TestRAW_BoolRoundTrip(t *testing.T) {
	p := newPlan(t, nil)
	for _, b := range []bool{true, false} {
		enc, err := raw.Codec{}.Encode(p, codec.BoolValue(b))
		if err != nil {
			t.Fatalf("Encode %v: %v", b, err)
		}
		if len(enc) != 1 {
			t.Fatalf("bool encoded to %d bytes, want 1", len(enc))
		}
		if (enc[0] != 0) != b {
			t.Errorf("Encode(%v) = %v", b, enc)
		}
	}
}

func TestRAW_BytesPassthrough(t *testing.T) {
	p := newPlan(t, nil)
	data := codec.BytesValue{0x01, 0x02, 0x03}
	enc, err := raw.Codec{}.Encode(p, data)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(enc, []byte(data)) {
		t.Errorf("Encode = %x, want %x", enc, data)
	}
}
