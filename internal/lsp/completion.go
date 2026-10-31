package lsp

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/printer"
	"github.com/nokia/ntt/ttcn3/syntax"
)

type BehaviourInfo struct {
	Label         string
	Signature     string
	Documentation string
	HasRunsOn     bool
	HasReturn     bool
	HasParameters bool
	TextFormat    protocol.InsertTextFormat
}

type BehavAttrib int

// The list of behaviours.
const (
	NONE        BehavAttrib = iota // neither return nor runs on spec
	WITH_RETURN                    // only retrurn spec
	WITH_RUNSON                    // only runs on spec
)

var (
	moduleDefKw               = []string{"import from ", "type ", "const ", "modulepar ", "template ", "function ", "external function ", "altstep ", "testcase ", "control ", "signature "}
	importAfterModName        = []string{"all [except {}];", "{}"}
	importAfterModNameSnippet = []string{"${1:all${2: except {$3\\}}};$0", "{$0}"}
	importKinds               = []string{"type ", "const ", "modulepar ", "template ", "function ", "external function ", "altstep ", "testcase ", "control", "signature "}
	predefinedTypes           = []string{"anytype ", "bitstring ", "boolean ", "charstring ", "default ", "float ", "hexstring ", "integer ", "octetstring ", "universal charstring ", "verdicttype "}
)

func (s *Server) completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		log.Debugf("Completion took %s.\n", elapsed)
	}()
	defer func() {
		if err := recover(); err != nil {
			// in case of a panic, just continue as this might be a common situation during typing
			log.Debugf("Info: %s.\n", err)
		}
	}()
	if !params.TextDocument.URI.SpanURI().IsFile() {
		log.Printf("for 'code completion' the new file %q needs to be saved at least once", string(params.TextDocument.URI))
		return &protocol.CompletionList{}, nil
	}

	defaultModuleId := baseName(params.TextDocument.URI.SpanURI().Filename())

	tree := ttcn3.ParseFile(params.TextDocument.URI.SpanURI().Filename())
	if tree.Root == nil || len(tree.Modules()) == 0 {
		ret := []protocol.CompletionItem{{
			Label:            "module",
			InsertText:       "module ${1:" + defaultModuleId + "} {\n\t${0}\n}",
			InsertTextFormat: protocol.SnippetTextFormat,
			Kind:             protocol.KeywordCompletion,
		}}
		return &protocol.CompletionList{IsIncomplete: false, Items: ret}, nil
	}

	pos := tree.PosFor(int(params.TextDocumentPositionParams.Position.Line+1), int(params.TextDocumentPositionParams.Position.Character+1))
	nodeStack := LastNonWsToken(tree.Root, pos)
	if len(nodeStack) == 0 {
		return nil, nil
	}

	// NOTE: having the current file owned by more then one suite should not
	// import from modules originating from both suites. This would
	// in most ways end up with cyclic imports.
	// Thus 'completion' shall collect items only from one suite.
	// Decision: first suite
	suites := s.Owners(params.TextDocument.URI)
	if len(suites) == 0 {
		return nil, fmt.Errorf("no suite found for file %q", string(params.TextDocument.URI))
	}

	return &protocol.CompletionList{IsIncomplete: false, Items: Complete(suites[0], pos, nodeStack, defaultModuleId)}, nil
}

// LastNonWsToken returns the last node slice before the given position.
func LastNonWsToken(n syntax.Node, pos int) []syntax.Node {

	if pos < 0 {
		return nil
	}

	var (
		completed bool
		isError   bool

		nodeStack, lastStack []syntax.Node
	)

	nodeStack = append(nodeStack, n)
	n.Inspect(func(n syntax.Node) bool {
		if isError {
			return false
		}

		// called on node exit
		if n == nil {
			if len(nodeStack) > 0 {
				nodeStack = nodeStack[:len(nodeStack)-1]
			}
			return false
		}

		if errNode, ok := n.(*syntax.ErrorNode); ok {
			if errNode.LastTok().End() == pos {
				isError = true
				nodeStack = append(nodeStack, n)
				lastStack = make([]syntax.Node, len(nodeStack))
				copy(lastStack, nodeStack)
				return false
			}
		}

		// We don't need to descend any deeper if we're passt the
		// position.
		if pos < n.Pos() {
			completed = true
			return false
		}

		nodeStack = append(nodeStack, n)
		lastStack = make([]syntax.Node, len(nodeStack))
		copy(lastStack, nodeStack)
		return !completed
	})

	return lastStack
}

func Complete(suite *Suite, pos int, nodes []syntax.Node, ownModName string) []protocol.CompletionItem {
	var list []protocol.CompletionItem
	l := len(nodes)
	switch {
	case isBehaviourBodyScope(nodes):
		prevNode := nodes[l-2]

		modName := ""
		if n, ok := prevNode.(*syntax.SelectorExpr); ok {
			if n.X == nil {
				return nil
			}
			modName = n.X.LastTok().String()
		}

		kinds := []syntax.Kind{syntax.FUNCTION}
		attrs := []BehavAttrib{WITH_RETURN}

		switch {
		case isStartOpArgument(nodes):
			attrs = append(attrs, WITH_RUNSON)
		case !isInsideExpression(nodes, modName != ""): // less restrictive than isStartOpArgument
			kinds = append(kinds, syntax.ALTSTEP)
			attrs = append(attrs, NONE, WITH_RUNSON)
		}

		if modName != "" {
			file, err := suite.FindModule(modName)
			if err == nil {
				tree := ttcn3.ParseFile(file)
				return CompleteBehaviours(tree, kinds, attrs, modName, " 1")
			}

			expr := prevNode.(*syntax.SelectorExpr)
			var tgt syntax.Expr
			if expr.Sel != nil && expr.Sel.LastTok().Pos() < pos {
				tgt = expr
			} else {
				tgt = expr.X
			}
			if n, ok := tgt.(*syntax.SelectorExpr); ok && pos == tgt.End() {
				tgt = n.X
			}
			return CompleteSelectorExpr(suite, ownModName, tgt)
		} else if n, ok := prevNode.(*syntax.IndexExpr); ok {
			return CompleteSelectorExpr(suite, ownModName, n)
		}

		if isArgListScope(nodes, pos) {
			list = CompleteArgNames(suite, ownModName, nodes[l-2].(syntax.Expr), " 1")
			if n, ok := nodes[l-2].(*syntax.BinaryExpr); ok && pos > n.Op.FirstTok().End() {
				return list
			}
		}
		list = append(list, CompleteAllLocalVariables(nodes, " 2")...)
		list = append(list, CompleteAllBehaviours(suite, kinds, attrs, ownModName)...)
		list = append(list, CompletePredefinedFunctions()...)
		list = append(list, CompleteAllModules(suite, ownModName, " 4")...)
		return list

	case isControlBodyScope(nodes):
		kinds := []syntax.Kind{syntax.FUNCTION}
		attrs := []BehavAttrib{WITH_RETURN}

		if n, ok := nodes[l-2].(*syntax.SelectorExpr); ok {
			if n.X == nil {
				return nil
			}
			modName := n.X.LastTok().String()

			if !isInsideExpression(nodes, true) { // less restrictive than isStartOpArgument
				attrs = append(attrs, NONE)
			}
			file, err := suite.FindModule(modName)
			if err != nil {
				log.Debugf("module %s not found\n", modName)
				return nil
			}
			tree := ttcn3.ParseFile(file)
			return CompleteBehaviours(tree, kinds, attrs, modName, " 1")
		}

		if !isInsideExpression(nodes, false) { // less restrictive than isStartOpArgument
			attrs = append(attrs, NONE, WITH_RUNSON)
		}

		list = CompleteAllBehaviours(suite, kinds, attrs, ownModName)
		list = append(list, CompletePredefinedFunctions()...)
		list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
		return list

	case isConstDeclScope(nodes):
		n := getConstDeclNode(nodes)
		if n == nil {
			break
		}

		scndNode, _ := nodes[l-2].(*syntax.SelectorExpr)

		switch {
		case n.Type == nil || (n.Type != nil && (n.Type.Pos() > pos)):
			if scndNode != nil && scndNode.X != nil {
				// NOTE: the parser produces a wrong ast under certain circumstances
				// see: func TestTemplateModuleDotType(t *testing.T)
				return CompleteTypes(suite, scndNode.X.LastTok().String(), "")
			}
			list = CompleteAllTypes(suite, ownModName)
			list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
			return list
		case scndNode != nil && scndNode.X != nil:
			modName := scndNode.X.LastTok().String()
			file, err := suite.FindModule(modName)
			if err != nil {
				log.Debugf("module %s not found\n", modName)
				return nil
			}
			tree := ttcn3.ParseFile(file)
			return CompleteBehaviours(tree, []syntax.Kind{syntax.FUNCTION},
				[]BehavAttrib{WITH_RETURN}, modName, " 1")
		default:
			list = CompleteAllBehaviours(suite, []syntax.Kind{syntax.FUNCTION},
				[]BehavAttrib{WITH_RETURN}, ownModName)
			list = append(list, CompletePredefinedFunctions()...)
			list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
			return list
		}
	case isTemplateDeclScope(nodes):
		n := getTemplateDeclNode(nodes)
		if n == nil {
			return nil
		}

		scndNode, _ := nodes[l-2].(*syntax.SelectorExpr)

		switch {
		case n.ModifiesTok != nil && n.AssignTok.Pos() > pos:
			if scndNode != nil && scndNode.X != nil {
				mname := scndNode.X.LastTok().String()
				file, err := suite.FindModule(mname)
				if err != nil {
					log.Debugf("module %s not found\n", mname)
					return nil
				}
				tree := ttcn3.ParseFile(file)
				list = CompleteValueDecls(tree, n.TemplateTok.Kind(), false)
				return list
			}
			list = CompleteAllValueDecls(suite, syntax.TEMPLATE)
			list = append(list, CompleteAllModules(suite, ownModName, "")...)
			return list

		case n.Type == nil || n.Name == nil || (n.Name != nil && (n.Name.Pos() > pos)):
			if scndNode != nil && scndNode.X != nil {
				// NOTE: the parser produces a wrong ast under certain circumstances
				// see: func TestTemplateModuleDotType(t *testing.T)
				return CompleteTypes(suite, scndNode.X.LastTok().String(), "")
			}
			list = CompleteAllTypes(suite, ownModName)
			list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
			return list
		default:
			switch {
			case scndNode != nil && scndNode.X != nil:
				modName := scndNode.X.LastTok().String()
				file, err := suite.FindModule(modName)
				if err != nil {
					log.Debugf("module %s not found\n", modName)
					return nil
				}
				tree := ttcn3.ParseFile(file)
				list = CompleteBehaviours(tree, []syntax.Kind{syntax.FUNCTION},
					[]BehavAttrib{WITH_RETURN}, modName, " 1")
				return list
			default:
				list = CompleteAllBehaviours(suite, []syntax.Kind{syntax.FUNCTION},
					[]BehavAttrib{WITH_RETURN}, ownModName)
				list = append(list, CompletePredefinedFunctions()...)
				list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
				return list
			}
		}
	default:
		switch n := nodes[l-1].(type) {
		case *syntax.Ident:
			if _, ok := nodes[0].(*syntax.Module); l == 2 && ok {
				return CompleteModuleDefKeywords()
			}
			if l <= 2 {
				return nil
			}

			switch scndNode := nodes[l-2].(type) {
			case *syntax.ModuleDef:
				return CompleteModuleDefKeywords()
			case *syntax.ImportDecl:
				switch {
				case scndNode.LBrace != nil:
					return CompleteImportkinds()
				case n.End() >= pos:
					// look for available modules for import
					return CompleteAllModules(suite, ownModName, " ")
				default:
					return CompleteImportAllKeyword()
				}
			case *syntax.DefKindExpr:
				// happens after
				// * the altstep/function/testcase kw while typing the identifier
				// * inside the exception list after { while typing the kind
				if l == 8 {
					if _, ok := nodes[l-3].(*syntax.ExceptExpr); ok {
						if scndNode.KindTok != nil {
							if impDecl, ok := nodes[l-5].(*syntax.ImportDecl); ok {
								return CompleteImportSpecs(suite, scndNode.KindTok.Kind(), impDecl.Module.Tok.String())
							}
						}
						return CompleteImportkinds()
					}
					break
				}
				if impDecl, ok := nodes[l-3].(*syntax.ImportDecl); ok {
					return CompleteImportSpecs(suite, scndNode.KindTok.Kind(), impDecl.Module.Tok.String())
				}

			case *syntax.ExceptExpr:
				return CompleteImportkinds()
			case *syntax.RunsOnSpec, *syntax.SystemSpec:
				list = CompleteAllComponentTypes(suite, " 1")
				list = append(list, CompleteAllModules(suite, ownModName, " 2")...)
				return list
			case *syntax.SelectorExpr:
				if scndNode.X != nil {
					switch nodes[l-3].(type) {
					case *syntax.RunsOnSpec, *syntax.SystemSpec, *syntax.ComponentTypeDecl:
						return CompleteComponentTypes(suite, scndNode.X.LastTok().String(), " 1")
					}
				}
			case *syntax.ComponentTypeDecl:
				// for ctrl+spc, after beginning to type an id after extends Token
				if scndNode.ExtendsTok.LastTok() != nil && scndNode.Body.LBrace.Pos() > pos {
					list = CompleteAllComponentTypes(suite, " 1")
					list = append(list, CompleteAllModules(suite, ownModName, " 2")...)
					return list
				}
			}

		case *syntax.ImportDecl:
			if n.Module == nil {
				// look for available modules for import
				return CompleteAllModules(suite, ownModName, " ")
			}
		case *syntax.DefKindExpr:
			if n.KindTok == nil {
				return CompleteImportkinds()
			}
			if impDecl, ok := nodes[l-2].(*syntax.ImportDecl); ok {
				return CompleteImportSpecs(suite, n.KindTok.Kind(), impDecl.Module.Tok.String())
			}
			if _, ok := nodes[l-2].(*syntax.ExceptExpr); ok {
				if impDecl, ok := nodes[l-4].(*syntax.ImportDecl); ok {
					return CompleteImportSpecs(suite, n.KindTok.Kind(), impDecl.Module.Tok.String())
				}
			}
		case *syntax.RunsOnSpec, *syntax.SystemSpec:
			list = CompleteAllComponentTypes(suite, " 1")
			list = append(list, CompleteAllModules(suite, ownModName, " 2")...)
			return list
		case *syntax.ErrorNode:
			// i.e. user started typing => syntax.Ident might be detected instead of a kw
			if l <= 1 {
				return nil
			}
			switch scndNode := nodes[l-2].(type) {
			case *syntax.ModuleDef:
				// start a new module def
				return CompleteModuleDefKeywords()
			case *syntax.DefKindExpr:
				// NOTE: not able to reproduce this situation. Maybe it is safe to remove this code.
				// happens streight after the altstep kw if ctrl+space is pressed
				if impDecl, ok := nodes[l-3].(*syntax.ImportDecl); ok {
					return CompleteImportSpecs(suite, scndNode.KindTok.Kind(), impDecl.Module.Tok.String())
				}
				if _, ok := nodes[l-3].(*syntax.ExceptExpr); ok {
					if impDecl, ok := nodes[l-5].(*syntax.ImportDecl); ok {
						return CompleteImportSpecs(suite, scndNode.KindTok.Kind(), impDecl.Module.Tok.String())
					}
				}
			}
		case *syntax.Declarator:
			if l <= 2 {
				return nil
			}
			if valueDecl, ok := nodes[l-2].(*syntax.ValueDecl); ok {
				if valueDecl.KindTok.Kind() == syntax.PORT {
					list = CompleteAllPortTypes(suite, ownModName)
					list = append(list, CompleteAllModules(suite, ownModName, " 3")...)
					return list
				}
			}
		default:
			log.Debugf("Node not considered yet: %#v\n", n)
		}
	}
	return list
}
func CompletePredefinedFunctions() []protocol.CompletionItem {
	ret := make([]protocol.CompletionItem, 0, len(PredefinedFunctions))
	for _, v := range PredefinedFunctions {
		markup := protocol.MarkupContent{Kind: "markdown", Value: v.Documentation}
		ret = append(ret, protocol.CompletionItem{
			Label: v.Label, Kind: protocol.FunctionCompletion,
			Detail:           v.Signature,
			InsertTextFormat: v.TextFormat,
			InsertText:       v.InsertText,
			Documentation:    markup})
	}
	return ret
}

func CompleteImportkinds() []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, v := range importKinds {
		ret = append(ret, protocol.CompletionItem{Label: v, Kind: protocol.KeywordCompletion})
	}
	return ret
}

func CompletePredefinedTypes() []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, v := range predefinedTypes {
		ret = append(ret, protocol.CompletionItem{Label: v, Kind: protocol.KeywordCompletion})
	}
	return ret
}

func CompleteBehaviours(tree *ttcn3.Tree, kinds []syntax.Kind, attribs []BehavAttrib, mname string, sortPref string) []protocol.CompletionItem {
	var (
		items []*BehaviourInfo
		ret   []protocol.CompletionItem
	)

	for _, kind := range kinds {
		items = append(items, behaviourInfos(tree, kind)...)
	}

	for _, v := range items {
		insertText := v.Label + "()"
		if v.HasParameters {
			insertText = v.Label + "($1)$0"
		}
		isSelected := false
		for _, attrib := range attribs {
			switch attrib {
			case NONE:
				isSelected = isSelected || !(v.HasReturn || v.HasRunsOn)
			case WITH_RETURN:
				isSelected = isSelected || v.HasReturn
			case WITH_RUNSON:
				isSelected = isSelected || v.HasRunsOn
			}
		}
		if isSelected {
			ret = append(ret, protocol.CompletionItem{
				Label:            v.Label + "()",
				InsertText:       insertText,
				Kind:             protocol.FunctionCompletion,
				SortText:         sortPref + v.Label,
				Detail:           v.Signature,
				Documentation:    v.Documentation,
				InsertTextFormat: v.TextFormat,
			})
		}
	}
	return ret
}

func CompleteAllBehaviours(suite *Suite, kinds []syntax.Kind, attribs []BehavAttrib, mname string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		fileName := baseName(f)
		file, err := suite.FindModule(fileName)
		if err != nil {
			log.Debugf("module %s not found\n", mname)
			return nil
		}
		tree := ttcn3.ParseFile(file)

		sortPref := " 2"
		if fileName == mname {
			sortPref = " 1"
		}
		ret = append(ret, CompleteBehaviours(tree, kinds, attribs, fileName, sortPref)...)
	}
	return ret
}

func CompleteValueDecls(tree *ttcn3.Tree, kind syntax.Kind, withDetail bool) []protocol.CompletionItem {
	var (
		ret   []protocol.CompletionItem
		mname string
	)
	tree.Inspect(func(n syntax.Node) bool {
		if n == nil {
			// called on node exit
			return false
		}

		switch node := n.(type) {
		case *syntax.Module:
			mname = node.Name.String()
			return true
		case *syntax.FuncDecl, *syntax.ComponentTypeDecl:
			// do not descent into TESTCASE, FUNCTION, ALTSTEP,
			// component type
			return false
		case *syntax.ValueDecl:
			if node.KindTok.Kind() != kind {
				return false
			}
			return true
		case *syntax.Declarator:
			break
		case *syntax.TemplateDecl:
			if kind == syntax.TEMPLATE {
				break
			}
			return false
		default:
			return true
		}

		v := syntax.Name(n)
		item := protocol.CompletionItem{
			Label: v,
			Kind:  protocol.ConstantCompletion,
		}
		if withDetail {
			item.Detail = joinNames(mname, v)
		}
		ret = append(ret, item)
		return false
	})
	return ret
}

func CompleteModuleDefKeywords() []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, v := range moduleDefKw {
		ret = append(ret, protocol.CompletionItem{Label: v, Kind: protocol.KeywordCompletion})
	}
	return ret
}

func CompleteImportAllKeyword() []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for i, v := range importAfterModName {
		ret = append(ret, protocol.CompletionItem{Label: v,
			InsertText:       importAfterModNameSnippet[i],
			InsertTextFormat: protocol.SnippetTextFormat, Kind: protocol.KeywordCompletion})
	}
	return ret
}

func CompleteImportSpecs(suite *Suite, kind syntax.Kind, mname string) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}
	tree := ttcn3.ParseFile(file)

	switch kind {
	case syntax.ALTSTEP, syntax.FUNCTION, syntax.TESTCASE:
		var ret []protocol.CompletionItem
		tree.Inspect(func(n syntax.Node) bool {
			f, ok := n.(*syntax.FuncDecl)
			if !ok {
				return true
			}

			if f.KindTok.Kind() == kind {
				ret = append(ret, protocol.CompletionItem{Label: syntax.Name(f), Kind: protocol.FunctionCompletion})
			}
			return false
		})
		ret = append(ret, protocol.CompletionItem{Label: "all;", Kind: protocol.KeywordCompletion})
		return ret

	case syntax.TEMPLATE, syntax.CONST, syntax.MODULEPAR:
		ret := CompleteValueDecls(tree, kind, false)
		ret = append(ret, protocol.CompletionItem{Label: "all;", Kind: protocol.KeywordCompletion})
		return ret

	case syntax.TYPE:
		var ret []protocol.CompletionItem
		tree.Inspect(func(n syntax.Node) bool {
			if !isType(n) {
				return true
			}
			// NOTE: instead of 'StructCompletion' a better matching kind could be used
			ret = append(ret, protocol.CompletionItem{Label: syntax.Name(n), Kind: protocol.StructCompletion})
			return false
		})
		ret = append(ret, protocol.CompletionItem{Label: "all;", Kind: protocol.KeywordCompletion})
		return ret

	default:
		log.Debugf("Kind not considered yet: %#v\n", kind)
		return nil
	}
}

func CompleteAllModules(suite *Suite, ownModName string, sortPref string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		fileName := baseName(f)
		if fileName == ownModName {
			continue
		}
		item := protocol.CompletionItem{
			Label: fileName,
			Kind:  protocol.ModuleCompletion,
		}
		if sortPref != "" {
			item.SortText = sortPref + fileName
		}
		ret = append(ret, item)
	}
	return ret
}

func CompleteTypes(suite *Suite, mname string, sortPref string) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}

	var ret []protocol.CompletionItem
	tree := ttcn3.ParseFile(file)
	tree.Inspect(func(n syntax.Node) bool {
		if mod, ok := n.(*syntax.Module); ok {
			mname = mod.Name.String()
		}
		if !isType(n) {
			return true
		}
		v := syntax.Name(n)
		item := protocol.CompletionItem{
			Label:  v + " ",
			Kind:   protocol.StructCompletion, // NOTE: instead of 'StructCompletion' a better matching kind could be used
			Detail: joinNames(mname, v),
		}
		if sortPref != "" {
			item.SortText = sortPref + v
		}
		ret = append(ret, item)
		return false
	})
	return ret
}

func CompleteComponentTypes(suite *Suite, mname string, sortPref string) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}

	var ret []protocol.CompletionItem
	tree := ttcn3.ParseFile(file)
	tree.Inspect(func(n syntax.Node) bool {
		if mod, ok := n.(*syntax.Module); ok {
			mname = mod.Name.String()
		}
		if _, ok := n.(*syntax.ComponentTypeDecl); !ok {
			return true
		}
		v := syntax.Name(n)
		item := protocol.CompletionItem{
			Label:  v,
			Kind:   protocol.StructCompletion,
			Detail: joinNames(mname, v),
		}
		if sortPref != "" {
			item.SortText = sortPref + v
		}
		ret = append(ret, item)
		return false
	})
	return ret
}

func CompletePortTypes(suite *Suite, mname string, sortPref string) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}

	var ret []protocol.CompletionItem
	tree := ttcn3.ParseFile(file)
	tree.Inspect(func(n syntax.Node) bool {
		if mod, ok := n.(*syntax.Module); ok {
			mname = mod.Name.String()
		}
		if _, ok := n.(*syntax.PortTypeDecl); !ok {
			return true
		}
		v := syntax.Name(n)
		item := protocol.CompletionItem{
			Label:  v,
			Kind:   protocol.InterfaceCompletion,
			Detail: joinNames(mname, v),
		}
		if sortPref != "" {
			item.SortText = sortPref + v
		}
		ret = append(ret, item)
		return false
	})
	return ret
}

func CollectParams(n syntax.Node) []protocol.CompletionItem {
	var list []protocol.CompletionItem

	switch n := n.(type) {
	case *syntax.FormalPars:
		for _, par := range n.List {
			list = append(list, protocol.CompletionItem{Label: par.Name.String(), Detail: syntax.Name(par.Type)})
		}
	case *syntax.StructTypeDecl:
		for _, field := range n.Fields {
			if name := syntax.Name(field); name != "" {
				list = append(list, protocol.CompletionItem{Label: name, Detail: syntax.Name(field.Type)})
			}
		}
	}

	return list
}

func CompleteParams(tgt syntax.Node, cur int, sortPref string, completeAssign bool) []protocol.CompletionItem {
	var insertText string
	if completeAssign {
		insertText = "%s := ${1:}"
	} else {
		insertText = "%s"
	}

	items := CollectParams(tgt)
	if len(items) > cur {
		items = items[cur:]
	}
	for i := range items {
		items[i].Documentation = ""
		items[i].Kind = protocol.EnumMemberCompletion
		items[i].SortText = sortPref + items[i].Label
		items[i].InsertText = fmt.Sprintf(insertText, items[i].Label)
		items[i].InsertTextFormat = protocol.SnippetTextFormat
	}

	return items
}

func CompleteArgNames(suite *Suite, mname string, n syntax.Expr, sortPref string) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}
	tree := ttcn3.ParseFile(file)

	completeAssignOperator := true
	if _, ok := n.(*syntax.BinaryExpr); ok {
		n = tree.ParentOf(n).(syntax.Expr)
		completeAssignOperator = false
	}

	if _, ok := n.(*syntax.ParenExpr); ok {
		n = tree.ParentOf(n).(syntax.Expr)
	}

	switch n := n.(type) {
	case *syntax.CallExpr:
		if defs := tree.LookupWithDB(n.Fun, suite.DB); len(defs) > 0 {
			def := defs[0]
			if params := getDeclarationParams(def.Node); params != nil {
				return CompleteParams(params, len(n.Args.List)-1, sortPref, completeAssignOperator)
			}
		}
	case *syntax.CompositeLiteral:
		defs := tree.TypeOf(n, suite.DB)
		if len(defs) > 0 {
			def := defs[0]
			n := def.Node
			if _, ok := n.(*syntax.ListSpec); ok {
				n, _ = ExtractActualType(def.Tree, suite.DB, n, 0)
			}
			return CompleteParams(n, 0, sortPref, completeAssignOperator)
		}
	}

	return nil
}

func CompleteFields(suite *Suite, tree *ttcn3.Tree, wtype syntax.Node, depth int) []protocol.CompletionItem {
	var ret []protocol.CompletionItem

	typ, typeDepth := ExtractActualType(tree, suite.DB, wtype, 0)

	switch n := typ.(type) {
	case *syntax.StructTypeDecl:
		if typeDepth == depth {
			for _, field := range n.Fields {
				name := syntax.Name(field)
				if name == "" {
					continue
				}
				ret = append(ret, protocol.CompletionItem{
					Label:         name,
					Kind:          protocol.FieldCompletion,
					Detail:        syntax.Name(field.Type),
					Documentation: "",
				})
			}
		}
	}

	return ret
}

func ExtractActualType(tree *ttcn3.Tree, db *ttcn3.DB, t syntax.Node, depth int) (syntax.Node, int) {
	q := []*ttcn3.Node{{Node: t, Tree: tree}}

	for len(q) > 0 {
		c := q[0]
		q = q[1:]

		switch n := c.Node.(type) {
		case *syntax.RefSpec:
			if defs := tree.TypeOf(n, db); len(defs) > 0 {
				def := defs[0]
				if def.Node != t {
					q = append(q, def)
					continue
				}
			}
		case *syntax.ListSpec:
			q = append(q, &ttcn3.Node{Node: n.ElemType, Tree: tree})
			depth++
		default:
			return n, depth
		}
	}

	return nil, -1
}

func CompleteSelectorExpr(suite *Suite, mname string, expr syntax.Expr) []protocol.CompletionItem {
	file, err := suite.FindModule(mname)
	if err != nil {
		log.Debugf("module %s not found\n", mname)
		return nil
	}

	depth := 0
	for {
		if n, ok := expr.(*syntax.IndexExpr); ok {
			expr = n.X
			depth++
			continue
		}
		break
	}

	tree := ttcn3.ParseFile(file)
	defs := tree.TypeOf(expr, suite.DB)
	if len(defs) == 0 {
		return nil
	}

	return CompleteFields(suite, tree, defs[0].Node, depth)
}

func CompleteAllComponentTypes(suite *Suite, sortPref string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		items := CompleteComponentTypes(suite, baseName(f), sortPref)
		ret = append(ret, items...)
	}
	return ret
}

func CompleteAllPortTypes(suite *Suite, ownModName string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		mName := baseName(f)
		prefix := " 2"
		if mName == ownModName {
			prefix = " 1"
		}
		items := CompletePortTypes(suite, mName, prefix)
		ret = append(ret, items...)
	}
	return ret
}

func CompleteAllTypes(suite *Suite, ownModName string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		mName := baseName(f)
		prefix := " 2"
		if mName == ownModName {
			prefix = " 1"
		}
		items := CompleteTypes(suite, mName, prefix)
		ret = append(ret, items...)
	}
	ret = append(ret, CompletePredefinedTypes()...)
	return ret
}

func CompleteAllValueDecls(suite *Suite, kind syntax.Kind) []protocol.CompletionItem {
	var ret []protocol.CompletionItem
	for _, f := range suite.Files() {
		mname := baseName(f)
		file, err := suite.FindModule(mname)
		if err != nil {
			log.Debugf("module %s not found\n", mname)
			return nil
		}
		tree := ttcn3.ParseFile(file)
		items := CompleteValueDecls(tree, kind, true)
		ret = append(ret, items...)
	}
	return ret
}

func CompleteAllLocalVariables(nodes []syntax.Node, sortPref string) []protocol.CompletionItem {
	var ret []protocol.CompletionItem

	var funcDecl *syntax.FuncDecl = nil
	for _, n := range nodes {
		if n, ok := n.(*syntax.FuncDecl); ok {
			funcDecl = n
			break
		}
	}
	if funcDecl == nil {
		return nil
	}

	funcDecl.Inspect(func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		switch n := n.(type) {
		case *syntax.Declarator:
			name := n.Name.String()
			ret = append(ret, protocol.CompletionItem{
				Label:    name,
				Kind:     protocol.VariableCompletion,
				SortText: sortPref + name,
			})
		default:
			return true
		}
		return false
	})

	return ret
}

func behaviourInfos(tree *ttcn3.Tree, kind syntax.Kind) []*BehaviourInfo {
	var ret []*BehaviourInfo
	mname := ""
	tree.Inspect(func(n syntax.Node) bool {
		if mod, ok := n.(*syntax.Module); ok {
			mname = mod.Name.String()
		}
		node, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}

		if node.KindTok.Kind() != kind {
			return false
		}

		var sig bytes.Buffer
		textFormat := protocol.PlainTextTextFormat
		sig.WriteString(node.KindTok.String() + " " + joinNames(mname, node.Name.String()))
		len1 := len(sig.String())
		printer.Print(&sig, node.Params)
		hasParams := (len(sig.String()) - len1) > 2
		if hasParams {
			textFormat = protocol.SnippetTextFormat
		}
		if node.RunsOn != nil {
			sig.WriteString("\n  ")
			printer.Print(&sig, node.RunsOn)
		}
		if node.System != nil {
			sig.WriteString("\n  ")
			printer.Print(&sig, node.System)
		}
		if node.Return != nil {
			sig.WriteString("\n  ")
			printer.Print(&sig, node.Return)
		}
		ret = append(ret, &BehaviourInfo{
			Label:         node.Name.String(),
			HasRunsOn:     (node.RunsOn != nil),
			HasReturn:     (node.Return != nil),
			Signature:     sig.String(),
			Documentation: syntax.Doc(node),
			HasParameters: hasParams,
			TextFormat:    textFormat})
		return false

	})
	return ret
}

func joinNames(s string, ss ...string) string {
	if len(ss) == 0 {
		return s
	}
	return s + "." + strings.Join(ss, ".")
}

func isType(n syntax.Node) bool {
	switch n.(type) {
	case *syntax.BehaviourTypeDecl,
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

func baseName(name string) string {
	name = filepath.Base(name)
	name = name[:len(name)-len(filepath.Ext(name))]
	return name
}

func isArgListScope(nodes []syntax.Node, pos int) bool {
	l := len(nodes)
	off := 2

	if n, ok := nodes[l-off].(*syntax.BinaryExpr); ok {
		if n.Op.FirstTok().Pos() < pos {
			return false
		}
		if n.Op.String() == ":=" {
			off++
		}
	}

	if _, ok := nodes[l-off].(*syntax.ParenExpr); ok {
		if _, ok := nodes[l-off-1].(*syntax.CallExpr); ok {
			return true
		}
	} else if _, ok := nodes[l-off].(*syntax.CompositeLiteral); ok {
		return true
	}

	return false
}

func isBehaviourBodyScope(nodes []syntax.Node) bool {
	insideBehav := false
	for _, node := range nodes {
		switch node.(type) {
		case *syntax.FuncDecl:
			insideBehav = true
		case *syntax.BlockStmt:
			if insideBehav {
				return true
			}
		}
	}
	return false
}
func isControlBodyScope(nodes []syntax.Node) bool {
	insideControl := false
	for _, node := range nodes {
		switch node.(type) {
		case *syntax.ControlPart:
			insideControl = true
		case *syntax.BlockStmt:
			if insideControl {
				return true
			}
		}
	}
	return false
}

func isConstDeclScope(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i > 0; i-- {
		if _, ok := nodes[i].(*syntax.ValueDecl); ok {
			if _, ok := nodes[i-1].(*syntax.ModuleDef); ok {
				return true
			}
		}
	}
	return false
}

func getConstDeclNode(nodes []syntax.Node) *syntax.ValueDecl {
	for _, n := range nodes {
		if val, ok := n.(*syntax.ValueDecl); ok {
			return val
		}
	}
	return nil
}

func isTemplateDeclScope(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i > 0; i-- {
		if _, ok := nodes[i].(*syntax.TemplateDecl); ok {
			if _, ok := nodes[i-1].(*syntax.ModuleDef); ok {
				return true
			}
		}
	}
	return false
}

func getTemplateDeclNode(nodes []syntax.Node) *syntax.TemplateDecl {
	for _, n := range nodes {
		if templ, ok := n.(*syntax.TemplateDecl); ok {
			return templ
		}
	}
	return nil
}

func isInsideExpression(nodes []syntax.Node, fromModuleDot bool) bool {
	i := len(nodes)
	if i > 1 {
		i--
		if _, ok := nodes[i].(*syntax.Ident); ok {
			i--
		}
		if fromModuleDot {
			if i == 0 {
				return false
			}
			i--
		}
		switch nodes[i].(type) {
		case *syntax.SelectorExpr:
			return false
		case *syntax.ExprStmt:
			return false
		case *syntax.BlockStmt:
			return false
		default:
			return true
		}
	}
	return false
}

func isStartOpArgument(nodes []syntax.Node) bool {
	for i := len(nodes) - 1; i >= 0; i-- {
		if _, ok := nodes[i].(*syntax.ParenExpr); ok {
			if i >= 1 {
				if n, ok := nodes[i-1].(*syntax.CallExpr); ok {
					if fun, ok := n.Fun.(*syntax.SelectorExpr); ok {
						return syntax.Name(fun.Sel) == "start"
					}
					return false
				}
			}
		}
	}
	return false
}
