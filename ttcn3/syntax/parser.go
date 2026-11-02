package syntax

import (
	"context"
	"fmt"
	"os"
	trc "runtime/trace"
)

// A Mode value is a set of flags (or 0).
// They control the amount of source code parsed and other optional
// parser functionality.
type Mode uint

type ParserOption func(*parser) error

// WithFilename sets the filename of the source code.
func WithFilename(filename string) ParserOption {
	return func(p *parser) error {
		p.Filename = filename
		return nil
	}
}

// WithNames sets the names map in which the parser will store the names of the
// parsed entities.
func WithNames(names map[string]bool) ParserOption {
	return func(p *parser) error {
		p.names = names
		return nil
	}
}

// WithUses sets the uses map in which the parser will store the uses of the
// parsed entities.
func WithUses(uses map[string]bool) ParserOption {
	return func(p *parser) error {
		p.uses = uses
		return nil
	}
}

const (
	PedanticSemicolon = 1 << iota // expect semicolons pedantically
	IgnoreComments                // ignore comments
	Trace                         // print a trace of parsed productions
)

func NewParser(src []byte) *parser {
	var p parser
	if s := os.Getenv("NTT_DEBUG"); s == "trace" {
		p.mode |= Trace
	}

	p.trace = p.mode&Trace != 0 // for convenience (p.trace is used frequently)
	p.semi = p.mode&PedanticSemicolon != 0

	p.ppDefs = make(map[string]bool)
	p.ppDefs["0"] = false
	p.ppDefs["1"] = true

	p.Root = newRoot(src)

	// fetch first token
	tok := p.peek(1)
	p.tok = tok.Kind()

	return &p
}

// Parse parses the source code of a TTCN-3 module and returns the corresponding AST.
func Parse(src []byte, opts ...ParserOption) (root *Root) {

	region := trc.StartRegion(context.Background(), "syntax.Parse")
	defer region.End()

	p := NewParser(src)
	for _, opt := range opts {
		if err := opt(p); err != nil {
			p.Root.errs = append(p.Root.errs, err)
			return p.Root
		}
	}
	skip := func(k Kind) bool { return !topLevelTokens[k] && !exprStart[k] }
	for p.tok != EOF {
		if p.tok == COMMA || p.tok == SEMICOLON {
			p.consume()
			continue
		}
		if !topLevelTokens[p.tok] && !exprStart[p.tok] {
			p.Nodes = append(p.Nodes, p.advanceWithError(skip, "unexpected token %s", p.tok))
			continue
		}

		tok := p.peek(1)
		p.Nodes = append(p.Nodes, p.parse())
		if p.peek(1) == tok || p.tok != EOF && !topLevelTokens[p.tok] {
			p.Nodes = append(p.Nodes, p.advanceWithError(skip, "unexpected token %s", p.tok))
		}
	}
	return p.Root
}

// The parser structure holds the parser's internal state.
type parser struct {
	// Tracing/debugging
	mode   Mode // parsing mode
	trace  bool // == (mode & Trace != 0)
	semi   bool // == (mode & PedanticSemicolon != 0)
	indent int  // indentation used for tracing output

	*Root

	// Tokens/Backtracking
	cursor  int
	queue   []Token
	markers []int
	tok     Kind // for convenience (p.tok is used frequently)

	// Semicolon helper
	seenBrace bool

	// Pre-processor Handling
	ppSkip bool
	ppDefs map[string]bool

	names map[string]bool
	uses  map[string]bool

	// altGuard is true while parsing the operation of an alt guard.
	// A '[' that follows it can start the next guard rather than an index.
	altGuard bool

	// noRedirect, noParam, noValue and noMatchAttr stop a nested expression
	// from consuming a token that belongs to the surrounding construct:
	// "->" after a to/from address, "param" after "-> value", "value" after
	// the right-hand side of ":", and "length"/"ifpresent" after "pattern".
	noRedirect  bool
	noParam     bool
	noValue     bool
	noMatchAttr bool

	// Error recovery
	// (used to limit the number of calls to advance
	// w/o making scanning progress - avoids potential endless
	// loops across multiple parser functions during error recovery)
	syncPos int // last synchronization position
	syncCnt int // number of advance calls without progress

}

func newRoot(src []byte) *Root {
	return &Root{
		Scanner: NewScanner(src),
	}
}

// Usage pattern: defer un(trace(p, "..."))
func un(p *parser) {
	p.indent--
	p.printTrace(")")
}

// Advance to the next token
func (p *parser) consume() Token {
	tok := p.queue[p.cursor]
	if p.trace {
		p.printTrace(tok.String())
	}

	// Track curly braces for TTCN-3 semicolon rules
	p.seenBrace = false
	if p.tok == RBRACE {
		p.seenBrace = true
	}

	p.cursor++
	if p.cursor == len(p.queue) && !p.speculating() {
		p.cursor = 0
		p.queue = p.queue[:0]
	}
	p.peek(1)
	p.tok = p.queue[p.cursor].Kind()
	return tok
}

func (p *parser) ignoreToken(tok Kind) bool {
	switch {
	case tok == COMMENT:
		return true
	case tok == PREPROC:
		return true
	default:
		if p.ppSkip && tok != EOF {
			return true
		}
		return false
	}
}

func (p *parser) grow(n int) {
	for n > 0 {
		kind, begin, end := p.Scan()
		if kind == IDENT && end-begin > 1 {
			kind = Lookup(p.src[begin:end])
		}
		tok := token{Kind: kind, Begin: begin, End: end}
		p.tokens = append(p.tokens, tok)
		if !p.ignoreToken(kind) {
			tn := &tokenNode{
				idx:  len(p.tokens) - 1,
				Root: p.Root,
			}
			if kind == MALFORMED || kind == UNTERMINATED {
				p.error(tn, "malformed or unterminated token")
			}
			p.queue = append(p.queue, tn)
			n--
		}
	}
}

func (p *parser) peek(i int) Token {
	idx := p.cursor + i - 1
	last := len(p.queue) - 1
	if idx > last {
		p.grow(idx - last)
	}
	return p.queue[idx]
}

func (p *parser) pos(i int) int {
	tok := p.peek(i)
	return tok.Pos()
}

func (p *parser) lit(i int) string {
	return p.peek(i).String()
}

func (p *parser) mark() {
	p.markers = append(p.markers, p.cursor)
}

func (p *parser) commit() {
	last := len(p.markers) - 1
	p.markers = p.markers[0:last]
}

func (p *parser) reset() {
	last := len(p.markers) - 1
	marker := p.markers[last]
	p.markers = p.markers[0:last]
	p.cursor = marker
	p.tok = p.queue[p.cursor].Kind()
}

func (p *parser) speculating() bool {
	return len(p.markers) > 0
}

// ----------------------------------------------------------------------------
// Tracing support

func (p *parser) printTrace(a ...interface{}) {
	const dots = ". . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . "
	const n = len(dots)
	pos := Begin(p.peek(1))
	fmt.Printf("%5d:%3d: ", pos.Line, pos.Column)
	i := 2 * p.indent
	for i > n {
		fmt.Print(dots)
		i -= n
	}
	// i <= n
	fmt.Print(dots[0:i])
	fmt.Println(a...)
}

func trace(p *parser, msg string) *parser {
	p.printTrace(msg, "(")
	p.indent++
	return p
}

func (p *parser) error(n Node, msg string, args ...interface{}) {
	err := Error{Node: n, Msg: fmt.Sprintf(msg, args...)}
	p.errs = append(p.errs, err)
}

func (p *parser) errorExpected(what string) {
	tok := p.peek(1)
	p.error(tok, "expected %s, found %s", what, tok.String())
}

// expect consumes the current token. If it is not of kind k, an error is
// reported. A missing delimiter k is not substituted by the current token, but
// returned as nil.
func (p *parser) expect(k Kind) Token {
	if p.tok != k {
		p.errorExpected("'" + k.String() + "'")
		if delimiters[k] && p.tok != EOF {
			return nil
		}
	}
	return p.consume() // make progress
}

var delimiters = map[Kind]bool{
	LPAREN:    true,
	RPAREN:    true,
	LBRACK:    true,
	RBRACK:    true,
	LBRACE:    true,
	RBRACE:    true,
	SEMICOLON: true,
}

func (p *parser) expectSemi(tok Token) {
	if p.tok == SEMICOLON {
		p.consume()
		return
	}

	// pedantic semicolon
	if p.semi {
		// semicolon is optional before a closing '}'
		if !p.seenBrace && p.tok == RBRACE && p.tok != EOF {
			p.errorExpected("';'")
			p.advance(stmtStart)
		}
	}
}

// advance consumes tokens until the current token p.tok
// is in the 'to' set, or EOF. For error recovery.
func (p *parser) advance(to map[Kind]bool) {
	for ; p.tok != EOF; p.consume() {
		if to[p.tok] {
			// Return only if parser made some progress since last
			// sync or if it has not reached 10 advance calls without
			// progress. Otherwise consume at least one token to
			// avoid an endless parser loop (it is possible that
			// both parseOperand and parseStmt call advance and
			// correctly do not advance, thus the need for the
			// invocation limit p.syncCnt).
			if p.pos(1) == p.syncPos && p.syncCnt < 10 {
				p.syncCnt++
				return
			}
			if p.pos(1) > p.syncPos {
				p.syncPos = p.pos(1)
				p.syncCnt = 0
				return
			}
			// Reaching here indicates a parser bug, likely an
			// incorrect token list in this function, but it only
			// leads to skipping of possibly correct code if a
			// previous error is present, and thus is preferred
			// over a non-terminating parse.
		}
	}
}

// advanceWithError reports an error and consumes the current token together
// with all following tokens for which skip, if not nil, returns true. The
// returned ErrorNode covers the consumed tokens, so they stay in the tree.
func (p *parser) advanceWithError(skip func(Kind) bool, msg string, args ...interface{}) *ErrorNode {
	x := &ErrorNode{From: p.peek(1), To: p.peek(1)}
	if p.tok != EOF {
		x.To = p.consume()
		for skip != nil && p.tok != EOF && skip(p.tok) {
			x.To = p.consume()
		}
	}
	p.error(x, msg, args...)
	return x
}

// parseList parses a comma separated list of items up to, but excluding, the
// closing token. Items must begin with a token in first. Other tokens are
// reported and passed to junk as ErrorNode. A token in recovery ends the list
// early, so the caller can report the missing closing token.
func parseList[T any](p *parser, what string, close Kind, first, recovery map[Kind]bool, item func() T, junk func(*ErrorNode) T) []T {
	const (
		wantItem = iota
		wantComma
		skipped
	)
	var list []T
	stop := func(k Kind) bool { return k == COMMA || k == close || first[k] || recovery[k] }
	state := wantItem
	for p.tok != close && p.tok != EOF {
		switch {
		case p.tok == COMMA && state != wantItem:
			p.consume()
			state = wantItem
		case first[p.tok]:
			if state == wantComma {
				p.errorExpected("','")
			}
			tok := p.peek(1)
			list = append(list, item())
			state = wantComma
			if p.peek(1) == tok {
				list = append(list, junk(p.advanceWithError(nil, "expected %s, found %s", what, tok)))
				state = skipped
			}
		case recovery[p.tok]:
			return list
		default:
			want := what
			if state == wantComma {
				want = "','"
			}
			list = append(list, junk(p.advanceWithError(func(k Kind) bool { return !stop(k) }, "expected %s, found %s", want, p.peek(1))))
			state = skipped
		}
	}
	return list
}

var stmtStart = map[Kind]bool{
	ALT:        true,
	ALTSTEP:    true,
	BREAK:      true,
	CASE:       true,
	CONST:      true,
	CONTINUE:   true,
	CONTROL:    true,
	DISPLAY:    true,
	DO:         true,
	ELSE:       true,
	ENCODE:     true,
	EXTENSION:  true,
	FOR:        true,
	FRIEND:     true,
	FUNCTION:   true,
	GOTO:       true,
	GROUP:      true,
	IF:         true,
	IMPORT:     true,
	INTERLEAVE: true,
	LABEL:      true,
	MAP:        true,
	MODULE:     true,
	MODULEPAR:  true,
	PORT:       true,
	PRIVATE:    true,
	PUBLIC:     true,
	RBRACE:     true,
	REPEAT:     true,
	RETURN:     true,
	SELECT:     true,
	SEMICOLON:  true,
	SIGNATURE:  true,
	TEMPLATE:   true,
	TESTCASE:   true,
	TIMER:      true,
	TYPE:       true,
	UNMAP:      true,
	VAR:        true,
	VARIANT:    true,
	WHILE:      true,
}

var operandStart = map[Kind]bool{
	ADDRESS:    true,
	ALL:        true,
	ANY:        true,
	ANYKW:      true,
	BSTRING:    true,
	NOTIMPL:    true,
	CHARSTRING: true,
	ERROR:      true,
	FAIL:       true,
	FALSE:      true,
	FLOAT:      true,
	//IDENT: true, TODO(5nord) fix conflict, see failing parser tests
	INCONC:    true,
	INT:       true,
	MAP:       true,
	MTC:       true,
	MUL:       true,
	NAN:       true,
	NONE:      true,
	NULL:      true,
	OMIT:      true,
	PASS:      true,
	STRING:    true,
	SYSTEM:    true,
	TESTCASE:  true,
	TIMER:     true,
	TRUE:      true,
	UNIVERSAL: true,
	UNMAP:     true,
}

var moduleDefStart = map[Kind]bool{
	ALTSTEP:   true,
	CONST:     true,
	CONTROL:   true,
	CREATE:    true,
	EXTERNAL:  true,
	FRIEND:    true,
	FUNCTION:  true,
	GROUP:     true,
	IMPORT:    true,
	MODULE:    true,
	MODULEPAR: true,
	PRIVATE:   true,
	PUBLIC:    true,
	RBRACE:    true,
	SEMICOLON: true,
	SIGNATURE: true,
	TEMPLATE:  true,
	TESTCASE:  true,
	TYPE:      true,
	VAR:       true,
}

// skipStmt reports whether k cannot start a statement and may be skipped
// during error recovery.
func skipStmt(k Kind) bool {
	switch k {
	case IDENT, TESTCASE, ANYKW, ALL, MAP, UNMAP, MTC, INC, DEC, LBRACK, LBRACE, MODIF,
		INT, FLOAT, STRING, BSTRING, TRUE, FALSE, PASS, FAIL, NONE, INCONC, ERROR, NOTIMPL:
		return false
	}
	return !stmtStart[k]
}

// skipItem reports whether k may be skipped during error recovery of a
// semicolon terminated item.
func skipItem(k Kind) bool {
	return k != SEMICOLON && k != RBRACE
}

// listRecovery contains the tokens at which a list is abandoned, because
// they likely belong to an enclosing construct.
var listRecovery = union(stmtStart, map[Kind]bool{
	EXTERNAL: true,
	LBRACE:   true,
	RBRACK:   true,
	RPAREN:   true,
	WITH:     true,
})

var typeListRecovery = union(listRecovery, map[Kind]bool{LPAREN: true})

// exprStart contains the tokens which can start an expression.
var exprStart = map[Kind]bool{
	ADD: true, EXCL: true, NOT: true, NOT4B: true, SUB: true, INC: true, DEC: true, COLONCOLON: true,
	ANYKW: true, ALL: true, UNIVERSAL: true,
	ADDRESS: true, CHARSTRING: true, CLASS: true, MAP: true, MTC: true, SYSTEM: true, TESTCASE: true, TIMER: true, UNMAP: true,
	IDENT: true, INT: true, ANY: true, NOTIMPL: true, BSTRING: true, ERROR: true, NULL: true, OMIT: true, FAIL: true, FALSE: true, FLOAT: true,
	INCONC: true, MUL: true, NAN: true, NONE: true, PASS: true, STRING: true, TRUE: true,
	LPAREN: true, LBRACK: true, LBRACE: true,
	MODIFIES: true, REGEXP: true, PATTERN: true, DECMATCH: true, MODIF: true, VALUE: true, PRESENT: true,
}

var formalParStart = map[Kind]bool{
	IN: true, OUT: true, INOUT: true,
	TEMPLATE: true, OMIT: true, VALUE: true, PRESENT: true, MODIF: true,
	RECORD: true, SET: true, UNION: true, ENUMERATED: true, MAP: true,
	IDENT: true, ADDRESS: true, CHARSTRING: true, UNIVERSAL: true, TIMER: true, ANYKW: true,
}

var typeFormalParStart = map[Kind]bool{
	IN: true, TYPE: true, SIGNATURE: true,
	IDENT: true, ADDRESS: true, CHARSTRING: true, UNIVERSAL: true,
}

var fieldStart = map[Kind]bool{
	MODIF:   true,
	ADDRESS: true, CHARSTRING: true, IDENT: true, NULL: true, UNIVERSAL: true,
	UNION: true, SET: true, RECORD: true, MAP: true, ENUMERATED: true,
	FUNCTION: true, ALTSTEP: true, TESTCASE: true,
}

var enumStart = map[Kind]bool{IDENT: true}

var withQualifierStart = map[Kind]bool{
	IDENT: true, LBRACK: true,
	TYPE: true, TEMPLATE: true, CONST: true, ALTSTEP: true, TESTCASE: true,
	FUNCTION: true, SIGNATURE: true, MODULEPAR: true, GROUP: true,
}

func union(sets ...map[Kind]bool) map[Kind]bool {
	m := make(map[Kind]bool)
	for _, s := range sets {
		for k, v := range s {
			m[k] = v
		}
	}
	return m
}

var topLevelTokens = map[Kind]bool{
	COMMA:      true,
	SEMICOLON:  true,
	MODULE:     true,
	CONTROL:    true,
	EXTERNAL:   true,
	FRIEND:     true,
	FUNCTION:   true,
	GROUP:      true,
	IMPORT:     true,
	MODULEPAR:  true,
	SIGNATURE:  true,
	TEMPLATE:   true,
	TYPE:       true,
	VAR:        true,
	ALTSTEP:    true,
	CONST:      true,
	PRIVATE:    true,
	PUBLIC:     true,
	TIMER:      true,
	PORT:       true,
	REPEAT:     true,
	BREAK:      true,
	CONTINUE:   true,
	LABEL:      true,
	GOTO:       true,
	RETURN:     true,
	SELECT:     true,
	ALT:        true,
	INTERLEAVE: true,
	LBRACK:     true,
	FOR:        true,
	WHILE:      true,
	DO:         true,
	IF:         true,
	LBRACE:     true,
	IDENT:      true,
	ANYKW:      true,
	ALL:        true,
	MAP:        true,
	UNMAP:      true,
	MTC:        true,
	TESTCASE:   true,
}

// parse is a generic entry point
func (p *parser) parse() Node {
	switch p.tok {
	case MODULE:
		return p.parseModule()

	case CONTROL,
		EXTERNAL,
		FRIEND,
		FUNCTION,
		GROUP,
		IMPORT,
		MODULEPAR,
		SIGNATURE,
		TEMPLATE,
		TYPE,
		VAR,
		ALTSTEP,
		CONST,
		PRIVATE,
		PUBLIC:
		return p.parseModuleDef()

	case TIMER, PORT,
		REPEAT, BREAK, CONTINUE,
		LABEL,
		GOTO,
		RETURN,
		SELECT,
		ALT, INTERLEAVE,
		LBRACK,
		FOR,
		WHILE,
		DO,
		IF,
		LBRACE,
		IDENT, ANYKW, ALL, MAP, UNMAP, MTC:
		return p.parseStmt()

	case TESTCASE:
		if p.peek(2).Kind() == DOT {
			return p.parseStmt()
		}
		return p.parseModuleDef()
	default:
		return p.parseExpr()
	}
}

/*************************************************************************
 * Expressions
 *************************************************************************/

// ExprList ::= Expr { "," Expr } [ "," ]
func (p *parser) parseExprList() (list []Expr) {
	if p.trace {
		defer un(trace(p, "ExprList"))
	}

	saved := p.noMatchAttr
	p.noMatchAttr = false
	defer func() { p.noMatchAttr = saved }()

	list = append(list, p.parseExpr())
	for p.tok == COMMA {
		p.consume()
		switch p.tok {
		case RPAREN, RBRACK, RBRACE:
			return list
		}
		list = append(list, p.parseExpr())
	}
	return list
}

// parseDelimitedExprList parses an ExprList up to the closing token close.
// An empty list is reported as missing operand, unless allowEmpty is set.
func (p *parser) parseDelimitedExprList(close Kind, allowEmpty bool) []Expr {
	if p.trace {
		defer un(trace(p, "ExprList"))
	}

	saved := p.noMatchAttr
	p.noMatchAttr = false
	defer func() { p.noMatchAttr = saved }()

	if p.tok == close && !allowEmpty {
		return []Expr{p.parseExpr()}
	}
	return parseList(p, "expression", close, exprStart, listRecovery, p.parseExpr, func(x *ErrorNode) Expr { return x })
}

// Expr ::= BinaryExpr
func (p *parser) parseExpr() Expr {
	if p.trace {
		defer un(trace(p, "Expr"))
	}

	x := p.parseBinaryExpr(LowestPrec + 1)
	if p.tok == HASH {
		x = p.parseRepetition(x)
	}
	return x
}

// Repetition ::= Expr "#" ( PrimaryExpr | "(" [Expr] ["," [Expr]] ")" )
func (p *parser) parseRepetition(x Expr) *RepetitionExpr {
	r := &RepetitionExpr{X: x, Hash: p.expect(HASH)}
	if p.tok != LPAREN {
		r.Min = p.parsePrimaryExpr()
		return r
	}
	r.LParen = p.consume()
	if p.tok != COMMA && p.tok != RPAREN {
		r.Min = p.parseExpr()
	}
	if p.tok == COMMA {
		r.Comma = p.consume()
		if p.tok != RPAREN {
			r.Max = p.parseExpr()
		}
	}
	r.RParen = p.expect(RPAREN)
	return r
}

// BinaryExpr ::= UnaryExpr
//
//	| BinaryExpr OP BinaryExpr
func (p *parser) parseBinaryExpr(prec1 int) Expr {
	x := p.parsePrefixExpr()
	for {
		prec := p.tok.Precedence()
		if prec < prec1 {
			return x
		}
		op := p.consume()
		// "value" matches the whole "Type : template", not the template
		// on the right of ":".
		if op.Kind() == COLON {
			saved := p.noValue
			p.noValue = true
			y := p.parseBinaryExpr(prec + 1)
			p.noValue = saved
			x = &BinaryExpr{X: x, Op: op, Y: y}
			if p.tok == VALUE && !p.noValue {
				x = &ValueExpr{X: x, Tok: p.consume(), Y: p.parseExpr()}
			}
			continue
		}
		// "length" and "ifpresent" after "implies" or "except" match the
		// whole template, not only the right operand.
		if op.Kind() == IMPLIES || op.Kind() == EXCEPT {
			saved := p.noMatchAttr
			p.noMatchAttr = true
			y := p.parseBinaryExpr(prec + 1)
			p.noMatchAttr = saved
			x = p.parseMatchAttrs(&BinaryExpr{X: x, Op: op, Y: y})
			continue
		}
		y := p.parseBinaryExpr(prec + 1)
		x = &BinaryExpr{X: x, Op: op, Y: y}
	}
}

// parsePrefixExpr parses "not" and "not4b". They bind looser than the
// operators above them in the precedence table: "not a == b" is
// "not (a == b)" and "not4b a + b" is "not4b (a + b)".
func (p *parser) parsePrefixExpr() Expr {
	if p.tok == NOT || p.tok == NOT4B {
		op := p.consume()
		return &UnaryExpr{Op: op, X: p.parseBinaryExpr(op.Kind().Precedence() + 1)}
	}
	return p.parsePostfixExpr()
}

func (p *parser) parsePostfixExpr() Expr {
	x := p.parseUnaryExpr()

	if p.tok == INC || p.tok == DEC || p.tok == ELIPSIS {
		x = &PostExpr{X: x, Op: p.consume()}
	}

	x = p.parseMatchAttrs(x)

	if p.tok == TO || p.tok == FROM {
		x = p.parseToFrom(x)
	}

	if p.tok == REDIR && !p.noRedirect {
		x = p.parseRedirect(x)
	}

	if p.tok == VALUE && !p.noValue {
		x = &ValueExpr{X: x, Tok: p.consume(), Y: p.parseExpr()}
	}

	if p.tok == PARAM && !p.noParam {
		x = &ParamExpr{X: x, Tok: p.consume(), Y: p.parseParenExpr()}
	}

	if p.tok == ALIVE {
		x = &UnaryExpr{Op: p.consume(), X: x}
	}

	return x
}

func (p *parser) parseMatchAttrs(x Expr) Expr {
	if p.tok == LENGTH && !p.noMatchAttr {
		x = p.parseLength(x)
	}

	if p.tok == IFPRESENT && !p.noMatchAttr {
		x = &UnaryExpr{Op: p.consume(), X: x}
	}
	return x
}

// UnaryExpr ::= "-"
//
//	| ("-"|"+"|"!") UnaryExpr
//	| PrimaryExpr
//
// "not" and "not4b" are parsed by parsePrefixExpr so they follow
// the precedence table instead of binding like unary "-".
func (p *parser) parseUnaryExpr() Expr {
	switch p.tok {
	case ADD, EXCL, SUB, INC, DEC:
		tok := p.consume()
		// handle unused expr '-'
		if tok.Kind() == SUB {
			switch p.tok {
			case COMMA, SEMICOLON, RBRACE, RBRACK, RPAREN, EOF:
				return &ValueLiteral{Tok: tok}
			}
		}
		return &UnaryExpr{Op: tok, X: p.parseUnaryExpr()}
	case COLONCOLON:
		tok := p.consume()
		return &BinaryExpr{Op: tok, Y: p.parseExpr()}
	}

	return p.parsePrimaryExpr()
}

// PrimaryExpr ::= Operand [{ExtFieldRef}] [Stuff]
//
// ExtFieldRef ::= "." ID
//               | "[" Expr "]"
//               | "(" ExprList ")"
//
// Stuff       ::= ["length"      "(" ExprList ")"]
//                 ["ifpresent"                   ]
//                 [("to"|"from") Expr            ]
//                 ["->"          Redirect        ]

// Redirect    ::= ["value"            ExprList   ]
//
//	["param"            ExprList   ]
//	["sender"           PrimaryExpr]
//	["@index" ["value"] PrimaryExpr]
//	["timestamp"        PrimaryExpr]
func (p *parser) parsePrimaryExpr() Expr {
	return p.parsePrimaryExprTail(p.parseOperand())
}

func (p *parser) parsePrimaryExprTail(x Expr) Expr {
L:
	for {
		switch p.tok {
		case DOT:
			x = p.parseSelectorExpr(x)
		case DECODE:
			// "=>" is evaluated left to right together with "." and "[]":
			// a.b=>e.f is (((a.b)=>e).f). The type is one identifier, or a
			// parenthesized type with an optional encoding.
			x = p.parseDecodeExpr(x)
		case LBRACK:
			// [] and [else], and a condition followed by another
			// operation, begin the next alt guard. They are not indexes.
			if p.bracketStartsAltGuard() {
				break L
			}
			x = p.parseIndexExpr(x)
		case LPAREN:
			x = p.parseCallExpr(x)
		default:
			break L
		}
	}
	return x
}

// Operand ::= ("any"|"all") ("component"|"port"|"timer"|"from" PrimaryExpr)
//
//	| Literal
//	| Reference
//
// Literal ::= INT | STRING | BSTRING | FLOAT
//
//	| "?" | "*"
//	| "none" | "inconc" | "pass" | "fail" | "error"
//	| "true" | "false"
//	| "not_a_number"
//
// Reference ::= ID
//
//	| "address" | ["unviersal"] "charstring" | "timer"
//	| "null" | "omit"
//	| "mtc" | "system" | "testcase"
//	| "map" | "unmap"
func (p *parser) parseOperand() Expr {
	etok := p.peek(1)
	switch p.tok {
	case ANYKW, ALL:
		tok := p.consume()
		switch p.tok {
		case COMPONENT, PORT, TIMER:
			return p.make_use(tok, p.consume())
		case FROM:
			return &FromExpr{
				KindTok: tok,
				FromTok: p.consume(),
				X:       p.parsePrimaryExpr(),
			}
		}

		return p.make_use(tok)

	case UNIVERSAL:
		return p.parseUniversalCharstring()

	case ADDRESS,
		CHARSTRING,
		CLASS,
		MAP,
		MTC,
		SYSTEM,
		TESTCASE,
		TIMER,
		UNMAP:
		return p.make_use(p.consume())

	case IDENT:
		// `objid { ... }` is the OBJECT IDENTIFIER value notation
		// (ETSI 6.1.0). `objid` is not a reserved word, so it only
		// becomes a literal when a brace list follows directly.
		if p.lit(1) == "objid" && p.peek(2).Kind() == LBRACE {
			return p.parseObjidLiteral()
		}
		return p.parseRef()

	case INT,
		ANY,
		NOTIMPL,
		BSTRING,
		ERROR,
		NULL,
		OMIT,
		FAIL,
		FALSE,
		FLOAT,
		INCONC,
		MUL,
		NAN,
		NONE,
		PASS,
		STRING,
		TRUE:
		return &ValueLiteral{Tok: p.consume()}

	case LPAREN:
		// can be template `x := (1,2,3)`, but also arithmetic expression: `1*(2+3)`
		return p.parseParenExpr()

	case LBRACK:
		return p.parseIndexExpr(nil)

	case LBRACE:
		return p.parseCompositeLiteral()

	case MODIFIES:
		return &ModifiesExpr{
			Tok:    p.consume(),
			X:      p.parsePrimaryExpr(),
			Assign: p.expect(ASSIGN),
			Y:      p.parseExpr(),
		}

	case REGEXP:
		return p.parseCallRegexp()

	case PATTERN:
		return p.parseCallPattern()

	case DECMATCH:
		return p.parseCallDecmatch()

	case MODIF:
		return p.parseModifier()

	case VALUE:
		return p.make_use(p.consume())

	case PRESENT:
		return p.parseCallExpr(p.make_use(p.consume()))

	}

	p.errorExpected("operand")
	if p.tok.IsKeyword() || operandRecovery[p.tok] {
		return &ErrorNode{From: etok, To: etok}
	}
	return &ErrorNode{From: etok, To: p.consume()}
}

// operandRecovery contains the non-keyword tokens which are not consumed by
// a missing operand, because the enclosing construct might expect them.
var operandRecovery = map[Kind]bool{
	EOF:       true,
	COMMA:     true,
	SEMICOLON: true,
	COLON:     true,
	ASSIGN:    true,
	REDIR:     true,
	GT:        true,
	RPAREN:    true,
	RBRACK:    true,
	RBRACE:    true,
}

func (p *parser) parseRef() Expr {
	id := p.parseIdent()
	if id == nil {
		return nil
	}

	if p.tok != LT {
		return id
	}

	p.mark()
	if x := p.tryTypeParameters(); x != nil && !operandStart[p.tok] {
		p.commit()
		return &ParametrizedIdent{Ident: id, Params: x}
	}
	p.reset()
	return id
}

// ExceptionSpec ::= "exception" "(" TypeOrNestedTypeDef {"," TypeOrNestedTypeDef} [","] ")"
//
// A type of the list that is a reference is parsed as an expression is
// (parseParenExpr).
func (p *parser) parseExceptionList() *ParenExpr {
	item := func() Expr {
		switch p.tok {
		case RECORD, SET, UNION, MAP, ENUMERATED:
			return p.parseTypeOrNestedTypeDef()
		}
		return p.parseExpr()
	}
	x := &ParenExpr{LParen: p.expect(LPAREN)}
	x.List = append(x.List, item())
	for p.tok == COMMA {
		p.consume()
		if p.tok == RPAREN {
			break
		}
		x.List = append(x.List, item())
	}
	x.RParen = p.expect(RPAREN)
	return x
}

func (p *parser) parseParenExpr() *ParenExpr {
	return &ParenExpr{
		LParen: p.expect(LPAREN),
		List:   p.parseDelimitedExprList(RPAREN, false),
		RParen: p.expect(RPAREN),
	}
}

func (p *parser) parseUniversalCharstring() *Ident {
	return p.make_use(p.expect(UNIVERSAL), p.expect(CHARSTRING))
}

// parseObjidLiteral parses the OBJECT IDENTIFIER value notation
// `objid { component+ }`. Components are whitespace separated (no
// commas): a name form (`itu_t`), a number form (`7`), or a
// name-and-number form (`question(1)`), each of which parses as one
// primary expression.
func (p *parser) parseObjidLiteral() *ObjidLiteral {
	x := new(ObjidLiteral)
	x.Tok = p.consume()
	x.LBrace = p.expect(LBRACE)
	for p.tok != RBRACE && p.tok != EOF {
		e := p.parsePrimaryExpr()
		x.List = append(x.List, e)
		if _, bad := e.(*ErrorNode); bad {
			break
		}
	}
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseCompositeLiteral() *CompositeLiteral {
	c := new(CompositeLiteral)
	c.LBrace = p.expect(LBRACE)
	c.List = p.parseDelimitedExprList(RBRACE, true)
	c.RBrace = p.expect(RBRACE)
	return c
}

func (p *parser) parseCallRegexp() *RegexpExpr {
	c := new(RegexpExpr)
	c.Tok = p.expect(REGEXP)
	if p.tok == MODIF {
		c.NoCase = p.consume()
	}
	c.X = p.parseParenExpr()
	return c
}

func (p *parser) parseCallPattern() *PatternExpr {
	c := new(PatternExpr)
	c.Tok = p.expect(PATTERN)
	if p.tok == MODIF {
		c.NoCase = p.consume()
	}
	// "length" and "ifpresent" match the whole pattern, so the pattern
	// body must not consume them. The caller applies them afterwards.
	saved := p.noMatchAttr
	p.noMatchAttr = true
	c.X = p.parseExpr()
	p.noMatchAttr = saved
	return c
}

func (p *parser) parseCallDecmatch() *DecmatchExpr {
	c := new(DecmatchExpr)
	c.Tok = p.expect(DECMATCH)
	if p.tok == LPAREN {
		// decmatch ["(" Expression ")"] TemplateInstance.
		// A parenthesized expression is itself a template, so
		// decmatch(x) keeps the parentheses as the template when
		// nothing that can start a template follows.
		paren := p.parseParenExpr()
		if !startsExpr(p.tok) {
			c.X = paren
			return c
		}
		c.Params = paren
	}
	c.X = p.parseExpr()
	return c
}

// startsExpr reports whether tok can start an expression or template.
func startsExpr(tok Kind) bool {
	return exprStart[tok]
}

// bracketStartsAltGuard reports whether the current '[' begins the next alt
// guard rather than an index of the guard operation. An empty '[]' or
// '[else]' does. A '[expr]' does when another operation follows the ']'.
func (p *parser) bracketStartsAltGuard() bool {
	if !p.altGuard || p.tok != LBRACK {
		return false
	}
	switch p.peek(2).Kind() {
	case RBRACK, ELSE:
		return true
	}

	depth := 0
	for i := 1; ; i++ {
		k := p.peek(i).Kind()
		switch k {
		case EOF:
			return false
		case LBRACK, LPAREN, LBRACE:
			depth++
		case RBRACK, RPAREN, RBRACE:
			depth--
			if depth > 0 {
				break
			}
			if k != RBRACK {
				return false
			}
			switch p.peek(i + 1).Kind() {
			case IDENT, ANYKW, ALL, MTC, SYSTEM:
				return true
			default:
				return false
			}
		}
	}
}

func (p *parser) parseModifier() Expr {
	if p.peek(1).String() == "@dynamic" {
		return p.parseDynamicModifier()
	}
	return p.parseDecodedModifier()
}

func (p *parser) parseDecodedModifier() *DecodedExpr {
	d := new(DecodedExpr)
	d.Tok = p.expect(MODIF)
	if s := d.Tok.String(); s != "@decoded" {
		p.error(d.Tok, "expected '@decoded', found %s", s)
	}

	if p.tok == LPAREN {
		d.Params = p.parseParenExpr()
	}
	d.X = p.parsePrimaryExpr()
	return d
}

func (p *parser) parseDynamicModifier() *DynamicExpr {
	d := new(DynamicExpr)
	d.Tok = p.expect(MODIF)
	d.Body = p.parseBlockStmt()
	return d
}

// parseToFrom parses "to" or "from" and its address. A following "->"
// belongs to the operation, not to the address.
func (p *parser) parseToFrom(x Expr) Expr {
	op := p.consume()
	saved := p.noRedirect
	p.noRedirect = true
	y := p.parseExpr()
	p.noRedirect = saved
	return &BinaryExpr{X: x, Op: op, Y: y}
}

// parseDecodeExpr parses "=>" Type. Dots and indexes after the type refer
// to the decoded value, so the type itself stops after one name.
func (p *parser) parseDecodeExpr(x Expr) Expr {
	op := p.consume()
	var y Expr
	switch p.tok {
	case LPAREN:
		y = p.parseParenExpr()
	case UNIVERSAL:
		y = p.parseUniversalCharstring()
	case ADDRESS, CHARSTRING, TIMER, PORT, CLASS:
		y = p.make_use(p.consume())
	default:
		y = p.parseRef()
	}
	return &BinaryExpr{X: x, Op: op, Y: y}
}

func (p *parser) parseSelectorExpr(x Expr) *SelectorExpr {
	dot := p.consume()
	switch p.tok {
	case ENCODE, VARIANT, DISPLAY, EXTENSION, OPTIONAL, PRESENT:
		return &SelectorExpr{X: x, Dot: dot, Sel: p.make_use(p.consume())}
	}
	return &SelectorExpr{X: x, Dot: dot, Sel: p.parseRef()}
}

func (p *parser) parseIndexExpr(x Expr) *IndexExpr {
	return &IndexExpr{
		X:      x,
		LBrack: p.expect(LBRACK),
		Index:  p.parseExpr(),
		RBrack: p.expect(RBRACK),
	}
}

func (p *parser) parseCallExpr(x Expr) *CallExpr {
	c := new(CallExpr)
	c.Fun = x
	c.Args = new(ParenExpr)
	c.Args.LParen = p.expect(LPAREN)
	if p.tok != RPAREN {
		switch p.tok {
		case TO, FROM, REDIR:
			var x Expr
			if p.tok == TO || p.tok == FROM {
				// TODO: Shouldn't this be a FromExpr?
				x = p.parseToFrom(x)
			}
			if p.tok == REDIR {
				x = p.parseRedirect(x)
			}
			c.Args.List = []Expr{x}
		default:
			c.Args.List = p.parseDelimitedExprList(RPAREN, true)
		}
	}
	c.Args.RParen = p.expect(RPAREN)
	return c
}

func (p *parser) parseLength(x Expr) *LengthExpr {
	return &LengthExpr{
		X:    x,
		Len:  p.expect(LENGTH),
		Size: p.parseParenExpr(),
	}
}

func (p *parser) parseRedirect(x Expr) *RedirectExpr {

	r := &RedirectExpr{
		X:   x,
		Tok: p.expect(REDIR),
	}

	if p.tok == VALUE {
		r.ValueTok = p.expect(VALUE)
		// "param", "sender" and "@index" that follow belong to this
		// redirect. "param" would otherwise become part of the value.
		saved := p.noParam
		p.noParam = true
		r.Value = p.parseExprList()
		p.noParam = saved
	}

	// `-> verdict v` captures a called/terminated component's verdict
	// (ETSI 21.3.10). `verdict` is not a reserved keyword - it lexes as
	// an IDENT - so match on the literal.
	if p.tok == IDENT && p.lit(1) == "verdict" {
		r.VerdictTok = p.consume()
		r.Verdict = p.parseExprList()
	}

	if p.tok == PARAM {
		r.ParamTok = p.expect(PARAM)
		r.Param = p.parseExprList()
	}

	if p.tok == SENDER {
		r.SenderTok = p.expect(SENDER)
		r.Sender = p.parsePrimaryExpr()
	}

	if p.tok == MODIF {
		if s := p.lit(1); s != "@index" {
			p.error(p.peek(1), "expected '@index', found %s", s)
		}

		r.IndexTok = p.consume()
		if p.tok == VALUE {
			r.IndexValueTok = p.consume()
		}
		r.Index = p.parsePrimaryExpr()
	}

	if p.tok == TIMESTAMP {
		r.TimestampTok = p.expect(TIMESTAMP)
		r.Timestamp = p.parsePrimaryExpr()
	}

	return r
}

func (p *parser) parseName() *Ident {
	switch p.tok {
	case IDENT, ADDRESS, CONTROL, CLASS:
		id := &Ident{Tok: p.consume(), IsName: true}
		if p.names != nil {
			p.names[id.String()] = true
		}
		return id
	}
	p.expectIdent()
	return nil

}

// expectIdent reports a missing identifier. Unlike expect, it does not
// consume a delimiter.
func (p *parser) expectIdent() {
	p.errorExpected("'IDENT'")
	if !delimiters[p.tok] {
		p.consume()
	}
}

func (p *parser) parseIdent() *Ident {
	switch p.tok {
	case UNIVERSAL:
		return p.parseUniversalCharstring()
	case IDENT, ADDRESS, ALIVE, CHARSTRING, CONTROL, TO, FROM, CREATE, VALUE, CLASS:
		return p.make_use(p.consume())
	default:
		p.expectIdent()
		return nil
	}
}

func (p *parser) parseArrayDefs() []*ParenExpr {
	var l []*ParenExpr
	for p.tok == LBRACK {
		l = append(l, p.parseArrayDef())
	}
	return l
}

func (p *parser) parseArrayDef() *ParenExpr {
	return &ParenExpr{
		LParen: p.expect(LBRACK),
		List:   p.parseDelimitedExprList(RBRACK, false),
		RParen: p.expect(RBRACK),
	}
}

func (p *parser) parseRefList() []Expr {
	l := make([]Expr, 0, 1)
	for {
		l = append(l, p.parseTypeRef())
		if p.tok != COMMA {
			break
		}
		p.consume()
		switch p.tok {
		case LBRACE, RBRACE, SEMICOLON:
			return l
		}
	}
	return l
}

func (p *parser) parseTypeRef() Expr {
	if p.trace {
		defer un(trace(p, "TypeRef"))
	}
	return p.parsePrimaryExpr()
}

// TypeOrNestedTypeDef ::= TypeRef | NestedTypeDef
//
// A nested type definition is recorded for NestedTypes.
func (p *parser) parseTypeOrNestedTypeDef() Expr {
	var x Expr
	switch p.tok {
	case RECORD, SET, UNION, ENUMERATED:
		x = p.parseTypeSpec().(Expr)
	case MAP:
		if p.peek(2).Kind() == FROM {
			x = p.parseMapSpec()
		}
	}
	if x == nil {
		return p.parseTypeRef()
	}
	p.Root.nested = append(p.Root.nested, x)
	return x
}

// parseModifiers parses an optional "@lazy" or "@fuzzy" modifier followed by
// an optional "@deterministic" modifier.
func (p *parser) parseModifiers() (modif Token, deterministic Token) {
	for p.tok == MODIF {
		tok := p.peek(1)
		switch {
		case tok.String() == "@deterministic" && deterministic == nil:
			deterministic = p.consume()
		case tok.String() != "@deterministic" && modif == nil && deterministic == nil:
			modif = p.consume()
		default:
			p.error(tok, "unexpected modifier %s", tok.String())
			p.consume()
		}
	}
	return modif, deterministic
}

func (p *parser) tryTypeParameters() *ParenExpr {
	if p.trace {
		defer un(trace(p, "tryTypeParameters"))
	}
	x := &ParenExpr{
		LParen: p.consume(),
	}
	for p.tok != GT {
		y := p.tryTypeParameter()
		if y == nil {
			return nil
		}
		x.List = append(x.List, y)
		if p.tok != COMMA {
			break
		}
		p.consume()
		if p.tok == GT {
			break
		}
	}

	if p.tok != GT {
		return nil
	}
	x.RParen = p.consume()
	return x
}

func (p *parser) tryTypeParameter() Expr {
	if p.trace {
		defer un(trace(p, "tryTypeParameter"))
	}
	x := p.tryTypeIdent()
L:
	for {
		switch p.tok {
		case DOT:
			x = &SelectorExpr{
				X:   x,
				Dot: p.consume(),
				Sel: p.tryTypeIdent(),
			}
			if x.(*SelectorExpr).Sel == nil {
				return nil
			}
		case LBRACK:
			lbrack := p.consume()
			dash := p.consume()
			rbrack := p.consume()
			if dash.Kind() != SUB || rbrack.Kind() != RBRACK {
				return nil
			}
			x = &IndexExpr{
				X:      x,
				LBrack: lbrack,
				Index:  &ValueLiteral{Tok: dash},
				RBrack: rbrack,
			}

		default:
			break L
		}
	}
	return x
}

func (p *parser) tryTypeIdent() Expr {
	if p.trace {
		defer un(trace(p, "tryTypeIdent"))
	}

	var x *Ident
	switch p.tok {
	case IDENT, ADDRESS, CHARSTRING:
		x = p.make_use(p.consume())
	case UNIVERSAL:
		x = p.parseUniversalCharstring()
	default:
		return nil
	}

	if p.tok == LT {
		if y := p.tryTypeParameters(); y == nil {
			return &ParametrizedIdent{
				Ident:  x,
				Params: y,
			}
		}
	}
	return x
}

/*************************************************************************
 * Module
 *************************************************************************/

func (p *parser) parseModule() *Module {
	if p.trace {
		defer un(trace(p, "Module"))
	}

	m := new(Module)
	m.Tok = p.expect(MODULE)
	m.Name = p.parseName()

	if p.tok == LANGUAGE {
		m.Language = p.parseLanguageSpec()
	} else if p.tok == STRING {
		// TTCN-3 OO 2018 (ETSI ES 203 022) allows a bare
		// language-string after the module name as in
		// `module M "TTCN-3:2018 Object-Oriented" { ... }`.
		// Accept the abbreviated form by synthesising the
		// implicit `language` keyword.
		m.Language = p.parseLanguageSpecImplicit()
	}

	m.LBrace = p.expect(LBRACE)

	for p.tok != RBRACE && p.tok != EOF {
		m.Defs = append(m.Defs, p.parseModuleDef())
		p.expectSemi(m.Defs[len(m.Defs)-1].LastTok())
	}
	m.RBrace = p.expect(RBRACE)
	m.With = p.parseWith()
	return m
}

func (p *parser) parseLanguageSpec() *LanguageSpec {
	l := new(LanguageSpec)
	l.Tok = p.consume()
	for {
		l.List = append(l.List, p.expect(STRING))
		if p.tok != COMMA {
			break
		}
		p.consume()
		if p.tok == LBRACE {
			break
		}
	}
	return l
}

// parseLanguageSpecImplicit handles the abbreviated `module M "..." {`
// form from the TTCN-3 OO 2018 extension. There is no `language`
// keyword in the source, so we leave the Tok zero-valued and only
// populate the list of strings.
func (p *parser) parseLanguageSpecImplicit() *LanguageSpec {
	l := new(LanguageSpec)
	for {
		l.List = append(l.List, p.expect(STRING))
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
	return l
}

func (p *parser) parseModuleDef() *ModuleDef {
	m := new(ModuleDef)
	switch p.tok {
	case PRIVATE, PUBLIC:
		m.Visibility = p.consume()
	case FRIEND:
		if p.peek(2).Kind() != MODULE {
			m.Visibility = p.consume()
		}
	}

	etok := p.peek(1)
	switch p.tok {
	case IMPORT:
		m.Def = p.parseImport()
	case GROUP:
		m.Def = p.parseGroup()
	case FRIEND:
		m.Def = p.parseFriend()
	case TYPE:
		m.Def = p.parseTypeDecl()
	case TEMPLATE:
		m.Def = p.parseTemplateDecl()
	case MODULEPAR:
		m.Def = p.parseModulePar()
	case VAR, CONST:
		m.Def = p.parseValueDecl()
	case SIGNATURE:
		m.Def = p.parseSignatureDecl()
	case FUNCTION, TESTCASE, ALTSTEP:
		m.Def = p.parseFuncDecl()
	case CREATE:
		m.Def = p.parseConstructorDecl()
	case CONTROL:
		m.Def = &ControlPart{Name: p.parseIdent(), Body: p.parseBlockStmt(), With: p.parseWith()}
	case EXTERNAL:
		switch p.peek(2).Kind() {
		case FUNCTION:
			m.Def = p.parseExtFuncDecl()
		case CONST:
			p.error(p.peek(1), "external constants are not supported anymore")
			p.consume()
			m.Def = p.parseValueDecl()
		default:
			p.errorExpected("'function'") // TODO: fix the found-token! (peek(1) vs. peek(2))
			p.advance(stmtStart)
			m.Def = &ErrorNode{From: etok, To: p.peek(1)}
		}
	case SEMICOLON, RBRACE, EOF:
		p.errorExpected("module definition")
		m.Def = &ErrorNode{From: etok, To: etok}
	default:
		depth := 0
		if p.tok == LBRACE {
			depth++
		}
		m.Def = p.advanceWithError(func(k Kind) bool {
			switch k {
			case LBRACE:
				depth++
			case RBRACE:
				depth--
				return depth >= 0
			}
			return depth > 0 || !moduleDefStart[k]
		}, "expected module definition, found %s", etok)
	}

	if m.Def != nil {
		p.addName(m.Def)
	}
	return m
}

func (p *parser) addName(n Node) {
	switch n := n.(type) {
	case *ValueDecl:
		for _, n := range n.Decls {
			p.addName(n)
		}
	default:
		if name := Name(n); p.names != nil && name != "" {
			p.names[name] = true
		}
	}
}

/*************************************************************************
 * Import Definition
 *************************************************************************/

func (p *parser) make_use(toks ...Token) *Ident {
	if len(toks) != 1 && len(toks) != 2 {
		panic("No support for multi-token identifiers.")
	}
	id := &Ident{Tok: toks[0]}
	if p.uses != nil {
		p.uses[toks[0].String()] = true
	}
	if len(toks) == 2 {
		id.Tok2 = toks[1]
		if p.uses != nil {
			p.uses[toks[1].String()] = true
		}
	}
	return id
}

func (p *parser) parseImport() *ImportDecl {
	if p.trace {
		defer un(trace(p, "Import"))
	}

	x := new(ImportDecl)
	x.ImportTok = p.consume()
	x.FromTok = p.expect(FROM)
	x.Module = p.parseIdent()

	if p.tok == LANGUAGE {
		x.Language = p.parseLanguageSpec()
	}

	if p.tok == IDENT && p.lit(1) == "recursive" {
		x.Recursive = p.consume()
	}

	if p.tok == REDIR {
		x.RedirTok = p.consume()
		x.Alias = p.parseName()
	}

	switch p.tok {
	case ALL:
		y := &DefKindExpr{}
		var z Expr = p.make_use(p.consume())
		if p.tok == EXCEPT {
			z = &ExceptExpr{
				X:         z,
				ExceptTok: p.consume(),
				LBrace:    p.expect(LBRACE),
				List:      p.parseExceptStmts(),
				RBrace:    p.expect(RBRACE),
			}
		}
		y.List = []Expr{z}
		x.List = append(x.List, y)
	case LBRACE:
		x.LBrace = p.expect(LBRACE)
		for p.tok != RBRACE && p.tok != EOF {
			x.List = append(x.List, p.parseImportStmt())
			p.expectSemi(x.List[len(x.List)-1].LastTok())
		}
		x.RBrace = p.expect(RBRACE)
	default:
		p.errorExpected("'all' or import spec")
	}

	x.With = p.parseWith()

	return x
}

func (p *parser) parseImportStmt() *DefKindExpr {
	x := new(DefKindExpr)
	switch p.tok {
	case ALTSTEP, CONST, FUNCTION, MODULEPAR,
		SIGNATURE, TEMPLATE, TESTCASE, TYPE:
		x.KindTok = p.consume()
		if p.tok == ALL {
			var y Expr = p.make_use(p.consume())
			if p.tok == EXCEPT {
				y = &ExceptExpr{
					X:         y,
					ExceptTok: p.consume(),
					List:      p.parseRefList(),
				}
			}
			x.List = []Expr{y}
		} else {
			x.List = p.parseRefList()
		}
	case GROUP:
		x.KindTok = p.consume()
		for {
			y := p.parseTypeRef()
			if p.tok == EXCEPT {
				y = &ExceptExpr{
					X:         y,
					ExceptTok: p.consume(),
					LBrace:    p.expect(LBRACE),
					List:      p.parseExceptStmts(),
					RBrace:    p.expect(RBRACE),
				}
			}
			x.List = append(x.List, y)
			if p.tok != COMMA {
				break
			}
			p.consume()
			switch p.tok {
			case RBRACE, SEMICOLON:
				return x
			}
		}
	case IMPORT:
		x.KindTok = p.consume()
		x.List = []Expr{p.make_use(p.expect(ALL))}
	case SEMICOLON, RBRACE, EOF:
		p.errorExpected("import definition qualifier")
	default:
		x.List = []Expr{p.advanceWithError(skipItem, "expected import definition qualifier, found %s", p.peek(1))}
	}
	return x
}

func (p *parser) parseExceptStmts() []Expr {
	var list []Expr
	for p.tok != RBRACE && p.tok != EOF {
		x := p.parseExceptStmt()
		p.expectSemi(x.LastTok())
		list = append(list, x)
	}
	return list
}

func (p *parser) parseExceptStmt() *DefKindExpr {
	x := new(DefKindExpr)
	switch p.tok {
	case ALTSTEP, CONST, FUNCTION, GROUP,
		IMPORT, MODULEPAR, SIGNATURE, TEMPLATE,
		TESTCASE, TYPE:
		x.KindTok = p.consume()
	case SEMICOLON, RBRACE, EOF:
		p.errorExpected("definition qualifier")
		return x
	default:
		x.List = []Expr{p.advanceWithError(skipItem, "expected definition qualifier, found %s", p.peek(1))}
		return x
	}

	if p.tok == ALL {
		x.List = []Expr{p.make_use(p.consume())}
	} else {
		x.List = p.parseRefList()
	}
	return x
}

/*************************************************************************
 * Group Definition
 *************************************************************************/

func (p *parser) parseGroup() *GroupDecl {
	x := new(GroupDecl)
	x.Tok = p.consume()
	x.Name = p.parseName()
	x.LBrace = p.expect(LBRACE)

	for p.tok != RBRACE && p.tok != EOF {
		x.Defs = append(x.Defs, p.parseModuleDef())
		p.expectSemi(x.Defs[len(x.Defs)-1].LastTok())
	}
	x.RBrace = p.expect(RBRACE)
	x.With = p.parseWith()
	return x
}

func (p *parser) parseFriend() *FriendDecl {
	x := new(FriendDecl)
	x.FriendTok = p.expect(FRIEND)
	x.ModuleTok = p.expect(MODULE)
	for {
		if id := p.parseIdent(); id != nil {
			x.Modules = append(x.Modules, id)
		}
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * With Attributes
 *************************************************************************/

func (p *parser) parseWith() *WithSpec {
	if p.tok != WITH {
		return nil
	}
	x := new(WithSpec)
	x.Tok = p.consume()
	x.LBrace = p.expect(LBRACE)
	for p.tok != RBRACE && p.tok != EOF {
		x.List = append(x.List, p.parseWithStmt())
		p.expectSemi(x.List[len(x.List)-1].LastTok())
	}
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseWithStmt() *WithStmt {
	if p.trace {
		defer un(trace(p, "WithStmt"))
	}

	x := new(WithStmt)

	switch p.tok {
	case ENCODE,
		VARIANT,
		DISPLAY,
		EXTENSION,
		OPTIONAL,
		STEPSIZE,
		OVERRIDE,
		TEMPLATE: // TTCN-3 V5: how a module's templates are evaluated
		x.KindTok = p.consume()
	case SEMICOLON, RBRACE, EOF:
		p.errorExpected("with-attribute")
		return x
	default:
		x.Value = p.advanceWithError(skipItem, "expected with-attribute, found %s", p.peek(1))
		return x
	}

	switch p.tok {
	case OVERRIDE:
		x.Override = p.consume()
	case MODIF:
		if p.lit(1) != "@local" {
			p.errorExpected("@local")
		}
		x.Override = p.consume()
	}

	if p.tok == LPAREN {
		x.LParen = p.consume()
		x.List = parseList(p, "with-qualifier", RPAREN, withQualifierStart, listRecovery, p.parseWithQualifier,
			func(x *ErrorNode) Expr { return x })
		x.RParen = p.expect(RPAREN)
	}

	if p.tok == LBRACE {
		x.Value = &SelectorExpr{
			X:   p.parseRelatedEncodings(),
			Dot: p.expect(DOT),
			Sel: &ValueLiteral{Tok: p.expect(STRING)},
		}
		return x
	}

	var v Expr = &ValueLiteral{Tok: p.expect(STRING)}
	if p.tok == DOT {
		v = &SelectorExpr{
			X:   v,
			Dot: p.consume(),
			Sel: &ValueLiteral{Tok: p.expect(STRING)},
		}
	}
	x.Value = v

	return x
}

// RelatedEncodings ::= "{" STRING {"," STRING} [","] "}"
func (p *parser) parseRelatedEncodings() *CompositeLiteral {
	c := new(CompositeLiteral)
	c.LBrace = p.expect(LBRACE)
	for {
		c.List = append(c.List, &ValueLiteral{Tok: p.expect(STRING)})
		if p.tok != COMMA {
			break
		}
		p.consume()
		if p.tok == RBRACE {
			break
		}
	}
	c.RBrace = p.expect(RBRACE)
	return c
}

func (p *parser) parseWithQualifier() Expr {
	etok := p.peek(1)
	switch p.tok {
	case IDENT:
		return p.parseTypeRef()
	case LBRACK:
		return p.parsePrimaryExprTail(nil)
	case TYPE, TEMPLATE, CONST, ALTSTEP, TESTCASE, FUNCTION, SIGNATURE, MODULEPAR, GROUP:
		x := new(DefKindExpr)
		x.KindTok = p.consume()
		var y Expr = p.make_use(p.expect(ALL))
		if p.tok == EXCEPT {
			y = &ExceptExpr{
				X:         y,
				ExceptTok: p.consume(),
				LBrace:    p.expect(LBRACE),
				List:      p.parseRefList(),
				RBrace:    p.expect(RBRACE),
			}
		}
		x.List = []Expr{y}
		return x
	default:
		p.errorExpected("with-qualifier")
		p.advance(stmtStart)
		return &ErrorNode{From: etok, To: p.peek(1)}
	}
}

/*************************************************************************
 * Type Definitions
 *************************************************************************/

func (p *parser) parseTypeDecl() Decl {
	switch p.peek(2).Kind() {
	case IDENT, ADDRESS, CHARSTRING, NULL, UNIVERSAL:
		return p.parseSubTypeDecl()
	case PORT:
		return p.parsePortTypeDecl()
	case COMPONENT:
		return p.parseComponentTypeDecl()
	case CLASS:
		return p.parseClassTypeDecl()
	case EXTERNAL:
		// `type external class ...` (ETSI 5.1.1.3 external classes).
		if p.peek(3).Kind() == CLASS {
			return p.parseClassTypeDecl()
		}
		etok := p.peek(1)
		p.errorExpected("type definition")
		p.advance(stmtStart)
		return &ErrorNode{From: etok, To: p.peek(1)}
	case UNION:
		return p.parseStructTypeDecl()
	case MAP:
		return p.parseMapTypeDecl()
	case SET, RECORD:
		if p.peek(3).Kind() == IDENT || p.peek(3).Kind() == ADDRESS {
			return p.parseStructTypeDecl()
		}
		// lists are also parsed by parseSubTypeDecl
		return p.parseSubTypeDecl()
	case ENUMERATED:
		return p.parseEnumTypeDecl()
	case FUNCTION, ALTSTEP, TESTCASE:
		return p.parseBehaviourTypeDecl()
	default:
		return p.advanceWithError(func(k Kind) bool { return !stmtStart[k] }, "expected type definition, found %s", p.peek(2))
	}
}

/*************************************************************************
 * Port Type
 *************************************************************************/

func (p *parser) parsePortTypeDecl() *PortTypeDecl {
	if p.trace {
		defer un(trace(p, "PortTypeDecl"))
	}
	x := new(PortTypeDecl)
	x.TypeTok = p.consume()
	x.PortTok = p.consume()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}

	switch p.tok {
	case MIXED, MESSAGE, PROCEDURE:
		x.KindTok = p.consume()
	case LBRACE, REALTIME:
		// A port type without its kind is a message port type
		// (TTCN-3 V5, ETSI TR 104 081 6.5.4).
	default:
		p.errorExpected("'message' or 'procedure'")
	}

	if p.tok == REALTIME {
		x.Realtime = p.consume()
	}

	x.LBrace = p.expect(LBRACE)
	for p.tok != RBRACE && p.tok != EOF {
		x.Attrs = append(x.Attrs, p.parsePortAttribute())
		p.expectSemi(x.Attrs[len(x.Attrs)-1].LastTok())
	}
	x.RBrace = p.expect(RBRACE)
	x.With = p.parseWith()
	return x
}

func (p *parser) parsePortAttribute() Node {
	if p.trace {
		defer un(trace(p, "PortAttribute"))
	}
	etok := p.peek(1)
	switch p.tok {
	case IN, OUT, INOUT, ADDRESS:
		return &PortAttribute{
			KindTok: p.consume(),
			Types:   p.parseRefList(),
		}
	case MAP, UNMAP:
		return &PortMapAttribute{
			MapTok:   p.consume(),
			ParamTok: p.expect(PARAM),
			Params:   p.parseFormalPars(),
		}
	case SEMICOLON, RBRACE, EOF:
		p.errorExpected("port attribute")
		return &ErrorNode{From: etok, To: etok}
	default:
		return p.advanceWithError(skipItem, "expected port attribute, found %s", etok)
	}
}

/*************************************************************************
 * Component Type
 *************************************************************************/

func (p *parser) parseComponentTypeDecl() *ComponentTypeDecl {
	if p.trace {
		defer un(trace(p, "ComponentTypeDecl"))
	}
	x := new(ComponentTypeDecl)
	x.TypeTok = p.consume()
	x.CompTok = p.consume()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}
	if p.tok == EXTENDS {
		x.ExtendsTok = p.consume()
		x.Extends = p.parseRefList()
	}
	x.Body = p.parseBlockStmt()
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Struct Type Declaration
 *************************************************************************/

func (p *parser) parseStructTypeDecl() *StructTypeDecl {
	if p.trace {
		defer un(trace(p, "StructTypeDecl"))
	}
	x := new(StructTypeDecl)
	x.TypeTok = p.consume()
	x.KindTok = p.consume()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}
	x.LBrace = p.expect(LBRACE)
	x.Fields = p.parseFieldList()
	x.RBrace = p.expect(RBRACE)
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Class Type Declaration
 *************************************************************************/

func (p *parser) parseClassTypeDecl() *ClassTypeDecl {
	if p.trace {
		defer un(trace(p, "ClassTypeDecl"))
	}

	x := new(ClassTypeDecl)

	x.TypeTok = p.consume()
	if p.tok == EXTERNAL {
		x.ExternalTok = p.consume()
	}
	x.KindTok = p.consume()
	if p.tok == MODIF {
		x.Modif = p.consume()
	}
	x.Name = p.parseName()
	if p.tok == EXTENDS {
		x.ExtendsTok = p.consume()
		x.Extends = p.parseRefList()
	}
	if p.tok == RUNS {
		x.RunsOn = p.parseRunsOn()
	}
	if p.tok == MTC {
		x.Mtc = p.parseMtc()
	}
	if p.tok == SYSTEM {
		x.System = p.parseSystem()
	}
	x.LBrace = p.expect(LBRACE)
	for p.tok != RBRACE && p.tok != EOF {
		x.Defs = append(x.Defs, p.parseClassMemberDef())
		p.expectSemi(x.Defs[len(x.Defs)-1].LastTok())
	}
	x.RBrace = p.expect(RBRACE)
	if p.tok == IDENT && p.lit(1) == "finally" {
		x.FinallyTok = p.consume()
		x.Finally = p.parseBlockStmt()
	}
	x.With = p.parseWith()
	return x
}

// parseClassMemberDef parses a single class member. It mirrors
// parseModuleDef but additionally accepts `port` and `timer` member
// declarations (ETSI 5.1.1: a class may own ports and timers), which
// are not valid at module scope. Everything else is delegated to
// parseModuleDef so module-level parsing is unaffected.
func (p *parser) parseClassMemberDef() *ModuleDef {
	kind := p.tok
	if kind == PRIVATE || kind == PUBLIC {
		kind = p.peek(2).Kind()
	}
	if kind != PORT && kind != TIMER {
		return p.parseModuleDef()
	}
	m := new(ModuleDef)
	if p.tok == PRIVATE || p.tok == PUBLIC {
		m.Visibility = p.consume()
	}
	m.Def = p.parseValueDecl()
	if m.Def != nil {
		p.addName(m.Def)
	}
	return m
}

/*************************************************************************
 * Map Type Declaration
 *************************************************************************/

func (p *parser) parseMapTypeDecl() *MapTypeDecl {
	if p.trace {
		defer un(trace(p, "MapTypeDecl"))
	}
	x := new(MapTypeDecl)
	x.TypeTok = p.consume()
	x.Spec = p.parseMapSpec()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Enumeration Type Declaration
 *************************************************************************/

func (p *parser) parseEnumTypeDecl() *EnumTypeDecl {
	if p.trace {
		defer un(trace(p, "EnumTypeDecl"))
	}

	x := new(EnumTypeDecl)
	x.TypeTok = p.consume()
	x.EnumTok = p.consume()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}
	x.LBrace = p.expect(LBRACE)
	x.Enums = parseList(p, "enumerated value", RBRACE, enumStart, listRecovery, p.parseEnum, func(x *ErrorNode) Expr { return x })
	x.RBrace = p.expect(RBRACE)
	x.With = p.parseWith()
	return x
}

func (p *parser) parseEnum() Expr {
	var firstIdent func(n Expr) *Ident
	firstIdent = func(n Expr) *Ident {
		switch n := n.(type) {
		case *CallExpr:
			return firstIdent(n.Fun)
		case *SelectorExpr:
			return firstIdent(n.X)
		case *Ident:
			return n
		default:
			return nil
		}
	}
	x := p.parseExpr()
	if id := firstIdent(x); p.names != nil && id != nil {
		p.names[id.String()] = true
		id.IsName = true
	}
	return x
}

/*************************************************************************
 * Behaviour Type Declaration
 *************************************************************************/

func (p *parser) parseBehaviourTypeDecl() *BehaviourTypeDecl {
	if p.trace {
		defer un(trace(p, "BehaviourTypeDecl"))
	}
	x := new(BehaviourTypeDecl)
	x.TypeTok = p.consume()
	x.KindTok = p.consume()

	if p.tok == INTERLEAVE {
		x.Interleave = p.consume()
	}

	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}

	x.Params = p.parseFormalPars()
	if p.tok == RUNS {
		x.RunsOn = p.parseRunsOn()
	}

	// A behaviour type (ETSI ES 202 785) has the mtc clause a
	// behaviour of its kind may have (ES 201 873-1 16.1).
	if p.tok == MTC {
		x.Mtc = p.parseMtc()
	}

	if p.tok == SYSTEM {
		x.System = p.parseSystem()
	}

	if p.tok == RETURN {
		x.Return = p.parseReturn()
	}
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Subtype
 *************************************************************************/

func (p *parser) parseSubTypeDecl() *SubTypeDecl {
	if p.trace {
		defer un(trace(p, "SubTypeDecl"))
	}
	x := new(SubTypeDecl)
	x.TypeTok = p.consume()
	x.Field = p.parseField(false)
	x.With = p.parseWith()
	return x
}

func (p *parser) parseFieldList() []*Field {
	return parseList(p, "field", RBRACE, fieldStart, listRecovery,
		func() *Field { return p.parseField(true) },
		func(x *ErrorNode) *Field { return &Field{Type: x} })
}

func (p *parser) parseField(allowEmbedded bool) *Field {
	if p.trace {
		defer un(trace(p, "Field"))
	}
	x := new(Field)

	if p.tok == MODIF {
		if p.lit(1) != "@default" {
			p.errorExpected("@default")
		}
		x.DefaultTok = p.consume()
	}
	x.Type = p.parseTypeSpec()

	embedded := allowEmbedded && (p.tok == COMMA || p.tok == RBRACE || p.tok == OPTIONAL)
	if !embedded {
		x.Name = p.parseName()
		if p.tok == LT {
			x.TypePars = p.parseTypeFormalPars()
		}

		if p.tok == LBRACK {
			x.ArrayDef = p.parseArrayDefs()
		}

		if p.tok == LPAREN {
			x.ValueConstraint = p.parseParenExpr()
		}
		if p.tok == LENGTH {
			x.LengthConstraint = p.parseLength(nil)
		}
	}

	if p.tok == OPTIONAL {
		x.Optional = p.consume()
	}
	return x
}

func (p *parser) parseTypeSpec() TypeSpec {
	if p.trace {
		defer un(trace(p, "TypeSpec"))
	}
	etok := p.peek(1)
	switch p.tok {
	case ADDRESS, CHARSTRING, IDENT, NULL, UNIVERSAL:
		return &RefSpec{X: p.parseTypeRef()}
	case UNION:
		return p.parseStructSpec()
	case SET, RECORD:
		if p.peek(2).Kind() == LBRACE {
			return p.parseStructSpec()
		}
		return p.parseListSpec()
	case MAP:
		return p.parseMapSpec()
	case ENUMERATED:
		return p.parseEnumSpec()
	case FUNCTION, ALTSTEP, TESTCASE:
		return p.parseBehaviourSpec()
	default:
		p.errorExpected("type definition")
		return &ErrorNode{From: etok, To: p.peek(1)}
	}
}

func (p *parser) parseStructSpec() *StructSpec {
	if p.trace {
		defer un(trace(p, "StructSpec"))
	}
	x := new(StructSpec)
	x.KindTok = p.consume()
	x.LBrace = p.expect(LBRACE)
	x.Fields = p.parseFieldList()
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseMapSpec() *MapSpec {
	if p.trace {
		defer un(trace(p, "MapSpec"))
	}
	x := new(MapSpec)
	x.MapTok = p.consume()
	x.FromTok = p.expect(FROM)
	x.FromType = p.parseTypeSpec()
	x.ToTok = p.expect(TO)
	x.ToType = p.parseTypeSpec()
	return x
}

func (p *parser) parseEnumSpec() *EnumSpec {
	if p.trace {
		defer un(trace(p, "ListSpec"))
	}
	x := new(EnumSpec)
	x.Tok = p.consume()
	x.LBrace = p.expect(LBRACE)
	x.Enums = parseList(p, "enumerated value", RBRACE, enumStart, listRecovery, p.parseEnum, func(x *ErrorNode) Expr { return x })
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseListSpec() *ListSpec {
	if p.trace {
		defer un(trace(p, "ListSpec"))
	}
	x := new(ListSpec)
	x.KindTok = p.consume()
	if p.tok == LENGTH {
		x.Length = p.parseLength(nil)
	}
	x.OfTok = p.expect(OF)
	x.ElemType = p.parseTypeSpec()
	return x
}

func (p *parser) parseBehaviourSpec() *BehaviourSpec {
	if p.trace {
		defer un(trace(p, "BehaviourSpec"))
	}

	x := new(BehaviourSpec)
	x.KindTok = p.consume()

	if p.tok == INTERLEAVE {
		x.Interleave = p.consume()
	}

	x.Params = p.parseFormalPars()

	if p.tok == RUNS {
		x.RunsOn = p.parseRunsOn()
	}

	if p.tok == SYSTEM {
		x.System = p.parseSystem()
	}

	if p.tok == RETURN {
		x.Return = p.parseReturn()
	}
	return x
}

/*************************************************************************
 * Template Declaration
 *************************************************************************/

func (p *parser) parseTemplateDecl() *TemplateDecl {
	if p.trace {
		defer un(trace(p, "TemplateDecl"))
	}

	x := &TemplateDecl{RestrictionSpec: &RestrictionSpec{}}
	x.TemplateTok = p.consume()

	if p.tok == LPAREN {
		x.LParen = p.consume() // consume '('
		x.Tok = p.consume()    // consume omit/value/...
		x.RParen = p.expect(RPAREN)
	}

	x.Modif, x.Deterministic = p.parseModifiers()

	x.Type = p.parseTypeRef()
	if _, ok := x.Type.(*ErrorNode); ok {
		return x
	}
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}
	if p.tok == LPAREN {
		x.Params = p.parseFormalPars()
	}
	if p.tok == MODIFIES {
		x.ModifiesTok = p.consume()
		x.Base = p.parsePrimaryExpr()
	}
	x.AssignTok = p.expect(ASSIGN)
	x.Value = p.parseExpr()
	x.With = p.parseWith()

	return x
}

/*************************************************************************
 * Module FormalPar
 *************************************************************************/

func (p *parser) parseModulePar() Decl {
	if p.trace {
		defer un(trace(p, "ModulePar"))
	}

	tok := p.consume()

	// parse deprecated module parameter group
	if p.tok == LBRACE {
		x := &ModuleParameterGroup{Tok: tok}
		x.LBrace = p.consume()
		for p.tok != RBRACE && p.tok != EOF {
			d := new(ValueDecl)
			d.TemplateRestriction = p.parseRestrictionSpec()
			if p.peek(2).Kind() != ASSIGN {
				d.Type = p.parseTypeOrNestedTypeDef()
			}
			d.Decls = p.parseDeclList()
			p.expectSemi(d.Decls[len(d.Decls)-1].LastTok())
			x.Decls = append(x.Decls, d)
		}
		x.RBrace = p.expect(RBRACE)
		x.With = p.parseWith()
		return x
	}

	x := &ValueDecl{KindTok: tok}
	x.TemplateRestriction = p.parseRestrictionSpec()
	if p.peek(2).Kind() != ASSIGN {
		x.Type = p.parseTypeOrNestedTypeDef()
	}
	x.Decls = p.parseDeclList()
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Value Declaration
 *************************************************************************/

func (p *parser) parseValueDecl() *ValueDecl {
	if p.trace {
		defer un(trace(p, "ValueDecl"))
	}
	x := &ValueDecl{}
	if p.tok != TIMER {
		x.KindTok = p.consume()
		x.TemplateRestriction = p.parseRestrictionSpec()
		x.Modif, x.Deterministic = p.parseModifiers()
	}
	if tok := p.peek(2).Kind(); tok != ASSIGN && tok != IN {
		x.Type = p.parseTypeOrNestedTypeDef()
	}
	x.Decls = p.parseDeclList()
	x.With = p.parseWith()
	return x
}

func (p *parser) parseRestrictionSpec() *RestrictionSpec {
	switch p.tok {
	case TEMPLATE:
		x := new(RestrictionSpec)
		x.TemplateTok = p.consume()
		if p.tok == LPAREN {
			x.LParen = p.consume()
			x.Tok = p.consume()
			x.RParen = p.expect(RPAREN)
		}
		return x

	case OMIT, VALUE, PRESENT:
		x := new(RestrictionSpec)
		x.Tok = p.consume()
		return x
	default:
		return nil
	}
}

func (p *parser) parseDeclList() (list []*Declarator) {
	if p.trace {
		defer un(trace(p, "DeclList"))
	}

	list = append(list, p.parseDeclarator())
	for p.tok == COMMA {
		p.consume()
		switch p.tok {
		case RBRACE, SEMICOLON:
			return list
		}
		list = append(list, p.parseDeclarator())
	}
	return
}

func (p *parser) parseDeclarator() *Declarator {
	x := &Declarator{}
	x.Name = p.parseName()
	if p.tok == LBRACK {
		x.ArrayDef = p.parseArrayDefs()
	}
	if p.tok == ASSIGN {
		x.AssignTok = p.consume()
		x.Value = p.parseExpr()
	}
	return x
}

/*************************************************************************
 * Behaviour Declaration
 *************************************************************************/

func (p *parser) parseFuncDecl() *FuncDecl {
	if p.trace {
		defer un(trace(p, "FuncDecl"))
	}

	x := new(FuncDecl)
	x.KindTok = p.consume()

	if p.tok == INTERLEAVE {
		x.Interleave = p.consume()
	}
	if p.tok == MODIF {
		x.Modif = p.consume()
	}

	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}

	x.Params = p.parseFormalPars()

	if p.tok == RUNS {
		x.RunsOn = p.parseRunsOn()
	}

	if p.tok == MTC {
		x.Mtc = p.parseMtc()
	}

	if p.tok == SYSTEM {
		x.System = p.parseSystem()
	}

	if p.tok == RETURN {
		x.Return = p.parseReturn()
	}

	// Optional object-oriented exception clause (ETSI 5.2):
	// `function f() ... exception(T1, T2) { ... }`.
	if p.tok == EXCEPTION {
		x.ExceptionTok = p.consume()
		x.Exception = p.parseExceptionList()
	}

	if p.tok == LBRACE {
		x.Body = p.parseBlockStmt()
	}

	// Optional `catch (...) { ... }` handlers and a trailing
	// `finally { ... }` block after the body (ETSI 5.2).
	for p.tok == IDENT && p.lit(1) == "catch" {
		x.Catch = append(x.Catch, p.parseCatchClause())
	}
	if p.tok == IDENT && p.lit(1) == "finally" {
		x.FinallyTok = p.consume()
		x.Finally = p.parseBlockStmt()
	}

	x.With = p.parseWith()
	return x
}

// parseCatchClause parses a single object-oriented exception handler
// `catch (Type [name]) { ... }` attached after a function, altstep or
// testcase body (ETSI ES 201 873-1 clause 5.2).
func (p *parser) parseCatchClause() *CatchClause {
	if p.trace {
		defer un(trace(p, "CatchClause"))
	}
	x := new(CatchClause)
	x.CatchTok = p.consume() // "catch"
	x.LParen = p.expect(LPAREN)
	x.Type = p.parseTypeRef()
	if p.tok == IDENT {
		x.Var = p.parseIdent()
	}
	x.RParen = p.expect(RPAREN)
	x.Body = p.parseBlockStmt()
	return x
}

/*************************************************************************
 * Class Constructor Declaration
 *************************************************************************/

func (p *parser) parseConstructorDecl() *ConstructorDecl {
	if p.trace {
		defer un(trace(p, "ConstructorDecl"))
	}

	x := new(ConstructorDecl)
	x.Name = p.parseIdent()
	x.Params = p.parseFormalPars()
	if p.tok == COLON {
		x.ColonTok = p.consume()
		x.Init = p.parsePrimaryExpr()
	}
	x.Body = p.parseBlockStmt()

	return x
}

/*************************************************************************
 * External Function Declaration
 *************************************************************************/

func (p *parser) parseExtFuncDecl() *FuncDecl {
	if p.trace {
		defer un(trace(p, "ExtFuncDecl"))
	}

	x := new(FuncDecl)
	x.External = p.consume()
	x.KindTok = p.consume()
	if p.tok == MODIF {
		x.Modif = p.consume()
	}

	x.Name = p.parseName()

	x.Params = p.parseFormalPars()

	if p.tok == RUNS {
		x.RunsOn = p.parseRunsOn()
	}

	if p.tok == MTC {
		x.Mtc = p.parseMtc()
	}

	if p.tok == SYSTEM {
		x.System = p.parseSystem()
	}

	if p.tok == RETURN {
		x.Return = p.parseReturn()
	}
	// Optional object-oriented exception clause on external functions
	// (ETSI 5.2.2): `external function f() exception(T);`.
	if p.tok == EXCEPTION {
		x.ExceptionTok = p.consume()
		x.Exception = p.parseExceptionList()
	}
	x.With = p.parseWith()
	return x
}

/*************************************************************************
 * Signature Declaration
 *************************************************************************/

func (p *parser) parseSignatureDecl() *SignatureDecl {
	if p.trace {
		defer un(trace(p, "SignatureDecl"))
	}

	x := new(SignatureDecl)
	x.Tok = p.consume()
	x.Name = p.parseName()
	if p.tok == LT {
		x.TypePars = p.parseTypeFormalPars()
	}

	x.Params = p.parseFormalPars()

	if p.tok == NOBLOCK {
		x.NoBlock = p.consume()
	}

	if p.tok == RETURN {
		x.Return = p.parseReturn()
	}

	if p.tok == EXCEPTION {
		x.ExceptionTok = p.consume()
		x.Exception = &ParenExpr{LParen: p.expect(LPAREN)}
		for p.tok != RPAREN && p.tok != EOF {
			x.Exception.List = append(x.Exception.List, p.parseTypeOrNestedTypeDef())
			if p.tok != COMMA {
				break
			}
			p.consume()
		}
		x.Exception.RParen = p.expect(RPAREN)
	}
	x.With = p.parseWith()
	return x
}

func (p *parser) parseRunsOn() *RunsOnSpec {
	return &RunsOnSpec{
		RunsTok: p.expect(RUNS),
		OnTok:   p.expect(ON),
		Comp:    p.parseTypeRef(),
	}
}

func (p *parser) parseSystem() *SystemSpec {
	return &SystemSpec{
		Tok:  p.expect(SYSTEM),
		Comp: p.parseTypeRef(),
	}
}

func (p *parser) parseMtc() *MtcSpec {
	return &MtcSpec{
		Tok:  p.expect(MTC),
		Comp: p.parseTypeRef(),
	}
}

func (p *parser) parseReturn() *ReturnSpec {
	x := new(ReturnSpec)
	x.Tok = p.consume()
	x.Restriction = p.parseRestrictionSpec()
	if p.tok == MODIF {
		x.Modif = p.consume()
	}
	x.Type = p.parseTypeOrNestedTypeDef()
	if p.tok == LBRACK {
		x.ArrayDef = p.parseArrayDefs()
	}
	return x
}

func (p *parser) parseFormalPars() *FormalPars {
	if p.trace {
		defer un(trace(p, "FormalPars"))
	}
	x := new(FormalPars)
	x.LParen = p.expect(LPAREN)
	if x.LParen == nil {
		return x
	}
	x.List = parseList(p, "formal parameter", RPAREN, formalParStart, listRecovery, p.parseFormalPar,
		func(x *ErrorNode) *FormalPar { return &FormalPar{Type: x} })
	x.RParen = p.expect(RPAREN)
	return x
}

func (p *parser) parseFormalPar() *FormalPar {
	if p.trace {
		defer un(trace(p, "FormalPar"))
	}
	x := new(FormalPar)

	switch p.tok {
	case IN:
		x.Direction = p.consume()
	case OUT:
		x.Direction = p.consume()
	case INOUT:
		x.Direction = p.consume()
	}

	x.TemplateRestriction = p.parseRestrictionSpec()
	x.Modif, x.Deterministic = p.parseModifiers()
	x.Type = p.parseTypeOrNestedTypeDef()
	x.Name = p.parseName()

	if p.tok == LBRACK {
		x.ArrayDef = p.parseArrayDefs()
	}
	if p.tok == ELIPSIS {
		x.Variadic = p.consume()
	}
	if p.tok == ASSIGN {
		x.AssignTok = p.consume()
		x.Value = p.parseExpr()
	}
	return x
}

func (p *parser) parseTypeFormalPars() *FormalPars {
	if p.trace {
		defer un(trace(p, "TypeFormalPars"))
	}
	x := new(FormalPars)
	x.LParen = p.expect(LT)
	x.List = parseList(p, "formal type parameter", GT, typeFormalParStart, typeListRecovery, p.parseTypeFormalPar,
		func(x *ErrorNode) *FormalPar { return &FormalPar{Type: x} })
	x.RParen = p.expect(GT)
	return x
}

func (p *parser) parseTypeFormalPar() *FormalPar {
	if p.trace {
		defer un(trace(p, "TypeFormalPar"))
	}

	x := new(FormalPar)

	if p.tok == IN {
		x.Direction = p.consume()
	}

	switch p.tok {
	case TYPE:
		x.Type = p.make_use(p.consume())
	case SIGNATURE:
		x.Type = p.make_use(p.consume())
	default:
		x.Type = p.parseTypeRef()
	}
	x.Name = p.make_use(p.expect(IDENT))
	x.Name.IsName = true
	if p.tok == ASSIGN {
		x.AssignTok = p.consume()
		x.Value = p.parseTypeRef()
	}

	return x
}

/*************************************************************************
 * Statements
 *************************************************************************/

func (p *parser) parseBlockStmt() *BlockStmt {
	if p.trace {
		defer un(trace(p, "BlockStmt"))
	}

	// A nested block, such as a guard body or '@dynamic' body, is not
	// part of the guard operation being parsed.
	savedGuard := p.altGuard
	p.altGuard = false
	defer func() { p.altGuard = savedGuard }()

	x := &BlockStmt{LBrace: p.expect(LBRACE)}
	for p.tok != RBRACE && p.tok != EOF {
		x.Stmts = append(x.Stmts, p.parseStmt())
		p.expectSemi(x.Stmts[len(x.Stmts)-1].LastTok())
	}
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseStmt() Stmt {
	if p.trace {
		defer un(trace(p, "Stmt"))
	}

	// Object-oriented `raise <expr>` statement (ETSI 5.2.3). `raise`
	// is not a reserved word, so match it contextually: a bare `raise`
	// at statement start directly followed by an expression token is
	// the throw statement. Proc-based `pt.raise(...)` starts with a
	// port name, and `raise(...)`/`raise := ...`/`raise;` fall through
	// to ordinary identifier parsing.
	if p.tok == IDENT && p.lit(1) == "raise" {
		switch p.peek(2).Kind() {
		case INT, FLOAT, STRING, BSTRING, TRUE, FALSE, IDENT, SUB, ADD, NOT, NOT4B:
			x := &RaiseStmt{Tok: p.consume()}
			x.X = p.parseExpr()
			return x
		}
	}

	etok := p.peek(1)
	switch p.tok {
	case TEMPLATE:
		return &DeclStmt{Decl: p.parseTemplateDecl()}
	case VAR, CONST, TIMER, PORT:
		return &DeclStmt{Decl: p.parseValueDecl()}
	case REPEAT, BREAK, CONTINUE:
		return &BranchStmt{Tok: p.consume()}
	case LABEL:
		return &BranchStmt{Tok: p.consume(), Label: p.make_use(p.expect(IDENT))}
	case GOTO:
		return &BranchStmt{Tok: p.consume(), Label: p.make_use(p.expect(IDENT))}
	case RETURN:
		x := &ReturnStmt{Tok: p.consume()}
		if !stmtStart[p.tok] && p.tok != SEMICOLON && p.tok != RBRACE {
			x.Result = p.parseExpr()
		}
		return x
	case SELECT:
		return p.parseSelect()
	case ALT, INTERLEAVE:
		alt := &AltStmt{Tok: p.consume()}
		if p.tok == MODIF {
			alt.NoDefault = p.consume()
		}
		alt.Body = p.parseBlockStmt()
		return alt
	case LBRACK:
		return p.parseAltGuard()
	case FOR:
		return p.parseForLoop()
	case WHILE:
		return p.parseWhileLoop()
	case DO:
		return p.parseDoWhileLoop()
	case IF:
		return p.parseIfStmt()
	case LBRACE:
		return p.parseBlockStmt()
	case IDENT, TESTCASE, ANYKW, ALL, MAP, UNMAP, MTC, INC, DEC:
		x := p.parseSimpleStmt()

		// A call with a body: "p.call(...) to c -> timestamp t { ... }".
		// "to" and "->" wrap the call, so it is not a bare CallExpr.
		if p.tok == LBRACE && isNamedCall(x.Expr, "call") {
			call := new(CallStmt)
			call.Stmt = x
			call.Body = p.parseBlockStmt()
			return call
		}
		return x
	// Interpret simple literal expressions like integers or strings as statement.
	// This exception was added to help implementing ast-evaluator code like this:
	//
	//       if (1 > 2) { 10 } else { 20 }
	//
	case INT, FLOAT, STRING, BSTRING, TRUE, FALSE, PASS, FAIL, NONE, INCONC, ERROR:
		return p.parseSimpleStmt()

	case NOTIMPL:
		return p.parseSimpleStmt()

	case MODIF:
		switch v := p.lit(1); v {
		case "@nodefault":
			tok := p.consume()
			x := p.parseSimpleStmt()
			x.NoDefault = tok
			return x

		// This enables parsing of Titan specific try/catch blocks.
		// Note, we only parse the structure for prevent syntax-errors,
		// Titan exceptions are not implemented in the rest of ntt.
		case "@try":
			p.consume()
			return p.parseBlockStmt()
		case "@catch":
			p.consume()
			if p.tok == LPAREN {
				p.consume()
				p.parseIdent()
				p.expect(RPAREN)
			}
			return p.parseBlockStmt()
		case "@lazy", "@fuzzy", "@deterministic":
			// TTCN-3 ed. 4.13+ permits a small set of modifiers
			// at statement position - they alter the runtime
			// scheduler's behaviour but don't change the
			// syntactic shape of the following statement.
			// Consume the modifier and parse the next statement
			// as-is; the modifier is currently discarded since
			// the interpreter doesn't model the distinction.
			p.consume()
			return p.parseStmt()
		}
		fallthrough
	default:
		switch p.tok {
		case TYPE, FUNCTION, ALTSTEP, SIGNATURE, IMPORT, MODULEPAR, GROUP, FRIEND, EXTERNAL, CONTROL, PRIVATE, PUBLIC:
			p.errorExpected("statement")
			p.parseModuleDef()
		case SEMICOLON, RBRACE, EOF:
			p.errorExpected("statement")
		default:
			return p.advanceWithError(skipStmt, "expected statement, found %s", etok)
		}
		return &ErrorNode{From: etok, To: p.peek(1)}
	}
}

// isNamedCall reports whether e is a call of name, allowing "to", "from"
// and "->" to wrap the call.
func isNamedCall(e Expr, name string) bool {
	switch e := e.(type) {
	case *RedirectExpr:
		return isNamedCall(e.X, name)
	case *BinaryExpr:
		if e.Op.Kind() == TO || e.Op.Kind() == FROM {
			return isNamedCall(e.X, name)
		}
	case *CallExpr:
		s, ok := e.Fun.(*SelectorExpr)
		if !ok {
			return false
		}
		id, ok := s.Sel.(*Ident)
		return ok && id.Tok.String() == name
	}
	return false
}

func (p *parser) parseForLoop() Stmt {
	forTok := p.consume()
	lParen := p.expect(LPAREN)

	var init Stmt
	if p.tok == VAR {
		init = &DeclStmt{Decl: p.parseValueDecl()}
	} else {
		init = &ExprStmt{Expr: p.parseExpr()}
	}

	if p.tok == IN {
		x := new(ForRangeStmt)
		x.Tok = forTok
		x.LParen = lParen
		x.Init = init
		x.InTok = p.consume()
		x.Range = p.parseExpr()
		x.RParen = p.expect(RPAREN)
		x.Body = p.parseBlockStmt()
		return x
	} else {
		x := new(ForStmt)
		x.Tok = forTok
		x.LParen = lParen
		x.Init = init
		x.InitSemi = p.expect(SEMICOLON)
		x.Cond = p.parseExpr()
		x.CondSemi = p.expect(SEMICOLON)
		x.Post = p.parseSimpleStmt()
		x.RParen = p.expect(RPAREN)
		x.Body = p.parseBlockStmt()
		return x
	}
}

// CondWithInit ::= [ ( VarInstance | Assignment ) ";" ] Expr
func (p *parser) parseCondWithInit() (init Stmt, semi Token, cond Expr) {
	if p.tok == VAR {
		init = &DeclStmt{Decl: p.parseValueDecl()}
		return init, p.expect(SEMICOLON), p.parseExpr()
	}
	x := p.parseExpr()
	if p.tok != SEMICOLON {
		return nil, nil, x
	}
	return &ExprStmt{Expr: x}, p.consume(), p.parseExpr()
}

func (p *parser) parseWhileLoop() *WhileStmt {
	x := &WhileStmt{
		Tok:    p.consume(),
		LParen: p.expect(LPAREN),
	}
	x.Init, x.InitSemi, x.Cond = p.parseCondWithInit()
	x.RParen = p.expect(RPAREN)
	x.Body = p.parseBlockStmt()
	return x
}

func (p *parser) parseDoWhileLoop() *DoWhileStmt {
	return &DoWhileStmt{
		DoTok:    p.consume(),
		Body:     p.parseBlockStmt(),
		WhileTok: p.expect(WHILE),
		LParen:   p.expect(LPAREN),
		Cond:     p.parseExpr(),
		RParen:   p.expect(RPAREN),
	}
}

func (p *parser) parseIfStmt() *IfStmt {
	x := &IfStmt{
		Tok:    p.consume(),
		LParen: p.expect(LPAREN),
	}
	x.Init, x.InitSemi, x.Cond = p.parseCondWithInit()
	x.RParen = p.expect(RPAREN)
	x.Then = p.parseBlockStmt()
	if p.tok == ELSE {
		x.ElseTok = p.consume()
		if p.tok == IF {
			x.Else = p.parseIfStmt()
		} else {
			x.Else = p.parseBlockStmt()
		}
	}
	return x
}

func (p *parser) parseSelect() *SelectStmt {
	x := new(SelectStmt)
	x.Tok = p.expect(SELECT)
	if p.tok == UNION {
		x.Union = p.consume()
	} else if p.tok == CLASS {
		x.Class = p.consume()
	}
	x.LParen = p.expect(LPAREN)
	x.Init, x.InitSemi, x.Tag = p.parseCondWithInit()
	x.RParen = p.expect(RPAREN)
	x.LBrace = p.expect(LBRACE)
	for p.tok == CASE {
		x.Body = append(x.Body, p.parseCaseStmt())
	}
	x.RBrace = p.expect(RBRACE)
	return x
}

func (p *parser) parseCaseStmt() *CaseClause {
	x := new(CaseClause)
	x.Tok = p.expect(CASE)
	if p.tok == ELSE {
		p.consume() // TODO(5nord) move token into AST
	} else {
		x.Case = p.parseParenExpr()
	}
	x.Body = p.parseBlockStmt()
	return x
}

func (p *parser) parseAltGuard() *CommClause {
	x := new(CommClause)
	x.LBrack = p.expect(LBRACK)
	if p.tok == ELSE {
		x.Else = p.consume()
		x.RBrack = p.expect(RBRACK)
		x.Body = p.parseBlockStmt()
		return x
	}

	if p.tok != RBRACK {
		x.X = p.parseExpr()
	}
	x.RBrack = p.expect(RBRACK)
	saved := p.altGuard
	p.altGuard = true
	x.Comm = p.parseSimpleStmt()
	p.altGuard = saved
	if p.tok == LBRACE {
		x.Body = p.parseBlockStmt()
	}
	return x
}

func (p *parser) parseSimpleStmt() *ExprStmt {
	if p.trace {
		defer un(trace(p, "SimpleStmt"))
	}

	return &ExprStmt{Expr: p.parseExpr()}
}
