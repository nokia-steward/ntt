package semantic

import "github.com/nokia/ntt/ttcn3/syntax"

// byRefParams are the formal parameters of a module's functions, altsteps
// and testcases, by behaviour name, telling which are out or inout: an
// actual parameter of one of those is written, as the left hand side of
// an assignment is, not read (5.4.2).
type byRefParams map[string][]byRefFormal

type byRefFormal struct {
	name  string
	byRef bool
}

// collectByRefParams collects the byRefParams of mod, in groups too.
func collectByRefParams(mod *syntax.Module) byRefParams {
	out := byRefParams{}
	var walk func(defs []*syntax.ModuleDef)
	walk = func(defs []*syntax.ModuleDef) {
		for _, d := range defs {
			if d == nil || d.Def == nil {
				continue
			}
			switch x := d.Def.(type) {
			case *syntax.GroupDecl:
				walk(x.Defs)
			case *syntax.FuncDecl:
				if x.Name == nil || x.Params == nil {
					continue
				}
				formals := make([]byRefFormal, len(x.Params.List))
				for i, p := range x.Params.List {
					if p == nil {
						continue
					}
					formals[i].name = syntax.Name(p.Name)
					if p.Direction != nil {
						k := p.Direction.Kind()
						formals[i].byRef = k == syntax.OUT || k == syntax.INOUT
					}
				}
				out[x.Name.String()] = formals
			}
		}
	}
	walk(mod.Defs)
	return out
}

// args returns the actual parameters of ce passed to an out or inout
// formal parameter, in list notation or assignment notation (`p := v`):
// those of a behaviour of the module, and the result parameter of the
// predefined functions that have one (outResultFuncs). A behaviour of
// another module, or a method, is not known.
func (b byRefParams) args(ce *syntax.CallExpr) []syntax.Expr {
	if ce == nil || ce.Args == nil {
		return nil
	}
	name := identName(ce.Fun)
	formals := b[name]
	var out []syntax.Expr
	for i, arg := range ce.Args.List {
		if be, ok := arg.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			if id, ok := be.X.(*syntax.Ident); ok {
				for _, f := range formals {
					if f.byRef && f.name == id.String() {
						out = append(out, be.Y)
					}
				}
				continue
			}
		}
		if (i < len(formals) && formals[i].byRef) || (outResultFuncs[name] && i == 1) {
			out = append(out, arg)
		}
	}
	return out
}
