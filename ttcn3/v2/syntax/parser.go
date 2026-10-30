package syntax

import (
	"context"
	"fmt"

	"github.com/nokia/ntt/internal/log"
)

// Parse parses the TTCN-3 source code in src and returns a syntax tree.
func Parse(src []byte) Node {
	p := &parser{Scanner: NewScanner(src)}
	p.content = src
	root := p.Push(Root)
	p.next = p.peek(1)
	for p.next != EOF {
		if !p.parseTTCN3() {
			e := p.Push(SyntaxError)
			p.consume()
			p.Pop()
			p.error(&NodeError{
				Node: e,
				Err:  fmt.Errorf("unexpected token %s", p.next),
				Hint: "expected declaration, statement, or expression",
			})
		}
	}

	// Consume remaining tokens
	for len(p.tokens) > 0 {
		p.pushToken()
	}

	p.Pop()
	root.tree.lines = p.Lines()
	return root
}

type parser struct {
	Builder
	*Scanner
	la     []Kind
	next   Kind
	tokens []token
	pos    int
}

type token struct {
	kind       Kind
	begin, end int
}

func (p *parser) scan() Kind {
	for {
		kind, begin, end := p.Scan()
		switch kind {
		case Comment, Preproc:
			p.tokens = append(p.tokens, token{kind, begin, end})
		case Identifier:
			// The scanner does not distinguish between identifiers and keywords.
			// Therefore the parser must check if the identifier is a keyword.
			if kw, ok := keywords[string(p.content[begin:end])]; ok {
				kind = kw
			}
			fallthrough
		default:
			if kind != EOF {
				p.tokens = append(p.tokens, token{kind, begin, end})
			}
			return kind
		}
	}
}

func (p *parser) peek(i int) Kind {
	idx := p.pos + i - 1
	last := len(p.la) - 1
	if idx > last {
		n := idx - last
		for i := 0; i < n; i++ {
			p.la = append(p.la, p.scan())
		}
	}
	return p.la[idx]
}

// consume returns the next token, skipping past any comments and preprocessor directives.
func (p *parser) consume() Node {
	p.pos++
	if p.pos == len(p.la) {
		p.pos = 0
		p.la = p.la[:0]
	}
	n := p.pushToken()
	p.next = p.peek(1)
	return n
}

// Push all tokens to the tree until the first non-comment or
// preprocessor directive.
func (p *parser) pushToken() Node {
	for len(p.tokens) > 0 {
		t := p.tokens[0]
		p.tokens = p.tokens[1:]
		n := p.PushToken(t.kind, t.begin, t.end)
		if t.kind != Comment && t.kind != Preproc {
			return n
		}
	}
	return Nil
}

func (p *parser) accept(kk ...Kind) bool {
	if len(kk) > 3 {
		log.Traceln(context.TODO(), "warning", "accept called with more than 3 arguments")
	}
	for _, k := range kk {
		if p.next == k {
			return true
		}
	}
	return false
}

func (p *parser) expect(k Kind) bool {
	if p.next == k {
		p.consume()
		return true
	} else {
		e := p.Push(SyntaxError)
		p.Pop()
		p.error(&NodeError{
			Node: e,
			Err:  fmt.Errorf("expected %s, got %s", k, p.next),
		})
		return false
	}
}

func (p *parser) error(err error) {
	p.tree.errs = append(p.tree.errs, err)
}

func (p *parser) parseDecl() bool { return p.parseTTCN3() }
func (p *parser) parseStmt() bool { return p.parseTTCN3() }

// parseTTCN3 parses any main TTCN-3 construct: modules, declarations,
// statements and expressions.
func (p *parser) parseTTCN3() bool {
	switch tok := p.next; tok {
	case AltKeyword, InterleaveKeyword:
		return p.parseAltStmt()
	case LeftBrace:
		return p.parseBlock()
	case DoKeyword:
		return p.parseDoStmt()
	case ForKeyword:
		return p.parseForStmt()
	case GotoKeyword:
		return p.parseGotoStmt()
	case LeftBracket:
		return p.parseGuardStmt()
	case IfKeyword:
		return p.parseIfStmt()
	case LabelKeyword:
		return p.parseLabelStmt()
	case PortKeyword:
		return p.parsePortDecl()
	case ReturnKeyword:
		return p.parseReturnStmt()
	case SelectKeyword:
		return p.parseSelectStmt()
	case WhileKeyword:
		return p.parseWhileStmt()
	case AltstepKeyword:
		return p.parseAltstep()
	case ConfigurationKeyword:
		return p.parseConfiguration()
	case CreateKeyword:
		return p.parseConstructor()
	case ControlKeyword:
		return p.parseControl()
	case FinallyKeyword:
		return p.parseDestructor()
	case FriendKeyword:
		return p.parseFriend()
	case ExternalKeyword:
		return p.parseFunction()
	case GroupKeyword:
		return p.parseGroup()
	case ImportKeyword:
		return p.parseImport()
	case SignatureKeyword:
		return p.parseSignature()
	case TemplateKeyword:
		return p.parseTemplate()
	case TestcaseKeyword:
		// Resolve conflict between `testcase.stop` and a testcase definition.
		if p.peek(2) == Dot {
			return p.parseExpr()
		}
		return p.parseTestcase()
	case TypeKeyword:
		switch p.peek(2) {
		case AltstepKeyword:
			return p.parseAltstepType()
		case ClassKeyword:
			return p.parseClass()
		case ComponentKeyword:
			return p.parseComponent()
		case EnumeratedKeyword:
			return p.parseEnum()
		case FunctionKeyword:
			return p.parseFunctionType()
		case MapKeyword:
			return p.parseMap()
		case PortKeyword:
			return p.parsePort()
		case TestcaseKeyword:
			return p.parseTestcaseType()
		case RecordKeyword, SetKeyword, UnionKeyword:
			switch p.peek(3) {
			case LengthKeyword, OfKeyword:
				return p.parseList()
			default:
				return p.parseStruct()
			}
		default:
			return p.parseSubType()
		}
	case ConstKeyword, ModuleparKeyword, VarKeyword:
		return p.parseVarDecl()

	case ModuleKeyword:
		return p.parseModule()

	case TimerKeyword:
		// There's a conflict between `timer.stop` (expression) and
		// `timer t` (declaration). We resolve it by looking ahead, if
		// the next token is an identifier, we assume it's a
		// declaration. Everything else is parsed as expression.
		if p.peek(2) == Identifier {
			return p.parseVarDecl()
		}
		return p.parseExpr()
	case EOF:
		return false

	default:
		return p.parseExpr()
	}
}

func (p *parser) expectSemicolon() bool {
	if p.next == Semicolon {
		p.consume()
	}
	return true
}

func (p *parser) expectComma() bool {
	switch p.next {
	case RightBrace, RightBracket, RightParen, Greater:
		return true
	default:
		return p.expect(Comma)
	}
}

func (p *parser) parseImportStmt() bool {
	// Conflict with Refs and "all except Refs"
	return false
}

func (p *parser) parseName() bool {
	if p.next == Identifier {
		n := p.consume()
		n.tree.events[n.idx].kind = uint16(Name)
		return true
	} else {
		e := p.Push(SyntaxError)
		p.Pop()
		p.error(&NodeError{
			Node: e,
			Err:  fmt.Errorf("expected identifier, got %s", p.next),
		})
		return false
	}
}

// parseRef has conflicts with various `all` and `any` references.
func (p *parser) parseRef() bool {

	switch p.next {
	case AddressKeyword:
		p.expect(AddressKeyword)
	case AllKeyword:
		p.expect(AllKeyword)
		switch p.next {
		case ComponentKeyword:
			return p.expect(ComponentKeyword)
		case PortKeyword:
			return p.expect(PortKeyword)
		case TimerKeyword:
			return p.expect(TimerKeyword)
		}
		return true
	case AnyKeyword:
		p.expect(AnyKeyword)
		switch p.next {
		case ComponentKeyword:
			return p.expect(ComponentKeyword)
		case PortKeyword:
			return p.expect(PortKeyword)
		case TimerKeyword:
			return p.expect(TimerKeyword)
		}
	case MapKeyword:
		p.expect(MapKeyword)
	case MtcKeyword:
		p.expect(MtcKeyword)
	case SelfKeyword:
		p.expect(SelfKeyword)
	case SystemKeyword:
		p.expect(SystemKeyword)
	case ThisKeyword:
		p.expect(ThisKeyword)
	case TimerKeyword:
		p.expect(TimerKeyword)
	case UniversalKeyword:
		p.expect(UniversalKeyword)
		p.expect(CharstringKeyword)
	case UnmapKeyword:
		p.expect(UnmapKeyword)
	case Identifier:
		// TODO(5nord): Evaluate if its faster to handle identifiers before this switch.
		p.expect(Identifier)
		if p.accept(Less) {
			return p.parseTypePars()
		}
	default:
		e := p.Push(SyntaxError)
		p.Pop()
		p.error(&NodeError{
			Node: e,
			Err:  fmt.Errorf("unexpected token %s", p.next),
			Hint: "expected a reference (e.g. identifier, self, all, any, ...)",
		})
		return false
	}
	return true
}

func (p *parser) parseNestedType() bool {
	switch p.next {
	case RecordKeyword, SetKeyword, UnionKeyword:
		if p.peek(2) == LeftBrace {
			return p.parseNestedStruct()
		} else {
			return p.parseNestedList()
		}
	case EnumeratedKeyword:
		return p.parseNestedEnum()
	default:
		return p.parseRef()
	}
}

func (p *parser) parseExpr() bool {
	// TODO(5nord) implement pratt parser
	return p.expect(Identifier)
}

func (p *parser) parseCallExpr() bool  { panic("binaryExpr: not implemented") }
func (p *parser) parseIndexExpr() bool { panic("binaryExpr: not implemented") }
func (p *parser) parseDotExpr() bool   { panic("binaryExpr: not implemented") }
