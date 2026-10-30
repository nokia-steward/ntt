// Package codec defines the public interface every TTCN-3 codec
// implementation honours. Concrete codecs live in subpackages
// (codec/raw, codec/json, codec/ber, ...) and are registered with the
// global registry at process start so the runtime can dispatch by the
// `with { encode "..." }` attribute parsed by ttcn3/attr.
//
// The codec API is intentionally small: codecs work over Values
// (typed payloads from the runtime) and []byte (wire form). Schema
// information - field order, tagging, length fields - comes from a
// Plan that the codec compiles ahead of time from the variant
// attributes; the same Plan is reused for every encode/decode call on a
// given type.
package codec

import (
	"fmt"
	"sort"
	"sync"

	"github.com/nokia/ntt/ttcn3/attr"
)

// Value is the runtime payload representation the codecs operate on.
// The model is deliberately a small subset of runtime.Object: integers,
// floats, strings, booleans, bytes, lists, and records. Concrete
// runtimes wrap their richer values around this interface.
type Value interface {
	Kind() Kind
	Inspect() string
}

// Kind enumerates the codec-visible value shapes. Numeric kinds carry
// their bit width via the IntValue / FloatValue concrete types.
type Kind int

const (
	KindUnknown Kind = iota
	KindInteger
	KindFloat
	KindBoolean
	KindCharstring
	KindBitstring
	KindOctetstring
	KindHexstring
	KindEnum
	KindRecord
	KindRecordOf
	KindUnion
	KindNull
)

// IntValue is a 64-bit signed integer payload.
type IntValue int64

// Kind implements Value.
func (IntValue) Kind() Kind          { return KindInteger }
func (v IntValue) Inspect() string   { return fmt.Sprintf("%d", int64(v)) }

// FloatValue is a 64-bit IEEE 754 payload.
type FloatValue float64

// Kind implements Value.
func (FloatValue) Kind() Kind        { return KindFloat }
func (v FloatValue) Inspect() string { return fmt.Sprintf("%g", float64(v)) }

// BoolValue carries a TTCN-3 boolean.
type BoolValue bool

// Kind implements Value.
func (BoolValue) Kind() Kind        { return KindBoolean }
func (v BoolValue) Inspect() string { return fmt.Sprintf("%t", bool(v)) }

// StringValue carries a charstring payload.
type StringValue string

// Kind implements Value.
func (StringValue) Kind() Kind        { return KindCharstring }
func (v StringValue) Inspect() string { return fmt.Sprintf("%q", string(v)) }

// BytesValue carries an octetstring payload.
type BytesValue []byte

// Kind implements Value.
func (BytesValue) Kind() Kind      { return KindOctetstring }
func (v BytesValue) Inspect() string {
	out := "'"
	for _, b := range v {
		out += fmt.Sprintf("%02X", b)
	}
	return out + "'O"
}

// RecordValue is an ordered set of named fields. Order matters for
// most codecs (especially RAW), so we keep an explicit Names slice in
// addition to the map.
type RecordValue struct {
	Names  []string
	Fields map[string]Value
}

// NewRecord builds a RecordValue from name/value pairs, preserving
// declaration order. Callers should use this constructor rather than
// literal struct initialisation to avoid forgetting to populate Names.
func NewRecord(pairs ...interface{}) RecordValue {
	if len(pairs)%2 != 0 {
		panic("codec.NewRecord: odd number of arguments")
	}
	r := RecordValue{Fields: map[string]Value{}}
	for i := 0; i < len(pairs); i += 2 {
		name, ok := pairs[i].(string)
		if !ok {
			panic("codec.NewRecord: keys must be strings")
		}
		val, ok := pairs[i+1].(Value)
		if !ok {
			panic("codec.NewRecord: values must implement Value")
		}
		r.Names = append(r.Names, name)
		r.Fields[name] = val
	}
	return r
}

// Kind implements Value.
func (RecordValue) Kind() Kind { return KindRecord }

// Inspect implements Value.
func (r RecordValue) Inspect() string {
	out := "{"
	for i, name := range r.Names {
		if i > 0 {
			out += ", "
		}
		out += name + " := " + r.Fields[name].Inspect()
	}
	return out + "}"
}

// ListValue is a record-of / set-of payload.
type ListValue struct {
	Elements []Value
}

// Kind implements Value.
func (ListValue) Kind() Kind { return KindRecordOf }

// Inspect implements Value.
func (l ListValue) Inspect() string {
	out := "{"
	for i, e := range l.Elements {
		if i > 0 {
			out += ", "
		}
		out += e.Inspect()
	}
	return out + "}"
}

// Plan is the codec-specific compiled view of a TTCN-3 type plus its
// `with { ... }` attributes. Different codecs use different concrete
// Plan types; the registry stores them as `interface{}` and each codec
// downcasts internally.
type Plan interface {
	// EncodingName reports the canonical codec name this plan was
	// compiled for (the value of the `encode` attribute).
	EncodingName() string
}

// Codec is the contract for a registered encoder/decoder.
type Codec interface {
	Name() string

	// Compile takes an attribute set and produces a Plan that Encode /
	// Decode reuse. Codecs that don't need ahead-of-time work can
	// return a no-op Plan.
	Compile(set *attr.AttributeSet) (Plan, error)

	// Encode serialises v according to plan. Returns the wire-form
	// bytes plus any error.
	Encode(plan Plan, v Value) ([]byte, error)

	// Decode parses data into a Value of the shape plan expects.
	Decode(plan Plan, data []byte) (Value, error)
}

// registry holds every codec keyed by its canonical name. Concrete
// codec packages call Register in their init() so user code only has
// to import the package to wire it in.
var (
	registryMu sync.RWMutex
	registry   = map[string]Codec{}
)

// Register installs c under its Name() in the global registry. Calling
// Register with a duplicate name panics: codec registration is a
// startup-time decision and an accidental collision is almost always
// a bug.
func Register(c Codec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	name := c.Name()
	if _, dup := registry[name]; dup {
		panic("codec already registered: " + name)
	}
	registry[name] = c
}

// Lookup returns the codec registered under name, or nil if none was.
// Names are matched case-insensitively to match the TTCN-3 convention
// where `with { encode "raw" }` and `with { encode "RAW" }` are the
// same codec.
func Lookup(name string) Codec {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if c, ok := registry[name]; ok {
		return c
	}
	for n, c := range registry {
		if eqFold(n, name) {
			return c
		}
	}
	return nil
}

// Names returns the registered codec names in sorted order. Useful for
// `ntt list codecs` and the diagnostics that suggest closest matches.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
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
