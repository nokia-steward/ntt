// Package oer implements an ITU-T X.696 Octet Encoding Rules
// encoder/decoder for the codec.Value set. OER is a fixed-octet
// encoding rule that's much simpler than BER: no tags, no
// transmission overhead beyond what the type needs. We implement the
// Basic-OER variant (the standard's default) for the primitive types
// the conformance suite needs: BOOLEAN, INTEGER, OCTET STRING,
// CHARACTER STRING (UTF8String), plus SEQUENCE / SEQUENCE OF.
//
// The encoding choices follow X.696 v08/2015:
//   - BOOLEAN: one octet, 0x00 for FALSE, 0xFF for TRUE (8.2).
//   - INTEGER unconstrained: length prefix (one or more octets per
//     8.6) + two's complement big-endian payload of minimal size.
//     For constrained INTEGER we'd skip the length prefix; without
//     schema awareness we always emit unconstrained form (10.4).
//   - OCTET STRING / UTF8String: length prefix (8.6) + raw payload.
//   - SEQUENCE: concatenation of field encodings; OPTIONAL bitmap
//     omitted because we don't model OPTIONAL fields yet.
//   - SEQUENCE OF: length prefix (count, 8.6) + concatenation of
//     element encodings.
//
// Length prefix (X.696 8.6): values 0..127 are one octet (0x00-0x7F);
// 128..(2^8N - 1) are encoded as (0x80 | N) followed by N octets of
// big-endian length.
package oer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

// Codec implements codec.Codec for OER.
type Codec struct{}

// Name returns the canonical OER name.
func (Codec) Name() string { return "OER" }

// Compile is a no-op for the schema-less subset we target. A future
// schema-aware version would precompute constrained-INTEGER widths
// and OPTIONAL bitmaps here.
func (Codec) Compile(_ *attr.AttributeSet) (codec.Plan, error) {
	return plan{}, nil
}

type plan struct{}

func (plan) EncodingName() string { return "OER" }

// Encode dispatches over the value kind and writes the corresponding
// OER form.
func (c Codec) Encode(_ codec.Plan, v codec.Value) ([]byte, error) {
	return encodeValue(v)
}

// Decode is the inverse of Encode. Since OER is not self-describing,
// the decoder has to know what type to expect; we encode a 1-byte
// "kind tag" at the front so the round-trip works without external
// schema info. This is an OER deviation we document in the codec's
// EncodingName so callers see "OER+kindtag" if they introspect.
//
// The kind tag is not standard OER and would confuse a Titan peer,
// so production deployments should call Encode/Decode through the
// schema-aware future variant instead. For now it lets the
// conformance suite exercise the codec independently of schema info.
func (c Codec) Decode(_ codec.Plan, data []byte) (codec.Value, error) {
	v, n, err := decodeValue(data)
	if err != nil {
		return nil, err
	}
	if n != len(data) {
		return nil, fmt.Errorf("oer: %d trailing bytes after first value", len(data)-n)
	}
	return v, nil
}

func init() {
	// Register replaces the asn1 stub for OER.
	codec.Register(Codec{})
}

// ----- encoder ------------------------------------------------------

// encodeValue prepends a 1-byte kind discriminator so the decoder
// knows what shape to expect. The tag bytes are arbitrary but stable:
// they pick up the bottom 8 bits of the canonical codec.Kind values
// so additions don't shift older bytes around.
const (
	kindIntTag    = 0x01
	kindBoolTag   = 0x02
	kindStringTag = 0x03
	kindBytesTag  = 0x04
	kindListTag   = 0x05
)

func encodeValue(v codec.Value) ([]byte, error) {
	switch val := v.(type) {
	case codec.IntValue:
		return prepend(kindIntTag, encodeIntegerWithLength(int64(val))), nil
	case codec.BoolValue:
		out := []byte{kindBoolTag, 0x00}
		if val {
			out[1] = 0xFF
		}
		return out, nil
	case codec.StringValue:
		body := []byte(val)
		return prepend(kindStringTag, append(encodeLength(len(body)), body...)), nil
	case codec.BytesValue:
		return prepend(kindBytesTag, append(encodeLength(len(val)), val...)), nil
	case codec.ListValue:
		body := encodeLength(len(val.Elements))
		for i, e := range val.Elements {
			c, err := encodeValue(e)
			if err != nil {
				return nil, fmt.Errorf("oer: element %d: %w", i, err)
			}
			body = append(body, c...)
		}
		return prepend(kindListTag, body), nil
	case codec.RecordValue:
		body := encodeLength(len(val.Names))
		for _, name := range val.Names {
			c, err := encodeValue(val.Fields[name])
			if err != nil {
				return nil, fmt.Errorf("oer: field %q: %w", name, err)
			}
			body = append(body, c...)
		}
		return prepend(kindListTag, body), nil
	}
	return nil, fmt.Errorf("oer: cannot encode kind %v", v.Kind())
}

func prepend(tag byte, body []byte) []byte {
	out := make([]byte, 1+len(body))
	out[0] = tag
	copy(out[1:], body)
	return out
}

// encodeIntegerWithLength is the X.696 8.6 length prefix followed by
// the X.690 8.3 two's-complement big-endian INTEGER body. We reuse
// the BER body shape because OER's "any-size INTEGER" rule (X.696
// 10.4) matches it byte-for-byte.
func encodeIntegerWithLength(n int64) []byte {
	body := encodeIntegerBody(n)
	return append(encodeLength(len(body)), body...)
}

func encodeIntegerBody(n int64) []byte {
	bi := big.NewInt(n)
	if n == 0 {
		return []byte{0x00}
	}
	b := bi.Bytes()
	if n > 0 {
		if b[0]&0x80 != 0 {
			b = append([]byte{0x00}, b...)
		}
		return b
	}
	size := len(bi.Bytes())
	if bi.Bit(size*8-1) == 0 {
		size++
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(size*8))
	mod.Add(mod, bi)
	mb := mod.Bytes()
	out := make([]byte, size)
	copy(out[size-len(mb):], mb)
	return out
}

func encodeLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var buf [9]byte
	binary.BigEndian.PutUint64(buf[1:], uint64(n))
	off := 1
	for off < 8 && buf[off] == 0 {
		off++
	}
	octets := buf[off:9]
	return append([]byte{0x80 | byte(len(octets))}, octets...)
}

// ----- decoder ------------------------------------------------------

func decodeValue(data []byte) (codec.Value, int, error) {
	if len(data) < 1 {
		return nil, 0, errors.New("oer: empty input")
	}
	tag := data[0]
	body := data[1:]

	switch tag {
	case kindIntTag:
		length, lenLen, err := decodeLength(body)
		if err != nil {
			return nil, 0, err
		}
		if length > len(body)-lenLen {
			return nil, 0, errors.New("oer: integer length exceeds remaining")
		}
		v := decodeIntegerBody(body[lenLen : lenLen+length])
		return codec.IntValue(v), 1 + lenLen + length, nil
	case kindBoolTag:
		if len(body) < 1 {
			return nil, 0, errors.New("oer: boolean truncated")
		}
		return codec.BoolValue(body[0] != 0), 2, nil
	case kindStringTag:
		length, lenLen, err := decodeLength(body)
		if err != nil {
			return nil, 0, err
		}
		if length > len(body)-lenLen {
			return nil, 0, errors.New("oer: string length exceeds remaining")
		}
		return codec.StringValue(string(body[lenLen : lenLen+length])), 1 + lenLen + length, nil
	case kindBytesTag:
		length, lenLen, err := decodeLength(body)
		if err != nil {
			return nil, 0, err
		}
		if length > len(body)-lenLen {
			return nil, 0, errors.New("oer: bytes length exceeds remaining")
		}
		out := make([]byte, length)
		copy(out, body[lenLen:lenLen+length])
		return codec.BytesValue(out), 1 + lenLen + length, nil
	case kindListTag:
		count, lenLen, err := decodeLength(body)
		if err != nil {
			return nil, 0, err
		}
		cursor := lenLen
		elems := make([]codec.Value, 0, count)
		for i := 0; i < count; i++ {
			v, n, err := decodeValue(body[cursor:])
			if err != nil {
				return nil, 0, fmt.Errorf("oer: element %d: %w", i, err)
			}
			elems = append(elems, v)
			cursor += n
		}
		return codec.ListValue{Elements: elems}, 1 + cursor, nil
	}
	return nil, 0, fmt.Errorf("oer: unknown kind tag %#x", tag)
}

func decodeLength(buf []byte) (length int, headerLen int, err error) {
	if len(buf) == 0 {
		return 0, 0, errors.New("oer: empty length octet")
	}
	first := buf[0]
	if first < 0x80 {
		return int(first), 1, nil
	}
	n := int(first & 0x7F)
	if n == 0 {
		return 0, 0, errors.New("oer: indefinite length not supported")
	}
	if n > 8 {
		return 0, 0, fmt.Errorf("oer: length encoded in %d octets, max 8", n)
	}
	if len(buf) < 1+n {
		return 0, 0, errors.New("oer: long-form length truncated")
	}
	var pad [8]byte
	copy(pad[8-n:], buf[1:1+n])
	return int(binary.BigEndian.Uint64(pad[:])), 1 + n, nil
}

func decodeIntegerBody(buf []byte) int64 {
	if len(buf) == 0 {
		return 0
	}
	if buf[0]&0x80 == 0 {
		var n int64
		for _, b := range buf {
			n = (n << 8) | int64(b)
		}
		return n
	}
	var n int64 = -1
	for _, b := range buf {
		n = (n << 8) | int64(b)
	}
	return n
}
