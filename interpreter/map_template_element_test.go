package interpreter_test

import "testing"

// An element of a map template holding AnyValue: AnyValueOrNone on the
// right; on the left the map is expanded, the element given and
// AnyElementsOrNone added (ETSI 15.6.6 c, f).
func TestMapTemplateElement_OfAnyValue(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type map from charstring to integer TMap;
		testcase tc() runs on C {
			var template TMap v_map := ?;
			var template integer v_int := v_map["test"];
			v_map["test"] := 2;
			var TMap v_ok := { ["test"] := 2, ["other"] := 7 };
			var TMap v_nok := { ["other"] := 2 };
			if (istemplatekind(v_int, "AnyValueOrNone") and istemplatekind(v_map, "AnyElementsOrNone") and
			    match(v_ok, v_map) and not match(v_nok, v_map)) { setverdict(pass); }
			else { setverdict(fail, v_map); }
		}
	}`)
}

// A field of an element is expanded in turn; omit expands without
// AnyElementsOrNone (15.6.6 a, c; 6.2.15.7).
func TestMapTemplateElement_NestedAndOmit(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer a, integer b }
		type map from charstring to R TMap;
		type map from charstring to integer IMap;
		testcase tc() runs on C {
			var template TMap v_map := { *, ["id"] := { a := 0, b := 0 } };
			var template IMap v_om := omit;
			v_map["k"].a := 1;
			v_om["a"] := 1;
			var TMap v_ok := { ["id"] := { a := 0, b := 0 }, ["k"] := { a := 1, b := 42 } };
			var TMap v_nok := { ["id"] := { a := 0, b := 0 }, ["k"] := { a := 2, b := 42 } };
			var IMap v_i := { ["a"] := 1 }, v_j := { ["a"] := 1, ["b"] := 2 };
			if (match(v_ok, v_map) and not match(v_nok, v_map) and match(v_i, v_om) and not match(v_j, v_om)) { setverdict(pass); }
			else { setverdict(fail, v_map, v_om); }
		}
	}`)
}

// An element of a map template holding AnyValueOrNone, on the right, is
// an error (15.6.6 d).
func TestMapTemplateElement_OfAnyValueOrNoneIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		type map from charstring to integer TMap;
		testcase tc() runs on C {
			var template TMap v_map := *;
			var template integer v_int;
			setverdict(pass);
			v_int := v_map["test"];
		}
	}`)
}
