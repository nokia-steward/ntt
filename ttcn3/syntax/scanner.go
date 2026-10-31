package syntax

// Tokenize given source code and return a root node with all the tokens.
func Tokenize(src []byte) *Root {
	root := newRoot(src)
	for {
		kind, begin, end := root.Scan()
		root.tokens = append(root.tokens, token{kind, begin, end})
		if kind == EOF {
			break
		}
	}
	return root
}

// NewScanner returns a new TTCN-3 scanner for src.
func NewScanner(src []byte) *Scanner {
	return &Scanner{
		src:   src,
		lines: []int{0},
	}
}

// Scanner scans a TTCN-3 source.
type Scanner struct {
	lines []int
	src   []byte
	pos   int
}

// Lines returns the line offsets of the source.
func (s *Scanner) Lines() []int {
	return s.lines
}

// Scan returns the next token and its range.
func (s *Scanner) Scan() (Kind, int, int) {
	s.scanWhitespace()

	if s.pos >= len(s.src) {
		return EOF, s.pos, s.pos + 1
	}

	pos := s.pos
	s.pos++
	ch := s.src[pos]

	var (
		typ  Kind
		next byte
	)

	if s.pos < len(s.src) {
		next = s.src[s.pos]
	}

	switch {
	case isAlpha(ch):
		typ = IDENT
		s.scanAlnum()
	case isDigit(ch):
		typ = s.scanNumber()
	case ch == ',':
		typ = COMMA
	case ch == '+':
		typ = ADD
		if next == '+' {
			typ = INC
			s.pos++
		}
	case ch == '*':
		typ = MUL
	case ch == '&':
		typ = CONCAT
	case ch == '?':
		typ = ANY
		if next == '?' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '?' {
			s.pos += 2
			typ = NOTIMPL
		}
	case ch == '(':
		typ = LPAREN
	case ch == '[':
		typ = LBRACK
	case ch == '{':
		typ = LBRACE
	case ch == ')':
		typ = RPAREN
	case ch == ']':
		typ = RBRACK
	case ch == '}':
		typ = RBRACE
	case ch == ';':
		typ = SEMICOLON
	case ch == '/':
		switch next {
		case '/':
			s.scanLine()
			typ = COMMENT
		case '*':
			typ = s.scanMultiLineComment()
		default:
			typ = DIV
		}
	case ch == '@':
		switch {
		case isAlpha(next):
			s.scanAlnum()
			typ = MODIF
		case next == '>':
			s.pos++
			typ = ROR
		default:
			typ = ILLEGAL
		}
	case ch == '%':
		if isAlpha(next) {
			s.scanAlnum()
			typ = IDENT
		}
	case ch == '!':
		typ = EXCL
		if next == '=' {
			s.pos++
			typ = NE
		}
	case ch == '-':
		switch next {
		case '>':
			s.pos++
			typ = REDIR
		case '-':
			s.pos++
			typ = DEC
		default:
			typ = SUB
		}
	case ch == '.':
		typ = DOT
		if next == '.' {
			s.pos++
			typ = RANGE

			if s.pos < len(s.src) && s.src[s.pos] == '.' {
				s.pos++
				typ = ELIPSIS
			}
		}
	case ch == ':':
		switch next {
		case ':':
			s.pos++
			typ = COLONCOLON
		case '=':
			s.pos++
			typ = ASSIGN
		default:
			typ = COLON
		}
	case ch == '<':
		switch next {
		case '<':
			s.pos++
			typ = SHL
		case '=':
			s.pos++
			typ = LE
		case '@':
			s.pos++
			typ = ROL
		default:
			typ = LT
		}
	case ch == '=':
		switch next {
		case '=':
			s.pos++
			typ = EQ
		case '>':
			s.pos++
			typ = DECODE
		}
	case ch == '>':
		switch next {
		case '>':
			s.pos++
			typ = SHR
		case '=':
			s.pos++
			typ = GE
		default:
			typ = GT
		}
	case ch == '\'':
		typ = s.scanBitstring()
	case ch == '#':
		s.scanLine()
		typ = PREPROC
	case ch == '"':
		typ = s.scanString()
	default:
		typ = ILLEGAL
	}

	return typ, pos, s.pos
}

func (s *Scanner) scanWhitespace() {
	for s.pos < len(s.src) {
		switch ch := s.src[s.pos]; ch {
		case ' ', '\t', '\r':
		case '\n', '\v', '\f':
			s.lines = append(s.lines, s.pos+1)
		default:
			return
		}
		s.pos++
	}
}

func (s *Scanner) scanLine() {
	for s.pos < len(s.src) && s.src[s.pos] != '\n' {
		s.pos++
	}
}

func (s *Scanner) scanAlnum() {
	for s.pos < len(s.src) && isAlnum(s.src[s.pos]) {
		s.pos++
	}
}

func (s *Scanner) scanMultiLineComment() Kind {
	s.pos++ // skip the first '*'
	for s.pos < len(s.src) {
		ch := s.src[s.pos]
		if ch == '\n' || ch == '\v' || ch == '\f' {
			s.lines = append(s.lines, s.pos+1)
		}
		s.pos++
		if ch == '*' && s.pos < len(s.src) && s.src[s.pos] == '/' {
			s.pos++
			return COMMENT
		}
	}

	return UNTERMINATED
}

func (s *Scanner) scanString() Kind {
	s.pos-- // backup for proper quoting ("")
	for {
		s.pos++
		if s.pos >= len(s.src) {
			return UNTERMINATED
		}

		switch ch := s.src[s.pos]; ch {
		case '\n', '\v', '\f':
			s.lines = append(s.lines, s.pos+1)
		case '\\':
			s.pos++
		case '"':
			s.pos++
			if s.pos >= len(s.src) || s.src[s.pos] != '"' {
				return STRING
			}
		}
	}
}

func (s *Scanner) scanBitstring() Kind {
L:
	for {
		if s.pos >= len(s.src) {
			return UNTERMINATED
		}
		switch ch := s.src[s.pos]; ch {
		case '\n', '\v', '\f':
			s.lines = append(s.lines, s.pos+1)
		case '\'':
			s.pos++
			break L
		}
		s.pos++
	}

	typ := BSTRING
	if s.pos >= len(s.src) || !isAlpha(s.src[s.pos]) {
		typ = MALFORMED
	}

	s.scanAlnum()
	return typ
}

func (s *Scanner) scanNumber() Kind {
	tok := INT

	if s.src[s.pos-1] != '0' {
		s.scanDigits()
	}

	// scan fractional part
	if s.pos < len(s.src) && s.src[s.pos] == '.' {
		// check '..' token
		if s.pos+1 < len(s.src) && s.src[s.pos+1] == '.' {
			return tok
		}
		tok = FLOAT
		s.pos++
		s.scanDigits()
	}

	// scan exponent
	if s.pos < len(s.src) && (s.src[s.pos] == 'e' || s.src[s.pos] == 'E') {
		tok = FLOAT
		s.pos++
		if s.pos < len(s.src) && (s.src[s.pos] == '+' || s.src[s.pos] == '-') {
			s.pos++
		}
		if s.pos >= len(s.src) || !isDigit(s.src[s.pos]) {
			tok = MALFORMED
		} else {
			s.scanDigits()
		}
	}

	// handle trailing carbage
	if s.pos < len(s.src) && isAlnum(s.src[s.pos]) {
		tok = MALFORMED
		s.scanAlnum()
	}

	return tok
}

func (s *Scanner) scanDigits() {
	for s.pos < len(s.src) && isDigit(s.src[s.pos]) {
		s.pos++
	}
}

func isAlnum(ch byte) bool {
	return isAlpha(ch) || isDigit(ch)
}

func isAlpha(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
