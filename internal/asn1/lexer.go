package asn1

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenKind classifies a lexical token from an ASN.1 source file.
//
// We intentionally use one constant per punctuation symbol but collapse
// every X.680 reserved word into KEYWORD. The parser can switch on a
// token's text to recognise specific keywords; this keeps the kind
// space manageable (there are over 90 reserved words in X.680).
type TokenKind uint8

const (
	INVALID TokenKind = iota
	EOF

	// Literals & references.
	NUMBER        // integer or real literal (parser distinguishes)
	BSTRING       // '...'B - binary string literal
	HSTRING       // '...'H - hexadecimal string literal
	CSTRING       // "..."  - character string literal
	TYPEREFERENCE // identifier starting with an uppercase letter
	IDENTIFIER    // identifier starting with a lowercase letter
	AMP_REF       // &Foo / &foo - object class field reference
	WORD          // 'WITH SYNTAX' literal word (multi-word identifiers inside WITH SYNTAX templates)

	// Reserved word - look at Token.Text for the specific keyword.
	KEYWORD

	// Punctuation.
	LBRACE          // {
	RBRACE          // }
	LBRACKET        // [
	RBRACKET        // ]
	DOUBLE_LBRACKET // [[
	DOUBLE_RBRACKET // ]]
	LPAREN          // (
	RPAREN          // )
	COMMA           // ,
	SEMICOLON       // ;
	COLON           // :
	DOT             // .
	DOUBLE_DOT      // ..
	ELLIPSIS        // ...
	HYPHEN          // -
	PLUS            // +
	LESS            // <
	GREATER         // >
	EQUAL           // =
	BANG            // !
	AT              // @
	PIPE            // |
	SLASH           // /
	STAR            // *
	CARET           // ^
	ASSIGN          // ::=
)

func (k TokenKind) String() string {
	switch k {
	case INVALID:
		return "INVALID"
	case EOF:
		return "EOF"
	case NUMBER:
		return "NUMBER"
	case BSTRING:
		return "BSTRING"
	case HSTRING:
		return "HSTRING"
	case CSTRING:
		return "CSTRING"
	case TYPEREFERENCE:
		return "TYPEREFERENCE"
	case IDENTIFIER:
		return "IDENTIFIER"
	case AMP_REF:
		return "AMP_REF"
	case WORD:
		return "WORD"
	case KEYWORD:
		return "KEYWORD"
	case LBRACE:
		return "LBRACE"
	case RBRACE:
		return "RBRACE"
	case LBRACKET:
		return "LBRACKET"
	case RBRACKET:
		return "RBRACKET"
	case DOUBLE_LBRACKET:
		return "DOUBLE_LBRACKET"
	case DOUBLE_RBRACKET:
		return "DOUBLE_RBRACKET"
	case LPAREN:
		return "LPAREN"
	case RPAREN:
		return "RPAREN"
	case COMMA:
		return "COMMA"
	case SEMICOLON:
		return "SEMICOLON"
	case COLON:
		return "COLON"
	case DOT:
		return "DOT"
	case DOUBLE_DOT:
		return "DOUBLE_DOT"
	case ELLIPSIS:
		return "ELLIPSIS"
	case HYPHEN:
		return "HYPHEN"
	case PLUS:
		return "PLUS"
	case LESS:
		return "LESS"
	case GREATER:
		return "GREATER"
	case EQUAL:
		return "EQUAL"
	case BANG:
		return "BANG"
	case AT:
		return "AT"
	case PIPE:
		return "PIPE"
	case SLASH:
		return "SLASH"
	case STAR:
		return "STAR"
	case CARET:
		return "CARET"
	case ASSIGN:
		return "ASSIGN"
	}
	return fmt.Sprintf("TokenKind(%d)", k)
}

// Token is the unit produced by the lexer. Pos and End are byte
// offsets into the source slice; End is exclusive.
type Token struct {
	Kind TokenKind
	Pos  int
	End  int
	Text string
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q)@%d:%d", t.Kind, t.Text, t.Pos, t.End)
}

// Lexer scans an ASN.1 source buffer into a stream of Tokens.
//
// The lexer is independent of the parser; it can be driven by any
// front-end that wants byte-precise token positions (LSP semantic
// highlighting, code formatters, diagnostics range computation).
type Lexer struct {
	src      []byte
	pos      int
	errors   []Diagnostic
	withSynt bool // true while inside a WITH SYNTAX template - upper-case "words" become WORD tokens
}

// NewLexer constructs a Lexer over src.
func NewLexer(src []byte) *Lexer {
	return &Lexer{src: src}
}

// Errors returns any lexical diagnostics accumulated so far.
func (l *Lexer) Errors() []Diagnostic { return l.errors }

// EnterWithSyntax / LeaveWithSyntax control X.681 WITH SYNTAX scanning.
// Inside a WITH SYNTAX template all-uppercase words separated by white
// space are tokenised as WORD instead of TYPEREFERENCE / KEYWORD.
func (l *Lexer) EnterWithSyntax() { l.withSynt = true }
func (l *Lexer) LeaveWithSyntax() { l.withSynt = false }

// All scans src to EOF and returns the resulting token slice plus any
// lexical diagnostics. Useful for tests; production code should drive
// Next() in a loop to avoid the intermediate allocation.
func (l *Lexer) All() ([]Token, []Diagnostic) {
	var toks []Token
	for {
		t := l.Next()
		toks = append(toks, t)
		if t.Kind == EOF {
			break
		}
	}
	return toks, l.errors
}

// Next returns the next token. After EOF, Next continues to return EOF
// indefinitely so callers can safely lookahead past the end of input.
func (l *Lexer) Next() Token {
	l.skipTrivia()
	if l.pos >= len(l.src) {
		return Token{Kind: EOF, Pos: l.pos, End: l.pos}
	}
	start := l.pos
	c := l.src[l.pos]

	switch {
	case c == '{':
		l.pos++
		return mk(LBRACE, start, l.pos, l.src)
	case c == '}':
		l.pos++
		return mk(RBRACE, start, l.pos, l.src)
	case c == '(':
		l.pos++
		return mk(LPAREN, start, l.pos, l.src)
	case c == ')':
		l.pos++
		return mk(RPAREN, start, l.pos, l.src)
	case c == ',':
		l.pos++
		return mk(COMMA, start, l.pos, l.src)
	case c == ';':
		l.pos++
		return mk(SEMICOLON, start, l.pos, l.src)
	case c == '|':
		l.pos++
		return mk(PIPE, start, l.pos, l.src)
	case c == '^':
		l.pos++
		return mk(CARET, start, l.pos, l.src)
	case c == '!':
		l.pos++
		return mk(BANG, start, l.pos, l.src)
	case c == '@':
		l.pos++
		return mk(AT, start, l.pos, l.src)
	case c == '+':
		l.pos++
		return mk(PLUS, start, l.pos, l.src)
	case c == '<':
		l.pos++
		return mk(LESS, start, l.pos, l.src)
	case c == '>':
		l.pos++
		return mk(GREATER, start, l.pos, l.src)
	case c == '=':
		l.pos++
		return mk(EQUAL, start, l.pos, l.src)
	case c == '/':
		l.pos++
		return mk(SLASH, start, l.pos, l.src)
	case c == '*':
		l.pos++
		return mk(STAR, start, l.pos, l.src)

	case c == '[':
		l.pos++
		if l.pos < len(l.src) && l.src[l.pos] == '[' {
			l.pos++
			return mk(DOUBLE_LBRACKET, start, l.pos, l.src)
		}
		return mk(LBRACKET, start, l.pos, l.src)

	case c == ']':
		l.pos++
		if l.pos < len(l.src) && l.src[l.pos] == ']' {
			l.pos++
			return mk(DOUBLE_RBRACKET, start, l.pos, l.src)
		}
		return mk(RBRACKET, start, l.pos, l.src)

	case c == ':':
		// ::= or bare :
		if l.pos+2 < len(l.src) && l.src[l.pos+1] == ':' && l.src[l.pos+2] == '=' {
			l.pos += 3
			return mk(ASSIGN, start, l.pos, l.src)
		}
		l.pos++
		return mk(COLON, start, l.pos, l.src)

	case c == '.':
		// ... or .. or .
		if l.pos+2 < len(l.src) && l.src[l.pos+1] == '.' && l.src[l.pos+2] == '.' {
			l.pos += 3
			return mk(ELLIPSIS, start, l.pos, l.src)
		}
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '.' {
			l.pos += 2
			return mk(DOUBLE_DOT, start, l.pos, l.src)
		}
		l.pos++
		return mk(DOT, start, l.pos, l.src)

	case c == '-':
		// Bare hyphen - the "--" comment form was already consumed
		// by skipTrivia, so any '-' at this point is the unary
		// minus / range separator.
		l.pos++
		return mk(HYPHEN, start, l.pos, l.src)

	case c == '&':
		return l.readAmpRef(start)

	case c == '\'':
		return l.readBhString(start)

	case c == '"':
		return l.readCString(start)

	case isDigit(c):
		return l.readNumber(start)

	case isLetter(c):
		return l.readWordOrIdent(start)
	}

	// Unknown byte - emit INVALID and advance one byte to make
	// progress. The parser will see this and synchronise.
	l.errorf(start, "unexpected character %q", c)
	l.pos++
	return mk(INVALID, start, l.pos, l.src)
}

// skipTrivia consumes whitespace and ASN.1 comments.
//
// Per X.680 §12.6 ASN.1 supports two comment forms:
//   - Pair-style "/* ... */" with nesting.
//   - Line-style "-- ... --" terminated by either another "--" or by
//     a newline (whichever comes first).
func (l *Lexer) skipTrivia() {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			l.pos++
		case c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-':
			l.skipLineComment()
		case c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			l.skipBlockComment()
		default:
			return
		}
	}
}

func (l *Lexer) skipLineComment() {
	l.pos += 2 // consume the opening "--"
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\n' {
			return
		}
		if c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-' {
			l.pos += 2
			return
		}
		l.pos++
	}
}

func (l *Lexer) skipBlockComment() {
	start := l.pos
	l.pos += 2 // "/*"
	depth := 1
	for l.pos+1 < len(l.src) && depth > 0 {
		switch {
		case l.src[l.pos] == '/' && l.src[l.pos+1] == '*':
			depth++
			l.pos += 2
		case l.src[l.pos] == '*' && l.src[l.pos+1] == '/':
			depth--
			l.pos += 2
		default:
			l.pos++
		}
	}
	if depth != 0 {
		l.errorf(start, "unterminated block comment")
	}
}

// readAmpRef scans a class field reference such as `&Foo` (typefield)
// or `&foo` (valuefield). The leading '&' is part of the token text.
func (l *Lexer) readAmpRef(start int) Token {
	l.pos++ // consume '&'
	if l.pos < len(l.src) && isLetter(l.src[l.pos]) {
		l.readIdentBody()
		return mk(AMP_REF, start, l.pos, l.src)
	}
	l.errorf(start, "expected identifier after '&'")
	return mk(INVALID, start, l.pos, l.src)
}

// readBhString scans either a binary ('01010'B) or hex ('AF01'H) string
// literal. The body is whitespace-tolerant per X.680 §11.10/§11.12 -
// inner whitespace is preserved verbatim in the token text so callers
// can re-parse if they care about the canonical value.
func (l *Lexer) readBhString(start int) Token {
	l.pos++ // consume opening quote
	for l.pos < len(l.src) && l.src[l.pos] != '\'' {
		l.pos++
	}
	if l.pos >= len(l.src) {
		l.errorf(start, "unterminated b/h-string literal")
		return mk(INVALID, start, l.pos, l.src)
	}
	l.pos++ // consume closing quote
	if l.pos >= len(l.src) {
		l.errorf(start, "expected 'B' or 'H' suffix after string literal")
		return mk(INVALID, start, l.pos, l.src)
	}
	switch l.src[l.pos] {
	case 'B', 'b':
		l.pos++
		return mk(BSTRING, start, l.pos, l.src)
	case 'H', 'h':
		l.pos++
		return mk(HSTRING, start, l.pos, l.src)
	}
	l.errorf(start, "expected 'B' or 'H' suffix after string literal")
	return mk(INVALID, start, l.pos, l.src)
}

// readCString scans a "..." character string literal. Doubled inner
// quotes ("") are an escaped quote per X.680 §11.14.
func (l *Lexer) readCString(start int) Token {
	l.pos++ // consume opening quote
	for l.pos < len(l.src) {
		if l.src[l.pos] == '"' {
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '"' {
				l.pos += 2
				continue
			}
			l.pos++ // consume closing quote
			return mk(CSTRING, start, l.pos, l.src)
		}
		l.pos++
	}
	l.errorf(start, "unterminated cstring literal")
	return mk(INVALID, start, l.pos, l.src)
}

// readNumber scans an integer or real literal. Reals follow X.680
// §11.8: digits, optional '.', optional 'eE'-exponent with optional
// sign. We never accept a leading sign - the parser handles unary
// minus via the HYPHEN token.
func (l *Lexer) readNumber(start int) Token {
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	// Fractional part - must not be `..` which is the range operator.
	if l.pos < len(l.src) && l.src[l.pos] == '.' &&
		!(l.pos+1 < len(l.src) && l.src[l.pos+1] == '.') {
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	// Exponent.
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.pos++
		}
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	return mk(NUMBER, start, l.pos, l.src)
}

// readWordOrIdent scans an identifier, type reference, keyword or a
// WITH SYNTAX template word. Identifiers in ASN.1 may contain hyphens
// but not a trailing hyphen, and a "--" mid-identifier introduces a
// line comment that terminates the current identifier early.
func (l *Lexer) readWordOrIdent(start int) Token {
	firstUpper := isUpper(l.src[l.pos])
	l.readIdentBody()
	text := string(l.src[start:l.pos])

	// Strip a trailing hyphen (X.680 forbids it - report and trim so
	// the parser sees a clean identifier).
	if strings.HasSuffix(text, "-") {
		l.errorf(start, "identifier %q ends with hyphen", text)
		l.pos--
		text = text[:len(text)-1]
	}

	if firstUpper && isKeyword(text) {
		// Inside a WITH SYNTAX template, even all-uppercase words
		// like INTEGER or SEQUENCE are *literals*, not keywords.
		if l.withSynt && isAllUpper(text) {
			return Token{Kind: WORD, Pos: start, End: l.pos, Text: text}
		}
		return Token{Kind: KEYWORD, Pos: start, End: l.pos, Text: text}
	}

	if firstUpper {
		// Could be a type reference (mixed case) or a WITH SYNTAX
		// word (all-upper). Disambiguate by context.
		if l.withSynt && isAllUpper(text) {
			return Token{Kind: WORD, Pos: start, End: l.pos, Text: text}
		}
		return Token{Kind: TYPEREFERENCE, Pos: start, End: l.pos, Text: text}
	}
	return Token{Kind: IDENTIFIER, Pos: start, End: l.pos, Text: text}
}

// readIdentBody consumes the longest run starting at l.pos that forms
// a valid ASN.1 identifier body, stopping before a "--" comment marker.
func (l *Lexer) readIdentBody() {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-' {
			// "--" starts a comment - end of identifier.
			return
		}
		if isIdentBody(c) {
			l.pos++
			continue
		}
		return
	}
}

func (l *Lexer) errorf(pos int, format string, args ...interface{}) {
	l.errors = append(l.errors, Diagnostic{
		Line:    1, // we keep the line/col deprecated form for now
		Column:  pos,
		Message: fmt.Sprintf(format, args...),
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mk(k TokenKind, start, end int, src []byte) Token {
	return Token{Kind: k, Pos: start, End: end, Text: string(src[start:end])}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
func isLower(b byte) bool { return b >= 'a' && b <= 'z' }

func isLetter(b byte) bool {
	if isUpper(b) || isLower(b) {
		return true
	}
	if b < utf8.RuneSelf {
		return false
	}
	// Non-ASCII letters are allowed in some ASN.1 dialects via
	// utf8String - decode and ask unicode.
	r, _ := utf8.DecodeRune([]byte{b})
	return unicode.IsLetter(r)
}

func isIdentBody(b byte) bool {
	return isLetter(b) || isDigit(b) || b == '-'
}

func isAllUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || isDigit(c) {
			continue
		}
		if !isUpper(c) {
			return false
		}
	}
	return true
}

// keywordSet lists every X.680/X.681/X.682/X.683 reserved word. The
// list is exhaustive on purpose - the parser relies on this membership
// test to decide whether an upper-case identifier is a type reference
// or a keyword.
var keywordSet = map[string]bool{
	"ABSENT":           true,
	"ABSTRACT-SYNTAX":  true,
	"ALL":              true,
	"APPLICATION":      true,
	"AUTOMATIC":        true,
	"BEGIN":            true,
	"BIT":              true,
	"BMPString":        true,
	"BOOLEAN":          true,
	"BY":               true,
	"CHARACTER":        true,
	"CHOICE":           true,
	"CLASS":            true,
	"COMPONENT":        true,
	"COMPONENTS":       true,
	"CONSTRAINED":      true,
	"CONTAINING":       true,
	"DATE":             true,
	"DATE-TIME":        true,
	"DEFAULT":          true,
	"DEFINITIONS":      true,
	"DURATION":         true,
	"EMBEDDED":         true,
	"ENCODED":          true,
	"ENCODING-CONTROL": true,
	"END":              true,
	"ENUMERATED":       true,
	"EXCEPT":           true,
	"EXPLICIT":         true,
	"EXPORTS":          true,
	"EXTENSIBILITY":    true,
	"EXTERNAL":         true,
	"FALSE":            true,
	"FROM":             true,
	"GeneralizedTime":  true,
	"GeneralString":    true,
	"GraphicString":    true,
	"IA5String":        true,
	"IDENTIFIER":       true,
	"IMPLICIT":         true,
	"IMPLIED":          true,
	"IMPORTS":          true,
	"INCLUDES":         true,
	"INSTANCE":         true,
	"INSTRUCTIONS":     true,
	"INTEGER":          true,
	"INTERSECTION":     true,
	"ISO646String":     true,
	"MAX":              true,
	"MIN":              true,
	"MINUS-INFINITY":   true,
	"NOT-A-NUMBER":     true,
	"NULL":             true,
	"NumericString":    true,
	"OBJECT":           true,
	"ObjectDescriptor": true,
	"OCTET":            true,
	"OF":               true,
	"OID-IRI":          true,
	"OPTIONAL":         true,
	"PATTERN":          true,
	"PDV":              true,
	"PLUS-INFINITY":    true,
	"PRESENT":          true,
	"PrintableString":  true,
	"PRIVATE":          true,
	"REAL":             true,
	"RELATIVE-OID":     true,
	"RELATIVE-OID-IRI": true,
	"SEQUENCE":         true,
	"SET":              true,
	"SETTINGS":         true,
	"SIZE":             true,
	"STRING":           true,
	"SYNTAX":           true,
	"T61String":        true,
	"TAGS":             true,
	"TeletexString":    true,
	"TIME":             true,
	"TIME-OF-DAY":      true,
	"TRUE":             true,
	"TYPE-IDENTIFIER":  true,
	"UNION":            true,
	"UNIQUE":           true,
	"UNIVERSAL":        true,
	"UniversalString":  true,
	"UTCTime":          true,
	"UTF8String":       true,
	"VideotexString":   true,
	"VisibleString":    true,
	"WITH":             true,
}

// IsKeyword reports whether s is an X.680/X.681/X.682/X.683 reserved
// word. Exported so other packages (notably the lowering pass, which
// needs to know whether a TTCN-3 identifier collides with an ASN.1
// keyword) can share the same lookup table.
func IsKeyword(s string) bool { return keywordSet[s] }

func isKeyword(s string) bool { return keywordSet[s] }
