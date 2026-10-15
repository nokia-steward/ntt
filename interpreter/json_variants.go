package interpreter

// json_variants.go reads the JSON encoding instructions (ETSI ES 201 873-11
// Annex B) a type and its fields carry as variant attributes, for the JSON
// codec (json_codec.go). An instruction it does not know, or one qualified
// by more than a field of the type, leaves the type to the codec's
// fallback, as before there was a JSON codec.

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// jsonVariants are the instructions a type, or a field of a type, carries.
type jsonVariants struct {
	nameAs     string // name as 'freetext'
	hasNameAs  bool
	nameCase   string // name as capitalized | uncapitalized | lowercased | uppercased
	allCase    string // name all as ..., for every field of the type
	omitAsNull bool
	asValue    bool
	useMinus   bool
	fraction   int // fractionDigits
	hasFrac    bool
	escape     string // escape as short | usi | transparent
	def        string // default (...)
	hasDefault bool
}

// add parses one instruction into v, and reports whether it is one the
// codec knows. An instruction qualified by another codec is none of its
// business, and known.
func (v *jsonVariants) add(instr string) bool {
	s := strings.TrimSpace(instr)
	if codec, rest, ok := codecQualified(s); ok {
		if !strings.EqualFold(codec, "JSON") && !strings.EqualFold(codec, "JSON RFC7159") {
			return true
		}
		s = strings.TrimSpace(rest)
	}
	if body, ok := jsonDefaultVariantBody(s); ok {
		v.def, v.hasDefault = body, true
		return true
	}
	w := strings.Fields(s)
	switch {
	case len(w) >= 3 && w[0] == "name" && w[1] == "as" && strings.HasPrefix(w[2], "'"):
		i, j := strings.IndexByte(s, '\''), strings.LastIndexByte(s, '\'')
		if j <= i || strings.TrimSpace(s[j+1:]) != "" {
			return false
		}
		v.nameAs, v.hasNameAs = s[i+1:j], true
	case len(w) == 3 && w[0] == "name" && w[1] == "as" && isNameCase(w[2]):
		v.nameCase = w[2]
	case len(w) == 4 && w[0] == "name" && w[1] == "all" && w[2] == "as" && isNameCase(w[3]):
		v.allCase = w[3]
	case len(w) == 3 && w[0] == "omit" && w[1] == "as" && w[2] == "null":
		v.omitAsNull = true
	case len(w) == 1 && w[0] == "asValue":
		v.asValue = true
	case len(w) == 1 && w[0] == "noType":
		// The codec writes no type-name wrapper, and reads a value with
		// or without one.
	case len(w) == 1 && w[0] == "useMinus":
		v.useMinus = true
	case len(w) == 2 && w[0] == "fractionDigits":
		n, err := strconv.Atoi(w[1])
		if err != nil || n < 0 {
			return false
		}
		v.fraction, v.hasFrac = n, true
	case len(w) == 3 && w[0] == "escape" && w[1] == "as" && (w[2] == "short" || w[2] == "usi" || w[2] == "transparent"):
		v.escape = w[2]
	case len(w) >= 1 && strings.HasPrefix(w[0], "JSON:"):
		// The JSON type identifications of the JSON module's types
		// (B.3.2) that map as their TTCN-3 type does; a literal's, an
		// object's and a member's (6.4.4, 6.4.5) do not.
		switch strings.TrimSpace(strings.TrimPrefix(strings.Join(w, ""), "JSON:")) {
		case "array", "integer", "number", "string":
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// fill sets what v leaves unset from o: an alias's instructions over
// those of the type it names.
func (v *jsonVariants) fill(o jsonVariants) {
	if !v.hasNameAs && o.hasNameAs {
		v.nameAs, v.hasNameAs = o.nameAs, true
	}
	if v.nameCase == "" {
		v.nameCase = o.nameCase
	}
	if v.allCase == "" {
		v.allCase = o.allCase
	}
	v.omitAsNull = v.omitAsNull || o.omitAsNull
	v.asValue = v.asValue || o.asValue
	v.useMinus = v.useMinus || o.useMinus
	if !v.hasFrac && o.hasFrac {
		v.fraction, v.hasFrac = o.fraction, true
	}
	if v.escape == "" {
		v.escape = o.escape
	}
	if !v.hasDefault && o.hasDefault {
		v.def, v.hasDefault = o.def, true
	}
}

// over returns v with what o sets in its place: a field's instructions
// over those of the field's type.
func (v jsonVariants) over(o *jsonVariants) jsonVariants {
	if o == nil {
		return v
	}
	if o.asValue {
		v.asValue = true
	}
	if o.useMinus {
		v.useMinus = true
	}
	if o.hasFrac {
		v.fraction, v.hasFrac = o.fraction, true
	}
	if o.escape != "" {
		v.escape = o.escape
	}
	return v
}

// codecQualified splits `Codec.instruction`, the form a variant of one of
// several encodings takes (ETSI ES 201 873-1 27.5).
func codecQualified(s string) (codec, rest string, ok bool) {
	i := strings.IndexByte(s, '.')
	if i <= 0 || strings.ContainsAny(s[:i], "'(\"") {
		return "", "", false
	}
	codec = strings.TrimSpace(s[:i])
	for _, r := range codec {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != ' ' {
			return "", "", false
		}
	}
	return codec, s[i+1:], true
}

func isNameCase(s string) bool {
	switch s {
	case "capitalized", "uncapitalized", "lowercased", "uppercased":
		return true
	}
	return false
}

// changeCase applies a `name as` case change to name.
func changeCase(name, c string) string {
	switch c {
	case "capitalized", "uncapitalized":
		r, n := utf8.DecodeRuneInString(name)
		if n == 0 {
			return name
		}
		if c == "capitalized" {
			r = unicode.ToUpper(r)
		} else {
			r = unicode.ToLower(r)
		}
		return string(r) + name[n:]
	case "lowercased":
		return strings.ToLower(name)
	case "uppercased":
		return strings.ToUpper(name)
	}
	return name
}

// jsonVariantsOf reads the instructions td carries: its own, and, when
// fields names the fields of a struct type, each field's — those of its own
// with; one an enclosing scope qualifies is another definition's. ok is
// false when one is not known, or is qualified by something other than a
// field, or by a name two fields share but for their case (the attributes
// keep a qualifier in lower case).
func jsonVariantsOf(td *runtime.TypeDesc, fields []*syntax.Field) (own jsonVariants, byField map[string]*jsonVariants, ok bool) {
	names := map[string]string{}
	twice := map[string]bool{}
	for _, f := range fields {
		if f != nil && f.Name != nil {
			low := strings.ToLower(f.Name.String())
			if _, seen := names[low]; seen {
				twice[low] = true
			}
			names[low] = f.Name.String()
		}
	}
	self := strings.ToLower(td.Name)
	for k, vals := range td.Attrs {
		var into *jsonVariants
		q := strings.TrimSuffix(k, ".variant")
		if _, mine := td.OwnAttrs[k]; !mine && k != "variant" {
			// An enclosing scope's qualifies a definition by its name:
			// this type (`variant (T) "..."`), a field of it (`variant
			// (T.f) "..."`), or another definition, whose it is.
			named := self != "" && (q == self || strings.HasPrefix(q, self+"."))
			if named && typeNameInCase(td) {
				// The qualifier is kept in lower case: it may name
				// another type, its name differing in case alone.
				return own, nil, false
			}
			switch {
			case named && q == self:
				k = "variant"
			case named:
				q = strings.TrimPrefix(q, self+".")
			default:
				continue
			}
		}
		switch {
		case k == "variant":
			into = &own
		case strings.HasSuffix(k, ".variant"):
			field, known := names[q]
			if !known || twice[q] {
				if len(vals) > 0 {
					return own, nil, false
				}
				continue
			}
			if byField == nil {
				byField = map[string]*jsonVariants{}
			}
			if byField[field] == nil {
				byField[field] = &jsonVariants{}
			}
			into = byField[field]
		default:
			continue
		}
		for _, instr := range vals {
			if !into.add(instr) {
				return own, nil, false
			}
		}
	}
	return own, byField, true
}

// jsonName is the name of field f's member in its object or union.
func (s jsonShape) jsonName(f *syntax.Field) string {
	name := f.Name.String()
	if fv := s.fv[name]; fv != nil {
		if fv.hasNameAs {
			return fv.nameAs
		}
		if fv.nameCase != "" {
			return changeCase(name, fv.nameCase)
		}
	}
	if s.tv.allCase != "" {
		return changeCase(name, s.tv.allCase)
	}
	return name
}

// fieldOfMember returns the field of the struct whose member is named
// member.
func (s jsonShape) fieldOfMember(member string) (*syntax.Field, jsonType) {
	for _, f := range s.fields {
		if f != nil && f.Name != nil && s.jsonName(f) == member {
			return f, s.fieldType(f)
		}
	}
	return nil, jsonType{}
}

// formatJSONFloat writes f with at most digits fraction digits, and an
// exponent where more are needed (B.3.5).
func formatJSONFloat(f float64, digits int) string {
	plain := strconv.FormatFloat(f, 'f', -1, 64)
	frac := 0
	if i := strings.IndexByte(plain, '.'); i >= 0 {
		frac = len(plain) - i - 1
	}
	if digits > 0 && frac <= digits {
		if frac == 0 {
			plain += ".0"
		}
		return plain
	}
	// f as a whole number of its significant digits times a power of ten.
	sign := ""
	if math.Signbit(f) {
		sign, f = "-", -f
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddddde±x
	mant, exps, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(exps)
	intDigits, fracDigits, _ := strings.Cut(mant, ".")
	all := intDigits + fracDigits
	exp -= len(fracDigits)
	if strings.Trim(all, "0") == "" {
		all, exp = "0", 0
	}
	if digits == 0 {
		return sign + all + "E" + strconv.Itoa(exp)
	}
	// digits fraction digits: shift the point that many places left.
	for len(all) <= digits {
		all = "0" + all
	}
	return sign + all[:len(all)-digits] + "." + all[len(all)-digits:] + "E" + strconv.Itoa(exp+digits)
}

// writeJSONStringEscaped writes s as a JSON string, its characters escaped
// as escape says (B.3.7): "" as the codec does by default, short escapes
// where there are, "usi" as \u escapes, "transparent" only the control
// characters.
func writeJSONStringEscaped(b *bytes.Buffer, s, escape string) {
	b.WriteByte('"')
	for _, r := range s {
		switch escape {
		case "usi":
			if r == '"' || r == '\\' || r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)>>4, 16) + strconv.FormatInt(int64(r)&0xF, 16)))
				continue
			}
		case "transparent":
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)>>4, 16) + strconv.FormatInt(int64(r)&0xF, 16)))
				continue
			}
		default: // short
			switch r {
			case '"':
				b.WriteString(`\"`)
				continue
			case '\\':
				b.WriteString(`\\`)
				continue
			case '/':
				b.WriteString(`\/`)
				continue
			case '\b':
				b.WriteString(`\b`)
				continue
			case '\f':
				b.WriteString(`\f`)
				continue
			case '\n':
				b.WriteString(`\n`)
				continue
			case '\r':
				b.WriteString(`\r`)
				continue
			case '\t':
				b.WriteString(`\t`)
				continue
			}
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)>>4, 16) + strconv.FormatInt(int64(r)&0xF, 16)))
				continue
			}
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
}

// jsonDefault is the value a `default (...)` instruction gives a field of
// type t that the JSON value leaves out: the value its text denotes, or,
// for a character or binary string, the text itself (B.3.9).
func jsonDefault(text string, t jsonType, env runtime.Scope) (runtime.Object, error) {
	shape, known := jsonTypeOf(t, env)
	// The value is the declaring module's, whoever decodes.
	scope := env
	if t.scope != nil {
		scope = t.scope
	}
	v := evalJSONDefaultExpr(text, scope)
	if runtime.IsError(v) {
		v = nil
	}
	if known {
		switch shape.kind {
		case "charstring", "universal charstring", "universalcharstring":
			if _, ok := v.(*runtime.String); !ok {
				return runtime.NewCharstring(text), nil
			}
		case "octetstring", "bitstring", "hexstring":
			if _, ok := v.(*runtime.Binarystring); !ok {
				return binaryFromDigits(text, shape.kind)
			}
		case "record", "set":
			// A record given positionally names its fields.
			if l, ok := v.(*runtime.List); ok && len(l.FieldNames) == 0 && len(l.Elements) <= len(shape.fields) {
				rec := runtime.NewRecord()
				for i, f := range shape.fields {
					name := f.Name.String()
					rec.Order = append(rec.Order, name)
					if i < len(l.Elements) {
						rec.Fields[name] = l.Elements[i]
					}
				}
				return rec, nil
			}
		}
	}
	if v == nil || v == runtime.Undefined {
		return nil, errJSONDefault(text)
	}
	return v, nil
}

type errJSONDefault string

func (e errJSONDefault) Error() string { return "JSON: no value: default (" + string(e) + ")" }

// typeNameInCase reports whether td's module declares another type whose
// name differs from td's in case alone.
func typeNameInCase(td *runtime.TypeDesc) bool {
	home, ok := td.Home.(*runtime.Env)
	if !ok {
		return false
	}
	other := false
	home.Each(func(name string, v runtime.Object) {
		if name == td.Name || !strings.EqualFold(name, td.Name) {
			return
		}
		switch v.(type) {
		case *runtime.TypeDesc, *runtime.EnumType, *runtime.ClassDesc:
			other = true
		}
	})
	return other
}
