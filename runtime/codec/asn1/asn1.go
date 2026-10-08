// Package asn1 registers placeholder codecs for the ASN.1 binary
// encoding families (BER, DER, CER, OER, PER aligned/unaligned, XER).
// They share the same minimal implementation: round-trip integers and
// octetstrings through length-prefixed wire forms so plumbing tests
// can exercise the full codec dispatch path before the dedicated
// implementations land.
//
// The deliberate split is to keep dispatch / discovery testable today
// (`ntt list codecs` can show every Annex-E supported name) without
// pretending the full ASN.1 encoding rules are implemented. Each
// production family gets its own package later (`runtime/codec/ber`
// etc) and registers there, displacing the placeholder.
package asn1

import (
	"encoding/binary"
	"fmt"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

func init() {
	// BER / DER / CER moved to runtime/codec/ber (real X.690 encoder).
	// OER moved to runtime/codec/oer (real X.696 encoder).
	// PER / XER remain stubs until their dedicated packages land.
	for _, name := range []string{"PER", "XER"} {
		codec.Register(stub{name: name})
	}
}

type stub struct{ name string }

func (s stub) Name() string                                       { return s.name }
func (s stub) Compile(*attr.AttributeSet) (codec.Plan, error)     { return plan{name: s.name}, nil }

type plan struct{ name string }

func (p plan) EncodingName() string { return p.name }

func (s stub) Encode(_ codec.Plan, v codec.Value) ([]byte, error) {
	switch val := v.(type) {
	case codec.IntValue:
		// 2-byte TL header + 8-byte big-endian payload.
		var buf [10]byte
		buf[0] = 0x02
		buf[1] = 0x08
		binary.BigEndian.PutUint64(buf[2:10], uint64(val))
		return append([]byte(nil), buf[:]...), nil
	case codec.BytesValue:
		out := make([]byte, 0, len(val)+2)
		out = append(out, 0x04, byte(len(val)))
		out = append(out, val...)
		return out, nil
	case codec.StringValue:
		out := make([]byte, 0, len(val)+2)
		out = append(out, 0x0C, byte(len(val)))
		out = append(out, []byte(val)...)
		return out, nil
	}
	return nil, fmt.Errorf("%s: stub does not yet encode kind %v", s.name, v.Kind())
}

func (s stub) Decode(_ codec.Plan, data []byte) (codec.Value, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("%s: short read (%d bytes)", s.name, len(data))
	}
	tag, length, body := data[0], int(data[1]), data[2:]
	if len(body) < length {
		return nil, fmt.Errorf("%s: truncated body, need %d, have %d", s.name, length, len(body))
	}
	body = body[:length]
	switch tag {
	case 0x02:
		if length != 8 {
			return nil, fmt.Errorf("%s: stub only handles 8-byte integers", s.name)
		}
		return codec.IntValue(int64(binary.BigEndian.Uint64(body))), nil
	case 0x04:
		return codec.BytesValue(append([]byte{}, body...)), nil
	case 0x0C:
		return codec.StringValue(string(body)), nil
	}
	return nil, fmt.Errorf("%s: stub does not recognise tag 0x%02X", s.name, tag)
}
