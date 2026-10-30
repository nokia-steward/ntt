package interpreter

import (
	"strings"
	"sync"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// imports.go resolves a module's names through its imports (ETSI ES 201
// 873-1 8.2.3). Each module's scope sits inside a scope of what the module
// imports, which sits inside the run's flat scope: a module sees its own
// definitions first, then those it imports, and only then a definition
// of any module of the run, which keeps resolving what the import
// statements do not name (an enumerated value, a definition imported
// through another module's imports).

// moduleDef is a definition of a module: its kind (TYPE, CONST, ...), its
// name and the groups it sits in, outermost first.
type moduleDef struct {
	kind   syntax.Kind
	name   string
	groups []string
	vis    syntax.Kind // PUBLIC, PRIVATE, FRIEND, or 0: public
}

// moduleDefs lists the definitions of mod, groups looked into.
func moduleDefs(mod *syntax.Module) []moduleDef {
	var out []moduleDef
	var walk func(defs []*syntax.ModuleDef, groups []string)
	walk = func(defs []*syntax.ModuleDef, groups []string) {
		for _, d := range defs {
			if d == nil {
				continue
			}
			var vis syntax.Kind
			if d.Visibility != nil {
				vis = d.Visibility.Kind()
			}
			add := func(kind syntax.Kind, name string) {
				if name != "" {
					out = append(out, moduleDef{kind: kind, name: name, groups: groups, vis: vis})
				}
			}
			switch n := d.Def.(type) {
			case *syntax.GroupDecl:
				walk(n.Defs, append(append([]string{}, groups...), syntax.Name(n.Name)))
			case *syntax.TemplateDecl:
				add(syntax.TEMPLATE, syntax.Name(n.Name))
			case *syntax.ValueDecl:
				if n.KindTok == nil {
					continue
				}
				for _, dec := range n.Decls {
					if dec != nil {
						add(n.KindTok.Kind(), syntax.Name(dec.Name))
					}
				}
			case *syntax.ModuleParameterGroup:
				for _, vd := range n.Decls {
					for _, dec := range vd.Decls {
						if dec != nil {
							add(syntax.MODULEPAR, syntax.Name(dec.Name))
						}
					}
				}
			case *syntax.FuncDecl:
				if n.KindTok != nil {
					add(n.KindTok.Kind(), syntax.Name(n.Name))
				}
			case *syntax.SignatureDecl:
				add(syntax.SIGNATURE, syntax.Name(n.Name))
			case *syntax.ClassTypeDecl:
				add(syntax.TYPE, syntax.Name(n.Name))
			case *syntax.BehaviourTypeDecl:
				add(syntax.TYPE, syntax.Name(n.Name))
			case *syntax.EnumTypeDecl:
				add(syntax.TYPE, syntax.Name(n.Name))
			default:
				if isTypeDecl(d.Def) {
					add(syntax.TYPE, syntax.Name(d.Def))
				}
			}
		}
	}
	walk(mod.Defs, nil)
	return out
}

// importSpec is what an import statement takes from its module: every
// definition, every one of some kinds, some by name, the definitions of
// some groups; less what it excepts.
type importSpec struct {
	all    bool
	kinds  map[syntax.Kind]bool
	names  map[string]bool
	groups map[string]bool
	except importExcept
}

type importExcept struct {
	kinds  map[syntax.Kind]bool
	names  map[string]bool
	groups map[string]bool
}

func (x *importExcept) add(spec *syntax.DefKindExpr) {
	if spec == nil || spec.KindTok == nil {
		return
	}
	kind := spec.KindTok.Kind()
	for _, e := range spec.List {
		switch {
		case isAllKeyword(e):
			if x.kinds == nil {
				x.kinds = map[syntax.Kind]bool{}
			}
			x.kinds[kind] = true
		case kind == syntax.GROUP:
			if x.groups == nil {
				x.groups = map[string]bool{}
			}
			x.groups[syntax.Name(e)] = true
		default:
			if x.names == nil {
				x.names = map[string]bool{}
			}
			x.names[syntax.Name(e)] = true
		}
	}
}

func (x *importExcept) excludes(d moduleDef) bool {
	if x.kinds[d.kind] || x.names[d.name] {
		return true
	}
	for _, g := range d.groups {
		if x.groups[g] {
			return true
		}
	}
	return false
}

func isAllKeyword(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	return ok && id != nil && id.Tok.Kind() == syntax.ALL
}

func newImportSpec(imp *syntax.ImportDecl) importSpec {
	s := importSpec{kinds: map[syntax.Kind]bool{}, names: map[string]bool{}, groups: map[string]bool{}}
	for _, spec := range imp.List {
		if spec == nil {
			continue
		}
		var kind syntax.Kind
		if spec.KindTok != nil {
			kind = spec.KindTok.Kind()
		}
		for _, e := range spec.List {
			x := e
			if ex, ok := e.(*syntax.ExceptExpr); ok {
				x = ex.X
				for _, l := range ex.List {
					switch l := l.(type) {
					case *syntax.DefKindExpr:
						s.except.add(l)
					default:
						s.except.add(&syntax.DefKindExpr{KindTok: spec.KindTok, List: []syntax.Expr{l}})
					}
				}
			}
			switch {
			case kind == syntax.IMPORT:
				// The imports of the module (8.2.3.7) resolve, as
				// before, in the run's flat scope.
			case spec.KindTok == nil && isAllKeyword(x):
				s.all = true
			case isAllKeyword(x):
				s.kinds[kind] = true
			case kind == syntax.GROUP:
				s.groups[syntax.Name(x)] = true
			default:
				s.names[syntax.Name(x)] = true
			}
		}
	}
	return s
}

func (s *importSpec) takes(d moduleDef) bool {
	taken := s.all || s.kinds[d.kind] || s.names[d.name]
	for _, g := range d.groups {
		if s.groups[g] {
			taken = true
		}
	}
	return taken && !s.except.excludes(d)
}

// importResolver resolves, for the scope of what module mod imports, a
// name through mod's import statements, in their order, to the scope of
// the module that defines it. It looks a name up when first asked and
// remembers the answer — that none of the imports names it, once every
// module imported from is set up — so a lookup costs no more than one of
// the module's own names.
type importResolver struct {
	root    runtime.Scope
	mod     *syntax.Module
	own     moduleDefIndex // mod's, which no import stands in for
	imports []importFrom
	mu      sync.RWMutex
	memo    map[string]*importFrom // nil: none of the imports names it
	init    *moduleInit            // mod's, while it is set up
}

// importFrom is one import statement: the module it imports from and what
// it takes.
type importFrom struct {
	module string
	spec   importSpec
}

// moduleDefIndex is a module's definitions by name, made when first
// needed, once per run.
type moduleDefIndex struct {
	once    sync.Once
	defs    map[string]moduleDef
	friends map[string]bool // the modules declared its friends
}

func (x *moduleDefIndex) get(mod *syntax.Module) map[string]moduleDef {
	x.once.Do(func() {
		x.defs = map[string]moduleDef{}
		for _, d := range moduleDefs(mod) {
			if _, seen := x.defs[d.name]; !seen {
				x.defs[d.name] = d
			}
		}
		x.friends = map[string]bool{}
		for _, d := range mod.Defs {
			if f, ok := d.Def.(*syntax.FriendDecl); ok {
				x.friends[syntax.Name(f.Module)] = true
			}
		}
	})
	return x.defs
}

// importable reports whether module importer may import d, a definition
// of the module x indexes (ETSI 8.2.3): not a private one, and a friend
// one only into a module x declares its friend.
func (x *moduleDefIndex) importable(d moduleDef, importer string) bool {
	switch d.vis {
	case syntax.PRIVATE:
		return false
	case syntax.FRIEND:
		return x.friends[importer]
	}
	return true
}

func newImportResolver(root runtime.Scope, mod *syntax.Module) *importResolver {
	r := &importResolver{root: root, mod: mod, memo: map[string]*importFrom{}}
	var walk func(defs []*syntax.ModuleDef)
	walk = func(defs []*syntax.ModuleDef) {
		for _, d := range defs {
			if d == nil {
				continue
			}
			switch n := d.Def.(type) {
			case *syntax.GroupDecl:
				walk(n.Defs)
			case *syntax.ImportDecl:
				from := syntax.Name(n.Module)
				if from == "" || from == syntax.Name(mod.Name) {
					continue
				}
				r.imports = append(r.imports, importFrom{module: from, spec: newImportSpec(n)})
			}
		}
	}
	walk(mod.Defs)
	return r
}

// Resolve implements runtime.Resolver.
func (r *importResolver) Resolve(name string) (runtime.Object, *runtime.Env, bool) {
	def := bindingName(name)
	// One of mod's own definitions, named before it is evaluated. One
	// that has no value — it names itself, or what is not bound yet —
	// fails, rather than another module's like-named definition standing
	// in for it; but an enumerated value named like it is that value.
	if m := r.init; m != nil && !m.closed {
		if m.named(def); m.env.Binds(name) {
			val, _ := m.env.Get(name)
			return val, m.env, true
		}
		if def == name {
			if err := m.unevaluated(name); err != nil && !r.namesEnumValue(name) {
				return err, m.env, true
			}
		}
	}
	if len(r.imports) == 0 {
		return nil, nil, false
	}
	r.mu.RLock()
	from, known := r.memo[def]
	r.mu.RUnlock()
	if !known {
		var final bool
		from, final = r.lookup(def)
		if final {
			r.mu.Lock()
			r.memo[def] = from
			r.mu.Unlock()
		}
	}
	if from == nil {
		return nil, nil, false
	}
	src, ok := lookupModuleScope(r.root, from.module)
	if ok && src.imports.init != nil {
		src.imports.init.named(def)
	}
	if !ok || !src.env.Binds(name) {
		return nil, nil, false
	}
	val, _ := src.env.Get(name)
	return exportedValue(src.env, from.module, name, val), src.env, true
}

// namesEnumValue reports whether name is a value of an enumerated type of
// mod's, or one it imports: a definition may be named like one (ETSI
// 6.2.4), which names the value where its type is expected.
func (r *importResolver) namesEnumValue(name string) bool {
	v, _ := r.root.Get(name)
	ev, ok := v.(*runtime.EnumValue)
	if !ok || ev.TypeName() == "" {
		return false
	}
	tn := bindingName(ev.TypeName())
	if _, own := r.own.get(r.mod)[tn]; own {
		return true
	}
	from, _ := r.lookup(tn)
	return from != nil
}

// lookup returns the first import statement that takes definition def
// from a module of the run, or nil; and whether that is the answer for
// the run — not while a module imported from, which the run does not list
// (runModules), is still to be set up.
func (r *importResolver) lookup(def string) (*importFrom, bool) {
	if _, ok := r.own.get(r.mod)[def]; ok {
		return nil, true
	}
	run := runModulesOf(r.root)
	final := true
	for i := range r.imports {
		imp := &r.imports[i]
		var mod *syntax.Module
		var defs *moduleDefIndex
		if m, ok := run[imp.module]; ok {
			mod, defs = m.mod, m.defs
		} else if src, ok := lookupModuleScope(r.root, imp.module); ok && src.mod != nil {
			mod, defs = src.mod, src.defs
		} else {
			final = final && run != nil
			continue
		}
		if d, ok := defs.get(mod)[def]; ok && imp.spec.takes(d) && defs.importable(d, syntax.Name(r.mod.Name)) {
			return imp, true
		}
	}
	return nil, final
}

// runModules are the modules of a run, by name, with their definitions:
// what a module imports from resolves before every module is set up.
type runModules struct{ byName map[string]runModule }

type runModule struct {
	mod  *syntax.Module
	defs *moduleDefIndex
}

const runModulesKey = "\x00ttcn3:run-modules"

func (*runModules) Type() runtime.ObjectType { return runtime.ObjectType("RUN_MODULES") }
func (*runModules) Inspect() string          { return "run modules" }
func (m *runModules) Equal(o runtime.Object) bool {
	other, ok := o.(*runModules)
	return ok && other == m
}

// bindRunModules records in root the modules of the run, those of trees;
// a module's definitions are indexed once for its tree, for every run.
func bindRunModules(root runtime.Scope, trees []*ttcn3.Tree) {
	run := &runModules{byName: map[string]runModule{}}
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		for _, m := range t.Modules() {
			mod, ok := m.Node.(*syntax.Module)
			if !ok {
				continue
			}
			name := syntax.Name(mod.Name)
			if _, seen := run.byName[name]; name == "" || seen {
				continue
			}
			defs := t.Memo("interpreter.defs:"+name, func() interface{} { return &moduleDefIndex{} }).(*moduleDefIndex)
			run.byName[name] = runModule{mod: mod, defs: defs}
		}
	}
	root.Set(runModulesKey, run)
}

func runModulesOf(root runtime.Scope) map[string]runModule {
	if v, ok := root.Get(runModulesKey); ok {
		if m, ok := v.(*runModules); ok {
			return m.byName
		}
	}
	return nil
}

// bindingName is the definition a scope binding belongs to: the name
// itself, or the name a NUL-prefixed key (moduleParKey, declaredTypeKey,
// ...) records something of.
func bindingName(key string) string {
	if strings.HasPrefix(key, "\x00") {
		if i := strings.LastIndexByte(key, ':'); i >= 0 {
			return key[i+1:]
		}
	}
	return key
}

// exportedValue is val, bound to key in module mod's scope s, as another
// module sees it: a definition's declared type is named by the module that
// declares the type.
func exportedValue(s *runtime.Env, mod, key string, val runtime.Object) runtime.Object {
	if strings.HasPrefix(key, declaredTypeKey("")) && mod != "" {
		if tn, ok := val.(*runtime.String); ok && !strings.Contains(string(tn.Value), ".") && s.Binds(string(tn.Value)) {
			return runtime.NewCharstring(mod + "." + string(tn.Value))
		}
	}
	return val
}
