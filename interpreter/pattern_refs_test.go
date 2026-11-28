package interpreter_test

import "testing"

// \N{...} (B.1.5.4) is one character of the set it names, inserted as
// that set: a value's character literally, a subtype's range, charstring
// any character, in a set expression too. A type of a list of values,
// whose set ntt doesn't keep, is not expanded yet: it must at least not
// match what is outside it. \q{} is a character, not a metacharacter.
// Concatenating @nocase patterns gives a @nocase pattern (15.11).
func TestPatternReferences(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type charstring Lower ("a".."z");
		type charstring Caret ("^".."z");
		type charstring AZ ("a", "z");
		const charstring c_q := "?";
		testcase tc() runs on C {
			var charstring fails := "";
			if (not match("m", pattern "\N{Lower}")) { fails := fails & "Lower " }
			if (match("M", pattern "\N{Lower}")) { fails := fails & "Lower-M " }
			if (not match("5", pattern "[\N{Lower}5]")) { fails := fails & "in-set " }
			if (match("A", pattern "\N{Caret}")) { fails := fails & "Caret " }
			if (match("x", pattern "\N{c_q}")) { fails := fails & "c_q " }
			if (not match("?", pattern "\N{c_q}")) { fails := fails & "c_q-? " }
			if (match("b", pattern "\N{AZ}")) { fails := fails & "AZ " }
			if (not match("xQy", pattern "x\N{charstring}y")) { fails := fails & "charstring " }
			if (match("abc", pattern "a\q{0,0,0,42}")) { fails := fails & "q-star " }
			if (not match("]", pattern "[a\q{0,0,0,93}]")) { fails := fails & "q-bracket " }
			if (not match("ABC", pattern @nocase "ab" & pattern @nocase "c")) { fails := fails & "nocase-concat " }
			if (not match("c", pattern "[\N{charstring}]") or not match("x", pattern "[0\N{charstring}]")) { fails := fails & "charstring-in-set " }
			if (not match("m", pattern "[]\N{Lower}]") or not match("]", pattern "[]\N{Lower}]") or match("m]", pattern "[]\N{Lower}]")) { fails := fails & "set-first-bracket " }
			if (fails == "") { setverdict(pass) } else { setverdict(fail, fails) }
		}
	}`)
}
