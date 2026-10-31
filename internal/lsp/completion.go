package lsp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/printer"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Lexical rank prefixes. A leading space sorts the group before ordinary labels.
const (
	sortFirst  = " 1"
	sortSecond = " 2"
	sortThird  = " 3"
	sortFourth = " 4"
	sortImport = " "
)

var (
	moduleDefLabels = []string{
		"import from ", "type ", "const ", "modulepar ", "template ",
		"function ", "external function ", "altstep ", "testcase ",
		"control ", "signature ",
	}
	// "control" has no trailing space; the other kinds do.
	importKindLabels = []string{
		"type ", "const ", "modulepar ", "template ", "function ",
		"external function ", "altstep ", "testcase ", "control", "signature ",
	}
	predefinedTypeLabels = []string{
		"anytype ", "bitstring ", "boolean ", "charstring ", "default ",
		"float ", "hexstring ", "integer ", "octetstring ",
		"universal charstring ", "verdicttype ",
	}
)

func (s *Server) completion(ctx context.Context, params *protocol.CompletionParams) (list *protocol.CompletionList, err error) {
	start := time.Now()
	uri := params.TextDocument.URI
	defer func() {
		if r := recover(); r != nil {
			// Incomplete trees are common while the user is typing.
			log.Debugf("completion at %s:%d:%d recovered from panic: %v\n", uri, params.Position.Line+1, params.Position.Character+1, r)
			list, err = &protocol.CompletionList{}, nil
		}
		log.Debugf("Completion took %s.\n", time.Since(start))
	}()

	if !uri.SpanURI().IsFile() {
		log.Printf("for 'code completion' the new file %q needs to be saved at least once", string(uri))
		return &protocol.CompletionList{}, nil
	}

	tree := ttcn3.ParseFile(uri.SpanURI().Filename())
	if tree.Root == nil || len(tree.Modules()) == 0 {
		return &protocol.CompletionList{Items: []protocol.CompletionItem{{
			Label:            "module",
			InsertText:       "module ${1:} {\n\t${0}\n}",
			InsertTextFormat: protocol.SnippetTextFormat,
			Kind:             protocol.KeywordCompletion,
		}}}, nil
	}

	suites := s.Owners(uri)
	if len(suites) == 0 {
		return nil, fmt.Errorf("no suite found for file %q", string(uri))
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	pos := tree.PosFor(line, col)
	nodes := LastNonWsToken(tree.Root, pos)
	if len(nodes) == 0 {
		return &protocol.CompletionList{}, nil
	}
	// A file owned by several suites must not mix their modules: that produces cyclic imports.
	return &protocol.CompletionList{Items: Complete(suites[0], tree, pos, nodes)}, nil
}

// LastNonWsToken returns the path from n down to the last node that begins at or before pos.
// A trailing error token that ends at pos is included.
func LastNonWsToken(n syntax.Node, pos int) []syntax.Node {
	if pos < 0 || n == nil {
		return nil
	}

	var (
		stop  bool
		stack = []syntax.Node{n}
		last  []syntax.Node
	)
	snap := func() {
		if cap(last) < len(stack) {
			last = make([]syntax.Node, len(stack))
		}
		last = last[:len(stack)]
		copy(last, stack)
	}

	n.Inspect(func(n syntax.Node) bool {
		if stop {
			return false
		}
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if errNode, ok := n.(*syntax.ErrorNode); ok {
			if tok := errNode.LastTok(); tok != nil && tok.End() == pos {
				stop = true
				stack = append(stack, n)
				snap()
				return false
			}
		}
		if pos < n.Pos() {
			stop = true
			return false
		}
		stack = append(stack, n)
		snap()
		return true
	})
	return last
}

// Complete returns completion items for the cursor at pos.
// nodes is the ancestor path from LastNonWsToken in tree, leaf last.
// The module being edited is the module declaration on that path.
func Complete(suite *Suite, tree *ttcn3.Tree, pos int, nodes []syntax.Node) []protocol.CompletionItem {
	if len(nodes) == 0 || tree == nil {
		return nil
	}
	c := completer{suite: suite, tree: tree, pos: pos, nodes: nodes, mod: enclosingModule(nodes)}
	return c.complete()
}

// completer is one completion request.
type completer struct {
	suite  *Suite
	tree   *ttcn3.Tree
	pos    int
	nodes  []syntax.Node
	mod    string
	parsed []*ttcn3.Tree
	loaded bool
}

// behavMatch selects function-like declarations.
// A declaration matches when any set flag matches its signature.
type behavMatch struct {
	kinds  []syntax.Kind
	plain  bool // no return and no runs-on
	ret    bool
	runsOn bool
}

func (m behavMatch) allows(fn *syntax.FuncDecl) bool {
	hasRet := fn.Return != nil
	hasRuns := fn.RunsOn != nil
	if (m.ret && hasRet) || (m.runsOn && hasRuns) {
		return true
	}
	return m.plain && !hasRet && !hasRuns
}

func (c *completer) complete() []protocol.CompletionItem {
	if inBlockOf[*syntax.FuncDecl](c.nodes) {
		return c.behaviourBody()
	}
	if inBlockOf[*syntax.ControlPart](c.nodes) {
		return c.controlBody()
	}
	if decl, ok := moduleLevel[*syntax.ValueDecl](c.nodes); ok {
		return c.constDecl(decl)
	}
	if decl, ok := moduleLevel[*syntax.TemplateDecl](c.nodes); ok {
		return c.templateDecl(decl)
	}
	return c.byLeaf()
}

func (c *completer) behaviourBody() []protocol.CompletionItem {
	prev := c.at(1)
	sel, _ := prev.(*syntax.SelectorExpr)
	if sel != nil && sel.X == nil {
		return nil
	}

	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	switch {
	case startArg(c.nodes):
		match.runsOn = true
	case !insideExpr(c.nodes, sel != nil):
		match.kinds = []syntax.Kind{syntax.FUNCTION, syntax.ALTSTEP}
		match.plain = true
		match.runsOn = true
	}

	if sel != nil {
		if tree := c.treeByModule(c.moduleRef(sel.X)); tree != nil {
			return behaviours(tree, match, constRank(sortFirst))
		}
		return c.fieldsOf(selectorTarget(sel, c.pos))
	}
	if idx, ok := prev.(*syntax.IndexExpr); ok {
		return c.fieldsOf(idx)
	}

	var items []protocol.CompletionItem
	if expr, ok := prev.(syntax.Expr); ok && c.inArgList() {
		items = c.argNames(expr)
		if bin, ok := prev.(*syntax.BinaryExpr); ok && bin.Op != nil && c.pos > bin.Op.FirstTok().End() {
			return items
		}
	}
	items = append(items, c.locals(sortSecond)...)
	items = append(items, c.allBehaviours(match)...)
	items = append(items, predefinedFunctionItems()...)
	return append(items, c.modules(sortFourth)...)
}

// moduleRef returns the module name x refers to, or "" when x is not a bare
// identifier or resolves to something other than a module.
func (c *completer) moduleRef(x syntax.Expr) string {
	id, ok := x.(*syntax.Ident)
	if !ok {
		return ""
	}
	for _, def := range c.tree.LookupWithDB(id, c.suite.DB) {
		if _, ok := def.Node.(*syntax.Module); !ok {
			return ""
		}
	}
	return tokText(id.LastTok())
}

func (c *completer) controlBody() []protocol.CompletionItem {
	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	if sel, ok := c.at(1).(*syntax.SelectorExpr); ok {
		if sel.X == nil {
			return nil
		}
		if !insideExpr(c.nodes, true) {
			match.plain = true
		}
		return behaviours(c.treeByModule(tokText(sel.X.LastTok())), match, constRank(sortFirst))
	}
	if !insideExpr(c.nodes, false) {
		match.plain = true
		match.runsOn = true
	}
	items := c.allBehaviours(match)
	items = append(items, predefinedFunctionItems()...)
	return append(items, c.modules(sortThird)...)
}

func (c *completer) constDecl(decl *syntax.ValueDecl) []protocol.CompletionItem {
	sel, _ := c.at(1).(*syntax.SelectorExpr)
	if decl.Type == nil || decl.Type.Pos() > c.pos {
		return c.typeNames(sel)
	}
	return c.returningFuncs(sel)
}

func (c *completer) templateDecl(decl *syntax.TemplateDecl) []protocol.CompletionItem {
	sel, _ := c.at(1).(*syntax.SelectorExpr)
	switch {
	case decl.ModifiesTok != nil && decl.AssignTok != nil && decl.AssignTok.Pos() > c.pos:
		return c.modifiedTemplate(sel)
	case decl.Type == nil || decl.Name == nil || decl.Name.Pos() > c.pos:
		return c.typeNames(sel)
	default:
		return c.returningFuncs(sel)
	}
}

func (c *completer) modifiedTemplate(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel != nil && sel.X != nil {
		return valueDecls(c.treeByModule(tokText(sel.X.LastTok())), syntax.TEMPLATE, false)
	}
	items := c.collect(func(tree *ttcn3.Tree) []protocol.CompletionItem {
		return valueDecls(tree, syntax.TEMPLATE, true)
	})
	return append(items, c.modules("")...)
}

func (c *completer) typeNames(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel != nil && sel.X != nil {
		return declItems(c.treeByModule(tokText(sel.X.LastTok())), constRank(""), " ", protocol.StructCompletion, isTypeDecl)
	}
	items := c.collect(func(tree *ttcn3.Tree) []protocol.CompletionItem {
		return declItems(tree, c.rank, " ", protocol.StructCompletion, isTypeDecl)
	})
	items = append(items, keywordItems(predefinedTypeLabels...)...)
	return append(items, c.modules(sortThird)...)
}

func (c *completer) returningFuncs(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	if sel != nil && sel.X != nil {
		return behaviours(c.treeByModule(tokText(sel.X.LastTok())), match, constRank(sortFirst))
	}
	items := c.allBehaviours(match)
	items = append(items, predefinedFunctionItems()...)
	return append(items, c.modules(sortThird)...)
}

func (c *completer) byLeaf() []protocol.CompletionItem {
	switch n := c.at(0).(type) {
	case *syntax.Ident:
		return c.ident(n)
	case *syntax.ImportDecl:
		if n.Module == nil {
			return c.modules(sortImport)
		}
	case *syntax.DefKindExpr:
		return c.defKind(n)
	case *syntax.RunsOnSpec, *syntax.SystemSpec:
		return c.componentTypesAndModules()
	case *syntax.ErrorNode:
		return c.onError()
	case *syntax.Declarator:
		return c.onDeclarator()
	default:
		log.Debugf("Node not considered yet: %#v\n", n)
	}
	return nil
}

func (c *completer) ident(id *syntax.Ident) []protocol.CompletionItem {
	switch parent := c.at(1).(type) {
	case *syntax.ModuleDef:
		return keywordItems(moduleDefLabels...)
	case *syntax.ImportDecl:
		return c.importIdent(id, parent)
	case *syntax.DefKindExpr:
		return c.defKindIdent(parent)
	case *syntax.ExceptExpr:
		return keywordItems(importKindLabels...)
	case *syntax.RunsOnSpec, *syntax.SystemSpec:
		return c.componentTypesAndModules()
	case *syntax.SelectorExpr:
		return c.selectorComponent(parent)
	case *syntax.ComponentTypeDecl:
		return c.componentExtends(parent)
	default:
		return nil
	}
}

func (c *completer) importIdent(id *syntax.Ident, imp *syntax.ImportDecl) []protocol.CompletionItem {
	switch {
	case imp.LBrace != nil:
		return keywordItems(importKindLabels...)
	case id == imp.Alias:
		return nil
	case id.End() >= c.pos:
		return c.modules(sortImport)
	default:
		return importAllItems()
	}
}

func (c *completer) defKindIdent(kind *syntax.DefKindExpr) []protocol.CompletionItem {
	if _, except := c.at(2).(*syntax.ExceptExpr); except && kind.KindTok == nil {
		return keywordItems(importKindLabels...)
	}
	return c.kindSpecs(kind, 1)
}

func (c *completer) defKind(kind *syntax.DefKindExpr) []protocol.CompletionItem {
	if kind.KindTok == nil {
		return keywordItems(importKindLabels...)
	}
	return c.kindSpecs(kind, 0)
}

func (c *completer) onError() []protocol.CompletionItem {
	switch parent := c.at(1).(type) {
	case *syntax.ModuleDef:
		return keywordItems(moduleDefLabels...)
	case *syntax.DefKindExpr:
		return c.kindSpecs(parent, 1)
	}
	return nil
}

// kindSpecs completes the definitions of kind, found at c.at(i), in its import's module.
// kind sits either directly in the import or in its except list.
func (c *completer) kindSpecs(kind *syntax.DefKindExpr, i int) []protocol.CompletionItem {
	if kind.KindTok == nil {
		return nil
	}
	var imp *syntax.ImportDecl
	switch p := c.at(i + 1).(type) {
	case *syntax.ImportDecl:
		imp = p
	case *syntax.ExceptExpr:
		imp, _ = c.at(i + 3).(*syntax.ImportDecl)
	}
	if imp == nil {
		return nil
	}
	return c.importSpecs(kind.KindTok.Kind(), imp)
}

func (c *completer) onDeclarator() []protocol.CompletionItem {
	decl, ok := c.at(1).(*syntax.ValueDecl)
	if !ok || decl.KindTok == nil || decl.KindTok.Kind() != syntax.PORT {
		return nil
	}
	items := c.collect(func(tree *ttcn3.Tree) []protocol.CompletionItem {
		return declItems(tree, c.rank, "", protocol.InterfaceCompletion, isPortTypeDecl)
	})
	return append(items, c.modules(sortThird)...)
}

func (c *completer) selectorComponent(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel.X == nil {
		return nil
	}
	switch c.at(2).(type) {
	case *syntax.RunsOnSpec, *syntax.SystemSpec, *syntax.ComponentTypeDecl:
		return componentTypes(c.treeByModule(tokText(sel.X.LastTok())))
	default:
		return nil
	}
}

func (c *completer) componentExtends(comp *syntax.ComponentTypeDecl) []protocol.CompletionItem {
	if comp.ExtendsTok == nil || comp.ExtendsTok.LastTok() == nil || comp.Body == nil {
		return nil
	}
	if comp.Body.LBrace != nil && comp.Body.LBrace.Pos() <= c.pos {
		return nil
	}
	return c.componentTypesAndModules()
}

func (c *completer) componentTypesAndModules() []protocol.CompletionItem {
	return append(c.collect(componentTypes), c.modules(sortSecond)...)
}

func componentTypes(tree *ttcn3.Tree) []protocol.CompletionItem {
	return declItems(tree, constRank(sortFirst), "", protocol.StructCompletion, isComponentDecl)
}

func (c *completer) importSpecs(kind syntax.Kind, imp *syntax.ImportDecl) []protocol.CompletionItem {
	if imp.Module == nil {
		return nil
	}
	tree := c.treeByModule(tokText(imp.Module.Tok))
	if tree == nil {
		return nil
	}

	var items []protocol.CompletionItem
	switch kind {
	case syntax.ALTSTEP, syntax.FUNCTION, syntax.TESTCASE:
		items = declNames(tree, protocol.FunctionCompletion, func(n syntax.Node) bool {
			fn, ok := n.(*syntax.FuncDecl)
			return ok && fn.KindTok != nil && fn.KindTok.Kind() == kind
		})
	case syntax.TEMPLATE, syntax.CONST, syntax.MODULEPAR:
		items = valueDecls(tree, kind, false)
	case syntax.TYPE:
		items = declNames(tree, protocol.StructCompletion, isTypeDecl)
	default:
		log.Debugf("Kind not considered yet: %#v\n", kind)
		return nil
	}
	return append(items, protocol.CompletionItem{Label: "all;", Kind: protocol.KeywordCompletion})
}

// trees returns one syntax tree per suite file. Module names are read from those trees.
func (c *completer) trees() []*ttcn3.Tree {
	if c.loaded {
		return c.parsed
	}
	c.loaded = true
	for _, file := range c.suite.Files() {
		c.parsed = append(c.parsed, ttcn3.ParseFile(file))
	}
	return c.parsed
}

// treeByModule returns the tree that declares name.
func (c *completer) treeByModule(name string) *ttcn3.Tree {
	if name == "" {
		return nil
	}
	if file, err := c.suite.FindModule(name); err == nil {
		if tree := ttcn3.ParseFile(file); declares(tree, name) {
			return tree
		}
	}
	// FindModule may guess by file name or miss files not yet indexed.
	for _, tree := range c.trees() {
		if declares(tree, name) {
			return tree
		}
	}
	log.Debugf("module %s not found\n", name)
	return nil
}

func declares(tree *ttcn3.Tree, name string) bool {
	for _, m := range tree.Modules() {
		if syntax.Name(m.Node) == name {
			return true
		}
	}
	return false
}

// collect concatenates f over all suite trees.
func (c *completer) collect(f func(*ttcn3.Tree) []protocol.CompletionItem) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, tree := range c.trees() {
		items = append(items, f(tree)...)
	}
	return items
}

func (c *completer) allBehaviours(match behavMatch) []protocol.CompletionItem {
	return c.collect(func(tree *ttcn3.Tree) []protocol.CompletionItem {
		return behaviours(tree, match, c.rank)
	})
}

// eachModuleDef calls f for every module-level definition in tree, including those in groups.
func eachModuleDef(tree *ttcn3.Tree, f func(mod string, def syntax.Node)) {
	if tree == nil {
		return
	}
	var walk func(mod string, defs []*syntax.ModuleDef)
	walk = func(mod string, defs []*syntax.ModuleDef) {
		for _, d := range defs {
			if d == nil || syntax.IsNil(d.Def) {
				continue
			}
			if g, ok := d.Def.(*syntax.GroupDecl); ok {
				walk(mod, g.Defs)
				continue
			}
			f(mod, d.Def)
		}
	}
	for _, m := range tree.Modules() {
		mod := m.Node.(*syntax.Module)
		walk(syntax.Name(mod), mod.Defs)
	}
}

// behaviours lists matching declarations grouped by match.kinds, in that order.
func behaviours(tree *ttcn3.Tree, match behavMatch, sortOf func(string) string) []protocol.CompletionItem {
	byKind := make([][]protocol.CompletionItem, len(match.kinds))
	eachModuleDef(tree, func(mod string, def syntax.Node) {
		fn, ok := def.(*syntax.FuncDecl)
		if !ok || fn.KindTok == nil || !match.allows(fn) {
			return
		}
		for i, kind := range match.kinds {
			if fn.KindTok.Kind() == kind {
				byKind[i] = append(byKind[i], behaviourItem(fn, mod, sortOf(mod)))
			}
		}
	})
	var items []protocol.CompletionItem
	for _, group := range byKind {
		items = append(items, group...)
	}
	return items
}

func behaviourItem(fn *syntax.FuncDecl, mod, sortPref string) protocol.CompletionItem {
	name := syntax.Name(fn)
	var sig strings.Builder
	sig.WriteString(fn.KindTok.String() + " " + mod + "." + name)
	before := sig.Len()
	printer.Print(&sig, fn.Params)
	hasParams := sig.Len()-before > 2
	writeClause := func(n syntax.Node) {
		if syntax.IsNil(n) {
			return
		}
		sig.WriteString("\n  ")
		printer.Print(&sig, n)
	}
	writeClause(fn.RunsOn)
	writeClause(fn.System)
	writeClause(fn.Return)

	format := protocol.PlainTextTextFormat
	insert := name + "()"
	if hasParams {
		format = protocol.SnippetTextFormat
		insert = name + "($1)$0"
	}
	return protocol.CompletionItem{
		Label:            name + "()",
		InsertText:       insert,
		Kind:             protocol.FunctionCompletion,
		SortText:         sortPref + name,
		Detail:           sig.String(),
		Documentation:    syntax.Doc(fn),
		InsertTextFormat: format,
	}
}

func (c *completer) locals(sortPref string) []protocol.CompletionItem {
	var fn *syntax.FuncDecl
	for _, n := range c.nodes {
		if f, ok := n.(*syntax.FuncDecl); ok {
			fn = f
			break
		}
	}
	if fn == nil {
		return nil
	}
	var items []protocol.CompletionItem
	fn.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		switch n.(type) {
		case *syntax.Declarator, *syntax.FormalPar:
		default:
			return true
		}
		name := syntax.Name(n)
		if name == "" {
			return false
		}
		items = append(items, protocol.CompletionItem{
			Label:    name,
			Kind:     protocol.VariableCompletion,
			SortText: sortPref + name,
		})
		return false
	})
	return items
}

func (c *completer) modules(sortPref string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)
	for _, tree := range c.trees() {
		for _, m := range tree.Modules() {
			name := syntax.Name(m.Node)
			if name == "" || name == c.mod || seen[name] {
				continue
			}
			seen[name] = true
			item := protocol.CompletionItem{Label: name, Kind: protocol.ModuleCompletion}
			if sortPref != "" {
				item.SortText = sortPref + name
			}
			items = append(items, item)
		}
	}
	return items
}

func (c *completer) rank(name string) string {
	if name == c.mod {
		return sortFirst
	}
	return sortSecond
}

func declItems(tree *ttcn3.Tree, sortOf func(string) string, suffix string, kind protocol.CompletionItemKind, want func(syntax.Node) bool) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	eachModuleDef(tree, func(mod string, def syntax.Node) {
		if !want(def) {
			return
		}
		name := syntax.Name(def)
		item := protocol.CompletionItem{
			Label:  name + suffix,
			Kind:   kind,
			Detail: mod + "." + name,
		}
		if pref := sortOf(mod); pref != "" {
			item.SortText = pref + name
		}
		items = append(items, item)
	})
	return items
}

// declNames lists the names of wanted module definitions without detail or rank.
func declNames(tree *ttcn3.Tree, kind protocol.CompletionItemKind, want func(syntax.Node) bool) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	eachModuleDef(tree, func(_ string, def syntax.Node) {
		if want(def) {
			items = append(items, protocol.CompletionItem{Label: syntax.Name(def), Kind: kind})
		}
	})
	return items
}

func valueDecls(tree *ttcn3.Tree, kind syntax.Kind, withDetail bool) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	add := func(mod, name string) {
		item := protocol.CompletionItem{Label: name, Kind: protocol.ConstantCompletion}
		if withDetail {
			item.Detail = mod + "." + name
		}
		items = append(items, item)
	}
	eachModuleDef(tree, func(mod string, def syntax.Node) {
		switch def := def.(type) {
		case *syntax.ValueDecl:
			if def.KindTok == nil || def.KindTok.Kind() != kind {
				return
			}
			for _, d := range def.Decls {
				if d != nil {
					add(mod, syntax.Name(d))
				}
			}
		case *syntax.TemplateDecl:
			if kind == syntax.TEMPLATE {
				add(mod, syntax.Name(def))
			}
		}
	})
	return items
}

func (c *completer) argNames(n syntax.Expr) []protocol.CompletionItem {
	tree := c.tree
	assign := true
	if _, ok := n.(*syntax.BinaryExpr); ok {
		parent, _ := tree.ParentOf(n).(syntax.Expr)
		n = parent
		assign = false
	}
	if _, ok := n.(*syntax.ParenExpr); ok {
		parent, _ := tree.ParentOf(n).(syntax.Expr)
		n = parent
	}
	switch n := n.(type) {
	case *syntax.CallExpr:
		if n.Args == nil {
			return nil
		}
		defs := tree.LookupWithDB(n.Fun, c.suite.DB)
		if len(defs) == 0 {
			return nil
		}
		params := getDeclarationParams(defs[0].Node)
		if params == nil {
			return nil
		}
		return paramItems(params, len(n.Args.List)-1, sortFirst, assign)
	case *syntax.CompositeLiteral:
		defs := tree.TypeOf(n, c.suite.DB)
		if len(defs) == 0 {
			return nil
		}
		// TypeOf resolves nested literals to the outermost type; each nesting level is one list element.
		nesting := 0
		for p := tree.ParentOf(n); ; p = tree.ParentOf(p) {
			if _, ok := p.(*syntax.CompositeLiteral); !ok {
				break
			}
			nesting++
		}
		typ, depth := actualType(c.suite.DB, defs[0])
		if depth != nesting {
			return nil
		}
		return paramItems(typ, 0, sortFirst, assign)
	default:
		return nil
	}
}

func paramItems(n syntax.Node, from int, sortPref string, assign bool) []protocol.CompletionItem {
	items := collectParams(n)
	if from > 0 {
		if from >= len(items) {
			return nil
		}
		items = items[from:]
	}
	insert := "%s"
	if assign {
		insert = "%s := ${1:}"
	}
	for i := range items {
		items[i].Documentation = ""
		items[i].Kind = protocol.EnumMemberCompletion
		items[i].SortText = sortPref + items[i].Label
		items[i].InsertText = fmt.Sprintf(insert, items[i].Label)
		items[i].InsertTextFormat = protocol.SnippetTextFormat
	}
	return items
}

func collectParams(n syntax.Node) []protocol.CompletionItem {
	switch n := n.(type) {
	case *syntax.FormalPars:
		items := make([]protocol.CompletionItem, 0, len(n.List))
		for _, par := range n.List {
			if par == nil {
				continue
			}
			items = append(items, protocol.CompletionItem{Label: syntax.Name(par), Detail: syntax.Name(par.Type)})
		}
		return items
	case *syntax.StructTypeDecl:
		items := make([]protocol.CompletionItem, 0, len(n.Fields))
		for _, field := range n.Fields {
			if name := syntax.Name(field); name != "" {
				items = append(items, protocol.CompletionItem{Label: name, Detail: syntax.Name(field.Type)})
			}
		}
		return items
	default:
		return nil
	}
}

func (c *completer) fieldsOf(expr syntax.Expr) []protocol.CompletionItem {
	if syntax.IsNil(expr) {
		return nil
	}
	depth := 0
	for {
		idx, ok := expr.(*syntax.IndexExpr)
		if !ok {
			break
		}
		expr = idx.X
		depth++
	}
	defs := c.tree.TypeOf(expr, c.suite.DB)
	if len(defs) == 0 {
		return nil
	}
	typ, typeDepth := actualType(c.suite.DB, defs[0])
	st, ok := typ.(*syntax.StructTypeDecl)
	if !ok || typeDepth != depth {
		return nil
	}
	items := make([]protocol.CompletionItem, 0, len(st.Fields))
	for _, field := range st.Fields {
		name := syntax.Name(field)
		if name == "" {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:         name,
			Kind:          protocol.FieldCompletion,
			Detail:        syntax.Name(field.Type),
			Documentation: "",
		})
	}
	return items
}

// actualType follows references and counts list wrappers around n.
// References resolve in the tree of the definition they appear in. Cycles yield (nil, -1).
func actualType(db *ttcn3.DB, n *ttcn3.Node) (syntax.Node, int) {
	depth := 0
	seen := make(map[syntax.Node]bool)
	for n != nil && !syntax.IsNil(n.Node) {
		if seen[n.Node] {
			return nil, -1
		}
		seen[n.Node] = true
		switch cur := n.Node.(type) {
		case *syntax.RefSpec:
			defs := n.Tree.TypeOf(cur, db)
			if len(defs) == 0 {
				return nil, -1
			}
			n = defs[0]
		case *syntax.ListSpec:
			n = &ttcn3.Node{Node: cur.ElemType, Tree: n.Tree}
			depth++
		default:
			return cur, depth
		}
	}
	return nil, -1
}

func predefinedFunctionItems() []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, len(PredefinedFunctions))
	for i, fn := range PredefinedFunctions {
		items[i] = protocol.CompletionItem{
			Label:            fn.Label,
			Kind:             protocol.FunctionCompletion,
			Detail:           fn.Signature,
			InsertText:       fn.InsertText,
			InsertTextFormat: fn.TextFormat,
			Documentation:    protocol.MarkupContent{Kind: protocol.Markdown, Value: fn.Documentation},
		}
	}
	return items
}

func keywordItems(labels ...string) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, len(labels))
	for i, label := range labels {
		items[i] = protocol.CompletionItem{Label: label, Kind: protocol.KeywordCompletion}
	}
	return items
}

func importAllItems() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{
			Label:            "all [except {}];",
			InsertText:       "${1:all${2: except {$3\\}}};$0",
			InsertTextFormat: protocol.SnippetTextFormat,
			Kind:             protocol.KeywordCompletion,
		},
		{
			Label:            "{}",
			InsertText:       "{$0}",
			InsertTextFormat: protocol.SnippetTextFormat,
			Kind:             protocol.KeywordCompletion,
		},
	}
}

func isComponentDecl(n syntax.Node) bool {
	_, ok := n.(*syntax.ComponentTypeDecl)
	return ok
}

func isPortTypeDecl(n syntax.Node) bool {
	_, ok := n.(*syntax.PortTypeDecl)
	return ok
}

func isTypeDecl(n syntax.Node) bool {
	switch n.(type) {
	case *syntax.BehaviourTypeDecl,
		*syntax.ClassTypeDecl,
		*syntax.ComponentTypeDecl,
		*syntax.EnumTypeDecl,
		*syntax.PortTypeDecl,
		*syntax.StructTypeDecl,
		*syntax.MapTypeDecl,
		*syntax.SubTypeDecl:
		return true
	default:
		return false
	}
}

// enclosingModule is the module declaration that contains the cursor.
func enclosingModule(nodes []syntax.Node) string {
	for _, n := range nodes {
		if m, ok := n.(*syntax.Module); ok {
			return syntax.Name(m)
		}
	}
	return ""
}

func constRank(pref string) func(string) string {
	return func(string) string { return pref }
}

// at returns the node i steps above the leaf. at(0) is the leaf.
func (c *completer) at(i int) syntax.Node {
	j := len(c.nodes) - 1 - i
	if j < 0 || j >= len(c.nodes) {
		return nil
	}
	return c.nodes[j]
}

// inBlockOf reports whether the path enters a block below a node of type T.
func inBlockOf[T syntax.Node](nodes []syntax.Node) bool {
	inside := false
	for _, n := range nodes {
		if _, ok := n.(T); ok {
			inside = true
		} else if _, ok := n.(*syntax.BlockStmt); ok && inside {
			return true
		}
	}
	return false
}

// moduleLevel returns the node of type T on the path that is a module definition.
func moduleLevel[T syntax.Node](nodes []syntax.Node) (T, bool) {
	for i := 1; i < len(nodes); i++ {
		if decl, ok := nodes[i].(T); ok {
			if _, ok := nodes[i-1].(*syntax.ModuleDef); ok {
				return decl, true
			}
		}
	}
	var zero T
	return zero, false
}

func insideExpr(nodes []syntax.Node, fromModule bool) bool {
	i := len(nodes)
	if i <= 1 {
		return false
	}
	i--
	if _, ok := nodes[i].(*syntax.Ident); ok {
		i--
	}
	if fromModule {
		if i == 0 {
			return false
		}
		i--
	}
	switch nodes[i].(type) {
	case *syntax.SelectorExpr, *syntax.ExprStmt, *syntax.BlockStmt:
		return false
	default:
		return true
	}
}

func startArg(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i >= 0; i-- {
		if _, ok := nodes[i].(*syntax.ParenExpr); !ok || i < 1 {
			continue
		}
		call, ok := nodes[i-1].(*syntax.CallExpr)
		if !ok {
			continue
		}
		fun, ok := call.Fun.(*syntax.SelectorExpr)
		if !ok {
			return false
		}
		return syntax.Name(fun.Sel) == "start"
	}
	return false
}

func (c *completer) inArgList() bool {
	n := len(c.nodes)
	off := 2
	if n < off {
		return false
	}
	if bin, ok := c.nodes[n-off].(*syntax.BinaryExpr); ok {
		if bin.Op == nil {
			return false
		}
		if bin.Op.FirstTok().Pos() < c.pos {
			return false
		}
		if bin.Op.String() == ":=" {
			off++
		}
	}
	if n < off {
		return false
	}
	if _, ok := c.nodes[n-off].(*syntax.ParenExpr); ok {
		if n-off-1 < 0 {
			return false
		}
		_, ok := c.nodes[n-off-1].(*syntax.CallExpr)
		return ok
	}
	_, ok := c.nodes[n-off].(*syntax.CompositeLiteral)
	return ok
}

func selectorTarget(sel *syntax.SelectorExpr, pos int) syntax.Expr {
	if sel == nil {
		return nil
	}
	var tgt syntax.Expr = sel.X
	if sel.Sel != nil && sel.Sel.LastTok() != nil && sel.Sel.LastTok().Pos() < pos {
		tgt = sel
	}
	if nested, ok := tgt.(*syntax.SelectorExpr); ok && pos == nested.End() {
		return nested.X
	}
	return tgt
}

func tokText(tok syntax.Token) string {
	if tok == nil {
		return ""
	}
	return tok.String()
}
