// Package format implements pretty printers for TTCN-3 source code.
//
// [CanonicalPrinter] adjusts whitespace and indentation only. It does not
// rewrite tokens, reorder declarations, or insert line breaks. Breaks that
// are already present are kept and indented. [Fprint] returns a syntax error,
// and writes nothing, when src does not parse.
//
// # Whitespace
//
// At most one space is kept between two tokens on the same line. Tabs and
// other horizontal whitespace in that gap become that single space. Tokens
// that were already adjacent stay adjacent, except for the comma and
// assignment rules below. Leading space before the first token and trailing
// space after the last token are removed.
//
// A line break between tokens is kept. More than one blank line collapses to
// one. \r\n, \v, and \f become \n. A lone \r is not a line break; it becomes a
// space. When the input contains any token, the result ends with a single
// newline. Empty input stays empty.
//
// A comma is always followed by one space (1,2 becomes 1, 2). That space is
// dropped when the next token is on the next line. := is always surrounded by
// single spaces (x:=1 becomes x := 1). No other operator gains spaces.
//
// # Indentation
//
// Each indent level is a tab. [CanonicalPrinter.TabWidth] defaults to 8.
// [CanonicalPrinter.UseSpaces] uses that many spaces per level instead.
// [CanonicalPrinter.Indent] is the base level added after every line break.
// It does not indent the first line, because leading space is removed.
//
// After {, [, or (, the level increases by one. Before }, ], or ), it
// decreases by one. The closing bracket stays on the line where it was
// written; the new level applies to following lines. Nested brackets stack:
//
//	{
//		[
//			(
//				1, 2
//			)
//		] p.receive
//	}
//
// # Broken lines
//
// A line break inside an object indents the continuation by one level, once
// for that object. Further breaks in the same object stay at that level.
// Nested objects each add their own level. The objects are expressions,
// parameter lists, individual parameters, declarations, declarators, and
// return statements.
//
// {, [, and ( already supply that one level for the object they delimit, so a
// parenthesized list is not indented again:
//
//	f(
//		a,
//		b)
//
// A break between formal parameters aligns the next parameter with the first
// one, at the output column of the first token after the opening ( or <. The
// line starts with the indent of the line that holds the delimiter, followed
// by spaces up to that column. If the first parameter is on a line of its
// own, later parameters are indented as above instead:
//
//	function f(int a,
//	           int b) {}
//
//	signature S(in integer p_in,
//	            inout integer p_inout,
//	            out integer p_out);
//
//	function f<type T,
//	           type U>() {}
//
// A broken parameter inside such a list is its own object and adds one more
// level on top of the parenthesis. A broken expression inside an argument
// list does the same:
//
//	function f(in
//			int
//			x) {}
//
//	f(a +
//			b,
//		c)
//
// A chain of one operator is a nest of binary expressions, and the
// continuations line up:
//
//	var int x := a +
//		b +
//		c
//
//	var int x := a.
//		b.
//		c
//
// A broken higher-precedence subexpression is indented one level further:
//
//	var int x := a +
//		b *
//			c
//
// A break before the value of a var, const, or template declaration, or
// before the result of a return, indents that continuation:
//
//	var int x :=
//		1
//
//	template charstring t :=
//		"a"
//
//	return
//		1
//
// The later declarators of a declaration continue it, once:
//
//	var int x := 1,
//		y := 2,
//		z := 3
//
// A line break before runs on, system, mtc, or a return type clause of a
// function, testcase, altstep, signature, behaviour type, class, or
// constructor indents that clause one level. The indent ends with the clause,
// so the next clause and the { stay at the level of the header. A clause on the
// line of the header is not indented. A return statement is not a clause:
//
//	testcase TC()
//		runs on C
//		system S
//	{
//		log(1)
//	}
//
//	function f()
//		return integer
//	{
//		return
//			1
//	}
//
// A newline before a { is never a continuation. The brace stays at the
// current level, and the brace indents its body. That holds for the body of a
// function or control part, the { of a nested record, set, union, or
// enumerated type, a with attribute list, and a template or variable value:
//
//	control
//	{}
//
//	type set
//	{
//		int x
//	} S
//
//	const int x := 1
//	with { extension "a" }
//
//	template MessageType t :=
//	{
//		field1 := 1
//	}
//
//	var R v :=
//	{
//		a := 1
//	}
//
// A newline between statements does not add a level beyond the brackets that
// enclose them.
//
// # Comments
//
// Each line of a comment is indented on its own. The first line is placed at
// the current level, or one space after the previous token when it stays on
// that token's line. In a multi-line comment, the indentation of the line the
// comment starts on and of each further non-empty line is measured in
// columns, a tab advancing to the next multiple of TabWidth. The least of
// these is aligned to the level of the line the comment starts on, and the
// other lines keep their relative indent as spaces. A line that starts
// inside a multi-line literal or comment counts as column 0. Empty lines
// inside the comment stay empty. Formatting the result again does not change
// it:
//
//	{
//		x := 25 /* a
//		           b */
//	}
//
// Comments on consecutive lines are aligned in a column, as are the
// assignments on those lines:
//
//	p := {
//		x := 1,   // a
//		x2 := 123 /* b */
//	}
//
// # Literals
//
// Newlines and tabs inside a literal are preserved. Whitespace anywhere else
// is normalized by the rules above.
package format
