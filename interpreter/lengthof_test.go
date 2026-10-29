package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestLengthofFollowsTheDeclaredType: lengthof a record of or set of is
// the index of its last initialized element plus 1, and no less than the
// minimal length its type restricts it to; an array's is the fixed size
// of its type, initialized or not (ETSI ES 201 873-1 C.2.1).
func TestLengthofFollowsTheDeclaredType(t *testing.T) {
	decls := `type component C {}
		type record of integer RoI;
		type set of integer SoI;
		type record length(4) of integer R4;
		type record length(3..5) of integer R35;
		type RoI RoI3 length(3..infinity);
		type integer A4[4];
		type integer A13[1..3];
		const integer c_n := 3;
		type record Rf { record of integer f length(3) }
		type RoI RoIx;
		type record Rs { RoIx f length(2) }
		function f_RoI() return RoI { return {1, -} }`
	for name, c := range map[string]struct{ body, want string }{
		"trailing uninitialized": {`var RoI v := {1, 2, -, -};`, "2"},
		"inner uninitialized":    {`var RoI v := {-, 0, -, 2};`, "4"},
		"set of":                 {`var SoI v := {1, -};`, "1"},
		"element set beyond":     {`var RoI v; v[5] := 1;`, "6"},
		"fixed length":           {`var R4 v; v[0] := 1;`, "4"},
		"minimal length":         {`var R35 v := {1, 2, 3, -, -};`, "3"},
		"synonym's length":       {`var RoI3 v := {1, -};`, "3"},
		"array declared":         {`var integer v[4]; v[0] := 1;`, "4"},
		"array type":             {`var A4 v := {1, -, -, -};`, "4"},
		"array of a range":       {`var A13 v; v[1] := 1;`, "3"},
		"array sized by a const": {`var integer v[c_n]; v[0] := 1;`, "3"},
		"inner array":            {`var integer v[2][5]; v[0][0] := 1;`, "5"},
		"function result":        {`var RoI v := f_RoI();`, "1"},
		"all initialized":        {`var RoI v := {1, 2, 3};`, "3"},
		"field's length":         {`var Rf r := { f := {1, 2, -} };`, "3"},
		"synonym field's length": {`var Rs r := { f := {1} };`, "2"},
	} {
		check := map[string]string{"inner array": "v[0]", "field's length": "r.f", "synonym field's length": "r.f"}[name]
		if check == "" {
			check = "v"
		}
		src := "module M {\n" + decls + "\ntestcase tc() runs on C {\n" + c.body +
			"\nif (lengthof(" + check + ") == " + c.want + ") { setverdict(pass) } else { setverdict(fail, lengthof(" + check + ")) }\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v, want %s", name, k.name, v, reason, err, c.want)
			}
		}
	}
}

// TestLengthofOfAnImportedSynonym: a synonym's list type is the one where
// the synonym is declared, not one of the same name where it is used.
func TestLengthofOfAnImportedSynonym(t *testing.T) {
	a := `module A {
		type record of integer L; type L Lsyn;
		type integer Arr[2]; type Arr ArrSyn;
	}`
	b := `module B {
		import from A { type Lsyn, ArrSyn }
		type component C {}
		type record length(5) of integer L;
		type integer Arr[7];
		testcase tc() runs on C {
			var Lsyn v := {1}; var ArrSyn w; w[0] := 1;
			if (lengthof(v) == 1 and lengthof(w) == 2) { setverdict(pass) }
			else { setverdict(fail, lengthof(v), " ", lengthof(w)) }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, a), parse(t, b)}, "B.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}
