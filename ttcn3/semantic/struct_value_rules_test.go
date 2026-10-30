package semantic

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestUnionValueListNotationRejected(t *testing.T) {
	// ETSI 6.2.5: union value notation must be
	// `{ alt := value }` with exactly one alternative.
	// `{ 5 }` is illegal because the runtime cannot pick
	// which alternative the value binds to (NegSem_0602
	// _TopLevel_001).
	tree := parse(t, `module M {
		type union U { integer a, charstring b }
		type component C {}
		testcase tc() runs on C {
			var U v := { 5 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-value-notation") {
		t.Fatalf("expected union-value-notation diagnostic, got %v", codes(diags))
	}
}

func TestUnionTwoAlternativesRejected(t *testing.T) {
	tree := parse(t, `module M {
		type union U { integer a, charstring b }
		type component C {}
		testcase tc() runs on C {
			var U v := { a := 1, b := "x" };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-value-notation") {
		t.Fatalf("expected union-value-notation diagnostic for two-alt union, got %v", codes(diags))
	}
}

func TestUnionSingleAlternativeAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type union U { integer a, charstring b }
		type component C {}
		testcase tc() runs on C {
			var U v := { a := 1 };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "union-value-notation") {
		t.Fatalf("did not expect union-value-notation diagnostic, got %v", codes(diags))
	}
}

func TestIndexAssignOnRecordRejected(t *testing.T) {
	// `v[0] := 99` on a record-typed variable is illegal per
	// 6.2 because records have no element index (NegSem_0602
	// _TopLevel_002).
	tree := parse(t, `module M {
		type record R { integer a, integer b }
		type component C {}
		testcase tc() runs on C {
			var R v := { a := 1, b := 2 };
			v[0] := 99;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "struct-index-not-allowed") {
		t.Fatalf("expected struct-index-not-allowed for record, got %v", codes(diags))
	}
}

func TestIndexAssignOnSetRejected(t *testing.T) {
	tree := parse(t, `module M {
		type set S { integer a, integer b }
		type component C {}
		testcase tc() runs on C {
			var S v := { a := 1, b := 2 };
			v[0] := 99;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "struct-index-not-allowed") {
		t.Fatalf("expected struct-index-not-allowed for set, got %v", codes(diags))
	}
}

func TestIndexAssignOnUnionRejected(t *testing.T) {
	tree := parse(t, `module M {
		type union U { integer a, charstring b }
		type component C {}
		testcase tc() runs on C {
			var U v := { a := 1 };
			v[0] := 99;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "struct-index-not-allowed") {
		t.Fatalf("expected struct-index-not-allowed for union, got %v", codes(diags))
	}
}

func TestIndexOnRecordOfArrayAccepted(t *testing.T) {
	// `record of` and `array` types remain indexable; the
	// rule must not flag those.
	tree := parse(t, `module M {
		type record of integer Ints;
		type component C {}
		testcase tc() runs on C {
			var Ints xs := { 1, 2, 3 };
			xs[0] := 99;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		if d.Code == "struct-index-not-allowed" {
			t.Fatalf("false positive on record-of indexed access: %s", d.Message)
		}
	}
}

func TestUnionAlternativeUnknownInExtendedRef(t *testing.T) {
	// ETSI 6.2.5.1: `type U.option3 T;` is illegal when option3
	// is not declared as an alternative of U.
	tree := parse(t, `module M {
		type union U { integer option1, charstring option2 }
		type U.option3 BadAlias;
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-unknown-alternative") {
		t.Fatalf("expected union-unknown-alternative, got %v", codes(diags))
	}
}

func TestUnionAlternativeKnownInExtendedRefAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type union U { integer option1, charstring option2 }
		type U.option1 GoodAlias;
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "union-unknown-alternative") {
		t.Fatalf("did not expect union-unknown-alternative, got %v", codes(diags))
	}
}

func TestRecordFieldSelfReferenceFlagged(t *testing.T) {
	// ETSI 6.2.1.1 mirror of the union self-ref rule for
	// records: a field whose type is `R.otherField` referring
	// back into R is illegal because no fixed size exists.
	tree := parse(t, `module M {
		type record R {
			integer field1,
			R.field2 field2 optional,
			boolean field3
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "struct-self-referencing-field") {
		t.Fatalf("expected struct-self-referencing-field, got %v", codes(diags))
	}
}

func TestSetFieldSelfReferenceFlagged(t *testing.T) {
	tree := parse(t, `module M {
		type set S {
			integer fa,
			S.fb fb optional
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "struct-self-referencing-field") {
		t.Fatalf("expected struct-self-referencing-field on set body, got %v", codes(diags))
	}
}

func TestUnionAlternativeSelfReferenceFlagged(t *testing.T) {
	// A union whose own alternative is declared as `U.otherAlt`
	// is a self-cycle; the runtime cannot allocate a fixed size
	// for the type. ETSI 6.2.5.1.
	tree := parse(t, `module M {
		type union U {
			integer option1,
			U.option2 option2
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "union-self-referencing-alternative") {
		t.Fatalf("expected union-self-referencing-alternative, got %v", codes(diags))
	}
}

func TestMixedNotationOnRecordAccepted(t *testing.T) {
	// Sem_0602_TopLevel_020: mixing positional + named entries
	// is explicitly allowed for record/set; the rule must not
	// flag it.
	tree := parse(t, `module M {
		type record R { integer a, charstring b, float c }
		type component C {}
		testcase tc() runs on C {
			var R v := { 5, c := 3.14, b := "x" };
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		if strings.HasPrefix(d.Code, "struct-value") || d.Code == "union-value-notation" {
			t.Fatalf("false positive on mixed record notation: %s", d.Message)
		}
	}
}
