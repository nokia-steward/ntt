package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Share gives v to one more holder — a variable, a parameter, a message
// queue — as a value of its own. TTCN-3 values have value semantics
// (ETSI ES 201 873-1 clause 6): assigning, initialising or passing a
// record, set, record of, set of, array or map gives the target its own
// value, so a later change to one is not seen through the other.
// References — to components, objects, timers, ports, functions,
// defaults — are shared, as the standard has them; charstrings and the
// other values never changed in place are given as they are.
//
// Rather than copied, v and every value it holds is marked shared, and
// is never changed in place from then on. A write through a variable
// replaces each shared value on its way by a copy of its own first
// (Unshare), which holds the same, shared, members; so a copy costs
// nothing until the copy, or the original, is changed, and then only the
// values on the way to the change. A value is marked after its members,
// so what is marked holds nothing that is not.
func Share(v Object) Object {
	switch x := v.(type) {
	case *Record:
		if x == nil || atomic.LoadUint32(&x.shared) != 0 {
			return v
		}
		for _, f := range x.Fields {
			Share(f)
		}
		markShared(v, &x.shared)
	case *List:
		if x == nil || atomic.LoadUint32(&x.shared) != 0 {
			return v
		}
		for _, e := range x.Elements {
			Share(e)
		}
		markShared(v, &x.shared)
	case *Map:
		if x == nil || atomic.LoadUint32(&x.shared) != 0 {
			return v
		}
		for _, bucket := range x.pairs {
			for _, p := range bucket {
				Share(p.Key)
				Share(p.Value)
			}
		}
		markShared(v, &x.shared)
	case *IfPresent:
		// A template's wrappers are not written through; what they
		// wrap may be another's value.
		if x != nil {
			Share(x.Inner)
		}
	case *LengthRestricted:
		if x != nil {
			Share(x.Inner)
		}
	}
	return v
}

// IsShared reports whether v is a record, list or map that is shared
// (Share), and so not to be changed in place.
func IsShared(v Object) bool {
	switch x := v.(type) {
	case *Record:
		return x != nil && atomic.LoadUint32(&x.shared) != 0
	case *List:
		return x != nil && atomic.LoadUint32(&x.shared) != 0
	case *Map:
		return x != nil && atomic.LoadUint32(&x.shared) != 0
	}
	return false
}

// Unshare returns v to change in place: v itself when it is not shared,
// else a copy of it of its own, holding v's members, which are shared.
// Unshare(Share(v)) is a copy of v to change whether v is shared or not:
// v and its members are not changed in place from then on.
func Unshare(v Object) Object {
	if !IsShared(v) {
		return v
	}
	switch x := v.(type) {
	case *Record:
		return x.clone()
	case *List:
		return x.clone()
	case *Map:
		out := NewMap()
		for h, bucket := range x.pairs {
			out.pairs[h] = append([]pair(nil), bucket...)
		}
		return out
	}
	return v
}

// clone returns a copy of r of its own, holding r's members.
func (r *Record) clone() *Record {
	out := &Record{Fields: make(map[string]Object, len(r.Fields)), Order: append([]string(nil), r.Order...)}
	for k, f := range r.Fields {
		out.Fields[k] = f
	}
	return out
}

// clone returns a copy of l of its own, holding l's members.
func (l *List) clone() *List {
	return &List{
		ListType:    l.ListType,
		Elements:    append([]Object(nil), l.Elements...),
		FieldNames:  append([]string(nil), l.FieldNames...),
		IndexOffset: l.IndexOffset,
	}
}

// sharedCheck, when NTT_SHARED_CHECK is set, remembers what each value
// was when it was shared, for CheckShared to tell one changed in place
// since: a write that did not Unshare. It is one record for the
// process: testcases run side by side (`ntt conformance --jobs`) check
// each other's values too, and the first to end forgets them.
var (
	sharedCheck = os.Getenv("NTT_SHARED_CHECK") != ""
	sharedSeen  sync.Map // Object -> string
)

func markShared(v Object, flag *uint32) {
	if sharedCheck {
		sharedSeen.LoadOrStore(v, fingerprint(v))
	}
	atomic.StoreUint32(flag, 1)
}

// CheckShared reports the shared values changed in place since they were
// shared, when NTT_SHARED_CHECK is set, and forgets them all.
func CheckShared() []string {
	if !sharedCheck {
		return nil
	}
	var bad []string
	sharedSeen.Range(func(k, was any) bool {
		if now := fingerprint(k.(Object)); now != was.(string) {
			bad = append(bad, fmt.Sprintf("%s, shared as %s", now, was))
		}
		sharedSeen.Delete(k)
		return true
	})
	sort.Strings(bad)
	return bad
}

// fingerprint writes out what a value holds: its members, for a record,
// list or map; the reference itself for one of what references are to,
// whose state is not the value's.
func fingerprint(v Object) string {
	var b strings.Builder
	var walk func(v Object)
	walk = func(v Object) {
		switch x := v.(type) {
		case *Record:
			if x == nil {
				b.WriteString("nil")
				return
			}
			fmt.Fprintf(&b, "{%v", x.Order)
			names := make([]string, 0, len(x.Fields))
			for n := range x.Fields {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				b.WriteString(" " + n + ":=")
				walk(x.Fields[n])
			}
			b.WriteString("}")
		case *List:
			if x == nil {
				b.WriteString("nil")
				return
			}
			fmt.Fprintf(&b, "[%s %v %d", x.ListType, x.FieldNames, x.IndexOffset)
			for _, e := range x.Elements {
				b.WriteString(" ")
				walk(e)
			}
			b.WriteString("]")
		case *Map:
			if x == nil {
				b.WriteString("nil")
				return
			}
			var kv []string
			for _, bucket := range x.pairs {
				for _, p := range bucket {
					kv = append(kv, fingerprint(p.Key)+"->"+fingerprint(p.Value))
				}
			}
			sort.Strings(kv)
			fmt.Fprintf(&b, "map%v", kv)
		case nil:
			b.WriteString("<nil>")
		case *IfPresent:
			b.WriteString("ifpresent(")
			walk(x.Inner)
			b.WriteString(")")
		case *LengthRestricted:
			fmt.Fprintf(&b, "length(%d..%d, ", x.Min, x.Max)
			walk(x.Inner)
			b.WriteString(")")
		default:
			switch v.Type() {
			case COMPONENT_REF, FUNCTION, TIMER_OBJ, CLASS_INSTANCE, CLASS_DESC, PORT_REF,
				TYPE_DESC, LAZY_THUNK, BUILTIN_OBJ, ENUM_TYPE:
				fmt.Fprintf(&b, "%T@%p", v, v)
			default:
				b.WriteString(v.Inspect())
			}
		}
	}
	walk(v)
	return b.String()
}
