package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestComponentCompatValues: a function runs on a component whose
// definitions have the same types, initial values, durations and
// template bodies as those of its runs on clause (6.3.3 b-d).
func TestComponentCompatValues(t *testing.T) {
	pairs := map[string][2]string{
		"timer durations":  {`timer T := 1.0;`, `timer T := 2.0;`},
		"variable types":   {`var integer v_x;`, `var float v_x;`},
		"initial values":   {`var integer v_x := 1;`, `var integer v_x := 2;`},
		"one initialised":  {`var integer v_x := 1;`, `var integer v_x;`},
		"template bodies":  {`template integer t_x := 1;`, `template integer t_x := 2;`},
		"constant values":  {`const charstring c := "a";`, `const charstring c := "b";`},
		"record types":     {`var R1 v_r;`, `var R2 v_r;`},
		"negative numbers": {`var integer v_x := -1;`, `var integer v_x := 1;`},
	}
	for name, p := range pairs {
		src := `module M { type record R1 { integer a } type record R2 { integer a }
			type component CA { ` + p[0] + ` } type component CB { ` + p[1] + ` }
			function f() runs on CA {} testcase tc() runs on CB { f(); } }`
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src)); !containsCode(diags, "runs-on-incompatible") {
			t.Errorf("%s: expected runs-on-incompatible, got %v", name, codes(diags))
		}
	}
	good := map[string][2]string{
		"same values":       {`timer T := 1.0; var integer v := 1; template integer t := 1;`, `timer T := 1.00; var integer v := (1); template integer t := 1;`},
		"unknown values":    {`var integer v := c_1;`, `var integer v := 1;`},
		"same named type":   {`var R1 v_r;`, `var R1 v_r;`},
		"no initial values": {`var integer v; timer T;`, `var integer v; timer T;`},
	}
	for name, p := range good {
		src := `module M { type record R1 { integer a } const integer c_1 := 1;
			type component CA { ` + p[0] + ` } type component CB { ` + p[1] + ` }
			function f() runs on CA {} testcase tc() runs on CB { f(); } }`
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src)); containsCode(diags, "runs-on-incompatible") {
			t.Errorf("%s: unexpected runs-on-incompatible", name)
		}
	}
}
