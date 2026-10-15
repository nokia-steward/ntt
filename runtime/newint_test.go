package runtime

import (
	"math/big"
	"testing"
)

func TestNewIntAcceptsAllIntegerWidths(t *testing.T) {
	// The cabi/cgo inject path decodes JSON ints to
	// int64 and feeds them to NewInt; the old switch only knew
	// `int` and `string` and panicked on every other width.
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"int", int(42), "42"},
		{"int8", int8(-7), "-7"},
		{"int16", int16(1234), "1234"},
		{"int32", int32(-987654), "-987654"},
		{"int64", int64(9223372036854775807), "9223372036854775807"},
		{"uint", uint(100), "100"},
		{"uint8", uint8(200), "200"},
		{"uint16", uint16(60000), "60000"},
		{"uint32", uint32(4000000000), "4000000000"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"string", "123456789012345678901234567890", "123456789012345678901234567890"},
		{"*big.Int", big.NewInt(7), "7"},
		{"big.Int", *big.NewInt(11), "11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NewInt(c.in).Inspect()
			if got != c.want {
				t.Fatalf("NewInt(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNewIntNilBigIntReturnsZero(t *testing.T) {
	if got := NewInt((*big.Int)(nil)).Inspect(); got != "0" {
		t.Fatalf("NewInt(nil *big.Int) = %q, want %q", got, "0")
	}
}
