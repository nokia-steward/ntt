package runtime

import "fmt"

// SizeOf returns the number of present fields in a record or set value.
func SizeOf(v any) (int, error) {
	slice, ok := v.([]any)
	if !ok {
		return 0, fmt.Errorf("sizeof: expected record or set, got %T", v)
	}
	n := 0
	for _, elem := range slice {
		if _, undef := elem.(Undefined); undef {
			continue
		}
		if elem == OmitVal || elem == AnyOrNone {
			continue
		}
		n++
	}
	return n, nil
}

// AllFrom expands a value list template body into its elements.
func AllFrom(v any, inPermutation bool) (any, error) {
	switch x := v.(type) {
	case []any:
		if inPermutation {
			for _, e := range x {
				if e == AnyOrNone {
					return nil, fmt.Errorf("allfrom: * inside permutation")
				}
			}
		}
		out := append([]any(nil), x...)
		return out, nil
	case ValueList:
		return AllFrom([]any(x), inPermutation)
	default:
		return nil, fmt.Errorf("allfrom: unsupported %T", v)
	}
}

// Int2Enum maps integer n to an EnumValue of typ, or an error if none exists.
func Int2Enum(n int, typ Type) (EnumValue, error) {
	base := BaseType(typ)
	et, ok := base.(EnumeratedType)
	if !ok {
		return EnumValue{}, fmt.Errorf("int2enum: not an enumerated type")
	}
	terms, ok := et.Body.([]*Var)
	if !ok {
		return EnumValue{}, fmt.Errorf("int2enum: invalid enum type")
	}
	for _, t := range terms {
		if ev, ok := t.Value.(EnumValue); ok && ev.Val == n {
			return ev, nil
		}
	}
	return EnumValue{}, fmt.Errorf("int2enum: no enumerated value for %d", n)
}
