package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestMapIndexKeyMismatchAssign(t *testing.T) {
	// NegSem_06021504_index_notation_001: octetstring literal
	// indexed into a charstring-keyed map.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 1 };
			v_map['152A'O] := 6;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-index-type-mismatch") {
		t.Fatalf("expected map-index-type-mismatch, got %v", codes(diags))
	}
}

func TestMapIndexKeyMismatchRead(t *testing.T) {
	// NegSem_06021504_index_notation_002: same but read-side
	// inside a var-decl initializer.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 1 };
			var integer v_val := v_map['152A'O];
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-index-type-mismatch") {
		t.Fatalf("expected map-index-type-mismatch on read, got %v", codes(diags))
	}
}

func TestMapValueMismatchAssign(t *testing.T) {
	// NegSem_06021504_index_notation_005: float literal
	// assigned to an integer-valued map.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 1 };
			v_map["xyz"] := 6.5;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-value-type-mismatch") {
		t.Fatalf("expected map-value-type-mismatch, got %v", codes(diags))
	}
}

func TestMapInitKeyMismatch(t *testing.T) {
	// NegSem_06021502_indexed_assignment_notation_001: octet
	// keys in a charstring-keyed map initializer.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ['AB04'O] := 1, ['C0'O] := 5 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-index-type-mismatch") {
		t.Fatalf("expected map-index-type-mismatch in initializer, got %v", codes(diags))
	}
}

func TestMapInitValueMismatch(t *testing.T) {
	// NegSem_06021502_indexed_assignment_notation_002: float /
	// bool values in an integer-valued map initializer.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 2.5, ["xyz"] := true };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-value-type-mismatch") {
		t.Fatalf("expected map-value-type-mismatch in initializer, got %v", codes(diags))
	}
}

func TestMapCompatibleAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 1 };
			v_map["xyz"] := 6;
			var integer v_val := v_map["test"];
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		if d.Code == "map-index-type-mismatch" || d.Code == "map-value-type-mismatch" {
			t.Fatalf("did not expect map mismatch on compatible types, got %v", codes(diags))
		}
	}
}

// Map values may be compared: restriction b of ETSI ES 201 873-1
// 6.2.15.1, which forbade it, is void as of V4.17.1 (ETSI
// NegSem_06021501_map_type_definition_003 still expects it).
func TestMapEqualityAllowed(t *testing.T) {
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map1 := { ["x"] := 1 }, v_map2 := { ["x"] := 1 };
			var boolean v_result := v_map1 == v_map2;
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", codes(diags))
	}
}

func TestUnmapKeyMismatchRejected(t *testing.T) {
	// NegSem_06021503_unmapping_keys_002: unmap(map, integer)
	// where keys are charstring.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["x"] := 1 };
			unmap(v_map, 1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-index-type-mismatch") {
		t.Fatalf("expected map-index-type-mismatch on unmap, got %v", codes(diags))
	}
}

func TestMapIntegerToFloatWidening(t *testing.T) {
	// Integer-literal assignment to a float-valued map should
	// pass under the only widening the spec allows.
	tree := parse(t, `module M {
		type map from charstring to float TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map := { ["test"] := 1.0 };
			v_map["xyz"] := 6;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		if d.Code == "map-value-type-mismatch" {
			t.Fatalf("did not expect map-value-type-mismatch on int->float widening, got %v", codes(diags))
		}
	}
}

// A map type may be a template's type, and `m[*] := b` takes a boolean
// (ETSI ES 201 873-1 6.2.15.1, void restriction a; 6.2.15.4); a
// charstring is a universal charstring key (6.3.1).
func TestMapTemplatesAllowed(t *testing.T) {
	tree := parse(t, `module M {
		type map from charstring to charstring TMap;
		type map from universal charstring to integer TU;
		type record R { TMap m }
		type component C {}
		template TMap t := {*, ["a"] := "b"};
		template R r := {m := {["a"] := ?}};
		testcase tc() runs on C {
			var template TMap v := {["a"] := "b"};
			v[*] := true;
			var boolean b := v[*];
			var TU u := {["x"] := 1};
			var integer i := u["x"];
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", codes(diags))
	}
}

// `m[*]` is a boolean: assigned another value, or read into a variable of
// another type, it is an error (ETSI 6.2.15.4).
func TestMapAnyElementsMisuse(t *testing.T) {
	for name, body := range map[string]string{
		"a charstring assigned": `var template TMap v := {["a"] := "b"}; v[*] := "x";`,
		"read as a charstring":  `var template TMap v := {["a"] := "b"}; var charstring s := v[*];`,
	} {
		tree := parse(t, `module M {
			type map from charstring to charstring TMap;
			type component C {}
			testcase tc() runs on C { `+body+` }
		}`)
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, "map-value-type-mismatch") {
			t.Errorf("%s: expected map-value-type-mismatch, got %v", name, codes(diags))
		}
	}
}

// A map template's notation gives `*` first and keys that are no
// matching symbols, and unmap removes AnyElementsOrNone with `*` only
// (ETSI 6.2.15.2, 6.2.15.9, 6.2.15.3).
func TestMapNotationRules(t *testing.T) {
	for name, c := range map[string]struct{ src, code string }{
		"* not first":         {`template TMap t := {["a"] := "b", *};`, "map-notation"},
		"a value by position": {`template TMap t := {"x", ["a"] := "b"};`, "map-notation"},
		"a matching key":      {`template TMap t := {[?] := "b"};`, "map-index-type-mismatch"},
		"unmap of a ? key": {`type component C {}
			testcase tc() runs on C { var template TMap v := {*, ["a"] := "b"}; unmap(v, ?); }`, "map-index-type-mismatch"},
	} {
		tree := parse(t, "module M {\ntype map from charstring to charstring TMap;\n"+c.src+"\n}")
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", name, c.code, codes(diags))
		}
	}
	tree := parse(t, `module M {
		type map from charstring to charstring TMap;
		type record of integer RI;
		template TMap t := {*, ["a"] := "b"};
		template RI r := {1, [2] := 3, *};
		type component C {}
		testcase tc() runs on C { var template TMap v := t; unmap(v, *); unmap(v, "a"); }
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		t.Errorf("unexpected %s: %s", d.Code, d.Message)
	}
}
