// Package json implements the TTCN-3 JSON codec per Annex E.5. JSON is
// the friendliest TTCN-3 codec because the wire form is human-readable
// and the Go standard library does almost all the heavy lifting; the
// interesting work is mapping TTCN-3 value kinds to the JSON shape
// rules in Annex E.5 (charstring -> JSON string, octetstring -> hex
// string, ...) and applying per-field variant directives (`alias`,
// `escape as short`, etc).
//
// This baseline covers the common shapes: integer, float, boolean,
// charstring, octetstring (encoded as a hex string), record (object)
// and record-of (array). Variant attributes parsed but only partially
// honoured land in the Plan so future iterations can extend behaviour
// without breaking the public API.
package json

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

// Codec is the JSON codec implementation.
type Codec struct{}

func init() { codec.Register(Codec{}) }

// Name implements codec.Codec.
func (Codec) Name() string { return "JSON" }

// Plan is the compiled view of a JSON-annotated type. The plan stores
// per-field aliases and the encoding hints (compact vs pretty); the
// runtime caches it the same way as the RAW plan.
type Plan struct {
	Compact bool
	Aliases map[string]string // ttcn3 field name -> JSON key
}

// EncodingName implements codec.Plan.
func (Plan) EncodingName() string { return "JSON" }

// Compile builds a Plan from the attribute set. Annex E.5 directives
// understood here: `JSON(compact)`, `JSON: name as "alias"` (the
// short and verbose forms), `JSON: as omit` for optional fields. The
// rest is captured for round-trip and ignored.
func (Codec) Compile(set *attr.AttributeSet) (codec.Plan, error) {
	plan := &Plan{Aliases: map[string]string{}}
	if set == nil {
		return plan, nil
	}
	for _, v := range set.Variants() {
		raw := strings.TrimSpace(v.Value)
		upper := strings.ToUpper(raw)
		if upper == "JSON(COMPACT)" || upper == "COMPACT" {
			plan.Compact = true
			continue
		}
		if alias, ok := parseAlias(raw); ok {
			key := strings.Join(v.Selectors, ".")
			plan.Aliases[key] = alias
			continue
		}
	}
	return plan, nil
}

// Encode serialises v as JSON.
func (Codec) Encode(p codec.Plan, v codec.Value) ([]byte, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("JSON: plan %T is not a JSON plan", p)
	}
	tree, err := toJSON(plan, v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := stdjson.NewEncoder(&buf)
	if !plan.Compact {
		enc.SetIndent("", "  ")
	}
	// Encoder.Encode appends a newline; strip it so the byte string
	// matches the codec test expectations.
	if err := enc.Encode(tree); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// Decode parses JSON data into a Value. The shape is inferred from the
// JSON: objects become RecordValue, arrays become ListValue, numbers
// become IntValue when integral, strings become StringValue. Octet-
// string heuristics belong to a future iteration once the plan carries
// per-field type information.
func (Codec) Decode(p codec.Plan, data []byte) (codec.Value, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("JSON: plan %T is not a JSON plan", p)
	}
	var any interface{}
	dec := stdjson.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&any); err != nil {
		return nil, err
	}
	return fromJSON(plan, any), nil
}

func toJSON(plan *Plan, v codec.Value) (interface{}, error) {
	switch val := v.(type) {
	case codec.IntValue:
		return int64(val), nil
	case codec.FloatValue:
		return float64(val), nil
	case codec.BoolValue:
		return bool(val), nil
	case codec.StringValue:
		return string(val), nil
	case codec.BytesValue:
		return hexEncode(val), nil
	case codec.RecordValue:
		out := map[string]interface{}{}
		keys := append([]string{}, val.Names...)
		for _, name := range keys {
			fv := val.Fields[name]
			child, err := toJSON(plan, fv)
			if err != nil {
				return nil, err
			}
			key := name
			if alias, ok := plan.Aliases[name]; ok {
				key = alias
			}
			out[key] = child
		}
		return out, nil
	case codec.ListValue:
		out := make([]interface{}, len(val.Elements))
		for i, e := range val.Elements {
			child, err := toJSON(plan, e)
			if err != nil {
				return nil, err
			}
			out[i] = child
		}
		return out, nil
	}
	return nil, fmt.Errorf("JSON: cannot encode kind %v", v.Kind())
}

func fromJSON(plan *Plan, x interface{}) codec.Value {
	switch v := x.(type) {
	case stdjson.Number:
		if n, err := v.Int64(); err == nil {
			return codec.IntValue(n)
		}
		if f, err := v.Float64(); err == nil {
			return codec.FloatValue(f)
		}
		return codec.StringValue(v.String())
	case bool:
		return codec.BoolValue(v)
	case string:
		return codec.StringValue(v)
	case []interface{}:
		out := codec.ListValue{Elements: make([]codec.Value, len(v))}
		for i, e := range v {
			out.Elements[i] = fromJSON(plan, e)
		}
		return out
	case map[string]interface{}:
		rec := codec.RecordValue{Fields: map[string]codec.Value{}}
		// Stable name order so round-trips are reproducible.
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			name := unalias(plan, k)
			rec.Names = append(rec.Names, name)
			rec.Fields[name] = fromJSON(plan, v[k])
		}
		return rec
	case nil:
		return nil
	}
	return nil
}

// parseAlias matches `name as "alias"` syntax.
func parseAlias(raw string) (string, bool) {
	const sentinel = " AS "
	upper := strings.ToUpper(raw)
	idx := strings.Index(upper, sentinel)
	if idx < 0 {
		return "", false
	}
	tail := strings.TrimSpace(raw[idx+len(sentinel):])
	if len(tail) >= 2 && tail[0] == '"' && tail[len(tail)-1] == '"' {
		return tail[1 : len(tail)-1], true
	}
	return tail, true
}

func unalias(plan *Plan, jsonKey string) string {
	for ttcn, alias := range plan.Aliases {
		if alias == jsonKey {
			return ttcn
		}
	}
	return jsonKey
}

func hexEncode(b []byte) string {
	out := make([]byte, 2*len(b))
	const hex = "0123456789ABCDEF"
	for i, x := range b {
		out[2*i] = hex[x>>4]
		out[2*i+1] = hex[x&0x0F]
	}
	return string(out)
}
