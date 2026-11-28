package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// The errors the predefined functions and the timer start have (ETSI ES
// 201 873-1 16.1.2 a 3, C.1.9, C.1.31, C.1.32, C.2.2, C.4.1, 23.2).
func TestPredefinedFunctionErrors(t *testing.T) {
	for name, c := range map[string]struct{ body, clause string }{
		"float2int of infinity":         {`var integer i := float2int(infinity);`, "C.1.9"},
		"float2int of -infinity":        {`var float f := -infinity; var integer i := float2int(f);`, "C.1.9"},
		"float2int of not_a_number":     {`var integer i := float2int(not_a_number);`, "C.1.9"},
		"regexp of a negative group":    {`var integer g := -1; var charstring s := regexp("abc", charstring:"(a)bc", g);`, "C.4.1"},
		"regexp of no such group":       {`var integer g := 1; var charstring s := regexp("abc", charstring:"(a)bc", g);`, "C.4.1"},
		"oct2unichar of a BOM":          {`var universal charstring s := oct2unichar('EFBBBF41'O, "UTF-8");`, "C.1.31"},
		"oct2unichar of no encoding":    {`var universal charstring s := oct2unichar('41'O, "UTF-7");`, "C.1.31"},
		"unichar2oct of no encoding":    {`var octetstring o := unichar2oct("A", "latin1");`, "C.1.32"},
		"sizeof of an unbound value":    {`var R r; var integer n := sizeof(r);`, "16.1.2"},
		"a conversion of an unbound":    {`var integer i; var charstring s := int2str(i);`, "16.1.2"},
		"a timer with no duration":      {`timer t; t.start;`, "23.2"},
		"a timer parameter with none":   {`timer t; f_start(t);`, "23.2"},
		"a timer started with nothing ": {`timer t; t.start();`, "23.2"},
	} {
		src := `module M {
			type record R { integer a }
			type component C {}
			function f_start(timer p) { p.start }
			testcase tc() runs on C {
				setverdict(pass);
				` + c.body + `
				setverdict(pass);
			}
		}`
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, c.clause) {
				t.Errorf("%s, %s clock: %s (%s) %v; want the %s error", name, k.name, v, reason, err, c.clause)
			}
		}
	}
}

// What the standard allows stays allowed: float2int truncates any finite
// float exactly; regexp of an existing group; oct2unichar of the seven
// encodings, lenient on their spelling; a timer with a default, or given
// a duration; a conversion of an initialised value.
func TestPredefinedFunctionsAllowed(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C {}
		function want(boolean b, charstring what) { if (not b) { setverdict(fail, what); stop } }
		testcase tc() runs on C {
			want(float2int(3.9) == 3 and float2int(-3.9) == -3, "truncation");
			want(float2int(1.0e30) == 1000000000000000019884624838656, "a large float, exactly");
			want(regexp("abc", charstring:"(a)(b)c", 1) == "b", "the second group");
			want(regexp("xyz", charstring:"(a)bc", 0) == "", "no match is the empty string");
			want(oct2unichar('0041'O, "UTF-16BE") == "A" and oct2unichar('41'O, "utf8") == "A", "encodings");
			want(unichar2oct("A", "UTF-32LE") == '41000000'O, "unichar2oct");
			want(oct2unichar('FEFF0068'O, "UTF-16BE") == char(0, 0, 254, 255) & "h", "U+FEFF of one byte order is a character");
			timer d := 0.1; d.start; d.timeout;
			timer n; n.start(0.1); n.timeout;
			var integer i := 5;
			want(int2str(i) == "5", "an initialised conversion");
			setverdict(pass);
		}
	}`, runtime.PassVerdict)
}
