// enum_v5_rules.go checks enumerated types and their values in a module
// written in the next major release of the core language (syntax.V5),
// which reads them differently from V4 (the draft's clauses 5.4 and
// 6.2.4, "Enumerations"):
//
//   - an enumerator's integers are integer expressions and ranges, whose
//     upper bound may be `infinity`; one declared without gets the lowest
//     integer from 0 on that no other has, and there has to be one;
//   - an enumerated value has exactly one integer: `Label(n)` names one of
//     the label's, and a label of several integers has to name one;
//   - a label is visible wherever its type is and nothing hides it: no
//     definition of the module, nor a local one of a behaviour that uses
//     the type, has its name;
//   - a label written alone takes the type it is the label of, unless it
//     is the label of several types.
//
// V4's rules for the same constructs (enum_rules.go, enum_use_rules.go)
// are not applied to such a module.
package semantic

import (
	"fmt"
	"math"
	"sort"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// v5Enumerator is an enumerator of a type, with the integers the analyzer
// can tell it has (known).
type v5Enumerator struct {
	name   string
	node   syntax.Node
	ranges []intRange
	known  bool
}

type intRange struct{ lo, hi int64 }

func (r intRange) contains(n int64) bool { return n >= r.lo && n <= r.hi }

// count is how many integers the enumerator has, 2 for "more than one".
func (e v5Enumerator) count() int {
	switch {
	case len(e.ranges) > 1:
		return 2
	case len(e.ranges) == 1 && e.ranges[0].lo != e.ranges[0].hi:
		return 2
	}
	return len(e.ranges)
}

func (a *Analyzer) checkEnumV5Rules(mod *syntax.Module) []Diagnostic {
	if mod.Edition() != syntax.V5 {
		return nil
	}
	types := map[string][]v5Enumerator{}
	labelTypes := map[string][]string{}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ed, ok := d.Def.(*syntax.EnumTypeDecl)
		if !ok || ed.Name == nil {
			continue
		}
		typeName := ed.Name.String()
		enums, missing := v5Enumerators(ed)
		types[typeName] = enums
		for _, e := range enums {
			labelTypes[e.name] = append(labelTypes[e.name], typeName)
		}
		if missing != nil {
			diags = append(diags, Diagnostic{
				Code:     "enum-no-integer-left",
				Severity: SeverityError,
				Message:  fmt.Sprintf("no integer is left for enumerator %q of %s: the others have them all", syntax.Name(missing), typeName),
				Node:     missing,
				Span:     syntax.SpanOf(missing),
			})
		}
	}
	if len(types) == 0 {
		return nil
	}
	isLabel := func(x syntax.Expr) (string, bool) {
		id, ok := x.(*syntax.Ident)
		if !ok || id.Tok == nil {
			return "", false
		}
		_, ok = labelTypes[id.String()]
		return id.String(), ok
	}

	diags = append(diags, v5HiddenLabels(mod, types)...)

	// What names a value besides a label: a local, a parameter, a
	// template's formal, a member. A name that does is no label where it
	// is written alone.
	values := map[string]bool{}
	// The variables declared of one of the module's enumerated types, by
	// name; one declared of several, or of another type too, is not.
	varTypes := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.EnumTypeDecl:
			return false
		case *syntax.FormalPar:
			if x.Name != nil {
				values[x.Name.String()] = true
			}
		case *syntax.ValueDecl:
			t := identName(x.Type)
			if _, ok := types[t]; !ok {
				t = ""
			}
			for _, dec := range x.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				name := dec.Name.String()
				values[name] = true
				if prev, seen := varTypes[name]; seen && prev != t {
					t = ""
				}
				varTypes[name] = t
			}
		}
		return true
	})
	// A value of enumerated type typeName, written where one is.
	checkValue := func(x syntax.Expr, typeName string) {
		if typeName == "" {
			return
		}
		ofType := func(name string) (v5Enumerator, bool) {
			if values[name] {
				return v5Enumerator{}, false
			}
			for _, e := range types[typeName] {
				if e.name == name {
					return e, e.known
				}
			}
			return v5Enumerator{}, false
		}
		switch v := x.(type) {
		case *syntax.Ident:
			if e, ok := ofType(v.String()); ok && e.count() > 1 {
				diags = append(diags, Diagnostic{
					Code:     "enum-value-ambiguous",
					Severity: SeverityError,
					Message:  fmt.Sprintf("enumerator %q has several integers: name the one meant, %s(n)", e.name, e.name),
					Node:     x,
					Span:     syntax.SpanOf(x),
				})
			}
		case *syntax.CallExpr:
			if v.Args == nil || len(v.Args.List) != 1 {
				return
			}
			e, ok := ofType(syntax.Name(v.Fun))
			n, lit := signedIntLiteral(v.Args.List[0])
			if ok && lit && !slicesContains(e.ranges, n) {
				diags = append(diags, Diagnostic{
					Code:     "enum-value-not-of-label",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%d is not an integer of enumerator %q", n, e.name),
					Node:     x,
					Span:     syntax.SpanOf(x),
				})
			}
		}
	}
	// A label written alone, of several types, where nothing tells which.
	ambiguousLabel := func(x syntax.Expr) bool {
		name, ok := isLabel(x)
		if !ok || values[name] || len(labelTypes[name]) < 2 {
			return false
		}
		ts := append([]string(nil), labelTypes[name]...)
		sort.Strings(ts)
		diags = append(diags, Diagnostic{
			Code:     "enum-label-ambiguous",
			Severity: SeverityError,
			Message:  fmt.Sprintf("%q is a label of %v: qualify it, or give it a type", name, ts),
			Node:     x,
			Span:     syntax.SpanOf(x),
		})
		return true
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.EnumTypeDecl:
			return false
		case *syntax.ValueDecl:
			t := identName(x.Type)
			for _, dec := range x.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				if x.Type == nil {
					ambiguousLabel(dec.Value)
					continue
				}
				checkValue(dec.Value, t)
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				break
			}
			switch x.Op.Kind() {
			case syntax.ASSIGN:
				if id, ok := x.X.(*syntax.Ident); ok {
					checkValue(x.Y, varTypes[id.String()])
				}
			case syntax.EQ, syntax.NE, syntax.LT, syntax.LE, syntax.GT, syntax.GE:
				_, lx := isLabel(x.X)
				_, ly := isLabel(x.Y)
				if lx && ly && !ambiguousLabel(x.X) {
					ambiguousLabel(x.Y)
				}
			}
		}
		return true
	})
	return diags
}

func slicesContains(rs []intRange, n int64) bool {
	for _, r := range rs {
		if r.contains(n) {
			return true
		}
	}
	return false
}

// v5Enumerators returns the enumerators of a type with their integers. When
// one written with integers has some the analyzer cannot tell, none is
// known. It also returns an enumerator written without integers for which
// none is left.
func v5Enumerators(ed *syntax.EnumTypeDecl) ([]v5Enumerator, syntax.Node) {
	var enums []v5Enumerator
	var used []intRange
	allKnown := true
	for _, x := range ed.Enums {
		e := v5Enumerator{node: x, known: true}
		switch v := x.(type) {
		case *syntax.Ident:
			e.name = v.String()
		case *syntax.CallExpr:
			e.name = syntax.Name(v.Fun)
			if v.Args == nil {
				break
			}
			for _, arg := range v.Args.List {
				r, ok := literalIntRange(arg)
				if !ok {
					e.known, allKnown = false, false
					e.ranges = nil
					break
				}
				e.ranges = append(e.ranges, r)
			}
			used = append(used, e.ranges...)
		default:
			continue
		}
		enums = append(enums, e)
	}
	if !allKnown {
		for i := range enums {
			enums[i].known = false
		}
		return enums, nil
	}
	var missing syntax.Node
	next := int64(0)
	for i, e := range enums {
		if len(e.ranges) > 0 {
			continue
		}
		n, ok := unusedInt(next, used)
		if !ok {
			if missing == nil {
				missing = e.node
			}
			enums[i].known = false
			continue
		}
		next = n
		enums[i].ranges = []intRange{{n, n}}
		used = append(used, enums[i].ranges...)
	}
	return enums, missing
}

// unusedInt is the lowest integer from n on that no range holds.
func unusedInt(n int64, used []intRange) (int64, bool) {
	for moved := true; moved; {
		moved = false
		for _, r := range used {
			if r.contains(n) {
				if r.hi == math.MaxInt64 {
					return 0, false
				}
				n, moved = r.hi+1, true
			}
		}
	}
	return n, true
}

// literalIntRange is the integers an integer literal or a range of
// literals, `infinity` an open bound, stands for.
func literalIntRange(x syntax.Expr) (intRange, bool) {
	if n, ok := signedIntLiteral(x); ok {
		return intRange{n, n}, true
	}
	be, ok := x.(*syntax.BinaryExpr)
	if !ok || be.Op == nil || be.Op.Kind() != syntax.RANGE {
		return intRange{}, false
	}
	lo, ok := literalBound(be.X, math.MinInt64)
	if !ok {
		return intRange{}, false
	}
	hi, ok := literalBound(be.Y, math.MaxInt64)
	if !ok || hi < lo {
		return intRange{}, false
	}
	return intRange{lo, hi}, true
}

func literalBound(x syntax.Expr, open int64) (int64, bool) {
	if n, ok := signedIntLiteral(x); ok {
		return n, true
	}
	neg := false
	if u, ok := x.(*syntax.UnaryExpr); ok && u.Op != nil && u.Op.Kind() == syntax.SUB {
		x, neg = u.X, true
	}
	// infinity bounds a range only on its own side.
	if isInfinity(x) && neg == (open < 0) {
		return open, true
	}
	return 0, false
}

// v5HiddenLabels reports the definitions that would hide a label: one of
// the module with a label's name, and a local one of a behaviour that
// refers to the label's type.
func v5HiddenLabels(mod *syntax.Module, types map[string][]v5Enumerator) []Diagnostic {
	labels := map[string]string{}
	for typeName, enums := range types {
		for _, e := range enums {
			labels[e.name] = typeName
		}
	}
	var diags []Diagnostic
	hidden := func(n syntax.Node, name, typeName, where string) {
		diags = append(diags, Diagnostic{
			Code:     "enum-label-hidden",
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s %q hides the label %q of %s", where, name, name, typeName),
			Node:     n,
			Span:     syntax.SpanOf(n),
		})
	}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		if _, ok := d.Def.(*syntax.EnumTypeDecl); ok {
			continue
		}
		for _, id := range definedNames(d.Def) {
			if typeName, ok := labels[id.String()]; ok {
				hidden(id, id.String(), typeName, "definition")
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		var body *syntax.BlockStmt
		switch x := n.(type) {
		case *syntax.FuncDecl:
			body = x.Body
		case *syntax.ControlPart:
			body = x.Body
		default:
			return true
		}
		if body == nil {
			return false
		}
		locals := map[string]*syntax.Ident{}
		refs := map[string]syntax.Node{}
		syntax.Inspect(body, func(m syntax.Node) bool {
			switch y := m.(type) {
			case *syntax.ValueDecl:
				for _, dec := range y.Decls {
					if dec != nil && dec.Name != nil {
						locals[dec.Name.String()] = dec.Name
					}
				}
				if t := identName(y.Type); t != "" {
					if _, ok := types[t]; ok {
						if _, seen := refs[t]; !seen {
							refs[t] = y.Type
						}
					}
				}
			}
			return true
		})
		for t, at := range refs {
			for _, e := range types[t] {
				if id, ok := locals[e.name]; ok {
					hidden(at, id.String(), t, "local definition")
				}
			}
		}
		return false
	})
	return diags
}

// definedNames returns the identifiers a module definition declares.
func definedNames(def syntax.Node) []*syntax.Ident {
	var out []*syntax.Ident
	add := func(x syntax.Expr) {
		if id, ok := x.(*syntax.Ident); ok && id != nil {
			out = append(out, id)
		}
	}
	switch d := def.(type) {
	case *syntax.ValueDecl:
		for _, dec := range d.Decls {
			if dec != nil {
				add(dec.Name)
			}
		}
	case *syntax.TemplateDecl:
		add(d.Name)
	case *syntax.FuncDecl:
		add(d.Name)
	case *syntax.SubTypeDecl:
		if d.Field != nil {
			add(d.Field.Name)
		}
	case *syntax.StructTypeDecl:
		add(d.Name)
	case *syntax.ComponentTypeDecl:
		add(d.Name)
	case *syntax.PortTypeDecl:
		add(d.Name)
	case *syntax.SignatureDecl:
		add(d.Name)
	}
	return out
}
