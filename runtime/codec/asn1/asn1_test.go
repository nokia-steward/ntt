package asn1_test

import (
	"testing"

	"github.com/nokia/ntt/runtime/codec"
	_ "github.com/nokia/ntt/runtime/codec/asn1"
)

// TestStubCodecs_Registered locks in the stub codecs that have not
// yet been replaced by dedicated packages. BER/DER/CER moved out to
// runtime/codec/ber; OER moved to runtime/codec/oer; PER/XER are
// still stubs.
func TestStubCodecs_Registered(t *testing.T) {
	for _, name := range []string{"PER", "XER"} {
		if c := codec.Lookup(name); c == nil {
			t.Errorf("%s codec not registered", name)
		}
	}
}

func TestStubCodecs_IntRoundTrip(t *testing.T) {
	for _, name := range []string{"PER"} {
		t.Run(name, func(t *testing.T) {
			c := codec.Lookup(name)
			if c == nil {
				t.Fatalf("%s not registered", name)
			}
			plan, _ := c.Compile(nil)
			in := codec.IntValue(0x1122334455667788)
			wire, err := c.Encode(plan, in)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			out, err := c.Decode(plan, wire)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if iv, ok := out.(codec.IntValue); !ok || iv != in {
				t.Errorf("round trip: got %v, want %v", out, in)
			}
		})
	}
}
