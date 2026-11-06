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

func TestMapEqualityRejected(t *testing.T) {
	// NegSem_06021501_map_type_definition_003: comparing two
	// map-typed variables.
	tree := parse(t, `module M {
		type map from charstring to integer TMap1;
		type component C {}
		testcase tc() runs on C {
			var TMap1 v_map1 := { ["x"] := 1 }, v_map2 := { ["x"] := 1 };
			var boolean v_result := v_map1 == v_map2;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "map-equality-not-allowed") {
		t.Fatalf("expected map-equality-not-allowed, got %v", codes(diags))
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
