package syntax

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	trc "runtime/trace"
	"strconv"
	"strings"

	"github.com/nokia/ntt/internal/fs"
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
	for p.tok != EOF {
		p.Nodes = append(p.Nodes, p.parse())

		if p.tok != EOF && !topLevelTokens[p.tok] {
			p.error(p.peek(1), "unexpected token %s", p.tok)
			break
		}

		if p.tok == COMMA || p.tok == SEMICOLON {
			p.consume()
		}

	}
	return p.Root
}

// If src != nil, readSource converts src to a []byte if possible;
// otherwise it returns an error. If src == nil, readSource returns
// the result of reading the file specified by filename.
func readSource(filename string, src interface{}) ([]byte, error) {
	if src != nil {
		switch s := src.(type) {
		case string:
			return []byte(s), nil
		case []byte:
			return s, nil
		case *bytes.Buffer:
			// is io.Reader, but src is already available in []byte form
			if s != nil {
				return s.Bytes(), nil
			}
		case io.Reader:
			return ioutil.ReadAll(s)
		}
		return nil, errors.New("invalid source")
	}
	return fs.Content(filename)
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
	ppLvl  int
	ppCnt  int
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

func (p *parser) handlePreproc(s string) {
	f := strings.Fields(s)
	switch b := false; f[0] {
	case "#ifndef":
		b = true
		fallthrough
	case "#ifdef", "#if":
		if len(f) < 2 {
			p.error(p.peek(1), "missing condition in preprocessor directive")
			break
		}
		if p.ppSkip == false && p.ppLvl == p.ppCnt {
			p.ppLvl++
			p.ppSkip = (p.ppDefs[f[1]] == b)
		}
		p.ppCnt++
	case "#else":
		if p.ppLvl == p.ppCnt {
			p.ppSkip = !p.ppSkip
		}
	case "#endif":
		if p.ppLvl == p.ppCnt {
			p.ppLvl--
			p.ppSkip = false
		}
		p.ppCnt--

	case "#define":
		switch len(f) {
		case 2:
			p.ppDefs[f[1]] = true
		case 3:
			if v, err := strconv.ParseBool(f[2]); err != nil {
				p.ppDefs[f[1]] = v
				break
			}
			p.error(p.peek(1), "not a boolean expression")
		default:
			p.error(p.peek(1), "malformed 'define' directive")
		}
	default:
		if !strings.HasPrefix(s, "#!") {
			p.error(p.peek(1), "unknown preprocessor directive")
		}
	}
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

// A bailout panic is raised to indicate early termination.
type bailout struct{}

func (p *parser) error(n Node, msg string, args ...interface{}) {
	err := Error{Node: n, Msg: fmt.Sprintf(msg, args...)}
	p.errs = append(p.errs, err)
}

func (p *parser) errorExpected(what string) {
	tok := p.peek(1)
	p.error(tok, "expected %s, found %s", what, tok.String())
}

func (p *parser) expect(k Kind) Token {
	if p.tok != k {
		p.errorExpected("'" + k.String() + "'")
	}
	return p.consume() // make progress
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
		if p.peek(1).Kind() == DOT {
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

// Expr ::= BinaryExpr
func (p *parser) parseExpr() Expr {
	if p.trace {
		defer un(trace(p, "Expr"))
	}

	return p.parseBinaryExpr(LowestPrec + 1)
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

	if p.tok == LENGTH && !p.noMatchAttr {
		x = p.parseLength(x)
	}

	if p.tok == IFPRESENT && !p.noMatchAttr {
		x = &UnaryExpr{Op: p.consume(), X: x}
	}

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
			// Not supporting chained function calls like 'get().x'
			// eleminates conflicts with alt-guards.
			break
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
		// can be template `x := (1,2,3)`, but also artihmetic expression: `1*(2+3)`
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

	default:
		p.errorExpected("operand")
	}

	return &ErrorNode{From: etok, To: p.peek(1)}
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

func (p *parser) parseParenExpr() *ParenExpr {
	return &ParenExpr{
		LParen: p.expect(LPAREN),
		List:   p.parseExprList(),
		RParen: p.expect(RPAREN),
	}
}

func (p *parser) parseUniversalCharstring() *Ident {
	return p.make_use(p.expect(UNIVERSAL), p.expect(CHARSTRING))
}

func (p *parser) parseCompositeLiteral() *CompositeLiteral {
	c := new(CompositeLiteral)
	c.LBrace = p.expect(LBRACE)
	if p.tok != RBRACE {
		c.List = p.parseExprList()
	}
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
	switch tok {
	case ADD, EXCL, NOT, NOT4B, SUB, INC, DEC, COLONCOLON,
		ANYKW, ALL, UNIVERSAL,
		ADDRESS, CHARSTRING, CLASS, MAP, MTC, SYSTEM, TESTCASE, TIMER, UNMAP,
		IDENT,
		INT, ANY, NOTIMPL, BSTRING, ERROR, NULL, OMIT, FAIL, FALSE, FLOAT,
		INCONC, MUL, NAN, NONE, PASS, STRING, TRUE,
		LPAREN, LBRACK, LBRACE,
		MODIFIES, REGEXP, PATTERN, DECMATCH, MODIF, VALUE, PRESENT:
		return true
	default:
		return false
	}
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
			c.Args.List = append(c.Args.List, p.parseExprList()...)
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
	p.expect(IDENT)
	return nil

}

func (p *parser) parseIdent() *Ident {
	switch p.tok {
	case UNIVERSAL:
		return p.parseUniversalCharstring()
	case IDENT, ADDRESS, ALIVE, CHARSTRING, CONTROL, TO, FROM, CREATE, VALUE, CLASS:
		return p.make_use(p.consume())
	default:
		p.expect(IDENT) // use expect() error handling
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
		List:   p.parseExprList(),
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
func (p *parser) parseTypeOrNestedTypeDef() Expr {
	switch p.tok {
	case RECORD, SET, UNION, ENUMERATED:
		return p.parseTypeSpec().(Expr)
	case MAP:
		if p.peek(2).Kind() == FROM {
			return p.parseMapSpec()
		}
	}
	return p.parseTypeRef()
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

func (p *parser) parseModuleList() []*Module {
	var list []*Module
	if m := p.parseModule(); m != nil {
		list = append(list, m)
		p.expectSemi(m.LastTok())
	}
	for p.tok == MODULE {
		if m := p.parseModule(); m != nil {
			list = append(list, m)
			p.expectSemi(m.LastTok())
		}
	}
	p.expect(EOF)
	return list
}

func (p *parser) parseModule() *Module {
	if p.trace {
		defer un(trace(p, "Module"))
	}

	m := new(Module)
	m.Tok = p.expect(MODULE)
	m.Name = p.parseName()

	if p.tok == LANGUAGE {
		m.Language = p.parseLanguageSpec()
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
	default:
		p.errorExpected("module definition")
		p.advance(stmtStart)
		m.Def = &ErrorNode{From: etok, To: p.peek(1)}
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
	default:
		p.errorExpected("import definition qualifier")
		p.advance(stmtStart)
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
	default:
		p.errorExpected("definition qualifier")
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
	x.Modules = append(x.Modules, p.parseIdent())
	for p.tok == COMMA {
		p.consume()
		x.Modules = append(x.Modules, p.parseIdent())
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
		OVERRIDE:
		x.KindTok = p.consume()
	default:
		p.errorExpected("with-attribute")
		p.advance(stmtStart)
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
		for {
			x.List = append(x.List, p.parseWithQualifier())
			if p.tok != COMMA {
				break
			}
			p.consume()
			if p.tok == RPAREN {
				break
			}
		}
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
	etok := p.peek(1)
	switch p.peek(2).Kind() {
	case IDENT, ADDRESS, CHARSTRING, NULL, UNIVERSAL:
		return p.parseSubTypeDecl()
	case PORT:
		return p.parsePortTypeDecl()
	case COMPONENT:
		return p.parseComponentTypeDecl()
	case CLASS:
		return p.parseClassTypeDecl()
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
		p.errorExpected("type definition")
		p.advance(stmtStart)
		return &ErrorNode{From: etok, To: p.peek(1)}
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
	default:
		p.errorExpected("port attribute")
		p.advance(stmtStart)
		return &ErrorNode{From: etok, To: p.peek(1)}
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
	for p.tok != RBRACE && p.tok != EOF {
		x.Fields = append(x.Fields, p.parseField(true))
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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
		x.Defs = append(x.Defs, p.parseModuleDef())
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
	for p.tok != RBRACE && p.tok != EOF {
		x.Enums = append(x.Enums, p.parseEnum())
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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
	for p.tok != RBRACE && p.tok != EOF {
		x.Fields = append(x.Fields, p.parseField(true))
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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
	for p.tok != RBRACE && p.tok != EOF {
		x.Enums = append(x.Enums, p.parseEnum())
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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

	if p.tok == LBRACE {
		x.Body = p.parseBlockStmt()
	}

	x.With = p.parseWith()
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
	for p.tok != RPAREN {
		x.List = append(x.List, p.parseFormalPar())
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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
	for p.tok != GT {
		x.List = append(x.List, p.parseTypeFormalPar())
		if p.tok != COMMA {
			break
		}
		p.consume()
	}
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
		}
		fallthrough
	default:
		p.errorExpected("statement")
		p.advance(stmtStart)
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

func hasAssignment(n Node) bool {
	switch n := n.(type) {
	case *DeclStmt:
		return hasAssignment(n.Decl)
	case *ExprStmt:
		return hasAssignment(n.Expr)
	case *ValueDecl:
		for _, d := range n.Decls {
			if d.AssignTok != nil {
				return true
			}
		}
		return false
	case *BinaryExpr:
		if n.Op.Kind() == ASSIGN {
			return true
		}
		return false
	default:
		return false
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
