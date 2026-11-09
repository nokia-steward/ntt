package runtime

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Undefined represents an undefined TTCN-3 value.
// It is returned when accessing elements outside the bounds of a container.
type Undefined struct{}

// String returns the string representation of undefined.
func (u Undefined) String() string {
	return "undefined"
}

// TemplateLiteral represents single-token template literals (omit, ?, and *).
type TemplateLiteral string

const (
	OmitVal   TemplateLiteral = "omit"
	Any       TemplateLiteral = "?"
	AnyOrNone TemplateLiteral = "*"
)

func (t TemplateLiteral) String() string { return string(t) }

// Infinity represents +infinity or -infinity in range bounds.
type Infinity struct{ Neg bool }

var (
	PosInfinity = Infinity{Neg: false}
	NegInfinity = Infinity{Neg: true}
)

func (inf Infinity) String() string {
	if inf.Neg {
		return "-infinity"
	}
	return "infinity"
}

// Range represents a range bound pair (Lo .. Hi).
type Range struct{ Lo, Hi any }

func (r Range) String() string { return fmt.Sprintf("(%v..%v)", r.Lo, r.Hi) }

// Pattern represents a string or binary pattern template literal.
type Pattern string

func (p Pattern) String() string { return Quote(string(p)) }

// Quote formats s as k3r logs a charstring: in double quotes, with a
// double quote doubled.
func Quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Match reports whether s matches p. ? and * stand for one and any number
// of characters; for binary strings s is the digit string, and k3r treats
// one nibble as a character there too. ref resolves {name} to the value it
// denotes.
func (p Pattern) Match(s string, ref func(name string) any) (bool, error) {
	re, err := p.Compile(ref)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// Compile translates p to a regular expression anchored at both ends. Its
// groups are the groups of p.
func (p Pattern) Compile(ref func(name string) any) (*regexp.Regexp, error) {
	expr, err := p.regexp(ref)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(`(?s)^(?:` + expr + `)$`)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %s: %w", string(p), err)
	}
	return re, nil
}

var patternClasses = map[byte]string{
	'd': `0-9`, 'w': `0-9a-zA-Z`, 't': `\t`, 'n': `\n-\r`, 'r': `\r`, 's': `\t-\r `,
}

// regexp translates p to Go regular expression syntax.
func (p Pattern) regexp(ref func(name string) any) (string, error) {
	var b strings.Builder
	s := string(p)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '?':
			b.WriteString(".")
		case '*':
			b.WriteString(".*")
		case '(', ')', '|', '+':
			b.WriteByte(c)
		case '"':
			b.WriteByte('"')
			if i+1 < len(s) && s[i+1] == '"' {
				i++
			}
		case '\\':
			if i+1 == len(s) {
				return "", fmt.Errorf("pattern %q ends in \\", s)
			}
			i++
			if cls, ok := patternClasses[s[i]]; ok {
				b.WriteString("[" + cls + "]")
			} else {
				b.WriteString(regexp.QuoteMeta(s[i : i+1]))
			}
		case '[':
			j := i + 1
			b.WriteByte('[')
			if j < len(s) && s[j] == '^' {
				b.WriteByte('^')
				j++
			}
			for ; j < len(s) && s[j] != ']'; j++ {
				switch {
				case s[j] == '\\' && j+1 < len(s):
					j++
					if cls, ok := patternClasses[s[j]]; ok {
						b.WriteString(cls)
					} else {
						b.WriteString(`\` + s[j:j+1])
					}
				case s[j] == '[' || s[j] == '^':
					b.WriteString(`\` + s[j:j+1])
				default:
					b.WriteByte(s[j])
				}
			}
			if j == len(s) {
				return "", fmt.Errorf("pattern %q: unterminated [", s)
			}
			b.WriteByte(']')
			i = j
		case '#':
			j := i + 1
			if j < len(s) && s[j] == '(' {
				k := strings.IndexByte(s[j:], ')')
				if k < 0 {
					return "", fmt.Errorf("pattern %q: unterminated #(", s)
				}
				lo, hi, comma := strings.Cut(s[j+1:j+k], ",")
				switch {
				case !comma:
					b.WriteString("{" + lo + "}")
				case lo == "" && hi == "":
					b.WriteString("*")
				default:
					b.WriteString("{" + cmp.Or(lo, "0") + "," + hi + "}")
				}
				i = j + k
			} else if j < len(s) {
				b.WriteString("{" + s[j:j+1] + "}")
				i = j
			}
		case '{':
			k := strings.IndexByte(s[i:], '}')
			if k < 0 {
				return "", fmt.Errorf("pattern %q: unterminated {", s)
			}
			name := s[i+1 : i+k]
			var sub Pattern
			switch v := ref(name).(type) {
			case Pattern:
				sub = v
			case string:
				sub = Pattern(v)
			default:
				return "", fmt.Errorf("pattern %q: cannot resolve {%s}", s, name)
			}
			re, err := sub.regexp(ref)
			if err != nil {
				return "", err
			}
			b.WriteString("(?:" + re + ")")
			i += k
		default:
			b.WriteString(regexp.QuoteMeta(s[i : i+1]))
		}
	}
	return b.String(), nil
}

// ValueList represents a template value list (a, b, c).
type ValueList []any

// TemplateList represents a combinator template (complement, subset, superset, permutation).
type TemplateList struct {
	Kind  string
	Items []any
}

// ModifiedTemplate wraps a template value with length(...) and/or ifpresent modifiers.
type ModifiedTemplate struct {
	Val       any
	Length    any
	IfPresent bool
}

// EnumValue represents an enumeration term value.
type EnumValue struct {
	Name string
	Val  int
}

func (e EnumValue) String() string { return e.Name }

// ApplyIfPresent attaches ifpresent to obj, normalizing ? and * wildcards to *.
func ApplyIfPresent(obj any) any {
	if obj == Any || obj == AnyOrNone {
		return AnyOrNone
	}
	if mt, ok := obj.(ModifiedTemplate); ok {
		if mt.Val == Any || mt.Val == AnyOrNone {
			return ModifiedTemplate{Val: AnyOrNone, Length: mt.Length}
		}
		mt.IfPresent = true
		return mt
	}
	return ModifiedTemplate{Val: obj, IfPresent: true}
}

// ApplyLength attaches a length restriction to obj.
func ApplyLength(obj, length any) ModifiedTemplate {
	if mt, ok := obj.(ModifiedTemplate); ok {
		mt.Length = length
		return mt
	}
	return ModifiedTemplate{Val: obj, Length: length}
}

func isTemplateOnly(v any) bool {
	switch v.(type) {
	case TemplateLiteral, Infinity, Range, Pattern, ValueList, TemplateList, ModifiedTemplate:
		return true
	}
	return false
}

// RecordFields returns the field descriptors if t is a RecordType or SetType.
func RecordFields(t Type) []*Var {
	switch b := BaseType(t).(type) {
	case RecordType:
		return b.Fields
	case *RecordType:
		return b.Fields
	case SetType:
		return b.Fields
	case *SetType:
		return b.Fields
	}
	return nil
}

// UnionFields returns the field descriptors if t is a UnionType.
func UnionFields(t Type) []*Var {
	switch b := BaseType(t).(type) {
	case UnionType:
		return b.Fields
	case *UnionType:
		return b.Fields
	}
	return nil
}

// ElemType returns the element type if t is a RecordOfType, SetOfType, or ArrayType.
func ElemType(t Type) (Type, bool) {
	switch b := BaseType(t).(type) {
	case RecordOfType:
		return b.ElementType, true
	case *RecordOfType:
		return b.ElementType, true
	case SetOfType:
		return b.ElementType, true
	case *SetOfType:
		return b.ElementType, true
	case ArrayType:
		return b.ElementType, true
	case *ArrayType:
		return b.ElementType, true
	}
	return nil, false
}

// MapKeyType returns the key type if t is a MapType.
func MapKeyType(t Type) (Type, bool) {
	switch b := BaseType(t).(type) {
	case MapType:
		return b.KeyType, true
	case *MapType:
		return b.KeyType, true
	}
	return nil, false
}

// MapValueType returns the value type if t is a MapType.
func MapValueType(t Type) (Type, bool) {
	switch b := BaseType(t).(type) {
	case MapType:
		return b.ValueType, true
	case *MapType:
		return b.ValueType, true
	}
	return nil, false
}

// MapKeys returns every key of m sorted for its key type. A nil map is empty.
func MapKeys(m map[any]any, keyType Type) []any {
	if len(m) == 0 {
		return []any{}
	}
	keys := make([]any, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortValues(keys, keyType)
	return keys
}

// MapValues returns every value of m sorted for its value type. A nil map is empty.
func MapValues(m map[any]any, valueType Type) []any {
	if len(m) == 0 {
		return []any{}
	}
	vals := make([]any, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	sortValues(vals, valueType)
	return vals
}

func sortValues(vals []any, typ Type) {
	typ = BaseType(typ)
	slices.SortFunc(vals, func(a, b any) int {
		switch typ {
		case IntegerType:
			return cmp.Compare(a.(int), b.(int))
		case FloatType:
			return cmp.Compare(a.(float64), b.(float64))
		case BooleanType:
			ai, bi := 0, 0
			if a.(bool) {
				ai = 1
			}
			if b.(bool) {
				bi = 1
			}
			return cmp.Compare(ai, bi)
		case CharstringType, UniversalCharstringType:
			return cmp.Compare(a.(string), b.(string))
		default:
			return cmp.Compare(fmt.Sprint(a), fmt.Sprint(b))
		}
	})
}

// BaseType unwraps NamedType, Subtype, and TemplateType wrappers.
func BaseType(t Type) Type {
	for {
		t = unwrapType(t)
		if tt, ok := t.(TemplateType); ok {
			t = tt.BaseType
			continue
		}
		return t
	}
}

func unwrapType(t Type) Type {
	for {
		switch u := t.(type) {
		case *NamedType:
			if u.Type == nil {
				return t
			}
			t = u.Type
		case Subtype:
			t = u.BaseType
		case *Subtype:
			t = u.BaseType
		case *TemplateType:
			t = *u
		default:
			return t
		}
	}
}

// IsCompatible checks if a value is compatible with the given TTCN-3 type.
// Returns an error if the value doesn't match the expected type.
func IsCompatible(value any, typ Type) error {
	if typ == nil {
		// No type constraint, anything is allowed
		return nil
	}

	// Handle Undefined values - they're compatible with any type
	if _, ok := value.(Undefined); ok {
		return nil
	}
	if _, ok := value.(Null); ok {
		if named, ok := typ.(*NamedType); ok && (named.Name == "address" || strings.HasSuffix(named.Name, ".address")) {
			return nil
		}
	}
	if named, ok := typ.(*NamedType); ok && (named.Name == "anytype" || strings.HasSuffix(named.Name, ".anytype")) {
		return nil
	}

	typ = unwrapType(typ)
	if tt, ok := typ.(TemplateType); ok {
		return isCompatibleTemplate(value, tt)
	}

	if named, ok := typ.(*NamedType); ok {
		switch named.Name {
		case "integer":
			if _, ok := value.(int); !ok {
				return fmt.Errorf("type mismatch: expected integer, got %T", value)
			}
			return nil
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("type mismatch: expected boolean, got %T", value)
			}
			return nil
		case "float":
			if _, ok := value.(float64); !ok {
				return fmt.Errorf("type mismatch: expected float, got %T", value)
			}
			return nil
		case "charstring", "universal charstring":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("type mismatch: expected %s, got %T", named.Name, value)
			}
			return nil
		default:
			if isTemplateOnly(value) {
				return fmt.Errorf("type mismatch: expected %s, got %T", named.Name, value)
			}
			return nil
		}
	}

	if _, ok := ElemType(typ); ok {
		slice, ok := value.([]any)
		if !ok {
			return fmt.Errorf("type mismatch: expected list, got %T", value)
		}
		for _, elem := range slice {
			if isTemplateOnly(elem) {
				return fmt.Errorf("type mismatch: template value %v not allowed in value list", elem)
			}
		}
		return nil
	}

	if fields := RecordFields(typ); fields != nil {
		return isCompatibleRecord(value, fields)
	}

	if UnionFields(typ) != nil {
		slice, ok := value.([]any)
		if !ok {
			return fmt.Errorf("type mismatch: expected union, got %T", value)
		}
		for _, elem := range slice {
			if isTemplateOnly(elem) {
				return fmt.Errorf("type mismatch: template value %v not allowed in union", elem)
			}
		}
		return nil
	}

	switch typ.(type) {
	case MapType, *MapType:
		if _, ok := value.(map[any]any); !ok {
			return fmt.Errorf("type mismatch: expected map, got %T", value)
		}
	default:
		if isTemplateOnly(value) {
			return fmt.Errorf("type mismatch: template value %v not allowed in value", value)
		}
	}

	return nil
}

func isCompatibleTemplate(value any, tt TemplateType) error {
	if _, ok := value.(Undefined); ok {
		return nil
	}
	if tt.Flags&Omit != 0 && value != OmitVal && isTemplateOnly(value) {
		return fmt.Errorf("type mismatch: %v not allowed in template(omit)", value)
	}
	if tt.Flags&Present != 0 && (value == OmitVal || value == AnyOrNone) {
		return fmt.Errorf("type mismatch: %v not allowed in template(present)", value)
	}
	switch v := value.(type) {
	case TemplateLiteral, Range, Pattern, ValueList, TemplateList:
		return nil
	case ModifiedTemplate:
		if tt.Flags&Present != 0 && v.IfPresent {
			return fmt.Errorf("type mismatch: ifpresent not allowed in template(present)")
		}
		return isCompatibleTemplate(v.Val, tt)
	case []any:
		return isCompatibleTemplateSlice(v, tt.BaseType, tt.Flags&Omit)
	default:
		return IsCompatible(value, tt.BaseType)
	}
}

func isCompatibleTemplateSlice(slice []any, base Type, elemFlags Flags) error {
	base = BaseType(base)
	if base == nil {
		return nil
	}
	if fields := RecordFields(base); fields != nil {
		if len(fields) > 0 && len(slice) > len(fields) {
			return fmt.Errorf("type mismatch: record value has %d fields, type has %d", len(slice), len(fields))
		}
		for i, elem := range slice {
			if i < len(fields) && fields[i].Type != nil {
				if err := isCompatibleTemplate(elem, TemplateType{Flags: elemFlags, BaseType: fields[i].Type}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if elemType, ok := ElemType(base); ok {
		for _, elem := range slice {
			if err := isCompatibleTemplate(elem, TemplateType{Flags: elemFlags, BaseType: elemType}); err != nil {
				return err
			}
		}
		return nil
	}
	switch base.(type) {
	case UnionType, *UnionType:
		return nil
	default:
		return IsCompatible(slice, base)
	}
}

func isCompatibleRecord(value any, fields []*Var) error {
	var size int
	switch v := value.(type) {
	case []any:
		size = len(v)
		for i, elem := range v {
			if elem == nil || i >= len(fields) {
				continue
			}
			if _, ok := elem.(Undefined); ok {
				continue
			}
			if elem == OmitVal && fields[i].Flags&Optional != 0 {
				continue
			}
			if isTemplateOnly(elem) {
				return fmt.Errorf("type mismatch: template value %v not allowed in record field %q", elem, fields[i].Name)
			}
		}
	case map[string]any:
		size = len(v)
	default:
		return fmt.Errorf("type mismatch: expected record, got %T", value)
	}
	if n := len(fields); n > 0 && size > n {
		return fmt.Errorf("type mismatch: record value has %d fields, type has %d", size, n)
	}
	return nil
}
