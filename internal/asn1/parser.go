package asn1

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/internal/asn1/ast"
)

// Parser is a recursive-descent ASN.1 parser. To make backtracking
// trivial we lex the entire source up-front into a slice and use a
// single int index. Save and restore is a one-int snapshot.
type Parser struct {
	src     []byte
	tokens  []Token
	pos     int
	cur     Token
	peek    Token
	diags   []ast.Diagnostic
	lexErrs []Diagnostic
}

// NewParser constructs a Parser over src.
func NewParser(src []byte) *Parser {
	l := NewLexer(src)
	toks, errs := l.All()
	p := &Parser{src: src, tokens: toks, lexErrs: errs}
	p.refresh()
	return p
}

// refresh repopulates cur/peek from the current pos.
func (p *Parser) refresh() {
	p.cur = p.tokAt(p.pos)
	p.peek = p.tokAt(p.pos + 1)
}

func (p *Parser) tokAt(i int) Token {
	if i < len(p.tokens) {
		return p.tokens[i]
	}
	if n := len(p.tokens); n > 0 {
		return Token{Kind: EOF, Pos: p.tokens[n-1].End, End: p.tokens[n-1].End}
	}
	return Token{Kind: EOF}
}

// save / restore enable cheap backtracking. The lexer state is no
// longer relevant because all tokens were lexed up-front.
type pmark int

func (p *Parser) save() pmark   { return pmark(p.pos) }
func (p *Parser) restore(m pmark) {
	p.pos = int(m)
	p.refresh()
}

// ParseModule parses src as a single ASN.1 module. The returned
// *ast.Module is always non-nil; check Diagnostics for any issues.
func ParseModule(src []byte) *ast.Module {
	p := NewParser(src)
	m := p.parseModule()
	for _, d := range p.lexErrs {
		m.Diagnostics = append(m.Diagnostics, ast.Diagnostic{
			Pos:      d.Column,
			End:      d.Column,
			Severity: ast.SeverityError,
			Code:     "lex",
			Message:  d.Message,
		})
	}
	m.Diagnostics = append(m.Diagnostics, p.diags...)
	return m
}

// ---------------------------------------------------------------------------
// Construction helpers - all return value types so concrete nodes can
// be built with `&ast.BuiltinType{TypeBase: tbase(s,e), ...}`.
// ---------------------------------------------------------------------------

func span(start, end int) ast.Span       { return ast.NewSpan(start, end) }
func tbase(start, end int) ast.TypeBase  { return ast.TypeBase{Span: span(start, end)} }
func vbase(start, end int) ast.ValueBase { return ast.ValueBase{Span: span(start, end)} }
func cbase(start, end int) ast.ConstraintElementBase {
	return ast.ConstraintElementBase{Span: span(start, end)}
}

// ---------------------------------------------------------------------------
// Lookahead helpers
// ---------------------------------------------------------------------------

func (p *Parser) advance() Token {
	prev := p.cur
	p.pos++
	p.refresh()
	return prev
}

func (p *Parser) at(k TokenKind) bool        { return p.cur.Kind == k }
func (p *Parser) atKeyword(text string) bool { return p.cur.Kind == KEYWORD && p.cur.Text == text }

func (p *Parser) eat(k TokenKind) (Token, bool) {
	if p.cur.Kind == k {
		t := p.cur
		p.advance()
		return t, true
	}
	return Token{}, false
}

func (p *Parser) eatKeyword(text string) (Token, bool) {
	if p.atKeyword(text) {
		t := p.cur
		p.advance()
		return t, true
	}
	return Token{}, false
}

func (p *Parser) expect(k TokenKind) (Token, bool) {
	if t, ok := p.eat(k); ok {
		return t, true
	}
	p.errorf(p.cur.Pos, "expected %s, got %s(%q)", k, p.cur.Kind, p.cur.Text)
	return p.cur, false
}

func (p *Parser) expectKeyword(text string) bool {
	if _, ok := p.eatKeyword(text); ok {
		return true
	}
	p.errorf(p.cur.Pos, "expected keyword %q, got %s(%q)", text, p.cur.Kind, p.cur.Text)
	return false
}

func (p *Parser) errorf(pos int, format string, args ...interface{}) {
	p.diags = append(p.diags, ast.Diagnostic{
		Pos:      pos,
		End:      pos,
		Severity: ast.SeverityError,
		Code:     "syntax",
		Message:  fmt.Sprintf(format, args...),
	})
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

func (p *Parser) parseModule() *ast.Module {
	start := p.cur.Pos
	m := &ast.Module{Span: span(start, start)}
	m.Identifier = p.parseModuleIdentifier()
	m.SetRange(start, m.Identifier.End())

	if !p.expectKeyword("DEFINITIONS") {
		return m
	}

	// Encoding reference instructions (e.g. TAG INSTRUCTIONS) - skip.
	for p.cur.Kind == TYPEREFERENCE && p.peek.Kind == KEYWORD && p.peek.Text == "INSTRUCTIONS" {
		p.advance()
		p.advance()
	}

	m.Tagging = ast.TagsExplicit
	switch {
	case p.atKeyword("EXPLICIT"):
		p.advance()
		m.Tagging = ast.TagsExplicit
		p.eatKeyword("TAGS")
	case p.atKeyword("IMPLICIT"):
		p.advance()
		m.Tagging = ast.TagsImplicit
		p.eatKeyword("TAGS")
	case p.atKeyword("AUTOMATIC"):
		p.advance()
		m.Tagging = ast.TagsAutomatic
		p.eatKeyword("TAGS")
	}

	if p.atKeyword("EXTENSIBILITY") {
		p.advance()
		p.eatKeyword("IMPLIED")
		m.Extensible = true
	}

	if !p.atKeyword("BEGIN") {
		if _, ok := p.eat(ASSIGN); !ok {
			p.errorf(p.cur.Pos, "expected ::= before BEGIN")
		}
	}
	if !p.expectKeyword("BEGIN") {
		return m
	}

	m.Exports = p.parseExports()
	m.Imports = p.parseImports()
	m.Assignments = p.parseAssignments()
	p.eatKeyword("END")
	m.SetRange(start, p.cur.Pos)
	return m
}

func (p *Parser) parseModuleIdentifier() ast.ModuleIdentifier {
	start := p.cur.Pos
	id := ast.ModuleIdentifier{Span: span(start, start)}
	if p.cur.Kind != TYPEREFERENCE {
		p.errorf(p.cur.Pos, "expected module identifier")
		return id
	}
	id.Name = p.cur.Text
	end := p.cur.End
	p.advance()
	if p.at(LBRACE) {
		oid := p.parseOID()
		id.OID = oid
		end = oid.End()
	}
	id.DefinitiveName = string(p.src[start:end])
	id.SetRange(start, end)
	return id
}

func (p *Parser) parseOID() *ast.OID {
	start := p.cur.Pos
	if _, ok := p.expect(LBRACE); !ok {
		return nil
	}
	oid := &ast.OID{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		oid.Components = append(oid.Components, p.parseOIDComponent())
	}
	end := p.cur.End
	p.expect(RBRACE)
	oid.Raw = string(p.src[start:end])
	oid.SetRange(start, end)
	return oid
}

func (p *Parser) parseOIDComponent() ast.OIDComponent {
	start := p.cur.Pos
	c := ast.OIDComponent{Span: span(start, start)}
	switch p.cur.Kind {
	case IDENTIFIER:
		c.Name = p.cur.Text
		p.advance()
		if p.at(LPAREN) {
			p.advance()
			if p.at(NUMBER) {
				if v, err := strconv.ParseInt(p.cur.Text, 10, 64); err == nil {
					c.Number = v
					c.HasNum = true
				}
				p.advance()
			}
			p.expect(RPAREN)
		}
	case NUMBER:
		if v, err := strconv.ParseInt(p.cur.Text, 10, 64); err == nil {
			c.Number = v
			c.HasNum = true
		}
		p.advance()
	default:
		p.errorf(p.cur.Pos, "expected OID component, got %s", p.cur.Kind)
		p.advance()
	}
	c.SetRange(start, p.cur.Pos)
	return c
}

// ---------------------------------------------------------------------------
// EXPORTS / IMPORTS
// ---------------------------------------------------------------------------

func (p *Parser) parseExports() *ast.Exports {
	if !p.atKeyword("EXPORTS") {
		return nil
	}
	start := p.cur.Pos
	p.advance()
	e := &ast.Exports{Span: span(start, start)}
	if p.atKeyword("ALL") {
		p.advance()
		e.All = true
	} else {
		for !p.at(SEMICOLON) && !p.at(EOF) {
			if p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER {
				e.Symbols = append(e.Symbols, p.cur.Text)
			}
			p.advance()
			if p.at(COMMA) {
				p.advance()
			}
		}
	}
	end := p.cur.End
	p.expect(SEMICOLON)
	e.SetRange(start, end)
	return e
}

func (p *Parser) parseImports() []*ast.Import {
	if !p.atKeyword("IMPORTS") {
		return nil
	}
	p.advance()
	var out []*ast.Import
	for !p.at(SEMICOLON) && !p.at(EOF) {
		imp := p.parseOneImport()
		if imp != nil {
			out = append(out, imp)
		}
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(SEMICOLON)
	return out
}

func (p *Parser) parseOneImport() *ast.Import {
	start := p.cur.Pos
	imp := &ast.Import{Span: span(start, start)}
	for !p.atKeyword("FROM") && !p.at(SEMICOLON) && !p.at(EOF) {
		if p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER {
			imp.Symbols = append(imp.Symbols, p.cur.Text)
		}
		p.advance()
		if p.at(COMMA) {
			p.advance()
		}
	}
	if !p.atKeyword("FROM") {
		return nil
	}
	p.advance()
	if p.cur.Kind != TYPEREFERENCE {
		p.errorf(p.cur.Pos, "expected module name after FROM")
		return nil
	}
	imp.From = p.cur.Text
	end := p.cur.End
	p.advance()
	if p.at(LBRACE) {
		oid := p.parseOID()
		imp.OID = oid
		end = oid.End()
	}
	imp.SetRange(start, end)
	return imp
}

// ---------------------------------------------------------------------------
// Assignments
// ---------------------------------------------------------------------------

func (p *Parser) parseAssignments() []ast.Assignment {
	var out []ast.Assignment
	for !p.atKeyword("END") && !p.at(EOF) {
		a := p.parseAssignment()
		if a != nil {
			out = append(out, a)
			continue
		}
		p.syncToNextAssignment()
	}
	return out
}

func (p *Parser) parseAssignment() ast.Assignment {
	start := p.cur.Pos

	if p.cur.Kind == TYPEREFERENCE && p.peek.Kind == ASSIGN {
		name := p.cur.Text
		p.advance()
		p.advance()
		if p.atKeyword("CLASS") {
			cls := p.parseObjectClassBody()
			return &ast.ObjectClassAssignment{
				Name:  name,
				Class: cls,
				Span:  span(start, cls.End()),
			}
		}
		t := p.parseType()
		return &ast.TypeAssignment{Name: name, Type: t, Span: span(start, t.End())}
	}

	if p.cur.Kind == TYPEREFERENCE && p.peek.Kind == LBRACE {
		if name, params, ok := p.tryParameterisedHeader(); ok {
			if p.atKeyword("CLASS") {
				cls := p.parseObjectClassBody()
				return &ast.ObjectClassAssignment{
					Name: name, Params: params,
					Class: cls,
					Span:  span(start, cls.End()),
				}
			}
			t := p.parseType()
			return &ast.TypeAssignment{
				Name: name, Params: params, Type: t,
				Span: span(start, t.End()),
			}
		}
	}

	if p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER {
		name := p.cur.Text
		isLower := p.cur.Kind == IDENTIFIER
		p.advance()
		t := p.parseType()
		if !p.at(ASSIGN) {
			p.errorf(p.cur.Pos, "expected '::=' in assignment for %q", name)
			return nil
		}
		p.advance()
		if isLower {
			v := p.parseValue()
			return &ast.ValueAssignment{
				Name: name, Type: t, Value: v,
				Span: span(start, p.cur.Pos),
			}
		}
		if p.at(LBRACE) {
			set := p.parseElementSet()
			return &ast.ValueSetTypeAssignment{
				Name: name, Type: t, Set: set,
				Span: span(start, p.cur.Pos),
			}
		}
		p.errorf(p.cur.Pos, "expected '{' for value set in assignment for %q", name)
		return nil
	}

	p.errorf(p.cur.Pos, "unexpected token %s(%q) at top level", p.cur.Kind, p.cur.Text)
	return nil
}

func (p *Parser) tryParameterisedHeader() (string, *ast.ParameterList, bool) {
	if p.cur.Kind != TYPEREFERENCE || p.peek.Kind != LBRACE {
		return "", nil, false
	}
	m := p.save()
	name := p.cur.Text
	p.advance()
	params := p.parseParameterList()
	if params == nil || !p.at(ASSIGN) {
		p.restore(m)
		return "", nil, false
	}
	p.advance()
	return name, params, true
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

func (p *Parser) parseType() ast.Type {
	t := p.parseUntaggedType()
	if t == nil {
		return &ast.AnyType{TypeBase: tbase(p.cur.Pos, p.cur.Pos)}
	}
	for p.at(LPAREN) {
		c := p.parseConstraint()
		t = &ast.ConstrainedType{
			TypeBase:   tbase(t.Pos(), c.End()),
			Inner:      t,
			Constraint: c,
		}
	}
	return t
}

func (p *Parser) parseUntaggedType() ast.Type {
	start := p.cur.Pos
	if p.at(LBRACKET) {
		return p.parseTaggedType(start)
	}
	switch p.cur.Kind {
	case KEYWORD:
		return p.parseBuiltinType()
	case TYPEREFERENCE:
		return p.parseReferencedType()
	case IDENTIFIER:
		ref := &ast.TypeRef{Name: p.cur.Text, Span: span(start, p.cur.End)}
		p.advance()
		return &ast.ReferencedType{TypeBase: tbase(start, p.cur.Pos), Ref: ref}
	}
	p.errorf(start, "expected type, got %s(%q)", p.cur.Kind, p.cur.Text)
	p.advance()
	return &ast.AnyType{TypeBase: tbase(start, p.cur.Pos)}
}

func (p *Parser) parseBuiltinType() ast.Type {
	start := p.cur.Pos
	switch p.cur.Text {
	case "BOOLEAN":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Boolean, Name: "BOOLEAN"}
	case "NULL":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Null, Name: "NULL"}
	case "REAL":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Real, Name: "REAL"}
	case "INTEGER":
		p.advance()
		t := &ast.IntegerType{TypeBase: tbase(start, p.cur.Pos)}
		if p.at(LBRACE) {
			t.NamedNumbers = p.parseNamedNumberList()
			t.SetRange(start, p.cur.Pos)
		}
		return t
	case "BIT":
		p.advance()
		p.expectKeyword("STRING")
		t := &ast.BitStringType{TypeBase: tbase(start, p.cur.Pos)}
		if p.at(LBRACE) {
			t.NamedBits = p.parseNamedNumberList()
			t.SetRange(start, p.cur.Pos)
		}
		return t
	case "OCTET":
		p.advance()
		p.expectKeyword("STRING")
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.OctetString, Name: "OCTET STRING"}
	case "OBJECT":
		p.advance()
		p.expectKeyword("IDENTIFIER")
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.ObjectIdentifier, Name: "OBJECT IDENTIFIER"}
	case "RELATIVE-OID":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.RelativeOID, Name: "RELATIVE-OID"}
	case "OID-IRI":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.OIDIRI, Name: "OID-IRI"}
	case "RELATIVE-OID-IRI":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.RelativeOIDIRI, Name: "RELATIVE-OID-IRI"}
	case "ENUMERATED":
		p.advance()
		return p.parseEnumeratedType(start)
	case "SEQUENCE":
		p.advance()
		return p.parseSequenceOrSetType(start, true)
	case "SET":
		p.advance()
		return p.parseSequenceOrSetType(start, false)
	case "CHOICE":
		p.advance()
		return p.parseChoiceType(start)
	case "EXTERNAL":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.External, Name: "EXTERNAL"}
	case "EMBEDDED":
		p.advance()
		p.eatKeyword("PDV")
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.EmbeddedPDV, Name: "EMBEDDED PDV"}
	case "CHARACTER":
		p.advance()
		p.eatKeyword("STRING")
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.CharacterString, Name: "CHARACTER STRING"}
	case "UTCTime":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.UTCTime, Name: "UTCTime"}
	case "GeneralizedTime":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.GeneralizedTime, Name: "GeneralizedTime"}
	case "DATE":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Date, Name: "DATE"}
	case "DATE-TIME":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.DateTime, Name: "DATE-TIME"}
	case "TIME":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Time, Name: "TIME"}
	case "TIME-OF-DAY":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.TimeOfDay, Name: "TIME-OF-DAY"}
	case "DURATION":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.Duration, Name: "DURATION"}
	case "ObjectDescriptor":
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: ast.ObjectDescriptor, Name: "ObjectDescriptor"}
	}
	if kind, ok := restrictedStringKind(p.cur.Text); ok {
		name := p.cur.Text
		p.advance()
		return &ast.BuiltinType{TypeBase: tbase(start, p.cur.Pos), Kind: kind, Name: name}
	}
	p.errorf(start, "unexpected keyword %q in type position", p.cur.Text)
	p.advance()
	return &ast.AnyType{TypeBase: tbase(start, p.cur.Pos)}
}

func restrictedStringKind(s string) (ast.BuiltinKind, bool) {
	switch s {
	case "BMPString":
		return ast.BMPString, true
	case "GeneralString":
		return ast.GeneralString, true
	case "GraphicString":
		return ast.GraphicString, true
	case "IA5String":
		return ast.IA5String, true
	case "ISO646String":
		return ast.ISO646String, true
	case "NumericString":
		return ast.NumericString, true
	case "PrintableString":
		return ast.PrintableString, true
	case "TeletexString":
		return ast.TeletexString, true
	case "T61String":
		return ast.T61String, true
	case "UniversalString":
		return ast.UniversalString, true
	case "UTF8String":
		return ast.UTF8String, true
	case "VideotexString":
		return ast.VideotexString, true
	case "VisibleString":
		return ast.VisibleString, true
	}
	return ast.UnknownBuiltin, false
}

func (p *Parser) parseNamedNumberList() []ast.NamedNumber {
	p.expect(LBRACE)
	var out []ast.NamedNumber
	for !p.at(RBRACE) && !p.at(EOF) {
		start := p.cur.Pos
		nn := ast.NamedNumber{Span: span(start, start)}
		if p.cur.Kind != IDENTIFIER {
			p.errorf(p.cur.Pos, "expected named number identifier")
			p.advance()
			continue
		}
		nn.Name = p.cur.Text
		p.advance()
		if p.at(LPAREN) {
			p.advance()
			nn.Value = p.parseValue()
			p.expect(RPAREN)
		}
		nn.SetRange(start, p.cur.Pos)
		out = append(out, nn)
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	return out
}

func (p *Parser) parseEnumeratedType(start int) *ast.EnumeratedType {
	t := &ast.EnumeratedType{TypeBase: tbase(start, start)}
	p.expect(LBRACE)
	target := &t.Items
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.at(ELLIPSIS) {
			t.Extensible = true
			target = &t.Extensions
			p.advance()
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		if p.cur.Kind != IDENTIFIER {
			p.errorf(p.cur.Pos, "expected ENUMERATED item identifier")
			p.advance()
			continue
		}
		itemStart := p.cur.Pos
		item := ast.EnumItem{Span: span(itemStart, itemStart), Name: p.cur.Text}
		p.advance()
		if p.at(LPAREN) {
			p.advance()
			item.Value = p.parseValue()
			p.expect(RPAREN)
		}
		item.SetRange(itemStart, p.cur.Pos)
		*target = append(*target, item)
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	t.SetRange(start, p.cur.Pos)
	return t
}

func (p *Parser) parseSequenceOrSetType(start int, isSequence bool) ast.Type {
	if p.atKeyword("OF") || (p.at(LPAREN) && lookaheadIsOfAfterParen(p)) {
		var c *ast.Constraint
		if p.at(LPAREN) {
			c = p.parseConstraint()
		}
		p.expectKeyword("OF")
		if p.cur.Kind == IDENTIFIER && p.peek.Kind != ASSIGN {
			p.advance()
		}
		elem := p.parseType()
		if isSequence {
			return &ast.SequenceOfType{TypeBase: tbase(start, elem.End()), Element: elem, Constraint: c}
		}
		return &ast.SetOfType{TypeBase: tbase(start, elem.End()), Element: elem, Constraint: c}
	}
	p.expect(LBRACE)
	var comps []ast.Component
	var ext []ast.ExtensionAddition
	extensible := false
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.at(ELLIPSIS) {
			extensible = true
			p.advance()
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		if extensible && p.at(DOUBLE_LBRACKET) {
			ext = append(ext, p.parseExtensionGroup(len(ext)+1))
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		c := p.parseComponent()
		if extensible {
			ext = append(ext, ast.ExtensionAddition{Components: []ast.Component{c}, Span: span(c.Pos(), c.End())})
		} else {
			comps = append(comps, c)
		}
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	end := p.cur.Pos
	if isSequence {
		return &ast.SequenceType{TypeBase: tbase(start, end), Components: comps, Extensible: extensible, Extensions: ext}
	}
	return &ast.SetType{TypeBase: tbase(start, end), Components: comps, Extensible: extensible, Extensions: ext}
}

func lookaheadIsOfAfterParen(p *Parser) bool {
	depth := 0
	for i := p.pos; i < len(p.tokens); i++ {
		t := p.tokens[i]
		switch {
		case t.Kind == LPAREN:
			depth++
		case t.Kind == RPAREN:
			depth--
			if depth == 0 {
				nx := p.tokAt(i + 1)
				return nx.Kind == KEYWORD && nx.Text == "OF"
			}
		}
	}
	return false
}

func (p *Parser) parseComponent() ast.Component {
	start := p.cur.Pos
	c := ast.Component{Span: span(start, start)}
	if p.atKeyword("COMPONENTS") {
		p.advance()
		p.expectKeyword("OF")
		c.ComponentsOf = true
		c.Type = p.parseType()
		c.SetRange(start, c.Type.End())
		return c
	}
	if p.cur.Kind != IDENTIFIER {
		p.errorf(p.cur.Pos, "expected component name")
		p.advance()
		return c
	}
	c.Name = p.cur.Text
	p.advance()
	c.Type = p.parseType()
	if p.atKeyword("OPTIONAL") {
		p.advance()
		c.Optional = true
	} else if p.atKeyword("DEFAULT") {
		p.advance()
		c.Default = p.parseValue()
		c.Optional = true
	}
	c.SetRange(start, p.cur.Pos)
	return c
}

func (p *Parser) parseExtensionGroup(num int) ast.ExtensionAddition {
	start := p.cur.Pos
	p.expect(DOUBLE_LBRACKET)
	if p.cur.Kind == NUMBER && p.peek.Kind == COLON {
		if v, err := strconv.Atoi(p.cur.Text); err == nil {
			num = v
		}
		p.advance()
		p.advance()
	}
	var comps []ast.Component
	for !p.at(DOUBLE_RBRACKET) && !p.at(EOF) {
		comps = append(comps, p.parseComponent())
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(DOUBLE_RBRACKET)
	end := p.cur.Pos
	return ast.ExtensionAddition{Span: span(start, end), Group: num, Components: comps}
}

func (p *Parser) parseChoiceType(start int) ast.Type {
	p.expect(LBRACE)
	var alts []ast.Component
	var ext []ast.ExtensionAddition
	extensible := false
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.at(ELLIPSIS) {
			extensible = true
			p.advance()
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		if extensible && p.at(DOUBLE_LBRACKET) {
			ext = append(ext, p.parseExtensionGroup(len(ext)+1))
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		a := p.parseComponent()
		if extensible {
			ext = append(ext, ast.ExtensionAddition{Span: span(a.Pos(), a.End()), Components: []ast.Component{a}})
		} else {
			alts = append(alts, a)
		}
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	end := p.cur.Pos
	return &ast.ChoiceType{TypeBase: tbase(start, end), Alternatives: alts, Extensible: extensible, Extensions: ext}
}

func (p *Parser) parseTaggedType(start int) ast.Type {
	p.expect(LBRACKET)
	tag := ast.Tag{Span: span(start, start), Class: ast.ContextSpecificTag}
	if p.atKeyword("UNIVERSAL") {
		tag.Class = ast.UniversalTag
		p.advance()
	} else if p.atKeyword("APPLICATION") {
		tag.Class = ast.ApplicationTag
		p.advance()
	} else if p.atKeyword("PRIVATE") {
		tag.Class = ast.PrivateTag
		p.advance()
	}
	tag.Number = p.parseValue()
	p.expect(RBRACKET)
	if p.atKeyword("IMPLICIT") {
		tag.Mode = ast.TagModeImplicit
		p.advance()
	} else if p.atKeyword("EXPLICIT") {
		tag.Mode = ast.TagModeExplicit
		p.advance()
	}
	inner := p.parseUntaggedType()
	tag.SetRange(start, p.cur.Pos)
	return &ast.TaggedType{TypeBase: tbase(start, inner.End()), Tag: tag, Underlying: inner}
}

func (p *Parser) parseReferencedType() ast.Type {
	start := p.cur.Pos
	ref := &ast.TypeRef{Span: span(start, start)}
	first := p.cur.Text
	p.advance()
	if p.at(DOT) && (p.peek.Kind == TYPEREFERENCE || p.peek.Kind == IDENTIFIER) {
		p.advance()
		ref.Module = first
		ref.Name = p.cur.Text
		p.advance()
	} else {
		ref.Name = first
	}
	ref.SetRange(start, p.cur.Pos)

	if p.at(DOT) && p.peek.Kind == AMP_REF {
		p.advance()
		field := p.cur.Text
		p.advance()
		return &ast.OpenTypeFieldType{TypeBase: tbase(start, p.cur.Pos), ClassRef: ref, Field: field}
	}

	rt := &ast.ReferencedType{TypeBase: tbase(start, p.cur.Pos), Ref: ref}
	if p.at(LBRACE) {
		rt.Actuals = p.parseActualParameterList()
		rt.SetRange(start, p.cur.Pos)
	}
	return rt
}

// ---------------------------------------------------------------------------
// Values
// ---------------------------------------------------------------------------

func (p *Parser) parseValue() ast.Value {
	start := p.cur.Pos
	switch p.cur.Kind {
	case NUMBER:
		text := p.cur.Text
		p.advance()
		if strings.ContainsAny(text, ".eE") {
			return &ast.RealValue{ValueBase: vbase(start, p.cur.Pos), Text: text}
		}
		return &ast.IntegerValue{ValueBase: vbase(start, p.cur.Pos), Text: text}
	case HYPHEN:
		p.advance()
		if p.at(NUMBER) {
			text := "-" + p.cur.Text
			p.advance()
			return &ast.IntegerValue{ValueBase: vbase(start, p.cur.Pos), Text: text}
		}
		p.errorf(start, "expected number after '-'")
		return &ast.IntegerValue{ValueBase: vbase(start, p.cur.Pos), Text: "-0"}
	case CSTRING:
		text := p.cur.Text
		p.advance()
		return &ast.StringValue{ValueBase: vbase(start, p.cur.Pos), Kind: ast.StringCString, Text: text}
	case BSTRING:
		text := p.cur.Text
		p.advance()
		return &ast.StringValue{ValueBase: vbase(start, p.cur.Pos), Kind: ast.StringBString, Text: text}
	case HSTRING:
		text := p.cur.Text
		p.advance()
		return &ast.StringValue{ValueBase: vbase(start, p.cur.Pos), Kind: ast.StringHString, Text: text}
	case KEYWORD:
		switch p.cur.Text {
		case "TRUE":
			p.advance()
			return &ast.BooleanValue{ValueBase: vbase(start, p.cur.Pos), Value: true}
		case "FALSE":
			p.advance()
			return &ast.BooleanValue{ValueBase: vbase(start, p.cur.Pos), Value: false}
		case "NULL":
			p.advance()
			return &ast.NullValue{ValueBase: vbase(start, p.cur.Pos)}
		case "MIN", "MAX", "PLUS-INFINITY", "MINUS-INFINITY", "NOT-A-NUMBER":
			text := p.cur.Text
			p.advance()
			return &ast.ReferenceValue{ValueBase: vbase(start, p.cur.Pos), Name: text}
		}
	case TYPEREFERENCE:
		mod := p.cur.Text
		p.advance()
		if p.at(DOT) && (p.peek.Kind == IDENTIFIER || p.peek.Kind == TYPEREFERENCE) {
			p.advance()
			name := p.cur.Text
			p.advance()
			return &ast.ReferenceValue{ValueBase: vbase(start, p.cur.Pos), Module: mod, Name: name}
		}
		return &ast.ReferenceValue{ValueBase: vbase(start, p.cur.Pos), Name: mod}
	case IDENTIFIER:
		name := p.cur.Text
		p.advance()
		if p.at(COLON) {
			p.advance()
			inner := p.parseValue()
			return &ast.ChoiceValue{ValueBase: vbase(start, p.cur.Pos), Alternative: name, Value: inner}
		}
		return &ast.ReferenceValue{ValueBase: vbase(start, p.cur.Pos), Name: name}
	case LBRACE:
		return p.parseBraceValue(start)
	}
	p.errorf(start, "expected value, got %s(%q)", p.cur.Kind, p.cur.Text)
	p.advance()
	return &ast.IntegerValue{ValueBase: vbase(start, p.cur.Pos), Text: "0"}
}

func (p *Parser) parseBraceValue(start int) ast.Value {
	p.expect(LBRACE)
	if p.at(RBRACE) {
		p.advance()
		return &ast.SequenceOfValue{ValueBase: vbase(start, p.cur.Pos)}
	}
	if p.cur.Kind == IDENTIFIER && p.peek.Kind != COMMA && p.peek.Kind != RBRACE && p.peek.Kind != COLON && p.peek.Kind != LPAREN {
		m := p.save()
		if seq := p.tryParseSequenceValue(start); seq != nil {
			return seq
		}
		p.restore(m)
	}
	if (p.cur.Kind == IDENTIFIER || p.cur.Kind == NUMBER) && oidLooks(p) {
		oid := p.parseOIDBody(start)
		return &ast.OIDValue{ValueBase: vbase(start, p.cur.Pos), OID: oid}
	}
	var elems []ast.Value
	for !p.at(RBRACE) && !p.at(EOF) {
		elems = append(elems, p.parseValue())
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	return &ast.SequenceOfValue{ValueBase: vbase(start, p.cur.Pos), Elements: elems}
}

func oidLooks(p *Parser) bool {
	depth := 0
	for i := p.pos; i < len(p.tokens); i++ {
		t := p.tokens[i]
		switch t.Kind {
		case LBRACE:
			depth++
		case RBRACE:
			if depth == 0 {
				return true
			}
			depth--
		case COLON:
			return false
		case COMMA:
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

func (p *Parser) parseOIDBody(start int) *ast.OID {
	oid := &ast.OID{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		oid.Components = append(oid.Components, p.parseOIDComponent())
	}
	end := p.cur.End
	p.expect(RBRACE)
	oid.Raw = string(p.src[start:end])
	oid.SetRange(start, end)
	return oid
}

func (p *Parser) tryParseSequenceValue(start int) ast.Value {
	var fields []ast.NamedValue
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.cur.Kind != IDENTIFIER {
			return nil
		}
		nm := p.cur.Text
		fstart := p.cur.Pos
		p.advance()
		v := p.parseValue()
		nv := ast.NamedValue{Span: span(fstart, p.cur.Pos), Name: nm, Value: v}
		fields = append(fields, nv)
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	return &ast.SequenceValue{ValueBase: vbase(start, p.cur.Pos), Fields: fields}
}

// ---------------------------------------------------------------------------
// Constraints
// ---------------------------------------------------------------------------

func (p *Parser) parseConstraint() *ast.Constraint {
	start := p.cur.Pos
	p.expect(LPAREN)
	set := p.parseElementSet()
	var exc *ast.Exception
	if p.at(BANG) {
		excStart := p.cur.Pos
		p.advance()
		raw := p.collectRawUntilParen()
		exc = &ast.Exception{Span: span(excStart, p.cur.Pos), Raw: raw}
	}
	p.expect(RPAREN)
	return &ast.Constraint{Span: span(start, p.cur.Pos), Set: set, Exception: exc}
}

func (p *Parser) collectRawUntilParen() string {
	var b strings.Builder
	depth := 0
	for !p.at(EOF) {
		if depth == 0 && p.at(RPAREN) {
			break
		}
		if p.at(LPAREN) {
			depth++
		}
		if p.at(RPAREN) {
			depth--
		}
		b.WriteString(p.cur.Text)
		b.WriteByte(' ')
		p.advance()
	}
	return strings.TrimSpace(b.String())
}

func (p *Parser) parseElementSet() *ast.ElementSet {
	start := p.cur.Pos
	set := &ast.ElementSet{Span: span(start, start)}
	set.Root = p.parseUnion()
	if p.at(COMMA) && p.peek.Kind == ELLIPSIS {
		p.advance()
		p.advance()
		set.Extensible = true
		if p.at(COMMA) {
			p.advance()
			set.Extension = p.parseUnion()
		}
	} else if p.at(ELLIPSIS) {
		p.advance()
		set.Extensible = true
	}
	set.SetRange(start, p.cur.Pos)
	return set
}

func (p *Parser) parseUnion() ast.UnionExpr {
	var u ast.UnionExpr
	u = append(u, p.parseIntersection())
	for p.at(PIPE) || p.atKeyword("UNION") {
		p.advance()
		u = append(u, p.parseIntersection())
	}
	return u
}

func (p *Parser) parseIntersection() ast.IntersectionExpr {
	var i ast.IntersectionExpr
	i = append(i, p.parseConstraintElement())
	for p.at(CARET) || p.atKeyword("INTERSECTION") {
		p.advance()
		i = append(i, p.parseConstraintElement())
	}
	return i
}

func (p *Parser) parseConstraintElement() ast.ConstraintElement {
	start := p.cur.Pos
	switch {
	case p.atKeyword("SIZE"):
		p.advance()
		c := p.parseConstraint()
		return &ast.SizeConstraint{ConstraintElementBase: cbase(start, c.End()), Constraint: c}
	case p.atKeyword("FROM"):
		p.advance()
		c := p.parseConstraint()
		return &ast.AlphabetConstraint{ConstraintElementBase: cbase(start, c.End()), Constraint: c}
	case p.atKeyword("PATTERN"):
		p.advance()
		v := p.parseValue()
		return &ast.PatternConstraint{ConstraintElementBase: cbase(start, v.End()), Pattern: v}
	case p.atKeyword("SETTINGS"):
		p.advance()
		if p.at(CSTRING) {
			s := &ast.PropertySettings{ConstraintElementBase: cbase(start, p.cur.End), Settings: p.cur.Text}
			p.advance()
			return s
		}
	case p.atKeyword("CONTAINING"):
		p.advance()
		t := p.parseType()
		return &ast.ContainedSubtype{ConstraintElementBase: cbase(start, t.End()), Type: t}
	case p.atKeyword("INCLUDES"):
		p.advance()
		t := p.parseType()
		return &ast.ContainedSubtype{ConstraintElementBase: cbase(start, t.End()), Type: t}
	case p.atKeyword("WITH"):
		p.advance()
		if p.atKeyword("COMPONENT") {
			p.advance()
			c := p.parseConstraint()
			return &ast.InnerTypeConstraint{ConstraintElementBase: cbase(start, c.End()), Single: true, Constraint: c}
		}
		if p.atKeyword("COMPONENTS") {
			p.advance()
			return p.parseWithComponents(start)
		}
	case p.atKeyword("ALL"):
		p.advance()
		if p.atKeyword("EXCEPT") {
			p.advance()
			ex := p.parseConstraintElement()
			return &ast.AllExceptConstraint{ConstraintElementBase: cbase(start, ex.End()), Exclude: ex}
		}
	case p.atKeyword("CONSTRAINED"):
		p.advance()
		if p.atKeyword("BY") {
			p.advance()
			raw := p.collectRawUntilParen()
			return &ast.UserDefinedConstraint{ConstraintElementBase: cbase(start, p.cur.Pos), Raw: raw}
		}
	}

	if p.at(LPAREN) {
		c := p.parseConstraint()
		return &ast.ContainedSubtype{
			ConstraintElementBase: cbase(start, c.End()),
			Type:                  &ast.ConstrainedType{TypeBase: tbase(start, c.End()), Constraint: c},
		}
	}

	// Table constraint: `{ObjectSet}` or `{ObjectSet}{@field.path}`.
	if p.at(LBRACE) {
		set := p.parseObjectSet()
		tc := &ast.TableConstraint{ObjectSet: set}
		if p.at(LBRACE) && p.peek.Kind == AT {
			tc.AtNotation = p.parseAtNotation()
		}
		tc.SetRange(start, p.cur.Pos)
		return tc
	}

	v := p.parseValue()
	if p.at(DOUBLE_DOT) {
		p.advance()
		open := false
		if p.at(LESS) {
			open = true
			p.advance()
		}
		var upper ast.Value
		upperMax := false
		if p.atKeyword("MAX") {
			upperMax = true
			upper = &ast.ReferenceValue{ValueBase: vbase(p.cur.Pos, p.cur.End), Name: "MAX"}
			p.advance()
		} else {
			upper = p.parseValue()
		}
		return &ast.ValueRangeConstraint{
			ConstraintElementBase: cbase(start, p.cur.Pos),
			Lower:                 v,
			Upper:                 upper,
			UpperOpen:             open,
			LowerIsMin:            isMinRef(v),
			UpperIsMax:            upperMax,
		}
	}
	return &ast.SingleValueConstraint{ConstraintElementBase: cbase(start, v.End()), Value: v}
}

func isMinRef(v ast.Value) bool {
	r, ok := v.(*ast.ReferenceValue)
	return ok && r.Name == "MIN"
}

func (p *Parser) parseWithComponents(start int) ast.ConstraintElement {
	p.expect(LBRACE)
	var comps []ast.InnerComponent
	partial := false
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.at(ELLIPSIS) {
			partial = true
			p.advance()
			if p.at(COMMA) {
				p.advance()
			}
			continue
		}
		ic := ast.InnerComponent{}
		icStart := p.cur.Pos
		if p.cur.Kind == IDENTIFIER {
			ic.Name = p.cur.Text
			p.advance()
		}
		if p.at(LPAREN) {
			ic.Constraint = p.parseConstraint()
		}
		switch {
		case p.atKeyword("PRESENT"):
			ic.Presence = ast.PresencePresent
			p.advance()
		case p.atKeyword("ABSENT"):
			ic.Presence = ast.PresenceAbsent
			p.advance()
		case p.atKeyword("OPTIONAL"):
			ic.Presence = ast.PresenceOptional
			p.advance()
		}
		ic.SetRange(icStart, p.cur.Pos)
		comps = append(comps, ic)
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	return &ast.InnerTypeConstraint{ConstraintElementBase: cbase(start, p.cur.Pos), Components: comps, PartialFlag: partial}
}

// ---------------------------------------------------------------------------
// X.681/X.682 object sets and table constraints
// ---------------------------------------------------------------------------

func (p *Parser) parseObjectSet() *ast.ObjectSet {
	start := p.cur.Pos
	p.expect(LBRACE)
	set := &ast.ObjectSet{Span: span(start, start)}
	target := &set.Root
	for !p.at(RBRACE) && !p.at(EOF) {
		if p.at(ELLIPSIS) {
			set.Extensible = true
			target = &set.Extension
			p.advance()
			if p.at(COMMA) || p.at(PIPE) {
				p.advance()
			}
			continue
		}
		el := p.parseObjectSetElement()
		if el != nil {
			*target = append(*target, el)
		}
		if p.at(PIPE) || p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	set.SetRange(start, p.cur.Pos)
	return set
}

func (p *Parser) parseObjectSetElement() ast.ObjectSetElement {
	start := p.cur.Pos
	switch p.cur.Kind {
	case LBRACE:
		// Object literal `{ &Field value, ... }` - parsed as a
		// generic value sequence for now; the X.681 driver in
		// Phase 7 reinterprets it against the class's WITH SYNTAX.
		obj := p.parseObjectLiteralBody()
		return &ast.ObjectLiteralElement{
			ObjectSetElementBase: ast.ObjectSetElementBase{Span: span(start, p.cur.Pos)},
			Object:               obj,
		}
	case TYPEREFERENCE:
		// Could be an object reference or an object-set reference;
		// they share the same syntactic shape. The resolver picks
		// the correct one based on declaration kind.
		ref := &ast.TypeRef{Span: span(start, p.cur.End), Name: p.cur.Text}
		p.advance()
		if p.at(DOT) && (p.peek.Kind == TYPEREFERENCE || p.peek.Kind == IDENTIFIER) {
			p.advance()
			ref.Module = ref.Name
			ref.Name = p.cur.Text
			ref.SetRange(start, p.cur.End)
			p.advance()
		}
		if isUpperFirst(ref.Name) {
			return &ast.ObjectSetReferenceElement{
				ObjectSetElementBase: ast.ObjectSetElementBase{Span: span(start, ref.End())},
				Ref:                  ref,
			}
		}
		return &ast.ObjectReferenceElement{
			ObjectSetElementBase: ast.ObjectSetElementBase{Span: span(start, ref.End())},
			Ref:                  ref,
		}
	case IDENTIFIER:
		ref := &ast.TypeRef{Span: span(start, p.cur.End), Name: p.cur.Text}
		p.advance()
		return &ast.ObjectReferenceElement{
			ObjectSetElementBase: ast.ObjectSetElementBase{Span: span(start, ref.End())},
			Ref:                  ref,
		}
	}
	p.errorf(p.cur.Pos, "expected object set element, got %s", p.cur.Kind)
	p.advance()
	return nil
}

func (p *Parser) parseObjectLiteralBody() *ast.Object {
	start := p.cur.Pos
	p.expect(LBRACE)
	obj := &ast.Object{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		setting := p.parseObjectSetting()
		if setting != nil {
			obj.Settings = append(obj.Settings, *setting)
		}
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	obj.SetRange(start, p.cur.Pos)
	return obj
}

func (p *Parser) parseObjectSetting() *ast.ObjectSetting {
	start := p.cur.Pos
	if !p.at(AMP_REF) {
		// Skip until next `&` or `}` - object literals can also be
		// driven by WITH SYNTAX templates which we don't fully
		// handle in Phase 4 (Phase 7 does).
		p.advance()
		return nil
	}
	field := p.cur.Text
	p.advance()
	setting := &ast.ObjectSetting{Span: span(start, start), FieldRef: field}
	if isAmpType(field) {
		setting.Type = p.parseType()
	} else {
		setting.Value = p.parseValue()
	}
	setting.SetRange(start, p.cur.Pos)
	return setting
}

func (p *Parser) parseAtNotation() *ast.AtNotation {
	start := p.cur.Pos
	p.expect(LBRACE)
	an := &ast.AtNotation{Span: span(start, start)}
	p.expect(AT)
	for p.at(DOT) {
		an.Level++
		p.advance()
	}
	for p.cur.Kind == IDENTIFIER || p.cur.Kind == TYPEREFERENCE {
		an.Path = append(an.Path, p.cur.Text)
		p.advance()
		if !p.at(DOT) {
			break
		}
		p.advance()
	}
	p.expect(RBRACE)
	an.SetRange(start, p.cur.Pos)
	return an
}

func isUpperFirst(s string) bool {
	if s == "" {
		return false
	}
	return isUpper(s[0])
}

func isAmpType(s string) bool {
	return len(s) >= 2 && isUpper(s[1])
}

// ---------------------------------------------------------------------------
// X.683 parameter lists
// ---------------------------------------------------------------------------

func (p *Parser) parseParameterList() *ast.ParameterList {
	if !p.at(LBRACE) {
		return nil
	}
	start := p.cur.Pos
	p.advance()
	pl := &ast.ParameterList{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		pl.Params = append(pl.Params, p.parseParameter())
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	pl.SetRange(start, p.cur.Pos)
	return pl
}

func (p *Parser) parseParameter() ast.Parameter {
	start := p.cur.Pos
	param := ast.Parameter{Span: span(start, start)}
	m := p.save()
	if p.cur.Kind == TYPEREFERENCE || p.cur.Kind == KEYWORD {
		gov := p.parseType()
		if p.at(COLON) {
			p.advance()
			param.Governor = gov
		} else {
			p.restore(m)
		}
	}
	if p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER {
		param.Reference = p.cur.Text
		p.advance()
	}
	param.SetRange(start, p.cur.Pos)
	return param
}

func (p *Parser) parseActualParameterList() *ast.ActualParameterList {
	if !p.at(LBRACE) {
		return nil
	}
	start := p.cur.Pos
	p.advance()
	al := &ast.ActualParameterList{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		al.Params = append(al.Params, p.parseActualParameter())
		if p.at(COMMA) {
			p.advance()
		}
	}
	p.expect(RBRACE)
	al.SetRange(start, p.cur.Pos)
	return al
}

func (p *Parser) parseActualParameter() ast.ActualParameter {
	start := p.cur.Pos
	ap := ast.ActualParameter{Span: span(start, start)}
	switch p.cur.Kind {
	case NUMBER, CSTRING, BSTRING, HSTRING, IDENTIFIER:
		ap.Value = p.parseValue()
	case TYPEREFERENCE, KEYWORD:
		ap.Type = p.parseType()
	default:
		ap.Value = p.parseValue()
	}
	ap.SetRange(start, p.cur.Pos)
	return ap
}

// ---------------------------------------------------------------------------
// Object class body (X.681 - Phase 4 expands this further)
// ---------------------------------------------------------------------------

func (p *Parser) parseObjectClassBody() *ast.ObjectClass {
	start := p.cur.Pos
	p.expectKeyword("CLASS")
	cls := &ast.ObjectClass{Span: span(start, start)}
	if p.at(LBRACE) {
		p.advance()
		for !p.at(RBRACE) && !p.at(EOF) {
			if fs := p.parseFieldSpec(); fs != nil {
				cls.Fields = append(cls.Fields, fs)
			}
			if p.at(COMMA) {
				p.advance()
			}
		}
		p.expect(RBRACE)
	}
	if p.atKeyword("WITH") {
		p.advance()
		if p.atKeyword("SYNTAX") {
			p.advance()
			cls.WithSyntax = p.parseWithSyntaxSpec()
		}
	}
	cls.SetRange(start, p.cur.Pos)
	return cls
}

func (p *Parser) parseFieldSpec() ast.FieldSpec {
	start := p.cur.Pos
	if !p.at(AMP_REF) {
		p.errorf(p.cur.Pos, "expected &Field in CLASS body")
		p.advance()
		return nil
	}
	name := p.cur.Text
	p.advance()
	isType := len(name) >= 2 && isUpper(name[1])
	if isType && (p.atKeyword("OPTIONAL") || p.atKeyword("DEFAULT") || p.at(COMMA) || p.at(RBRACE)) {
		tf := &ast.TypeFieldSpec{Span: span(start, p.cur.Pos), Name: name}
		if p.atKeyword("OPTIONAL") {
			tf.Optional = true
			p.advance()
		} else if p.atKeyword("DEFAULT") {
			p.advance()
			tf.Default = p.parseType()
			tf.Optional = true
		}
		tf.SetRange(start, p.cur.Pos)
		return tf
	}
	t := p.parseType()
	if !isType {
		fv := &ast.FixedTypeValueFieldSpec{Span: span(start, p.cur.Pos), Name: name, Type: t}
		if p.atKeyword("UNIQUE") {
			fv.Unique = true
			p.advance()
		}
		if p.atKeyword("OPTIONAL") {
			fv.Optional = true
			p.advance()
		} else if p.atKeyword("DEFAULT") {
			p.advance()
			fv.Default = p.parseValue()
			fv.Optional = true
		}
		fv.SetRange(start, p.cur.Pos)
		return fv
	}
	fs := &ast.FixedTypeValueSetFieldSpec{Span: span(start, p.cur.Pos), Name: name, Type: t}
	if p.atKeyword("OPTIONAL") {
		fs.Optional = true
		p.advance()
	}
	fs.SetRange(start, p.cur.Pos)
	return fs
}

func (p *Parser) parseWithSyntaxSpec() *ast.WithSyntaxSpec {
	start := p.cur.Pos
	p.expect(LBRACE)
	ws := &ast.WithSyntaxSpec{Span: span(start, start)}
	for !p.at(RBRACE) && !p.at(EOF) {
		ws.Tokens = append(ws.Tokens, p.parseWithSyntaxToken())
	}
	p.expect(RBRACE)
	ws.SetRange(start, p.cur.Pos)
	return ws
}

// parseWithSyntaxToken reads one entry from a WITH SYNTAX template.
// The token stream is pre-lexed in normal mode (KEYWORD for upper-case
// reserved words, TYPEREFERENCE for the others). Either form represents
// a literal word in template context, so we accept both.
func (p *Parser) parseWithSyntaxToken() ast.WithSyntaxToken {
	start := p.cur.Pos
	switch {
	case p.at(LBRACKET):
		p.advance()
		t := ast.WithSyntaxToken{Span: span(start, start), Kind: ast.WSKOptionalGroup}
		for !p.at(RBRACKET) && !p.at(EOF) {
			t.Group = append(t.Group, p.parseWithSyntaxToken())
		}
		p.expect(RBRACKET)
		t.SetRange(start, p.cur.Pos)
		return t
	case p.at(AMP_REF):
		text := p.cur.Text
		p.advance()
		return ast.WithSyntaxToken{Span: span(start, p.cur.Pos), Kind: ast.WSKFieldRef, Text: text}
	case p.cur.Kind == WORD || p.cur.Kind == KEYWORD || p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER:
		text := p.cur.Text
		p.advance()
		return ast.WithSyntaxToken{Span: span(start, p.cur.Pos), Kind: ast.WSKWord, Text: text}
	}
	p.advance()
	return ast.WithSyntaxToken{Span: span(start, p.cur.Pos), Kind: ast.WSKWord}
}

// ---------------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------------

func (p *Parser) syncToNextAssignment() {
	for !p.at(EOF) {
		if p.atKeyword("END") {
			return
		}
		if (p.cur.Kind == TYPEREFERENCE || p.cur.Kind == IDENTIFIER) &&
			(p.peek.Kind == ASSIGN || p.peek.Kind == LBRACE) {
			return
		}
		p.advance()
	}
}
