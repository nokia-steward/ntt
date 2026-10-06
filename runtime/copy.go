package runtime

// CopyValue returns a copy of v that a variable can hold on its own.
// TTCN-3 values have value semantics (ETSI ES 201 873-1 clause 6):
// assigning, initialising or passing a record, set, record of, set of,
// array or map gives the target its own value, so a later change to one
// is not seen through the other. References — to components, objects,
// timers, ports, functions, defaults — are shared, as the standard has
// them. Values never changed in place — charstrings among them, whose
// element assignment makes a new one — are returned as they are.
func CopyValue(v Object) Object {
	switch x := v.(type) {
	case *Record:
		if x == nil {
			return x
		}
		out := &Record{Fields: make(map[string]Object, len(x.Fields)), Order: append([]string(nil), x.Order...)}
		for k, f := range x.Fields {
			out.Fields[k] = CopyValue(f)
		}
		return out
	case *List:
		if x == nil {
			return x
		}
		cp := *x
		cp.Elements = make([]Object, len(x.Elements))
		for i, e := range x.Elements {
			cp.Elements[i] = CopyValue(e)
		}
		return &cp
	case *String:
		// A charstring is never changed in place (String.WithRuneAt):
		// the copy can be the value itself.
		return x
	case *Map:
		if x == nil {
			return x
		}
		out := NewMap()
		for h, bucket := range x.pairs {
			nb := make([]pair, len(bucket))
			for i, p := range bucket {
				nb[i] = pair{Key: p.Key, Value: CopyValue(p.Value)}
			}
			out.pairs[h] = nb
		}
		return out
	}
	return v
}
