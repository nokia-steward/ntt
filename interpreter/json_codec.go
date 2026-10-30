package interpreter

// json_codec.go encodes values to JSON and decodes JSON to values by their
// declared types, for encvalue / decvalue on a type with the JSON encode
// attribute (ETSI ES 201 873-11, "Using JSON with TTCN-3"): a record or set
// is an object with its fields in declaration order, an omitted optional
// field absent; a record of, set of or array is an array; a union is an
// object with its one chosen alternative; an enumerated value is its name;
// integers, floats, booleans and character strings are their JSON
// counterparts; an octetstring, bitstring or hexstring is a string of its
// digits. Decoding builds the value its type describes, and ignores
// members the type does not declare.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// jsonType is a type as the codec walks it: a name to resolve, or a type
// specification written in place, and the scope its names resolve in —
// the declaring module's — or nil for the caller's.
type jsonType struct {
	name  string
	spec  syntax.TypeSpec
	scope runtime.Scope
	// fv: the instructions of the field of this type, if it is one.
	fv *jsonVariants
}

// jsonShape is a type resolved: its kind (record, set, union, list,
// enumerated, or a base type's name), a struct's fields or a list's
// element type, an enumerated type's values, and the scope the names in
// those resolve in.
type jsonShape struct {
	kind   string
	fields []*syntax.Field
	elem   jsonType
	enum   *runtime.EnumType
	scope  runtime.Scope
	// The JSON encoding instructions of the type, and of its fields by
	// name (json_variants.go); unsupported: one the codec does not apply
	// (errJSONUnsupported).
	tv          jsonVariants
	fv          map[string]*jsonVariants
	unsupported bool
}

// errJSONUnsupported: the type is one the codec does not handle; the
// caller falls back to what it did before there was a JSON codec.
var errJSONUnsupported = errors.New("JSON: a type with variant attributes the codec does not apply")

// jsonSyntaxError: the text is no JSON.
type jsonSyntaxError struct{ err error }

func (e jsonSyntaxError) Error() string { return "JSON: " + e.err.Error() }

// resolveTypeName looks the type name up as scope sees it; a qualified
// name (`M.T`) in module M's scope. It returns the binding and the scope
// it was found in.
func resolveTypeName(name string, scope runtime.Scope) (runtime.Object, runtime.Scope, bool) {
	if v, ok := scope.Get(name); ok {
		return forceThunk(v), scope, true
	}
	if mod, local, ok := strings.Cut(name, "."); ok && mod != "" && local != "" {
		root := runtime.RootScope(scope)
		if _, known := root.Get(moduleScopeKey(mod)); known {
			ms := moduleScopeOf(root, mod)
			if v, ok := ms.Get(local); ok {
				return forceThunk(v), ms, true
			}
		}
	}
	return nil, scope, false
}

// jsonTypeOf resolves t: to a struct declaration (record, set, union), a
// list, an enumerated type, or a base type. ok is false when the codec
// cannot tell, and goes by the value instead.
func jsonTypeOf(t jsonType, env runtime.Scope) (jsonShape, bool) {
	scope := env
	if t.scope != nil {
		scope = t.scope
	}
	// The instructions of the types named on the way, an alias's first.
	var tv jsonVariants
	for depth := 0; depth < 32; depth++ {
		if t.spec != nil {
			switch s := t.spec.(type) {
			case *syntax.StructSpec:
				return jsonShape{kind: strings.ToLower(s.KindTok.String()), fields: s.Fields, scope: scope, tv: tv}, true
			case *syntax.ListSpec:
				return jsonShape{kind: "list", elem: jsonType{spec: s.ElemType, scope: scope}, scope: scope, tv: tv}, true
			case *syntax.RefSpec:
				t = jsonType{name: syntax.Name(s.X)}
				continue
			default:
				return jsonShape{}, false
			}
		}
		name := t.name
		if name == "" {
			return jsonShape{}, false
		}
		if _, isBase := baseObjectTypeForName(name); isBase || strings.EqualFold(name, "verdicttype") {
			return jsonShape{kind: strings.ToLower(name), scope: scope, tv: tv}, true
		}
		v, where, found := resolveTypeName(name, scope)
		if !found {
			return jsonShape{}, false
		}
		scope = where
		switch d := v.(type) {
		case *runtime.EnumType:
			if a, ok := scope.Get(enumAttrKey(d.Name)); ok {
				if td, ok := a.(*runtime.TypeDesc); ok {
					own, _, ok := jsonVariantsOf(td, nil)
					if !ok {
						return jsonShape{unsupported: true}, true
					}
					tv.fill(own)
				}
			}
			return jsonShape{kind: "enumerated", enum: d, scope: scope, tv: tv}, true
		case *runtime.TypeDesc:
			var fields []*syntax.Field
			if d.Struct != nil {
				fields = d.Struct.Fields
			}
			own, fv, ok := jsonVariantsOf(d, fields)
			if !ok {
				return jsonShape{unsupported: true}, true
			}
			tv.fill(own)
			// The type's own names resolve where it was declared.
			if d.Home != nil {
				scope = d.Home
			}
			if d.Struct != nil {
				shape := jsonShape{kind: strings.ToLower(d.Struct.KindTok.String()), fields: d.Struct.Fields, scope: scope, tv: tv, fv: fv}
				// A member named twice is no object (`name as` another
				// field's name).
				members := map[string]bool{}
				for _, f := range shape.fields {
					if f == nil || f.Name == nil {
						continue
					}
					if m := shape.jsonName(f); members[m] {
						return jsonShape{unsupported: true}, true
					} else {
						members[m] = true
					}
				}
				return shape, true
			}
			if d.Spec != nil {
				if _, isList := d.Spec.(*syntax.ListSpec); !isList && d.IsList {
					// An array of the referenced type: `type integer A[3]`.
					return jsonShape{kind: "list", elem: jsonType{spec: d.Spec, scope: scope}, scope: scope, tv: tv}, true
				}
				t = jsonType{spec: d.Spec}
				continue
			}
			if d.Underlying != "" && d.Underlying != name {
				t = jsonType{name: d.Underlying}
				continue
			}
		}
		return jsonShape{}, false
	}
	return jsonShape{}, false
}

// field returns the type of the struct field named name.
func (s jsonShape) field(name string) (*syntax.Field, jsonType) {
	for _, f := range s.fields {
		if f != nil && f.Name != nil && f.Name.String() == name {
			return f, s.fieldType(f)
		}
	}
	return nil, jsonType{}
}

func (s jsonShape) fieldType(f *syntax.Field) jsonType {
	fv := s.fv[f.Name.String()]
	if len(f.ArrayDef) > 0 {
		// `integer a[3]` in a record: a list of the field's type.
		return jsonType{spec: &syntax.ListSpec{ElemType: f.Type}, scope: s.scope, fv: fv}
	}
	return jsonType{spec: f.Type, scope: s.scope, fv: fv}
}

// encodeJSON renders v as JSON text by its type t.
func encodeJSON(v runtime.Object, t jsonType, env runtime.Scope) (string, error) {
	var b bytes.Buffer
	if err := writeJSON(&b, v, t, env); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeJSON(b *bytes.Buffer, v runtime.Object, t jsonType, env runtime.Scope) error {
	shape, known := jsonTypeOf(t, env)
	if shape.unsupported {
		return errJSONUnsupported
	}
	eff := shape.tv.over(t.fv)
	switch x := v.(type) {
	case runtime.Int:
		b.WriteString(x.String())
	case runtime.Float:
		// The special values are strings (ES 201 873-11, 8.2).
		f := float64(x)
		switch {
		case math.IsInf(f, 1):
			b.WriteString(`"infinity"`)
		case math.IsInf(f, -1):
			b.WriteString(`"-infinity"`)
		case math.IsNaN(f):
			b.WriteString(`"not_a_number"`)
		case eff.hasFrac:
			b.WriteString(formatJSONFloat(f, eff.fraction))
		default:
			// A float as a number with a fraction or an exponent, which
			// no integer is (7.2.10).
			t := strconv.FormatFloat(f, 'g', -1, 64)
			if !strings.ContainsAny(t, ".e") {
				t += ".0"
			}
			b.WriteString(t)
		}
	case runtime.Bool:
		b.WriteString(strconv.FormatBool(bool(x)))
	case *runtime.String:
		if eff.escape != "" {
			writeJSONStringEscaped(b, string(x.Value), eff.escape)
			return nil
		}
		return writeJSONString(b, string(x.Value))
	case *runtime.EnumValue:
		return writeJSONString(b, x.Key())
	case runtime.Verdict:
		return writeJSONString(b, string(x))
	case *runtime.Binarystring:
		return writeJSONString(b, binaryDigits(x))
	case *runtime.Record:
		if known && shape.kind == "union" {
			if len(x.Fields) != 1 {
				return fmt.Errorf("JSON: a union value with %d alternatives", len(x.Fields))
			}
			for k, fv := range x.Fields {
				f, ft := shape.field(k)
				// As its alternative's value alone (B.3.10).
				if eff.asValue {
					return writeJSON(b, fv, ft, env)
				}
				member := k
				if f != nil {
					member = shape.jsonName(f)
				}
				b.WriteByte('{')
				if err := writeJSONString(b, member); err != nil {
					return err
				}
				b.WriteByte(':')
				err := writeJSON(b, fv, ft, env)
				b.WriteByte('}')
				return err
			}
			return fmt.Errorf("JSON: a union value with no alternative")
		}
		return writeJSONObject(b, x.Fields, nil, shape, known, env)
	case *runtime.List:
		if len(x.FieldNames) > 0 && len(x.FieldNames) == len(x.Elements) {
			m := make(map[string]runtime.Object, len(x.Elements))
			for i, n := range x.FieldNames {
				m[n] = x.Elements[i]
			}
			return writeJSONObject(b, m, x.FieldNames, shape, known, env)
		}
		if known && (shape.kind == "record" || shape.kind == "set") && len(shape.fields) > 0 && len(x.Elements) <= len(shape.fields) {
			// A record value given positionally.
			m := make(map[string]runtime.Object, len(x.Elements))
			for i, e := range x.Elements {
				m[shape.fields[i].Name.String()] = e
			}
			return writeJSONObject(b, m, nil, shape, true, env)
		}
		b.WriteByte('[')
		for i, e := range x.Elements {
			if i > 0 {
				b.WriteByte(',')
			}
			if e == nil || e == runtime.Undefined {
				return fmt.Errorf("JSON: element %d is unbound", i)
			}
			if err := writeJSON(b, e, shape.elem, env); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		if v == runtime.Omit {
			b.WriteString("null")
			return nil
		}
		if v == nil || v == runtime.Undefined {
			return fmt.Errorf("JSON: an unbound value")
		}
		return fmt.Errorf("JSON: cannot encode a %s", v.Type())
	}
	return nil
}

// writeJSONObject writes a record's fields: in declaration order when the
// type is known, else in order (or by name), leaving out an omitted one.
// A mandatory field of a known type must be bound.
func writeJSONObject(b *bytes.Buffer, m map[string]runtime.Object, order []string, shape jsonShape, known bool, env runtime.Scope) error {
	var names []string
	known = known && len(shape.fields) > 0
	if known {
		for _, f := range shape.fields {
			if f != nil && f.Name != nil {
				names = append(names, f.Name.String())
			}
		}
	} else if order != nil {
		names = order
	} else {
		for k := range m {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	b.WriteByte('{')
	first := true
	for _, n := range names {
		f, ft := shape.field(n)
		member := n
		if f != nil {
			member = shape.jsonName(f)
		}
		fv, ok := m[n]
		if !ok || fv == nil || fv == runtime.Omit || fv == runtime.Undefined {
			if known && f != nil && f.Optional == nil {
				return fmt.Errorf("JSON: mandatory field %s is unbound", n)
			}
			if known && f != nil && fv == runtime.Undefined {
				return fmt.Errorf("JSON: optional field %s is unbound", n)
			}
			// An omitted field as null (B.3.8).
			if known && f != nil && ft.fv != nil && ft.fv.omitAsNull {
				if !first {
					b.WriteByte(',')
				}
				first = false
				if err := writeJSONString(b, member); err != nil {
					return err
				}
				b.WriteString(":null")
			}
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		if err := writeJSONString(b, member); err != nil {
			return err
		}
		b.WriteByte(':')
		if err := writeJSON(b, fv, ft, env); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// writeJSONString writes s as a JSON string, leaving <, > and & as they
// are.
func writeJSONString(b *bytes.Buffer, s string) error {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	b.Write(bytes.TrimSuffix(out.Bytes(), []byte("\n")))
	return nil
}

// binaryDigits renders a bit, hex or octet string as its digits.
func binaryDigits(x *runtime.Binarystring) string {
	switch x.Unit {
	case runtime.Octet:
		return strings.ToUpper(fmt.Sprintf("%x", octetstringBytes(x)))
	}
	s := x.Inspect()
	if i := strings.IndexByte(s, '\''); i >= 0 {
		if j := strings.LastIndexByte(s, '\''); j > i {
			return s[i+1 : j]
		}
	}
	return s
}

func octetstringBytes(x *runtime.Binarystring) []byte {
	out := make([]byte, x.Length)
	if x.Value == nil {
		return out
	}
	raw := x.Value.Bytes()
	if len(raw) > len(out) {
		raw = raw[len(raw)-len(out):]
	}
	copy(out[len(out)-len(raw):], raw)
	return out
}

// decodeJSON builds the value of type t that JSON text src encodes. The
// text must be one JSON value, with no member named twice in an object.
func decodeJSON(src string, t jsonType, env runtime.Scope) (runtime.Object, error) {
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	x, err := readJSONValue(dec)
	if err != nil {
		return nil, jsonSyntaxError{err}
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, jsonSyntaxError{errors.New("more than one value")}
	}
	// A value wrapped in an object naming its type, `{"M.T": value}`.
	if m, ok := x.(map[string]interface{}); ok && len(m) == 1 {
		if name := qualifiedTypeName(t, env); name != "" {
			if v, ok := m[name]; ok {
				x = v
			}
		}
	}
	return fromJSONValue(x, t, env)
}

// readJSONValue reads one JSON value token by token — as Unmarshal would
// into interface{}, but refusing an object that names a member twice.
func readJSONValue(dec *json.Decoder) (interface{}, error) {
	tok, err := dec.Token()
	if err != nil {
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	switch d := tok.(type) {
	case json.Delim:
		switch d {
		case '{':
			m := map[string]interface{}{}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := k.(string)
				if _, dup := m[key]; dup {
					return nil, fmt.Errorf("member %q given twice", key)
				}
				v, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				m[key] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return m, nil
		case '[':
			arr := []interface{}{}
			for dec.More() {
				v, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected %v", d)
	}
	return tok, nil
}

func fromJSONValue(x interface{}, t jsonType, env runtime.Scope) (runtime.Object, error) {
	shape, known := jsonTypeOf(t, env)
	if shape.unsupported {
		return nil, errJSONUnsupported
	}
	if !known {
		return inferJSONValue(x), nil
	}
	eff := shape.tv.over(t.fv)
	kind := shape.kind
	switch kind {
	case "record", "set":
		m, ok := x.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("JSON: want an object for a %s, got %v", kind, x)
		}
		rec := runtime.NewRecord()
		for _, f := range shape.fields {
			if f == nil || f.Name == nil {
				continue
			}
			name := f.Name.String()
			rec.Order = append(rec.Order, name)
			ft := shape.fieldType(f)
			jv, present := m[shape.jsonName(f)]
			// A field left out takes its default (B.3.9).
			if !present && ft.fv != nil && ft.fv.hasDefault {
				dv, err := jsonDefault(ft.fv.def, ft, env)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
				rec.Fields[name] = dv
				continue
			}
			if !present || jv == nil {
				if f.Optional != nil {
					rec.Fields[name] = runtime.Omit
					continue
				}
				return nil, fmt.Errorf("JSON: field %s missing", name)
			}
			fv, err := fromJSONValue(jv, ft, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			rec.Fields[name] = fv
		}
		return rec, nil
	case "union":
		// The value alone: the first alternative it is a value of
		// (B.3.10).
		if eff.asValue {
			for _, f := range shape.fields {
				if f == nil || f.Name == nil {
					continue
				}
				fv, err := fromJSONValue(x, shape.fieldType(f), env)
				if errors.Is(err, errJSONUnsupported) {
					return nil, err
				}
				if err == nil {
					rec := runtime.NewRecord()
					rec.Fields[f.Name.String()] = fv
					return rec, nil
				}
			}
			return nil, fmt.Errorf("JSON: %v is a value of no alternative of the union", x)
		}
		m, ok := x.(map[string]interface{})
		if !ok || len(m) != 1 {
			return nil, fmt.Errorf("JSON: want an object with one alternative for a union")
		}
		for k, jv := range m {
			f, ft := shape.fieldOfMember(k)
			if f == nil {
				return nil, fmt.Errorf("JSON: %s is no alternative of the union", k)
			}
			fv, err := fromJSONValue(jv, ft, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			rec := runtime.NewRecord()
			rec.Fields[f.Name.String()] = fv
			return rec, nil
		}
	case "list":
		arr, ok := x.([]interface{})
		if !ok {
			return nil, fmt.Errorf("JSON: want an array, got %v", x)
		}
		l := &runtime.List{ListType: runtime.RECORD_OF}
		for i, e := range arr {
			ev, err := fromJSONValue(e, shape.elem, env)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			l.Elements = append(l.Elements, ev)
		}
		return l, nil
	case "enumerated":
		s, ok := x.(string)
		if !ok || shape.enum == nil {
			return nil, fmt.Errorf("JSON: want an enumerated value's name, got %v", x)
		}
		ev, err := runtime.NewEnumValueByKey(shape.enum, s)
		if err != nil {
			return nil, fmt.Errorf("JSON: %q is no value of %s", s, shape.enum.Name)
		}
		return ev, nil
	case "verdicttype":
		s, _ := x.(string)
		switch v := runtime.Verdict(s); v {
		case runtime.NoneVerdict, runtime.PassVerdict, runtime.InconcVerdict, runtime.FailVerdict, runtime.ErrorVerdict:
			return v, nil
		}
		return nil, fmt.Errorf("JSON: want a verdict, got %v", x)
	case "integer":
		n, ok := x.(json.Number)
		if !ok {
			return nil, fmt.Errorf("JSON: want an integer, got %v", x)
		}
		i, ok := new(big.Int).SetString(string(n), 10)
		if !ok {
			return nil, fmt.Errorf("JSON: %s is no integer", n)
		}
		return runtime.NewInt(i.String()), nil
	case "float":
		switch v := x.(type) {
		case json.Number:
			f, err := v.Float64()
			if err != nil {
				return nil, fmt.Errorf("JSON: %w", err)
			}
			// A negative zero is a zero, unless useMinus (B.3.6).
			if f == 0 && !eff.useMinus {
				f = 0
			}
			return runtime.Float(f), nil
		case string:
			switch v {
			case "infinity":
				return runtime.Float(math.Inf(1)), nil
			case "-infinity":
				return runtime.Float(math.Inf(-1)), nil
			case "not_a_number":
				return runtime.Float(math.NaN()), nil
			}
		}
		return nil, fmt.Errorf("JSON: want a number, got %v", x)
	case "boolean":
		b, ok := x.(bool)
		if !ok {
			return nil, fmt.Errorf("JSON: want a boolean, got %v", x)
		}
		return runtime.NewBool(b), nil
	case "charstring", "universal charstring", "universalcharstring":
		s, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("JSON: want a string, got %v", x)
		}
		return runtime.NewCharstring(s), nil
	case "octetstring", "bitstring", "hexstring":
		s, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("JSON: want a string of digits, got %v", x)
		}
		return binaryFromDigits(s, kind)
	}
	return inferJSONValue(x), nil
}

// qualifiedTypeName is the named type t as `Module.Type`, its module the
// declaring one, or "".
func qualifiedTypeName(t jsonType, env runtime.Scope) string {
	if t.name == "" {
		return ""
	}
	scope := env
	if t.scope != nil {
		scope = t.scope
	}
	v, where, ok := resolveTypeName(t.name, scope)
	if !ok {
		return ""
	}
	if td, ok := v.(*runtime.TypeDesc); ok && td.Home != nil {
		where = td.Home
	}
	mod := moduleNameFromEnv(where)
	if mod == "" {
		return ""
	}
	return mod + "." + t.name[strings.LastIndexByte(t.name, '.')+1:]
}

// binaryFromDigits is the bit, hex or octet string whose digits s holds:
// only digits of its base, and whole octets for an octetstring.
func binaryFromDigits(s, kind string) (runtime.Object, error) {
	unit, digits := runtime.Octet, "0123456789abcdefABCDEF"
	switch kind {
	case "bitstring":
		unit, digits = runtime.Bit, "01"
	case "hexstring":
		unit = runtime.Hex
	}
	for _, r := range s {
		if !strings.ContainsRune(digits, r) {
			return nil, fmt.Errorf("JSON: %q is no %s", s, kind)
		}
	}
	if unit == runtime.Octet && len(s)%2 != 0 {
		return nil, fmt.Errorf("JSON: %q is no whole number of octets", s)
	}
	suffix := map[string]string{"octetstring": "O", "bitstring": "B", "hexstring": "H"}[kind]
	if s == "" {
		// As the literal ''O is.
		if v, err := runtime.NewBinarystring("''" + suffix); err == nil {
			return v, nil
		}
		return &runtime.Binarystring{Unit: unit, Value: big.NewInt(0), Length: 0}, nil
	}
	v, err := runtime.NewBinarystring("'" + s + "'" + suffix)
	if err != nil {
		return nil, fmt.Errorf("JSON: %q is no %s", s, kind)
	}
	return v, nil
}

// inferJSONValue is a JSON value as the runtime holds it when no type says
// otherwise.
func inferJSONValue(x interface{}) runtime.Object {
	switch v := x.(type) {
	case map[string]interface{}:
		rec := runtime.NewRecord()
		for k, e := range v {
			rec.Fields[k] = inferJSONValue(e)
		}
		return rec
	case []interface{}:
		l := &runtime.List{ListType: runtime.RECORD_OF}
		for _, e := range v {
			l.Elements = append(l.Elements, inferJSONValue(e))
		}
		return l
	case string:
		return runtime.NewCharstring(v)
	case json.Number:
		if i, ok := new(big.Int).SetString(string(v), 10); ok {
			return runtime.NewInt(i.String())
		}
		f, _ := v.Float64()
		return runtime.Float(f)
	case bool:
		return runtime.NewBool(v)
	}
	return runtime.Omit
}
