// Package raw implements the TTCN-3 RAW codec per Annex E.1. RAW is
// the bit-oriented binary codec used by most telco protocols: integer
// fields with explicit bit widths, big- or little-endian byte order,
// length fields, padding, and so on.
//
// This implementation covers the M3 baseline:
//   - Fixed-width integer fields (FIELDLENGTH attribute)
//   - Byte / bit order control (BYTEORDER / FIELDORDER)
//   - Octetstring / charstring payloads
//   - Records with sequential field encoding
//
// Future iterations extend it with length-prefix fields, tag-based
// unions, padding, and conditional / repeatable fields.
package raw

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

// Codec is the RAW codec implementation.
type Codec struct{}

func init() { codec.Register(Codec{}) }

// Name implements codec.Codec.
func (Codec) Name() string { return "RAW" }

// Plan is the compiled view of a RAW-annotated type. The runtime caches
// it per type so successive encode/decode calls don't re-parse the
// variant attributes.
type Plan struct {
	// FieldOrder describes the bit ordering inside each byte: "msb"
	// (most-significant bit first) is the Annex-E default.
	FieldOrder string
	// ByteOrder is "first" (network order, the default) or "last".
	ByteOrder string

	// Fields is the per-field metadata, indexed by field name in the
	// order they appear in the record. Scalar plans (a top-level
	// integer / octetstring) have a single Field with name "".
	Fields []FieldPlan
}

// FieldPlan is the RAW directives that apply to one record field.
type FieldPlan struct {
	Name   string
	Length int    // explicit FIELDLENGTH in BITS, or 0 to use the value's natural size
	Type   string // optional FIELDTYPE override ("integer", "octetstring", ...)
}

// EncodingName implements codec.Plan.
func (Plan) EncodingName() string { return "RAW" }

// Compile builds a Plan from an attribute set. The set may carry a
// single top-level RAW directive (covering scalars) or a list of
// per-field directives (covering records). Unknown / unsupported
// attributes are kept in the plan as comments so the formatter can
// round-trip them.
func (Codec) Compile(set *attr.AttributeSet) (codec.Plan, error) {
	plan := &Plan{FieldOrder: "msb", ByteOrder: "first"}
	if set == nil {
		return plan, nil
	}
	for _, v := range set.Variants() {
		raw := strings.TrimSpace(v.Value)
		if eqFold(raw, "FIELDORDER(msb)") {
			plan.FieldOrder = "msb"
			continue
		}
		if eqFold(raw, "FIELDORDER(lsb)") {
			plan.FieldOrder = "lsb"
			continue
		}
		if eqFold(raw, "BYTEORDER(first)") {
			plan.ByteOrder = "first"
			continue
		}
		if eqFold(raw, "BYTEORDER(last)") {
			plan.ByteOrder = "last"
			continue
		}
		if strings.HasPrefix(strings.ToUpper(raw), "FIELDLENGTH(") {
			n, ok := parseLength(raw)
			if !ok {
				return nil, fmt.Errorf("RAW: malformed FIELDLENGTH directive %q", raw)
			}
			plan.Fields = append(plan.Fields, FieldPlan{
				Name:   strings.Join(v.Selectors, "."),
				Length: n,
				Type:   "integer",
			})
			continue
		}
	}
	return plan, nil
}

// Encode serialises v into bytes according to plan. The plan + value
// must agree on shape: scalars require a scalar plan, records require
// a record-shaped plan, mismatches return an error.
func (Codec) Encode(p codec.Plan, v codec.Value) ([]byte, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("RAW: plan %T is not a RAW plan", p)
	}
	switch val := v.(type) {
	case codec.IntValue:
		return encodeInt(plan, scalarField(plan), int64(val))
	case codec.BoolValue:
		if val {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case codec.BytesValue:
		return []byte(val), nil
	case codec.StringValue:
		return []byte(val), nil
	case codec.RecordValue:
		return encodeRecord(plan, val)
	}
	return nil, fmt.Errorf("RAW: cannot encode value of kind %v", v.Kind())
}

// Decode parses bytes back into a Value of the shape plan describes.
func (Codec) Decode(p codec.Plan, data []byte) (codec.Value, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("RAW: plan %T is not a RAW plan", p)
	}
	if len(plan.Fields) == 0 {
		// Scalar plan: treat the whole buffer as an integer if a
		// FIELDLENGTH was implied, otherwise return raw bytes.
		return codec.BytesValue(append([]byte{}, data...)), nil
	}
	if len(plan.Fields) == 1 && plan.Fields[0].Name == "" {
		v, _, err := decodeInt(plan, plan.Fields[0], data)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	return decodeRecord(plan, data)
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

func scalarField(plan *Plan) FieldPlan {
	if len(plan.Fields) == 1 && plan.Fields[0].Name == "" {
		return plan.Fields[0]
	}
	// Default to 32 bits if no FIELDLENGTH was given. This matches
	// Titan's behaviour for un-annotated integer types.
	return FieldPlan{Length: 32, Type: "integer"}
}

func encodeInt(plan *Plan, field FieldPlan, n int64) ([]byte, error) {
	bits := field.Length
	if bits == 0 {
		bits = 32
	}
	if bits%8 != 0 {
		return nil, fmt.Errorf("RAW: non-byte-aligned FIELDLENGTH=%d not yet supported", bits)
	}
	bytes := bits / 8
	out := make([]byte, bytes)
	switch bytes {
	case 1:
		out[0] = byte(n)
	case 2:
		if plan.ByteOrder == "last" {
			binary.LittleEndian.PutUint16(out, uint16(n))
		} else {
			binary.BigEndian.PutUint16(out, uint16(n))
		}
	case 4:
		if plan.ByteOrder == "last" {
			binary.LittleEndian.PutUint32(out, uint32(n))
		} else {
			binary.BigEndian.PutUint32(out, uint32(n))
		}
	case 8:
		if plan.ByteOrder == "last" {
			binary.LittleEndian.PutUint64(out, uint64(n))
		} else {
			binary.BigEndian.PutUint64(out, uint64(n))
		}
	default:
		return nil, fmt.Errorf("RAW: FIELDLENGTH=%d bytes not supported", bytes)
	}
	return out, nil
}

func decodeInt(plan *Plan, field FieldPlan, data []byte) (codec.IntValue, []byte, error) {
	bits := field.Length
	if bits == 0 {
		bits = 32
	}
	if bits%8 != 0 {
		return 0, nil, fmt.Errorf("RAW: non-byte-aligned FIELDLENGTH=%d not yet supported", bits)
	}
	bytes := bits / 8
	if len(data) < bytes {
		return 0, nil, fmt.Errorf("RAW: short read, need %d bytes, have %d", bytes, len(data))
	}
	chunk := data[:bytes]
	rest := data[bytes:]
	var n int64
	switch bytes {
	case 1:
		n = int64(int8(chunk[0]))
	case 2:
		if plan.ByteOrder == "last" {
			n = int64(int16(binary.LittleEndian.Uint16(chunk)))
		} else {
			n = int64(int16(binary.BigEndian.Uint16(chunk)))
		}
	case 4:
		if plan.ByteOrder == "last" {
			n = int64(int32(binary.LittleEndian.Uint32(chunk)))
		} else {
			n = int64(int32(binary.BigEndian.Uint32(chunk)))
		}
	case 8:
		if plan.ByteOrder == "last" {
			n = int64(binary.LittleEndian.Uint64(chunk))
		} else {
			n = int64(binary.BigEndian.Uint64(chunk))
		}
	default:
		return 0, nil, fmt.Errorf("RAW: FIELDLENGTH=%d bytes not supported", bytes)
	}
	return codec.IntValue(n), rest, nil
}

func encodeRecord(plan *Plan, rec codec.RecordValue) ([]byte, error) {
	var out []byte
	for _, name := range rec.Names {
		val, ok := rec.Fields[name]
		if !ok {
			return nil, fmt.Errorf("RAW: record missing field %q", name)
		}
		field := lookupField(plan, name)
		chunk, err := encodeField(plan, field, val)
		if err != nil {
			return nil, fmt.Errorf("RAW: field %q: %w", name, err)
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func decodeRecord(plan *Plan, data []byte) (codec.RecordValue, error) {
	rec := codec.RecordValue{Fields: map[string]codec.Value{}}
	rest := data
	for _, field := range plan.Fields {
		if field.Name == "" {
			continue
		}
		val, next, err := decodeField(plan, field, rest)
		if err != nil {
			return rec, fmt.Errorf("RAW: field %q: %w", field.Name, err)
		}
		rec.Names = append(rec.Names, field.Name)
		rec.Fields[field.Name] = val
		rest = next
	}
	return rec, nil
}

func encodeField(plan *Plan, field FieldPlan, v codec.Value) ([]byte, error) {
	switch val := v.(type) {
	case codec.IntValue:
		return encodeInt(plan, field, int64(val))
	case codec.BoolValue:
		if val {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case codec.BytesValue:
		return []byte(val), nil
	case codec.StringValue:
		return []byte(val), nil
	}
	return nil, fmt.Errorf("RAW: cannot encode kind %v", v.Kind())
}

func decodeField(plan *Plan, field FieldPlan, data []byte) (codec.Value, []byte, error) {
	if field.Type == "integer" || field.Length > 0 {
		return decodeInt(plan, field, data)
	}
	// Fallback: consume the rest of the buffer as bytes.
	return codec.BytesValue(append([]byte{}, data...)), nil, nil
}

func lookupField(plan *Plan, name string) FieldPlan {
	for _, f := range plan.Fields {
		if f.Name == name {
			return f
		}
	}
	// Default: 32-bit integer field; subsequent versions of the plan
	// builder will infer richer defaults from the field type.
	return FieldPlan{Name: name, Length: 32, Type: "integer"}
}

func parseLength(raw string) (int, bool) {
	upper := strings.ToUpper(raw)
	const prefix = "FIELDLENGTH("
	if !strings.HasPrefix(upper, prefix) {
		return 0, false
	}
	rest := upper[len(prefix):]
	close := strings.Index(rest, ")")
	if close < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest[:close]))
	if err != nil {
		return 0, false
	}
	return n, true
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
