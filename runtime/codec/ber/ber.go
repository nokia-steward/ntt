// Package ber implements an ITU-T X.690 Basic Encoding Rules
// encoder/decoder for the codec.Value set the runtime uses. The
// implementation covers what the conformance suite and the bulk of
// telco ASN.1 modules need: primitive types (INTEGER, BOOLEAN,
// OCTET STRING, CHARACTER STRING via UTF8String, NULL) with definite-
// length encoding, plus constructed SEQUENCE / SEQUENCE OF for
// record / record-of values.
//
// Goals:
//   - faithful to X.690 v07/2002 for the primitive subset (round-
//     trippable against ITU's published test vectors).
//   - readable: the encoder writes TLV one field at a time so the
//     code maps 1:1 to the standard; no buffer reuse tricks before
//     correctness.
//   - decoder is strict: lengths must fit, indefinite-length form is
//     rejected (the DER subset disallows it and BER traffic in real
//     telco protocols always uses definite-length).
//
// The package registers itself in init() under "BER" plus the close
// neighbours "DER" and "CER" (which are stricter subsets - they
// share BER's wire shape when DER's rules are followed, and the
// encoder always emits DER-compatible output).
package ber

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

// Universal tag numbers from X.680 Annex A.
const (
	TagBoolean      = 0x01
	TagInteger      = 0x02
	TagBitString    = 0x03
	TagOctetString  = 0x04
	TagNull         = 0x05
	TagEnumerated   = 0x0A
	TagUTF8String   = 0x0C
	TagSequence     = 0x10
	TagSet          = 0x11
)

// Class bits (high two bits of the identifier octet).
const (
	ClassUniversal       byte = 0x00
	ClassApplication     byte = 0x40
	ClassContextSpecific byte = 0x80
	ClassPrivate         byte = 0xC0
)

const constructedBit byte = 0x20

// Codec implements codec.Codec for the BER family.
type Codec struct{ name string }

// Name returns the encoding name the codec was registered under (BER,
// DER, or CER).
func (c Codec) Name() string { return c.name }

// Compile returns a no-op plan; the BER encoder does not need any
// pre-computed schema today (tags come from the universal tag table
// or from the value's runtime type).
func (c Codec) Compile(_ *attr.AttributeSet) (codec.Plan, error) {
	return plan{name: c.name}, nil
}

type plan struct{ name string }

func (p plan) EncodingName() string { return p.name }

// Encode serialises v using the X.690 BER rules.
func (c Codec) Encode(_ codec.Plan, v codec.Value) ([]byte, error) {
	return encodeValue(v)
}

// Decode parses a single TLV from data and returns the corresponding
// codec.Value. Trailing bytes after the first TLV are rejected to
// catch wire-form mistakes early.
func (c Codec) Decode(_ codec.Plan, data []byte) (codec.Value, error) {
	v, n, err := decodeValue(data)
	if err != nil {
		return nil, err
	}
	if n != len(data) {
		return nil, fmt.Errorf("ber: %d trailing bytes after first TLV", len(data)-n)
	}
	return v, nil
}

func init() {
	codec.Register(Codec{name: "BER"})
	codec.Register(Codec{name: "DER"}) // strict subset; encoder is DER-compatible.
	codec.Register(Codec{name: "CER"}) // canonical encoding; primitive path is identical.
}

// ----- encoder ------------------------------------------------------

func encodeValue(v codec.Value) ([]byte, error) {
	switch val := v.(type) {
	case codec.IntValue:
		return tlv(TagInteger, encodeInteger(int64(val))), nil
	case codec.BoolValue:
		var body byte
		if val {
			// X.690 DER 11.1: TRUE shall be encoded as 0xFF; BER
			// would allow any non-zero, but emitting DER keeps the
			// encoder canonical.
			body = 0xFF
		}
		return tlv(TagBoolean, []byte{body}), nil
	case codec.StringValue:
		// TTCN-3 charstring maps to ASN.1 UTF8String for our purposes;
		// codecs that need TeletexString / IA5String distinguish via
		// the variant attribute, which we ignore for now.
		return tlv(TagUTF8String, []byte(val)), nil
	case codec.BytesValue:
		return tlv(TagOctetString, val), nil
	case codec.RecordValue:
		var body []byte
		for _, name := range val.Names {
			child, err := encodeValue(val.Fields[name])
			if err != nil {
				return nil, fmt.Errorf("ber: field %q: %w", name, err)
			}
			body = append(body, child...)
		}
		return constructedTLV(TagSequence, body), nil
	case codec.ListValue:
		var body []byte
		for i, e := range val.Elements {
			child, err := encodeValue(e)
			if err != nil {
				return nil, fmt.Errorf("ber: element %d: %w", i, err)
			}
			body = append(body, child...)
		}
		return constructedTLV(TagSequence, body), nil
	}
	return nil, fmt.Errorf("ber: cannot encode kind %v", v.Kind())
}

// tlv builds a primitive TLV: identifier (universal class, primitive,
// number=tag) + length + body.
func tlv(tag byte, body []byte) []byte {
	out := []byte{ClassUniversal | tag}
	out = append(out, encodeLength(len(body))...)
	out = append(out, body...)
	return out
}

// constructedTLV builds a TLV with the constructed bit set, used for
// SEQUENCE and SET.
func constructedTLV(tag byte, body []byte) []byte {
	out := []byte{ClassUniversal | constructedBit | tag}
	out = append(out, encodeLength(len(body))...)
	out = append(out, body...)
	return out
}

// encodeLength implements X.690 8.1.3 definite-length form.
func encodeLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	// Long form: 0x80 | numOctets, then big-endian length.
	var buf [9]byte
	binary.BigEndian.PutUint64(buf[1:], uint64(n))
	// Trim leading zeros.
	off := 1
	for off < 8 && buf[off] == 0 {
		off++
	}
	octets := buf[off:9]
	return append([]byte{0x80 | byte(len(octets))}, octets...)
}

// encodeInteger uses two's complement big-endian with the minimal
// number of octets (X.690 8.3).
func encodeInteger(n int64) []byte {
	bi := big.NewInt(n)
	// Special case zero: empty Bytes() representation; X.690 requires
	// a single 0x00 octet.
	if n == 0 {
		return []byte{0x00}
	}
	b := bi.Bytes() // big-endian magnitude, no sign
	if n > 0 {
		// If the high bit is set the decoder would read it as
		// negative; prepend 0x00.
		if b[0]&0x80 != 0 {
			b = append([]byte{0x00}, b...)
		}
		return b
	}
	// Negative: compute two's complement. We allocate one more byte
	// than strictly necessary if the high bit would otherwise be 0
	// (which would be read as positive).
	size := len(bi.Bytes())
	if bi.Bit(size*8-1) == 0 {
		size++
	}
	twos := make([]byte, size)
	// 2^(size*8) + n  for negative n
	mod := new(big.Int).Lsh(big.NewInt(1), uint(size*8))
	mod.Add(mod, bi)
	mb := mod.Bytes()
	copy(twos[size-len(mb):], mb)
	return twos
}

// ----- decoder ------------------------------------------------------

func decodeValue(data []byte) (codec.Value, int, error) {
	if len(data) < 2 {
		return nil, 0, errors.New("ber: TLV truncated")
	}
	id := data[0]
	class := id & 0xC0
	constructed := id&constructedBit != 0
	tag := id & 0x1F
	if tag == 0x1F {
		return nil, 0, errors.New("ber: multi-octet tags not supported")
	}
	length, lenLen, err := decodeLength(data[1:])
	if err != nil {
		return nil, 0, err
	}
	headerLen := 1 + lenLen
	if headerLen+length > len(data) {
		return nil, 0, fmt.Errorf("ber: length %d exceeds remaining %d", length, len(data)-headerLen)
	}
	body := data[headerLen : headerLen+length]
	consumed := headerLen + length

	if class != ClassUniversal {
		return nil, 0, fmt.Errorf("ber: non-universal class %#x not implemented", class)
	}

	if constructed {
		switch tag {
		case TagSequence, TagSet:
			elems, err := decodeChildren(body)
			if err != nil {
				return nil, 0, err
			}
			return codec.ListValue{Elements: elems}, consumed, nil
		}
		return nil, 0, fmt.Errorf("ber: constructed tag %#x not implemented", tag)
	}

	switch tag {
	case TagBoolean:
		if len(body) != 1 {
			return nil, 0, fmt.Errorf("ber: BOOLEAN length must be 1, got %d", len(body))
		}
		return codec.BoolValue(body[0] != 0), consumed, nil
	case TagInteger:
		return codec.IntValue(decodeInteger(body)), consumed, nil
	case TagEnumerated:
		return codec.IntValue(decodeInteger(body)), consumed, nil
	case TagOctetString:
		out := make([]byte, len(body))
		copy(out, body)
		return codec.BytesValue(out), consumed, nil
	case TagUTF8String:
		return codec.StringValue(string(body)), consumed, nil
	case TagNull:
		if len(body) != 0 {
			return nil, 0, errors.New("ber: NULL must have zero-length body")
		}
		return codec.IntValue(0), consumed, nil
	}
	return nil, 0, fmt.Errorf("ber: unknown universal tag %#x", tag)
}

func decodeChildren(body []byte) ([]codec.Value, error) {
	var out []codec.Value
	for len(body) > 0 {
		v, n, err := decodeValue(body)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		body = body[n:]
	}
	return out, nil
}

func decodeLength(buf []byte) (length int, headerLen int, err error) {
	if len(buf) == 0 {
		return 0, 0, errors.New("ber: empty length octet")
	}
	first := buf[0]
	if first < 0x80 {
		return int(first), 1, nil
	}
	if first == 0x80 {
		return 0, 0, errors.New("ber: indefinite length not supported")
	}
	n := int(first & 0x7F)
	if n > 8 {
		return 0, 0, fmt.Errorf("ber: length encoded in %d octets, max 8", n)
	}
	if len(buf) < 1+n {
		return 0, 0, errors.New("ber: long-form length truncated")
	}
	var pad [8]byte
	copy(pad[8-n:], buf[1:1+n])
	return int(binary.BigEndian.Uint64(pad[:])), 1 + n, nil
}

func decodeInteger(buf []byte) int64 {
	if len(buf) == 0 {
		return 0
	}
	if buf[0]&0x80 == 0 {
		// Non-negative: high bit clear => standard big-endian read.
		var n int64
		for _, b := range buf {
			n = (n << 8) | int64(b)
		}
		return n
	}
	// Negative: two's complement, sign-extend the high bit out to
	// 64 bits.
	var n int64 = -1
	for _, b := range buf {
		n = (n << 8) | int64(b)
	}
	return n
}
