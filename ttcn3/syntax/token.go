package syntax

import (
	"strconv"
	"strings"
)

type token struct {
	Kind
	Begin, End int
}

// Kind is the set of lexical tokens of the Go programming language.
type Kind int

// The list of tokens.
const (
	// Special tokens
	ILLEGAL Kind = iota
	EOF
	UNTERMINATED
	MALFORMED

	literal_beg
	COMMENT
	PREPROC

	// Identifiers and basic type literals
	// (these tokens stand for classes of literals)
	IDENT   // main
	INT     // 12345
	FLOAT   // 123.45
	STRING  // "abc"
	BSTRING // '101?F'H
	MODIF   // @fuzzy
	literal_end

	operator_beg
	// Operators and delimiters
	INC // ++
	ADD // +
	SUB // -
	DEC // --
	MUL // *
	DIV // /

	SHL    // <<
	ROL    // <@
	SHR    // >>
	ROR    // @>
	CONCAT // &

	REDIR      // ->
	DECODE     // =>
	ANY        // ?
	EXCL       // !
	RANGE      // ..
	ELIPSIS    // ...
	ASSIGN     // :=
	COLONCOLON // ::

	EQ // ==
	NE // !=
	LT // <
	LE // <=
	GT // >
	GE // >=

	LPAREN // (
	LBRACK // [
	LBRACE // {
	COMMA  // ,
	DOT    // .

	RPAREN    // )
	RBRACK    // ]
	RBRACE    // }
	SEMICOLON // ;
	COLON     // :
	operator_end

	keyword_beg
	// Keywords
	MOD // mod
	REM // rem

	AND // and
	OR  // or
	XOR // xor
	NOT // not

	AND4B // and4b
	OR4B  // or4b
	XOR4B // xor4b
	NOT4B // not4b

	ADDRESS
	ALIVE
	ALL
	ALT
	ALTSTEP
	ANYKW
	BREAK
	CASE
	CHARSTRING
	CLASS
	COMPONENT
	CONST
	CONTINUE
	CONTROL
	CREATE
	DECMATCH
	DISPLAY
	DO
	ELSE
	ENCODE
	ENUMERATED
	ERROR
	EXCEPT
	EXCEPTION
	EXTENDS
	EXTENSION
	EXTERNAL
	FAIL
	FALSE
	FOR
	FRIEND
	FROM
	FUNCTION
	GOTO
	GROUP
	IF
	IFPRESENT
	IMPORT
	IN
	INCONC
	INOUT
	INTERLEAVE
	LABEL
	LANGUAGE
	LENGTH
	MAP
	MESSAGE
	MIXED
	MODIFIES
	MODULE
	MODULEPAR
	MTC
	NAN
	NOBLOCK
	NONE
	NULL
	OF
	OMIT
	ON
	OPTIONAL
	OUT
	OVERRIDE
	PARAM
	PASS
	PATTERN
	PORT
	PRESENT
	PRIVATE
	PROCEDURE
	PUBLIC
	REALTIME
	RECORD
	REGEXP
	REPEAT
	RETURN
	RUNS
	SELECT
	SENDER
	SET
	SIGNATURE
	STEPSIZE
	SYSTEM
	TEMPLATE
	TESTCASE
	TIMER
	TIMESTAMP
	TO
	TRUE
	TYPE
	UNION
	UNIVERSAL
	UNMAP
	VALUE
	VAR
	VARIANT
	WHILE
	WITH
	keyword_end
	token_end
)

var tokens = [...]string{
	ILLEGAL: "ILLEGAL",

	EOF:     "EOF",
	COMMENT: "COMMENT",
	PREPROC: "PREPROC",

	IDENT:   "IDENT",
	INT:     "INT",
	FLOAT:   "FLOAT",
	STRING:  "STRING",
	BSTRING: "BSTRING",
	MODIF:   "MODIF",

	ADD: "+",
	INC: "++",
	SUB: "-",
	DEC: "--",
	MUL: "*",
	DIV: "/",

	SHL:    "<<",
	ROL:    "<@",
	SHR:    ">>",
	ROR:    "@>",
	CONCAT: "&",

	REDIR:   "->",
	DECODE:  "=>",
	ANY:     "?",
	EXCL:    "!",
	RANGE:   "..",
	ELIPSIS: "...",

	ASSIGN:     ":=",
	COLONCOLON: "::",
	EQ:         "==",
	NE:         "!=",
	LT:         "<",
	LE:         "<=",
	GT:         ">",
	GE:         ">=",

	LPAREN:    "(",
	LBRACK:    "[",
	LBRACE:    "{",
	COMMA:     ",",
	DOT:       ".",
	RPAREN:    ")",
	RBRACK:    "]",
	RBRACE:    "}",
	SEMICOLON: ";",
	COLON:     ":",

	MOD: "mod",
	REM: "rem",

	AND: "and",
	OR:  "or",
	XOR: "xor",
	NOT: "not",

	AND4B: "and4b",
	OR4B:  "or4b",
	XOR4B: "xor4b",
	NOT4B: "not4b",

	ADDRESS:    "address",
	ALIVE:      "alive",
	ALL:        "all",
	ALT:        "alt",
	ALTSTEP:    "altstep",
	ANYKW:      "any",
	BREAK:      "break",
	CASE:       "case",
	CHARSTRING: "charstring",
	CLASS:      "class",
	COMPONENT:  "component",
	CONST:      "const",
	CONTINUE:   "continue",
	CONTROL:    "control",
	CREATE:     "create",
	DECMATCH:   "decmatch",
	DISPLAY:    "display",
	DO:         "do",
	ELSE:       "else",
	ENCODE:     "encode",
	ENUMERATED: "enumerated",
	ERROR:      "error",
	EXCEPT:     "except",
	EXCEPTION:  "exception",
	EXTENDS:    "extends",
	EXTENSION:  "extension",
	EXTERNAL:   "external",
	FAIL:       "fail",
	FALSE:      "false",
	FOR:        "for",
	FRIEND:     "friend",
	FROM:       "from",
	FUNCTION:   "function",
	GOTO:       "goto",
	GROUP:      "group",
	IF:         "if",
	IFPRESENT:  "ifpresent",
	IMPORT:     "import",
	IN:         "in",
	INCONC:     "inconc",
	INOUT:      "inout",
	INTERLEAVE: "interleave",
	LABEL:      "label",
	LANGUAGE:   "language",
	LENGTH:     "length",
	MAP:        "map",
	MESSAGE:    "message",
	MIXED:      "mixed",
	MODIFIES:   "modifies",
	MODULE:     "module",
	MODULEPAR:  "modulepar",
	MTC:        "mtc",
	NAN:        "not_a_number",
	NOBLOCK:    "noblock",
	NONE:       "none",
	NULL:       "null",
	OF:         "of",
	OMIT:       "omit",
	ON:         "on",
	OPTIONAL:   "optional",
	OUT:        "out",
	OVERRIDE:   "override",
	PARAM:      "param",
	PASS:       "pass",
	PATTERN:    "pattern",
	PORT:       "port",
	PRESENT:    "present",
	PRIVATE:    "private",
	PROCEDURE:  "procedure",
	PUBLIC:     "public",
	REALTIME:   "realtime",
	RECORD:     "record",
	REGEXP:     "regexp",
	REPEAT:     "repeat",
	RETURN:     "return",
	RUNS:       "runs",
	SELECT:     "select",
	SENDER:     "sender",
	SET:        "set",
	SIGNATURE:  "signature",
	STEPSIZE:   "stepsize",
	SYSTEM:     "system",
	TEMPLATE:   "template",
	TESTCASE:   "testcase",
	TIMER:      "timer",
	TIMESTAMP:  "timestamp",
	TO:         "to",
	TRUE:       "true",
	TYPE:       "type",
	UNION:      "union",
	UNIVERSAL:  "universal",
	UNMAP:      "unmap",
	VALUE:      "value",
	VAR:        "var",
	VARIANT:    "variant",
	WHILE:      "while",
	WITH:       "with",
}

// String returns the string corresponding to the token tok.
// For operators, delimiters, and keywords the string is the actual
// token character sequence (e.g., for the token ADD, the string is
// "+"). For all other tokens the string corresponds to the token
// constant name (e.g. for the token IDENT, the string is "IDENT").
func (tok Kind) String() string {
	s := ""
	if 0 <= tok && tok < Kind(len(tokens)) {
		s = tokens[tok]
	}
	if s == "" {
		s = "token(" + strconv.Itoa(int(tok)) + ")"
	}
	return s
}

// A set of constants for precedence-based expression parsing.
// Non-operators have lowest precedence, followed by operators
// starting with precedence 1 up to unary operators. The highest
// precedence serves as "catch-all" precedence for selector,
// indexing, and other operator and delimiter tokens.
const (
	LowestPrec  = 0 // non-operators
	UnaryPrec   = 6
	HighestPrec = 15
)

// Precedence returns the operator precedence of the binary
// operator op. If op is not a binary operator, the result
// is LowestPrecedence.
func (tok Kind) Precedence() int {
	switch tok {
	case ASSIGN:
		return 1
	case COLON:
		return 2
	case DECODE:
		return 3
	case RANGE:
		return 4
	case EXCL:
		return 5
	case OR:
		return 6
	case XOR:
		return 7
	case AND:
		return 8
	case NOT:
		return 9
	case EQ, NE:
		return 10
	case LT, LE, GT, GE:
		return 11
	case SHR, SHL, ROR, ROL:
		return 12
	case OR4B:
		return 13
	case XOR4B:
		return 14
	case AND4B:
		return 15
	case NOT4B:
		return 16
	case ADD, SUB, CONCAT:
		return 17
	case MUL, DIV, REM, MOD:
		return 18
	case COLONCOLON:
		return 19
	case OF:
		return 20
	}
	return LowestPrec
}

var keywords map[string]Kind

func init() {
	keywords = make(map[string]Kind)
	for i := keyword_beg + 1; i < keyword_end; i++ {
		keywords[tokens[i]] = i
	}
}

// Lookup maps an identifier to its keyword token or IDENT (if not a keyword).
func Lookup(ident []byte) Kind {
	if tok, isKeyword := keywords[string(ident)]; isKeyword {
		return tok
	}
	return IDENT
}

// Unquote interprets s as a quoted TTCN-3 string literal,
// returning the string value that s quotes.
func Unquote(s string) (string, error) {
	if len(s) < 2 {
		return s, strconv.ErrSyntax
	}
	inner := s[1 : len(s)-1]
	// TTCN-3 escapes an embedded double quote by doubling it. Try the
	// Go unquoter first so the common C-style escapes (\n, \t, \uXXXX,
	// ...) that TTCN-3 tooling accepts keep working.
	if out, err := strconv.Unquote(`"` + strings.ReplaceAll(inner, `""`, `\"`) + `"`); err == nil {
		return out, nil
	}
	// The literal is not a valid Go-escaped string: a backslash
	// appears before whitespace or before a TTCN-3-only sequence such
	// as a USI quadruple `\q{...}` or a pattern reference `\N{...}`. In
	// a plain (universal) charstring value those backslashes are
	// ordinary characters (ETSI 6.1.1), so fall back to treating every
	// backslash literally; only the doubled-quote escape applies.
	return strings.ReplaceAll(inner, `""`, `"`), nil
}

// Predicates

// IsLiteral returns true for tokens corresponding to identifiers
// and basic type literals; it returns false otherwise.
func (tok Kind) IsLiteral() bool { return literal_beg < tok && tok < literal_end }

// IsOperator returns true for tokens corresponding to operators and
// delimiters; it returns false otherwise.
func (tok Kind) IsOperator() bool { return operator_beg < tok && tok < operator_end }

// IsKeyword returns true for tokens corresponding to keywords;
// it returns false otherwise.
func (tok Kind) IsKeyword() bool { return keyword_beg < tok && tok < keyword_end }
