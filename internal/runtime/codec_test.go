package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncDecValueInteger(t *testing.T) {
	bits, err := EncValue(42, IntegerType)
	require.NoError(t, err)
	assert.Equal(t, "'0011010000110010'B", bits.String())

	var v any
	code, rest := DecValue(bits, IntegerType, &v)
	assert.Equal(t, 0, code)
	assert.Equal(t, Binary{Unit: 'B'}, rest)
	assert.Equal(t, 42, v)
}

func TestEncValueInt32Variant(t *testing.T) {
	typ := &NamedType{
		Name: "I",
		Type: IntegerType,
		Attributes: Attributes{Variant: Attribute{Value: "32 bit"}},
	}
	bits, err := EncValue(1094861636, typ)
	require.NoError(t, err)
	assert.Len(t, bits.bits(), 32)

	var v any
	code, _ := DecValue(bits, typ, &v)
	assert.Equal(t, 0, code)
	assert.Equal(t, 1094861636, v)
}

func TestDecValueInsufficientBits(t *testing.T) {
	typ := &NamedType{
		Name: "I",
		Type: IntegerType,
		Attributes: Attributes{Variant: Attribute{Value: "32 bit"}},
	}
	short := Binary{Unit: 'B', Digits: "0011000100110000"}
	var v any = 99
	code, _ := DecValue(short, typ, &v)
	assert.Equal(t, 2, code)
	assert.Equal(t, 99, v)
}

func TestSizeOfRecord(t *testing.T) {
	n, err := SizeOf([]any{1, true})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestAllFrom(t *testing.T) {
	out, err := AllFrom([]any{1, 2}, false)
	require.NoError(t, err)
	assert.Equal(t, []any{1, 2}, out)
}

func TestInt2Enum(t *testing.T) {
	typ := EnumeratedType{Body: []*Var{
		{Value: EnumValue{Name: "e_first", Val: 0}},
		{Value: EnumValue{Name: "e_third", Val: 5}},
	}}
	ev, err := Int2Enum(5, typ)
	require.NoError(t, err)
	assert.Equal(t, "e_third", ev.Name)

	_, err = Int2Enum(4, typ)
	assert.Error(t, err)
}
