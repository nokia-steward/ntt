package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EncValue encodes v according to the TTCN-3 type attributes on typ, as k3r does
// with the default ttcn3 codec (ASCII value syntax) and the ETSI "32 bit" integer variant.
func EncValue(v any, typ Type) (Binary, error) {
	if typ == nil {
		typ = IntegerType
	}
	if attrs := typeAttributes(typ); attrs.Variant.Value == "32 bit" {
		if isIntegerType(BaseType(typ)) {
			return encodeInt32(v)
		}
	}
	s, err := encodeValueString(v, typ)
	if err != nil {
		return Binary{}, err
	}
	return Binary{Unit: 'B', Digits: asciiBits(s)}, nil
}

// DecValue decodes bits into target using typ. It returns 0 on success, 1 on
// failure, and 2 when typ uses the "32 bit" variant and fewer than 32 bits
// are available. On non-success the target is left unchanged.
func DecValue(enc Binary, typ Type, target *any) (code int, rest Binary) {
	if typ == nil {
		typ = IntegerType
	}
	bits := enc.bits()
	if attrs := typeAttributes(typ); attrs.Variant.Value == "32 bit" {
		if isIntegerType(BaseType(typ)) {
			if len(bits) < 32 {
				return 2, enc
			}
			if len(bits) == 32 {
				v := int(decodeUint32(bits))
				*target = v
				return 0, Binary{Unit: 'B'}
			}
			v := int(decodeUint32(bits[:32]))
			*target = v
			return 0, Binary{Unit: 'B', Digits: bits[32:]}
		}
	}
	if len(bits)%8 != 0 {
		return 1, enc
	}
	s := bitsToASCII(bits)
	v, n, err := decodeValueString(s, typ)
	if err != nil || n != len(s) {
		return 1, enc
	}
	*target = v
	if len(bits) > len(s)*8 {
		return 0, Binary{Unit: 'B', Digits: bits[len(s)*8:]}
	}
	return 0, Binary{Unit: 'B'}
}

func typeAttributes(typ Type) Attributes {
	for typ != nil {
		switch t := typ.(type) {
		case *NamedType:
			return t.Attributes
		case TemplateType:
			typ = t.BaseType
		case *TemplateType:
			typ = t.BaseType
		case Subtype:
			typ = t.BaseType
		case *Subtype:
			typ = t.BaseType
		default:
			return Attributes{}
		}
	}
	return Attributes{}
}

func isIntegerType(t Type) bool {
	if t == IntegerType {
		return true
	}
	if nt, ok := t.(*NamedType); ok && nt.Name == "integer" {
		return true
	}
	return false
}

func encodeInt32(v any) (Binary, error) {
	i, ok := v.(int)
	if !ok {
		return Binary{}, fmt.Errorf("encvalue: expected integer, got %T", v)
	}
	if i < 0 || i > math.MaxUint32 {
		return Binary{}, fmt.Errorf("encvalue: integer out of range for 32 bit variant")
	}
	b := make([]byte, 4)
	b[0] = byte(i >> 24)
	b[1] = byte(i >> 16)
	b[2] = byte(i >> 8)
	b[3] = byte(i)
	var digits strings.Builder
	for _, c := range b {
		fmt.Fprintf(&digits, "%08b", c)
	}
	return Binary{Unit: 'B', Digits: digits.String()}, nil
}

func decodeUint32(bits string) uint32 {
	var v uint32
	for i := 0; i < 32 && i < len(bits); i++ {
		v <<= 1
		if bits[i] == '1' {
			v |= 1
		}
	}
	return v
}

func asciiBits(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "%08b", s[i])
	}
	return b.String()
}

func bitsToASCII(bits string) string {
	var s strings.Builder
	for i := 0; i+8 <= len(bits); i += 8 {
		var c byte
		for j := 0; j < 8; j++ {
			c <<= 1
			if bits[i+j] == '1' {
				c |= 1
			}
		}
		s.WriteByte(c)
	}
	return s.String()
}

func encodeValueString(v any, typ Type) (string, error) {
	base := BaseType(typ)
	switch x := v.(type) {
	case int:
		if et, ok := base.(EnumeratedType); ok {
			if terms, ok := et.Body.([]*Var); ok {
				for _, t := range terms {
					if ev, ok := t.Value.(EnumValue); ok && ev.Val == x {
						return ev.Name, nil
					}
				}
			}
		}
		return strconv.Itoa(x), nil
	case EnumValue:
		return x.Name, nil
	case string:
		return Quote(x), nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case float64:
		return FormatFloat(x), nil
	case Binary:
		return x.String(), nil
	case []any:
		if fields := RecordFields(base); fields != nil {
			_, isSet := base.(SetType)
			var b strings.Builder
			b.WriteByte('{')
			n := len(x)
			if isSet {
				n = len(fields)
			}
			for i := 0; i < n; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				var elem any = Undefined{}
				if i < len(x) {
					elem = x[i]
				}
				var ft Type
				if i < len(fields) {
					if isSet {
						b.WriteString(fields[i].Name)
						b.WriteString(":=")
					}
					ft = fields[i].Type
				}
				part, err := encodeValueString(elem, ft)
				if err != nil {
					return "", err
				}
				b.WriteString(part)
			}
			b.WriteByte('}')
			return b.String(), nil
		}
		if uFields := UnionFields(base); uFields != nil {
			for _, f := range uFields {
				if f.Index >= 0 && f.Index < len(x) {
					if _, undef := x[f.Index].(Undefined); !undef && x[f.Index] != nil {
						part, err := encodeValueString(x[f.Index], f.Type)
						if err != nil {
							return "", err
						}
						return "{" + f.Name + ":=" + part + "}", nil
					}
				}
			}
			return "{}", nil
		}
		elemType, _ := ElemType(base)
		var b strings.Builder
		b.WriteByte('{')
		for i, elem := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			part, err := encodeValueString(elem, elemType)
			if err != nil {
				return "", err
			}
			b.WriteString(part)
		}
		b.WriteByte('}')
		return b.String(), nil
	default:
		return fmt.Sprint(x), nil
	}
}

func decodeValueString(s string, typ Type) (any, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, 0, fmt.Errorf("empty decode")
	}
	base := BaseType(typ)
	switch base {
	case IntegerType:
		if i, err := strconv.Atoi(s); err == nil {
			return i, len(s), nil
		}
	case BooleanType:
		if s == "true" {
			return true, 4, nil
		}
		if s == "false" {
			return false, 5, nil
		}
	case FloatType:
		if s == "infinity" {
			return math.Inf(1), len(s), nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, len(s), nil
		}
	case CharstringType:
		if len(s) >= 2 && s[0] == '"' {
			end := strings.Index(s[1:], `"`)
			if end >= 0 {
				body := s[1 : 1+end]
				body = strings.ReplaceAll(body, `""`, `"`)
				return body, 1 + end + 1, nil
			}
		}
	}
	if et, ok := base.(EnumeratedType); ok {
		if terms, ok := et.Body.([]*Var); ok {
			for _, t := range terms {
				if ev, ok := t.Value.(EnumValue); ok && ev.Name == s {
					return ev, len(s), nil
				}
			}
		}
	}
	if s[0] == '{' {
		return decodeRecordString(s, typ)
	}
	return nil, 0, fmt.Errorf("decode: unsupported %q for %T", s, base)
}

func decodeRecordString(s string, typ Type) (any, int, error) {
	if !strings.HasSuffix(s, "}") {
		return nil, 0, fmt.Errorf("decode record: missing }")
	}
	base := BaseType(typ)
	fields := RecordFields(base)
	if fields == nil {
		return nil, 0, fmt.Errorf("decode record: not a record type")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		out := make([]any, len(fields))
		for i := range out {
			out[i] = Undefined{}
		}
		return out, len(s), nil
	}
	_, isSet := base.(SetType)
	if isSet {
		out := make([]any, len(fields))
		for i := range out {
			out[i] = Undefined{}
		}
		for _, part := range splitCommaAssign(inner) {
			name, val, ok := strings.Cut(part, ":=")
			if !ok {
				return nil, 0, fmt.Errorf("decode set: %q", part)
			}
			name = strings.TrimSpace(name)
			val = strings.TrimSpace(val)
			for i, f := range fields {
				if f.Name == name {
					v, _, err := decodeValueString(val, f.Type)
					if err != nil {
						return nil, 0, err
					}
					out[i] = v
					break
				}
			}
		}
		return out, len(s), nil
	}
	// record without field names: {7} or {1,2}
	parts := splitCommaAssign(inner)
	out := make([]any, len(fields))
	for i := range out {
		out[i] = Undefined{}
	}
	for i := 0; i < len(parts) && i < len(fields); i++ {
		v, _, err := decodeValueString(strings.TrimSpace(parts[i]), fields[i].Type)
		if err != nil {
			return nil, 0, err
		}
		out[i] = v
	}
	return out, len(s), nil
}

func splitCommaAssign(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(', '"':
			if s[i] == '"' {
				for j := i + 1; j < len(s); j++ {
					if s[j] == '"' && (j == 0 || s[j-1] != '\\') {
						i = j
						break
					}
				}
			} else {
				depth++
			}
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}
