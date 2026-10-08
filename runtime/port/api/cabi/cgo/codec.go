//go:build cgo

package cgo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nokia/ntt/runtime"
)

// encodePayload turns a runtime.Object into the wire bytes the C test
// port will see. The encoding rules are tuned for an external test-port
// suite where HttpRequest / HttpResponse records flow over a
// port:
//
//   - raw transports stay raw: []byte and string pass through with
//     their bytes verbatim (charstring & octetstring already arrive
//     here as []byte / string from the interpreter's value->binary
//     conversions).
//   - typed records (runtime.*Record), lists, maps, ints, bools, etc.
//     are JSON-encoded so the C port can decode them with any
//     mainstream JSON library; the encoding is stable (records emit
//     fields in declaration-discovered, lexicographic order) so the
//     C side can compare bytes for equality without surprises.
//   - opaque kinds (TypeDesc, functions, lazy thunks, ...) fail with
//     an explicit "cannot encode" error so we don't quietly hand the
//     C port a meaningless inspect-string.
//
// The function is intentionally codec-agnostic on the wire: a single
// JSON envelope keeps the dependency surface small and makes the
// payload trivially readable from C with libraries like cJSON,
// nlohmann/json, or jansson. Future work may add a binary codec
// (BER/JER per the existing runtime codec) selected via a
// per-port-type annotation.
func encodePayload(o runtime.Object) ([]byte, error) {
	if o == nil {
		return []byte("null"), nil
	}
	v, err := toJSON(o)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// toJSON converts a runtime.Object into a value the standard
// encoding/json package can serialise. Returns an error rather than a
// best-effort string so callers see the gap instead of corrupted
// wire data.
func toJSON(o runtime.Object) (interface{}, error) {
	switch v := o.(type) {
	case nil:
		return nil, nil
	case runtime.Bool:
		return bool(v), nil
	case runtime.Int:
		if v.Int == nil {
			return 0, nil
		}
		// JSON has no arbitrary-precision integer, but we keep
		// the full big.Int by way of json.Number so big values
		// round-trip exactly through marshallers that honour
		// json.Number (cJSON / nlohmann::json with the
		// 'NumberIntegerType' = int64_t setting).
		return json.Number(v.Int.String()), nil
	case runtime.Float:
		return float64(v), nil
	case runtime.Verdict:
		return string(v), nil
	case *runtime.String:
		return v.String(), nil
	case *runtime.Binarystring:
		// Wire ports usually want raw octets. We surface the
		// canonical TTCN-3 string form (e.g. "'48656C6C6F'O")
		// so the C port has the unit annotation; ports that
		// need raw bytes should declare their payload as
		// octetstring and feed it through Octets() directly.
		return v.String, nil
	case *runtime.List:
		out := make([]interface{}, 0, len(v.Elements))
		for _, e := range v.Elements {
			j, err := toJSON(e)
			if err != nil {
				return nil, err
			}
			out = append(out, j)
		}
		return out, nil
	case *runtime.Record:
		// Sort field names so the wire bytes are deterministic
		// across runs - the C side can hash / diff them
		// reliably and the conformance comparator stays
		// reproducible.
		keys := make([]string, 0, len(v.Fields))
		for k := range v.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(keys))
		for _, k := range keys {
			j, err := toJSON(v.Fields[k])
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", k, err)
			}
			out[k] = j
		}
		return out, nil
	case *runtime.Map:
		pairs := v.Pairs()
		out := make(map[string]interface{}, len(pairs))
		for _, p := range pairs {
			ks, err := mapKeyString(p.Key)
			if err != nil {
				return nil, err
			}
			jv, err := toJSON(p.Value)
			if err != nil {
				return nil, fmt.Errorf("map[%s]: %w", ks, err)
			}
			out[ks] = jv
		}
		return out, nil
	case *runtime.EnumValue:
		return v.Inspect(), nil
	}
	// `Undefined`, `Null`, and the other singletons land here.
	// Treat Undefined / Null as JSON null so the C side sees an
	// explicit absence marker rather than a synthesised string.
	switch o {
	case runtime.Undefined, runtime.Null:
		return nil, nil
	}
	return nil, fmt.Errorf("encode: unsupported type %T", o)
}

// decodePayload turns the JSON bytes a C test port hands back via
// the inject() callback into a runtime.Object the interpreter can
// match against TTCN-3 templates. It is the inverse of encodePayload
// for the subset we use on the wire (records, lists, strings,
// integers, floats, booleans, nulls). Field maps are walked
// recursively so nested records / lists round-trip exactly.
//
// The decoder is intentionally schema-less: TTCN-3 record templates
// match on field name, so we produce a runtime.Record with the same
// field names the C side wrote. The interpreter's template-match
// path coerces our generic Int / Float / String values to the
// destination record's declared field types at compare time.
func decodePayload(data []byte) (runtime.Object, error) {
	if len(data) == 0 {
		return runtime.Null, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // preserve int / float distinction
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return fromJSONValue(v), nil
}

// fromJSONValue is decodePayload's per-node helper. We accept the
// shapes encoding/json (with UseNumber) emits: map[string]interface{},
// []interface{}, json.Number, string, bool, nil.
func fromJSONValue(v interface{}) runtime.Object {
	switch x := v.(type) {
	case nil:
		return runtime.Null
	case bool:
		return runtime.Bool(x)
	case string:
		return runtime.NewCharstring(x)
	case json.Number:
		// Prefer integer when the literal has no decimal point /
		// exponent so TTCN-3 `integer`-typed fields match
		// exactly. runtime.NewInt now accepts int64 directly so
		// values up to 2^63-1 round-trip without going through
		// the string form; anything bigger keeps the string
		// path so big integers don't lose precision.
		if n, err := x.Int64(); err == nil {
			return runtime.NewInt(n)
		}
		if f, err := x.Float64(); err == nil {
			return runtime.Float(f)
		}
		return runtime.NewCharstring(string(x))
	case float64:
		// Plain decode (no UseNumber): integers come through as
		// float64 too. Promote to integer when the value is
		// exactly representable.
		if x == float64(int64(x)) {
			return runtime.NewInt(int64(x))
		}
		return runtime.Float(x)
	case []interface{}:
		elems := make([]runtime.Object, len(x))
		for i, e := range x {
			elems[i] = fromJSONValue(e)
		}
		return &runtime.List{Elements: elems}
	case map[string]interface{}:
		fields := make(map[string]runtime.Object, len(x))
		for k, vv := range x {
			fields[k] = fromJSONValue(vv)
		}
		return &runtime.Record{Fields: fields}
	}
	return runtime.Undefined
}

// mapKeyString reduces a runtime.Object map key to the string form
// JSON object keys require. We accept the same primitive shapes the
// rest of the encoder handles and stringify them via Inspect for
// anything more exotic - the JSON object format has no notion of
// non-string keys, so callers that care about preserving rich keys
// should declare their payload as a list of {key, value} pairs.
func mapKeyString(k runtime.Object) (string, error) {
	switch v := k.(type) {
	case *runtime.String:
		return v.String(), nil
	case runtime.Bool:
		if bool(v) {
			return "true", nil
		}
		return "false", nil
	case runtime.Int:
		if v.Int == nil {
			return "0", nil
		}
		return v.Int.String(), nil
	case runtime.Float:
		return fmt.Sprintf("%g", float64(v)), nil
	}
	if k == nil {
		return "", fmt.Errorf("encode: nil map key")
	}
	return k.Inspect(), nil
}
