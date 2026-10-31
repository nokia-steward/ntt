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
// parameter lists, individual parameters, declarators, and return statements.
//
// {, [, and ( already supply that one level for the object they delimit, so a
// parenthesized list is not indented again:
//
//	f(
//		a,
//		b)
//
//	function f(int a,
//		int b) {}
//
// A type-parameter list uses < and >, which do not change the level, so a
// break there is indented as a continuation:
//
//	function f<type T,
//		type U>() {}
//
// A broken parameter inside such a list is its own object and adds one more
// level. A broken expression inside an argument list does the same:
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
// A break before the value of a declaration, or before the result of a
// return, indents that continuation:
//
//	var int x :=
//		1
//
//	return
//		1
//
// A newline before the { of a function or control body is not a continuation.
// The brace stays at the header's level, and the brace indents its body:
//
//	function f()
//	{
//		var int x := 1
//	}
//
//	control
//	{}
//
// A newline between statements does not add a level beyond the brackets that
// enclose them.
//
// # Comments
//
// Each line of a comment is indented on its own. The first line is placed at
// the current level, or one space after the previous token when it stays on
// that token's line. In a multi-line comment, the least-indented non-empty
// line is aligned to the current level and the other lines keep their
// relative indent. Empty lines inside the comment stay empty.
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
