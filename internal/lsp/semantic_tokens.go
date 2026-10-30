package lsp

import (
	"context"
	"fmt"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

type SemanticTokenType uint32

const (
	Namespace SemanticTokenType = iota
	/**
	 * Represents a generic type. Acts as a fallback for types which
	 * can't be mapped to a specific type like class or enum.
	 */
	Type
	Class
	Enum
	Interface
	Struct
	Map
	TypeParameter
	Parameter
	Variable
	Property
	EnumMember
	Event
	Function
	Method
	Macro
	Keyword
	Modifier
	Comment
	String
	Number
	Regexp
	Operator
	// Not part of the LSP
	None
)

var TokenTypes = []string{
	Namespace:     "namespace",
	Type:          "type",
	Class:         "class",
	Enum:          "enum",
	Interface:     "interface",
	Struct:        "struct",
	Map:           "map",
	TypeParameter: "typeParameter",
	Parameter:     "parameter",
	Variable:      "variable",
	Property:      "property",
	EnumMember:    "enumMember",
	Event:         "event",
	Function:      "function",
	Method:        "method",
	Macro:         "macro",
	Keyword:       "keyword",
	Modifier:      "modifier",
	Comment:       "comment",
	String:        "string",
	Number:        "number",
	Regexp:        "regexp",
	Operator:      "operator",
}

type SemanticTokenModifiers uint32

const (
	Undefined   SemanticTokenModifiers = 0
	Declaration SemanticTokenModifiers = 1 << iota
	Definition
	Readonly
	Static
	Deprecated
	Abstract
	Async
	Modification
	Documentation
	DefaultLibrary
)

var TokenModifiers = []string{
	"undefined",
	"declaration",
	"definition",
	"readonly",
	"static",
	"deprecated",
	"abstract",
	"async",
	"modification",
	"documentation",
	"defaultLibrary",
}

var builtins = map[string]SemanticTokenType{
	"any2unistr":           Function,
	"anytype":              Type,
	"bit2hex":              Function,
	"bit2int":              Function,
	"bit2oct":              Function,
	"bit2str":              Function,
	"bitstring":            Type,
	"boolean":              Type,
	"char2int":             Function,
	"char2oct":             Function,
	"charstring":           Type,
	"decvalue":             Function,
	"decvalue_o":           Function,
	"decvalue_unichar":     Function,
	"default":              Type,
	"encvalue":             Function,
	"encvalue_o":           Function,
	"encvalue_unichar":     Function,
	"enum2int":             Function,
	"float":                Type,
	"float2int":            Function,
	"get_stringencoding":   Function,
	"hex2bit":              Function,
	"hex2int":              Function,
	"hex2oct":              Function,
	"hex2str":              Function,
	"hexstring":            Type,
	"hostid":               Function,
	"int2bit":              Function,
	"int2char":             Function,
	"int2enum":             Function,
	"int2float":            Function,
	"int2hex":              Function,
	"int2oct":              Function,
	"int2str":              Function,
	"int2unichar":          Function,
	"integer":              Type,
	"isbound":              Function,
	"ischosen":             Function,
	"ispresent":            Function,
	"istemplatekind":       Function,
	"isvalue":              Function,
	"lengthof":             Function,
	"log":                  Function,
	"match":                Function,
	"oct2bit":              Function,
	"oct2char":             Function,
	"oct2hex":              Function,
	"oct2int":              Function,
	"oct2str":              Function,
	"oct2unichar":          Function,
	"octetstring":          Type,
	"regexp":               Function,
	"remove_bom":           Function,
	"replace":              Function,
	"rnd":                  Function,
	"setverdict":           Function,
	"sizeof":               Function,
	"str2float":            Function,
	"str2hex":              Function,
	"str2int":              Function,
	"str2oct":              Function,
	"substr":               Function,
	"testcasename":         Function,
	"unichar2int":          Function,
	"unichar2oct":          Function,
	"universal charstring": Type,
	"verdicttype":          Type,
}

func (s *Server) semanticTokens(ctx context.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	if !s.serverConfig.SemantikTokensEnabled {
		return nil, nil
	}

	file := string(params.TextDocument.URI)
	tree := ttcn3.ParseFile(file)
	begin := tree.Pos()
	end := tree.End()
	return s.semanticTokensRecover(tree, &s.db, begin, end)
}

func (s *Server) semanticTokensRange(ctx context.Context, params *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	if !s.serverConfig.SemantikTokensEnabled {
		return nil, nil
	}

	file := string(params.TextDocument.URI)
	tree := ttcn3.ParseFile(file)
	begin := tree.PosFor(int(params.Range.Start.Line)+1, int(params.Range.Start.Character+1))
	end := tree.PosFor(int(params.Range.End.Line+1), int(params.Range.End.Character+1))
	return s.semanticTokensRecover(tree, &s.db, begin, end)
}

func (s *Server) semanticTokensRecover(tree *ttcn3.Tree, db *ttcn3.DB, begin int, end int) (*protocol.SemanticTokens, error) {
	start := time.Now()
	defer func() {
		// In case of a panic, just continue as this might be a common situation during typing
		if err := recover(); err != nil {
			log.Debugf("Recovered from panic: %v\n", err)
		}
		log.Debug(fmt.Sprintf("SemanticTokens for %s took %s.", tree.Filename(), time.Since(start)))
	}()

	return &protocol.SemanticTokens{Data: SemanticTokens(tree, db, begin, end)}, nil
}

func SemanticTokens(tree *ttcn3.Tree, db *ttcn3.DB, begin int, end int) []uint32 {
	var tokens []uint32
	line := 0
	col := 0
	tree.Inspect(func(n syntax.Node) bool {
		if n == nil || n.End() < begin || end < n.Pos() {
			return false
		}

		if id, ok := n.(*syntax.Ident); ok {

			// token type and modifiers
			var (
				typ SemanticTokenType
				mod SemanticTokenModifiers
			)
			if id.IsName {
				typ, mod = DefinitionToken(tree, id)
			} else {
				typ, mod = ReferenceToken(tree, db, id)
			}

			if typ != None {
				tokens, line, col = appendToken(tokens, id.Tok, tree, typ, mod, line, col)
				if id.Tok2 != nil {
					tokens, line, col = appendToken(tokens, id.Tok2, tree, typ, mod, line, col)
				}
			}
		}
		return true
	})
	log.Debugf("SemanticTokens: %d identifiers, %d bytes\n", len(tokens)/5, end-begin)
	return tokens
}

func appendToken(tokens []uint32, tok syntax.Token, tree *ttcn3.Tree, typ SemanticTokenType, mod SemanticTokenModifiers, line int, col int) ([]uint32, int, int) {
	pos := syntax.Begin(tok)
	pos.Line -= 1
	pos.Column -= 1

	// relative line
	relLine := pos.Line - line
	if relLine != 0 {
		col = 0
	}
	line = pos.Line

	// relative column
	relCol := pos.Column - col
	col = pos.Column

	// token width
	width := tok.End() - tok.Pos()
	tokens = append(tokens, uint32(relLine), uint32(relCol), uint32(width), uint32(typ), uint32(mod))
	return tokens, line, col
}

func DefinitionToken(tree *ttcn3.Tree, id syntax.Node) (SemanticTokenType, SemanticTokenModifiers) {
	switch n := tree.ParentOf(id).(type) {
	case *syntax.Module:
		return Namespace, Definition
	case *syntax.ImportDecl:
		return Namespace, Undefined
	case *syntax.FormalPar:
		return Parameter, Declaration
	case *syntax.StructTypeDecl:
		return Struct, Definition
	case *syntax.ClassTypeDecl:
		return Class, Definition
	case *syntax.MapTypeDecl:
		return Map, Definition
	case *syntax.EnumTypeDecl:
		if id == n.Name {
			return Enum, Definition
		}
		return EnumMember, Definition
	case *syntax.EnumSpec:
		return EnumMember, Definition
	case *syntax.Field:
		if _, ok := tree.ParentOf(n).(*syntax.SubTypeDecl); ok {
			return Type, Definition
		}
	case *syntax.PortTypeDecl:
		return Interface, Definition
	case *syntax.ComponentTypeDecl:
		return Class, Definition
	case *syntax.FuncDecl:
		return Function, Definition
	case *syntax.TemplateDecl:
		return Variable, Declaration | Readonly
	case *syntax.ValueDecl:
		typ := Variable
		mod := Declaration
		if _, ok := tree.ParentOf(tree.ParentOf(tree.ParentOf(n))).(*syntax.ComponentTypeDecl); ok {
			typ = Property
		}
		if n.KindTok != nil {
			switch n.KindTok.Kind() {
			case syntax.CONST, syntax.MODULEPAR:
				mod |= Readonly
			case syntax.PORT:
				typ = Interface
			}
		}
		return typ, mod
	case *syntax.Declarator:
		return DefinitionToken(tree, n)
	}
	return None, Undefined
}

func ReferenceToken(tree *ttcn3.Tree, db *ttcn3.DB, id *syntax.Ident) (SemanticTokenType, SemanticTokenModifiers) {
	name := id.String()
	if typ, ok := builtins[name]; ok {
		return typ, DefaultLibrary
	}
	defs := tree.LookupWithDB(id, db)
	if len(defs) == 0 {
		return None, Undefined
	}
	if len(defs) > 1 {
		log.Debugf("ReferenceToken: multiple definitions for %s\n", name)
	}
	typ, mod := DefinitionToken(defs[0].Tree, defs[0].Ident)
	return typ, mod &^ (Definition | Declaration)
}
