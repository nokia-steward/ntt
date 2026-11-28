package runtime

// json.go encodes values to JSON and decodes JSON to values by their
// types, for encvalue and decvalue on a type with the JSON encode
// attribute (ETSI ES 201 873-11): a record or set is an object with its
// fields in declaration order, an omitted optional field absent; a record
// of, set of or array is an array; a union is an object with its one
// chosen alternative; an enumerated value is its name; integers, floats,
// booleans and character strings are their JSON counterparts, a float's
// special values "infinity", "-infinity" and "not_a_number"; a bit, hex
// or octet string is a string of its digits. Decoding ignores members
// the type does not declare. Variant attributes are not looked at.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// isJSON reports whether typ is encoded as JSON.
func isJSON(typ Type) bool {
	return typeAttributes(typ).Encode.Value == "JSON"
}

// EncodeJSON returns the JSON text of v, a value of type typ.
func EncodeJSON(v any, typ Type) (string, error) {
	var b bytes.Buffer
	if err := writeJSON(&b, v, typ); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeJSON(b *bytes.Buffer, v any, typ Type) error {
	base := BaseType(typ)
	switch x := v.(type) {
	case nil, Undefined:
		return errors.New("JSON: unbound value")
	case bool:
		b.WriteString(strconv.FormatBool(x))
		return nil
	case int:
		b.WriteString(strconv.Itoa(x))
		return nil
	case float64:
		switch {
		case math.IsInf(x, 1):
			b.WriteString(`"infinity"`)
		case math.IsInf(x, -1):
			b.WriteString(`"-infinity"`)
		case math.IsNaN(x):
			b.WriteString(`"not_a_number"`)
		default:
			b.WriteString(strconv.FormatFloat(x, 'g', -1, 64))
		}
		return nil
	case string:
		return writeJSONString(b, x)
	case Verdict:
		return writeJSONString(b, string(x))
	case EnumValue:
		return writeJSONString(b, x.Name)
	case Binary:
		return writeJSONString(b, x.Digits)
	case []any:
		if fields := UnionFields(base); fields != nil {
			for _, f := range fields {
				if f.Index >= 0 && f.Index < len(x) && isBound(x[f.Index]) {
					b.WriteByte('{')
					if err := writeJSONString(b, f.Name); err != nil {
						return err
					}
					b.WriteByte(':')
					if err := writeJSON(b, x[f.Index], f.Type); err != nil {
						return err
					}
					b.WriteByte('}')
					return nil
				}
			}
			return errors.New("JSON: union with no chosen alternative")
		}
		if fields := RecordFields(base); fields != nil {
			b.WriteByte('{')
			n := 0
			for _, f := range fields {
				var fv any = Undefined{}
				if f.Index >= 0 && f.Index < len(x) {
					fv = x[f.Index]
				}
				if fv == OmitVal {
					continue
				}
				if n > 0 {
					b.WriteByte(',')
				}
				n++
				if err := writeJSONString(b, f.Name); err != nil {
					return err
				}
				b.WriteByte(':')
				if err := writeJSON(b, fv, f.Type); err != nil {
					return fmt.Errorf("%w (field %s)", err, f.Name)
				}
			}
			b.WriteByte('}')
			return nil
		}
		et, _ := ElemType(base)
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, e, et); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	}
	return fmt.Errorf("JSON: cannot encode %T", v)
}

// fieldSlots returns the unbound value of a record, set or union with the
// given fields: a slot for each field, at the field's index.
func fieldSlots(fields []*Var) []any {
	n := 0
	for _, f := range fields {
		n = max(n, f.Index+1)
	}
	slots := make([]any, n)
	for i := range slots {
		slots[i] = Undefined{}
	}
	return slots
}

func isBound(v any) bool {
	switch v.(type) {
	case nil, Undefined:
		return false
	}
	return true
}

func writeJSONString(b *bytes.Buffer, s string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	b.Write(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return nil
}

// DecodeJSON returns the value of type typ that the JSON text src holds.
// Text holding more than one value, a member given twice or a value that
// does not fit the type is an error.
func DecodeJSON(src string, typ Type) (any, error) {
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	x, err := readJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("JSON: more than one value")
	}
	return fromJSON(x, typ)
}

// jsonObject is a decoded object, its members in the order given.
type jsonObject struct {
	keys []string
	vals map[string]any
}

// readJSONValue reads one value, keeping objects' member order and
// refusing a member given twice.
func readJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON: %w", err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := jsonObject{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, fmt.Errorf("JSON: %w", err)
				}
				k, _ := kt.(string)
				if _, dup := obj.vals[k]; dup {
					return nil, fmt.Errorf("JSON: member %q given twice", k)
				}
				v, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj.keys = append(obj.keys, k)
				obj.vals[k] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, fmt.Errorf("JSON: %w", err)
			}
			return obj, nil
		case '[':
			var arr []any
			for dec.More() {
				v, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, fmt.Errorf("JSON: %w", err)
			}
			if arr == nil {
				arr = []any{}
			}
			return arr, nil
		}
		return nil, fmt.Errorf("JSON: unexpected %v", t)
	}
	return tok, nil
}

func fromJSON(x any, typ Type) (any, error) {
	base := BaseType(typ)
	if fields := UnionFields(base); fields != nil {
		obj, ok := x.(jsonObject)
		if !ok || len(obj.keys) != 1 {
			return nil, fmt.Errorf("JSON: want an object of one alternative, got %v", x)
		}
		for _, f := range fields {
			if f.Name == obj.keys[0] {
				v, err := fromJSON(obj.vals[f.Name], f.Type)
				if err != nil {
					return nil, err
				}
				u := fieldSlots(fields)
				u[f.Index] = v
				return u, nil
			}
		}
		return nil, fmt.Errorf("JSON: no alternative %q", obj.keys[0])
	}
	if fields := RecordFields(base); fields != nil {
		obj, ok := x.(jsonObject)
		if !ok {
			return nil, fmt.Errorf("JSON: want an object, got %v", x)
		}
		rec := fieldSlots(fields)
		for _, f := range fields {
			fx, ok := obj.vals[f.Name]
			if !ok || fx == nil {
				if f.Flags&Optional == 0 {
					return nil, fmt.Errorf("JSON: field %s missing", f.Name)
				}
				rec[f.Index] = OmitVal
				continue
			}
			v, err := fromJSON(fx, f.Type)
			if err != nil {
				return nil, err
			}
			rec[f.Index] = v
		}
		return rec, nil
	}
	if et, ok := ElemType(base); ok {
		arr, ok := x.([]any)
		if !ok {
			return nil, fmt.Errorf("JSON: want an array, got %v", x)
		}
		out := make([]any, len(arr))
		for i, e := range arr {
			v, err := fromJSON(e, et)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	if et, ok := base.(EnumeratedType); ok {
		name, _ := x.(string)
		terms, _ := et.Body.([]*Var)
		for _, t := range terms {
			if ev, ok := t.Value.(EnumValue); ok && ev.Name == name {
				return ev, nil
			}
		}
		return nil, fmt.Errorf("JSON: no enumerated value %v", x)
	}
	name := ""
	if nt, ok := base.(*NamedType); ok {
		name = nt.Name
	}
	switch name {
	case "integer":
		if n, ok := x.(json.Number); ok {
			if i, err := strconv.Atoi(string(n)); err == nil {
				return i, nil
			}
		}
	case "float":
		switch v := x.(type) {
		case json.Number:
			if f, err := v.Float64(); err == nil {
				return f, nil
			}
		case string:
			switch v {
			case "infinity":
				return math.Inf(1), nil
			case "-infinity":
				return math.Inf(-1), nil
			case "not_a_number":
				return math.NaN(), nil
			}
		}
	case "boolean":
		if v, ok := x.(bool); ok {
			return v, nil
		}
	case "charstring", "universal charstring":
		if v, ok := x.(string); ok {
			return v, nil
		}
	case "verdicttype":
		if v, ok := x.(string); ok {
			switch Verdict(v) {
			case "none", "pass", "inconc", "fail", "error":
				return Verdict(v), nil
			}
		}
	case "bitstring", "hexstring", "octetstring":
		if v, ok := x.(string); ok {
			unit, digits := byte('B'), "01"
			switch name {
			case "hexstring":
				unit, digits = 'H', hexDigits
			case "octetstring":
				unit, digits = 'O', hexDigits
			}
			v = strings.ToUpper(v)
			if strings.Trim(v, digits) == "" && (unit != 'O' || len(v)%2 == 0) {
				return Binary{Unit: unit, Digits: v}, nil
			}
		}
	}
	return nil, fmt.Errorf("JSON: %v is not a %s", x, name)
}
