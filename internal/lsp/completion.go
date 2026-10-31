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

func (s *Server) completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			// Incomplete trees are common while the user is typing.
			log.Debugf("Info: %s.\n", err)
		}
		log.Debugf("Completion took %s.\n", time.Since(start))
	}()

	uri := params.TextDocument.URI
	if !uri.SpanURI().IsFile() {
		log.Printf("for 'code completion' the new file %q needs to be saved at least once", string(uri))
		return &protocol.CompletionList{}, nil
	}

	file := uri.SpanURI().Filename()
	tree := ttcn3.ParseFile(file)
	if tree.Root == nil || len(tree.Modules()) == 0 {
		return &protocol.CompletionList{Items: []protocol.CompletionItem{{
			Label:            "module",
			InsertText:       "module ${1:} {\n\t${0}\n}",
			InsertTextFormat: protocol.SnippetTextFormat,
			Kind:             protocol.KeywordCompletion,
		}}}, nil
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	pos := tree.PosFor(line, col)
	nodes := LastNonWsToken(tree.Root, pos)
	if len(nodes) == 0 {
		return nil, nil
	}

	suites := s.Owners(uri)
	if len(suites) == 0 {
		return nil, fmt.Errorf("no suite found for file %q", string(uri))
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
	switch {
	case behaviourBody(c.nodes):
		return c.behaviourBody()
	case controlBody(c.nodes):
		return c.controlBody()
	case constScope(c.nodes):
		return c.constDecl()
	case templateScope(c.nodes):
		return c.templateDecl()
	default:
		return c.byLeaf()
	}
}

func (c *completer) behaviourBody() []protocol.CompletionItem {
	prev := c.at(1)
	sel, _ := prev.(*syntax.SelectorExpr)
	mod := ""
	if sel != nil {
		if sel.X == nil {
			return nil
		}
		mod = tokText(sel.X.LastTok())
	}

	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	switch {
	case startArg(c.nodes):
		match.runsOn = true
	case !insideExpr(c.nodes, mod != ""):
		match.kinds = []syntax.Kind{syntax.FUNCTION, syntax.ALTSTEP}
		match.plain = true
		match.runsOn = true
	}

	if mod != "" {
		if tree := c.treeByModule(mod); tree != nil {
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

func (c *completer) controlBody() []protocol.CompletionItem {
	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	if sel, ok := c.at(1).(*syntax.SelectorExpr); ok {
		if sel.X == nil {
			return nil
		}
		if !insideExpr(c.nodes, true) {
			match.plain = true
		}
		mod := tokText(sel.X.LastTok())
		tree := c.treeByModule(mod)
		if tree == nil {
			log.Debugf("module %s not found\n", mod)
			return nil
		}
		return behaviours(tree, match, constRank(sortFirst))
	}
	if !insideExpr(c.nodes, false) {
		match.plain = true
		match.runsOn = true
	}
	items := c.allBehaviours(match)
	items = append(items, predefinedFunctionItems()...)
	return append(items, c.modules(sortThird)...)
}

func (c *completer) constDecl() []protocol.CompletionItem {
	decl := valueDeclNode(c.nodes)
	if decl == nil {
		return nil
	}
	sel, _ := c.at(1).(*syntax.SelectorExpr)
	if decl.Type == nil || decl.Type.Pos() > c.pos {
		return c.typeNames(sel)
	}
	return c.returningFuncs(sel)
}

func (c *completer) templateDecl() []protocol.CompletionItem {
	decl := templateDeclNode(c.nodes)
	if decl == nil {
		return nil
	}
	sel, _ := c.at(1).(*syntax.SelectorExpr)
	switch {
	case decl.ModifiesTok != nil && decl.AssignTok != nil && decl.AssignTok.Pos() > c.pos:
		return c.modifiedTemplate(decl, sel)
	case decl.Type == nil || decl.Name == nil || decl.Name.Pos() > c.pos:
		return c.typeNames(sel)
	default:
		return c.returningFuncs(sel)
	}
}

func (c *completer) modifiedTemplate(decl *syntax.TemplateDecl, sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel != nil && sel.X != nil {
		mod := tokText(sel.X.LastTok())
		tree := c.treeByModule(mod)
		if tree == nil {
			log.Debugf("module %s not found\n", mod)
			return nil
		}
		return valueDecls(tree, templateKind(decl), false)
	}
	return append(c.allValueDecls(syntax.TEMPLATE), c.modules("")...)
}

func (c *completer) typeNames(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel != nil && sel.X != nil {
		return c.typesIn(tokText(sel.X.LastTok()), "")
	}
	return append(c.allTypes(), c.modules(sortThird)...)
}

func (c *completer) returningFuncs(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	match := behavMatch{kinds: []syntax.Kind{syntax.FUNCTION}, ret: true}
	if sel != nil && sel.X != nil {
		mod := tokText(sel.X.LastTok())
		tree := c.treeByModule(mod)
		if tree == nil {
			log.Debugf("module %s not found\n", mod)
			return nil
		}
		return behaviours(tree, match, constRank(sortFirst))
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
	if _, ok := c.nodes[0].(*syntax.Module); ok && len(c.nodes) == 2 {
		return keywordItems(moduleDefLabels...)
	}
	parent := c.at(1)
	if parent == nil {
		return nil
	}
	switch parent := parent.(type) {
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
	// Eight ancestors is the except-list shape: import, all except, '{', kind, ident.
	if len(c.nodes) == 8 {
		if _, ok := c.at(2).(*syntax.ExceptExpr); !ok {
			return nil
		}
		if kind.KindTok != nil {
			if imp, ok := c.at(4).(*syntax.ImportDecl); ok {
				return c.importSpecs(kind.KindTok.Kind(), imp)
			}
		}
		return keywordItems(importKindLabels...)
	}
	if imp, ok := c.at(2).(*syntax.ImportDecl); ok {
		if kind.KindTok == nil {
			return nil
		}
		return c.importSpecs(kind.KindTok.Kind(), imp)
	}
	return nil
}

func (c *completer) defKind(kind *syntax.DefKindExpr) []protocol.CompletionItem {
	if kind.KindTok == nil {
		return keywordItems(importKindLabels...)
	}
	if imp, ok := c.at(1).(*syntax.ImportDecl); ok {
		return c.importSpecs(kind.KindTok.Kind(), imp)
	}
	if _, ok := c.at(1).(*syntax.ExceptExpr); ok {
		if imp, ok := c.at(3).(*syntax.ImportDecl); ok {
			return c.importSpecs(kind.KindTok.Kind(), imp)
		}
	}
	return nil
}

func (c *completer) onError() []protocol.CompletionItem {
	switch parent := c.at(1).(type) {
	case *syntax.ModuleDef:
		return keywordItems(moduleDefLabels...)
	case *syntax.DefKindExpr:
		if parent.KindTok == nil {
			return nil
		}
		if imp, ok := c.at(2).(*syntax.ImportDecl); ok {
			return c.importSpecs(parent.KindTok.Kind(), imp)
		}
		if _, ok := c.at(2).(*syntax.ExceptExpr); ok {
			if imp, ok := c.at(4).(*syntax.ImportDecl); ok {
				return c.importSpecs(parent.KindTok.Kind(), imp)
			}
		}
	}
	return nil
}

func (c *completer) onDeclarator() []protocol.CompletionItem {
	decl, ok := c.at(1).(*syntax.ValueDecl)
	if !ok || decl.KindTok == nil || decl.KindTok.Kind() != syntax.PORT {
		return nil
	}
	return append(c.allPortTypes(), c.modules(sortThird)...)
}

func (c *completer) selectorComponent(sel *syntax.SelectorExpr) []protocol.CompletionItem {
	if sel.X == nil {
		return nil
	}
	switch c.at(2).(type) {
	case *syntax.RunsOnSpec, *syntax.SystemSpec, *syntax.ComponentTypeDecl:
		return c.componentsIn(tokText(sel.X.LastTok()), sortFirst)
	default:
		return nil
	}
}

func (c *completer) componentExtends(comp *syntax.ComponentTypeDecl) []protocol.CompletionItem {
	if comp.ExtendsTok == nil || comp.ExtendsTok.LastTok() == nil || comp.Body == nil || comp.Body.LBrace == nil {
		return nil
	}
	if comp.Body.LBrace.Pos() <= c.pos {
		return nil
	}
	return c.componentTypesAndModules()
}

func (c *completer) componentTypesAndModules() []protocol.CompletionItem {
	return append(c.allComponentTypes(sortFirst), c.modules(sortSecond)...)
}

func (c *completer) importSpecs(kind syntax.Kind, imp *syntax.ImportDecl) []protocol.CompletionItem {
	if imp.Module == nil {
		return nil
	}
	mod := tokText(imp.Module.Tok)
	tree := c.treeByModule(mod)
	if tree == nil {
		log.Debugf("module %s not found\n", mod)
		return nil
	}

	var items []protocol.CompletionItem
	switch kind {
	case syntax.ALTSTEP, syntax.FUNCTION, syntax.TESTCASE:
		items = funcNames(tree, kind)
	case syntax.TEMPLATE, syntax.CONST, syntax.MODULEPAR:
		items = valueDecls(tree, kind, false)
	case syntax.TYPE:
		items = importTypeNames(tree)
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
	// FindModule may guess by file name, or miss files indexed under another path spelling.
	for _, tree := range c.trees() {
		if declares(tree, name) {
			return tree
		}
	}
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

func (c *completer) allBehaviours(match behavMatch) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, tree := range c.trees() {
		items = append(items, behaviours(tree, match, c.rank)...)
	}
	return items
}

func behaviours(tree *ttcn3.Tree, match behavMatch, sortOf func(string) string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, kind := range match.kinds {
		items = append(items, behavioursOfKind(tree, kind, match, sortOf)...)
	}
	return items
}

func behavioursOfKind(tree *ttcn3.Tree, kind syntax.Kind, match behavMatch, sortOf func(string) string) []protocol.CompletionItem {
	var (
		items []protocol.CompletionItem
		mod   string
	)
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		if m, ok := n.(*syntax.Module); ok {
			mod = syntax.Name(m)
			return true
		}
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if fn.KindTok != nil && fn.KindTok.Kind() == kind && match.allows(fn) {
			items = append(items, behaviourItem(fn, mod, sortOf(mod)))
		}
		return false
	})
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
		d, ok := n.(*syntax.Declarator)
		if !ok {
			return true
		}
		name := syntax.Name(d)
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

func (c *completer) allTypes() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, tree := range c.trees() {
		items = append(items, declItems(tree, c.rank, " ", protocol.StructCompletion, isTypeDecl)...)
	}
	return append(items, keywordItems(predefinedTypeLabels...)...)
}

func (c *completer) allComponentTypes(sortPref string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	rank := constRank(sortPref)
	for _, tree := range c.trees() {
		items = append(items, declItems(tree, rank, "", protocol.StructCompletion, isComponentDecl)...)
	}
	return items
}

func (c *completer) allPortTypes() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, tree := range c.trees() {
		items = append(items, declItems(tree, c.rank, "", protocol.InterfaceCompletion, isPortDecl)...)
	}
	return items
}

func (c *completer) typesIn(mod, sortPref string) []protocol.CompletionItem {
	return c.declsIn(mod, constRank(sortPref), " ", protocol.StructCompletion, isTypeDecl)
}

func (c *completer) componentsIn(mod, sortPref string) []protocol.CompletionItem {
	return c.declsIn(mod, constRank(sortPref), "", protocol.StructCompletion, isComponentDecl)
}

func (c *completer) declsIn(mod string, sortOf func(string) string, suffix string, kind protocol.CompletionItemKind, want func(syntax.Node) bool) []protocol.CompletionItem {
	tree := c.treeByModule(mod)
	if tree == nil {
		log.Debugf("module %s not found\n", mod)
		return nil
	}
	return declItems(tree, sortOf, suffix, kind, want)
}

func declItems(tree *ttcn3.Tree, sortOf func(string) string, suffix string, kind protocol.CompletionItemKind, want func(syntax.Node) bool) []protocol.CompletionItem {
	var (
		items []protocol.CompletionItem
		modID string
	)
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		if m, ok := n.(*syntax.Module); ok {
			modID = syntax.Name(m)
			return true
		}
		if !want(n) {
			return true
		}
		name := syntax.Name(n)
		item := protocol.CompletionItem{
			Label:  name + suffix,
			Kind:   kind,
			Detail: modID + "." + name,
		}
		if sortOf != nil {
			if pref := sortOf(modID); pref != "" {
				item.SortText = pref + name
			}
		}
		items = append(items, item)
		return false
	})
	return items
}

func (c *completer) allValueDecls(kind syntax.Kind) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, tree := range c.trees() {
		items = append(items, valueDecls(tree, kind, true)...)
	}
	return items
}

func valueDecls(tree *ttcn3.Tree, kind syntax.Kind, withDetail bool) []protocol.CompletionItem {
	var (
		items []protocol.CompletionItem
		mod   string
	)
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		switch n := n.(type) {
		case *syntax.Module:
			mod = syntax.Name(n)
			return true
		case *syntax.FuncDecl, *syntax.ComponentTypeDecl:
			return false
		case *syntax.ValueDecl:
			return n.KindTok != nil && n.KindTok.Kind() == kind
		case *syntax.TemplateDecl:
			if kind != syntax.TEMPLATE {
				return false
			}
		case *syntax.Declarator:
		default:
			return true
		}
		name := syntax.Name(n)
		item := protocol.CompletionItem{Label: name, Kind: protocol.ConstantCompletion}
		if withDetail {
			item.Detail = mod + "." + name
		}
		items = append(items, item)
		return false
	})
	return items
}

func funcNames(tree *ttcn3.Tree, kind syntax.Kind) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if fn.KindTok != nil && fn.KindTok.Kind() == kind {
			items = append(items, protocol.CompletionItem{Label: syntax.Name(fn), Kind: protocol.FunctionCompletion})
		}
		return false
	})
	return items
}

func importTypeNames(tree *ttcn3.Tree) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		if !isTypeDecl(n) {
			return true
		}
		items = append(items, protocol.CompletionItem{Label: syntax.Name(n), Kind: protocol.StructCompletion})
		return false
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
		node := defs[0].Node
		if _, ok := node.(*syntax.ListSpec); ok {
			node, _ = actualType(c.suite.DB, defs[0])
		}
		return paramItems(node, 0, sortFirst, assign)
	default:
		return nil
	}
}

func paramItems(n syntax.Node, from int, sortPref string, assign bool) []protocol.CompletionItem {
	items := collectParams(n)
	if from >= 0 && from < len(items) {
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
			Documentation:    protocol.MarkupContent{Kind: "markdown", Value: fn.Documentation},
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

func templateKind(decl *syntax.TemplateDecl) syntax.Kind {
	if decl.RestrictionSpec != nil && decl.TemplateTok != nil {
		return decl.TemplateTok.Kind()
	}
	return syntax.TEMPLATE
}

func isComponentDecl(n syntax.Node) bool {
	_, ok := n.(*syntax.ComponentTypeDecl)
	return ok
}

func isPortDecl(n syntax.Node) bool {
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

func behaviourBody(nodes []syntax.Node) bool {
	inside := false
	for _, n := range nodes {
		switch n.(type) {
		case *syntax.FuncDecl:
			inside = true
		case *syntax.BlockStmt:
			if inside {
				return true
			}
		}
	}
	return false
}

func controlBody(nodes []syntax.Node) bool {
	inside := false
	for _, n := range nodes {
		switch n.(type) {
		case *syntax.ControlPart:
			inside = true
		case *syntax.BlockStmt:
			if inside {
				return true
			}
		}
	}
	return false
}

func constScope(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i > 0; i-- {
		if _, ok := nodes[i].(*syntax.ValueDecl); !ok {
			continue
		}
		if _, ok := nodes[i-1].(*syntax.ModuleDef); ok {
			return true
		}
	}
	return false
}

func templateScope(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i > 0; i-- {
		if _, ok := nodes[i].(*syntax.TemplateDecl); !ok {
			continue
		}
		if _, ok := nodes[i-1].(*syntax.ModuleDef); ok {
			return true
		}
	}
	return false
}

func valueDeclNode(nodes []syntax.Node) *syntax.ValueDecl {
	for _, n := range nodes {
		if v, ok := n.(*syntax.ValueDecl); ok {
			return v
		}
	}
	return nil
}

func templateDeclNode(nodes []syntax.Node) *syntax.TemplateDecl {
	for _, n := range nodes {
		if t, ok := n.(*syntax.TemplateDecl); ok {
			return t
		}
	}
	return nil
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
