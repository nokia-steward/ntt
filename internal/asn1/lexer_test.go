package asn1

import (
	"strings"
	"testing"
)

func lex(t *testing.T, src string) []Token {
	t.Helper()
	l := NewLexer([]byte(src))
	toks, diags := l.All()
	for _, d := range diags {
		t.Logf("lex diag: %s", d.Message)
	}
	return toks
}

func kinds(toks []Token) []TokenKind {
	out := make([]TokenKind, 0, len(toks))
	for _, t := range toks {
		out = append(out, t.Kind)
	}
	return out
}

func TestLexer_BasicPunctuation(t *testing.T) {
	toks := lex(t, "{ } [ ] ( ) , ; : .. ... ::= [[ ]]")
	want := []TokenKind{
		LBRACE, RBRACE,
		LBRACKET, RBRACKET,
		LPAREN, RPAREN,
		COMMA, SEMICOLON, COLON,
		DOUBLE_DOT, ELLIPSIS,
		ASSIGN,
		DOUBLE_LBRACKET, DOUBLE_RBRACKET,
		EOF,
	}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

func TestLexer_Keywords(t *testing.T) {
	toks := lex(t, "BEGIN END SEQUENCE OF INTEGER BOOLEAN CHOICE")
	for i, k := range []string{"BEGIN", "END", "SEQUENCE", "OF", "INTEGER", "BOOLEAN", "CHOICE"} {
		if toks[i].Kind != KEYWORD {
			t.Errorf("token %d: kind = %s, want KEYWORD", i, toks[i].Kind)
		}
		if toks[i].Text != k {
			t.Errorf("token %d: text = %q, want %q", i, toks[i].Text, k)
		}
	}
	if toks[len(toks)-1].Kind != EOF {
		t.Errorf("missing EOF terminator")
	}
}

func TestLexer_TypeRefVsKeyword(t *testing.T) {
	toks := lex(t, "MyType SEQUENCE myValue MY-TYPE")
	if toks[0].Kind != TYPEREFERENCE || toks[0].Text != "MyType" {
		t.Errorf("MyType: got %v", toks[0])
	}
	if toks[1].Kind != KEYWORD || toks[1].Text != "SEQUENCE" {
		t.Errorf("SEQUENCE: got %v", toks[1])
	}
	if toks[2].Kind != IDENTIFIER || toks[2].Text != "myValue" {
		t.Errorf("myValue: got %v", toks[2])
	}
	if toks[3].Kind != TYPEREFERENCE || toks[3].Text != "MY-TYPE" {
		t.Errorf("MY-TYPE: got %v", toks[3])
	}
}

func TestLexer_HyphenInsideIdent(t *testing.T) {
	toks := lex(t, "RRC-PDU-Definitions")
	if toks[0].Kind != TYPEREFERENCE || toks[0].Text != "RRC-PDU-Definitions" {
		t.Errorf("got %v", toks[0])
	}
}

func TestLexer_DoubleHyphenEndsIdent(t *testing.T) {
	// "Foo--bar" should tokenise as TYPEREFERENCE(Foo) then a
	// line comment that swallows the rest.
	toks := lex(t, "Foo--bar baz\nNext")
	if toks[0].Kind != TYPEREFERENCE || toks[0].Text != "Foo" {
		t.Errorf("first token: got %v", toks[0])
	}
	if toks[1].Kind != TYPEREFERENCE || toks[1].Text != "Next" {
		t.Errorf("second token: got %v", toks[1])
	}
}

func TestLexer_Numbers(t *testing.T) {
	toks := lex(t, "0 42 3.14 1e10 2.5E-3")
	for i, want := range []string{"0", "42", "3.14", "1e10", "2.5E-3"} {
		if toks[i].Kind != NUMBER {
			t.Errorf("token %d: kind = %s, want NUMBER", i, toks[i].Kind)
		}
		if toks[i].Text != want {
			t.Errorf("token %d: text = %q, want %q", i, toks[i].Text, want)
		}
	}
}

func TestLexer_RangeOperatorPrecedesDot(t *testing.T) {
	toks := lex(t, "1..10")
	if toks[0].Kind != NUMBER || toks[0].Text != "1" {
		t.Errorf("first: got %v", toks[0])
	}
	if toks[1].Kind != DOUBLE_DOT {
		t.Errorf("dotdot: got %v", toks[1])
	}
	if toks[2].Kind != NUMBER || toks[2].Text != "10" {
		t.Errorf("second: got %v", toks[2])
	}
}

func TestLexer_Strings(t *testing.T) {
	toks := lex(t, `"hello" 'AF'H '0101'B "with ""quote"" inside"`)
	if toks[0].Kind != CSTRING {
		t.Errorf("cstring: got %v", toks[0])
	}
	if toks[1].Kind != HSTRING || toks[1].Text != "'AF'H" {
		t.Errorf("hstring: got %v", toks[1])
	}
	if toks[2].Kind != BSTRING || toks[2].Text != "'0101'B" {
		t.Errorf("bstring: got %v", toks[2])
	}
	if toks[3].Kind != CSTRING {
		t.Errorf("escaped cstring: got %v", toks[3])
	}
	if !strings.Contains(toks[3].Text, `""quote""`) {
		t.Errorf("escaped quote text lost: %q", toks[3].Text)
	}
}

func TestLexer_AmpRef(t *testing.T) {
	toks := lex(t, "&Type &id")
	if toks[0].Kind != AMP_REF || toks[0].Text != "&Type" {
		t.Errorf("&Type: got %v", toks[0])
	}
	if toks[1].Kind != AMP_REF || toks[1].Text != "&id" {
		t.Errorf("&id: got %v", toks[1])
	}
}

func TestLexer_LineCommentTerminators(t *testing.T) {
	toks := lex(t, "X -- inline -- Y\nZ -- to end of line\nW")
	want := []string{"X", "Y", "Z", "W"}
	for i, w := range want {
		if toks[i].Text != w {
			t.Errorf("token %d: %q want %q", i, toks[i].Text, w)
		}
	}
}

func TestLexer_BlockComment(t *testing.T) {
	toks := lex(t, "A /* outer /* nested */ still inside */ B")
	if toks[0].Text != "A" || toks[1].Text != "B" {
		t.Errorf("nested block comment not handled: %v", toks)
	}
}

func TestLexer_WithSyntaxWords(t *testing.T) {
	l := NewLexer([]byte("&Type"))
	l.EnterWithSyntax()
	_, _ = l.All()
	l.LeaveWithSyntax()

	l = NewLexer([]byte("CATEGORY CODE TYPE &Type"))
	l.EnterWithSyntax()
	toks, _ := l.All()
	for i, want := range []string{"CATEGORY", "CODE", "TYPE"} {
		if toks[i].Kind != WORD {
			t.Errorf("token %d (%q): kind = %s, want WORD", i, want, toks[i].Kind)
		}
		if toks[i].Text != want {
			t.Errorf("token %d: text %q want %q", i, toks[i].Text, want)
		}
	}
	if toks[3].Kind != AMP_REF || toks[3].Text != "&Type" {
		t.Errorf("AMP_REF after WORDs: got %v", toks[3])
	}
}

func TestLexer_Assign(t *testing.T) {
	toks := lex(t, "Foo ::= INTEGER")
	if toks[1].Kind != ASSIGN || toks[1].Text != "::=" {
		t.Errorf("got %v", toks[1])
	}
}

func TestLexer_Ranges(t *testing.T) {
	toks := lex(t, "INTEGER (0..255)")
	want := []TokenKind{KEYWORD, LPAREN, NUMBER, DOUBLE_DOT, NUMBER, RPAREN, EOF}
	if k := kinds(toks); !equalKinds(k, want) {
		t.Errorf("got %v want %v", k, want)
	}
}

func TestLexer_TolerantOfGarbage(t *testing.T) {
	l := NewLexer([]byte("@#$"))
	toks, diags := l.All()
	if len(toks) < 2 {
		t.Fatalf("expected at least one error token + EOF, got %d", len(toks))
	}
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
}

func equalKinds(a, b []TokenKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
