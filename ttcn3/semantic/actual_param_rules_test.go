package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestActualParamRangeOnOmitRestrictionRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_111: passing (0..10)
	// (a value range matcher) to a `omit integer` parameter.
	tree := parse(t, `module M {
		type component C {}
		function f_test (omit integer p_val) {}
		testcase tc() runs on C {
			f_test((0..10));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "actual-parameter-restriction-violation") {
		t.Fatalf("expected actual-parameter-restriction-violation, got %v", codes(diags))
	}
}

func TestActualParamValueListOnTemplateValueRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_112: (1, 2, 3) value
	// list on `template(value) integer`.
	tree := parse(t, `module M {
		type component C {}
		function f_test (template(value) integer p_val) {}
		testcase tc() runs on C {
			f_test((1, 2, 3));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "actual-parameter-restriction-violation") {
		t.Fatalf("expected actual-parameter-restriction-violation, got %v", codes(diags))
	}
}

func TestActualParamAnyOrNoneOnTemplatePresentRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_113: `*` on
	// `template(present) integer`.
	tree := parse(t, `module M {
		type component C {}
		function f_test (template(present) integer p_val) {}
		testcase tc() runs on C {
			f_test(*);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "actual-parameter-restriction-violation") {
		t.Fatalf("expected actual-parameter-restriction-violation, got %v", codes(diags))
	}
}

func TestActualParamPlainTemplateAccepts(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_test (template integer p_val) {}
		testcase tc() runs on C {
			f_test(*);
			f_test((0..10));
			f_test((1, 2, 3));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "actual-parameter-restriction-violation") {
		t.Fatalf("unexpected actual-parameter-restriction-violation on plain template, got %v", codes(diags))
	}
}

func TestLazyArgWithInoutSideEffectRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_121
	tree := parse(t, `module M {
		type component C {}
		function f_eval(inout integer p_val) return integer {
			p_val := p_val + 1;
			return p_val;
		}
		function f_test(@lazy integer p_val) {}
		testcase tc() runs on C {
			var integer v_val := 0;
			f_test(1 + f_eval(v_val));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "lazy-fuzzy-arg-side-effect") {
		t.Fatalf("expected lazy-fuzzy-arg-side-effect, got %v", codes(diags))
	}
}

func TestFuzzyArgWithOutSideEffectRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_124
	tree := parse(t, `module M {
		type component C {}
		function f_eval(out integer p_val) return integer {
			p_val := 1;
			return p_val;
		}
		function f_test(@fuzzy integer p_val) {}
		testcase tc() runs on C {
			var integer v_val;
			f_test(1 + f_eval(v_val));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "lazy-fuzzy-arg-side-effect") {
		t.Fatalf("expected lazy-fuzzy-arg-side-effect, got %v", codes(diags))
	}
}

func TestParameterizedTemplateWithoutArgsRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_114
	tree := parse(t, `module M {
		type record R { integer field1, integer field2 optional }
		type component C {}
		template R mw_rec(template integer p_field2) := {
			field1 := 1,
			field2 := p_field2
		}
		function f_test(template R p_match) {}
		testcase tc() runs on C {
			f_test(mw_rec);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "parameterized-template-without-args") {
		t.Fatalf("expected parameterized-template-without-args, got %v", codes(diags))
	}
}

func TestParameterizedTemplateWithArgsAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type record R { integer field1, integer field2 optional }
		type component C {}
		template R mw_rec(template integer p_field2) := {
			field1 := 1,
			field2 := p_field2
		}
		function f_test(template R p_match) {}
		testcase tc() runs on C {
			f_test(mw_rec(omit));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "parameterized-template-without-args") {
		t.Fatalf("unexpected diag on mw_rec(omit), got %v", codes(diags))
	}
}

func TestActualParamTemplateWildcardFieldReferenceRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		type record R {
			integer field1,
			record { integer subfield1, integer subfield2 } field2 optional
		}
		template R mw_rec := {
			field1 := 1,
			field2 := *
		}
		function f_test(in template integer p_val) {}
		testcase tc() runs on C {
			f_test(mw_rec.field2.subfield1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "actual-parameter-template-field-reference") {
		t.Fatalf("expected actual-parameter-template-field-reference, got %v", codes(diags))
	}
}

func TestActualParamTemplateValueListFieldReferenceRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		type record R {
			integer field1,
			record { integer subfield1, integer subfield2 } field2 optional
		}
		function f_test(out template integer p_val) {
			p_val := 10;
		}
		testcase tc() runs on C {
			var template R v_rec := {
				field1 := 1,
				field2 := ({ subfield1 := 0, subfield2 := 1}, { subfield1 := 2, subfield2 := 3 })
			};
			f_test(v_rec.field2.subfield1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "actual-parameter-template-field-reference") {
		t.Fatalf("expected actual-parameter-template-field-reference, got %v", codes(diags))
	}
}

func TestActualParamConcreteTemplateFieldReferenceAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		type record R {
			integer field1,
			record { integer subfield1, integer subfield2 } field2 optional
		}
		template R mw_rec := {
			field1 := 1,
			field2 := { subfield1 := 0, subfield2 := 1 }
		}
		function f_test(in template integer p_val) {}
		testcase tc() runs on C {
			f_test(mw_rec.field2.subfield1);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "actual-parameter-template-field-reference") {
		t.Fatalf("unexpected template-field-reference diag, got %v", codes(diags))
	}
}

func TestLazyVarToInoutRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_125
	tree := parse(t, `module M {
		type component C {}
		function f_test(inout integer p_val) {}
		testcase tc() runs on C {
			var @lazy integer v_val := 1;
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "lazy-fuzzy-var-to-out-inout") {
		t.Fatalf("expected lazy-fuzzy-var-to-out-inout, got %v", codes(diags))
	}
}

func TestFuzzyVarToOutRejected(t *testing.T) {
	// NegSem_050402_actual_parameters_128
	tree := parse(t, `module M {
		type component C {}
		function f_test(out integer p_val) {
			p_val := 5;
		}
		testcase tc() runs on C {
			var @fuzzy integer v_val := 1;
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "lazy-fuzzy-var-to-out-inout") {
		t.Fatalf("expected lazy-fuzzy-var-to-out-inout, got %v", codes(diags))
	}
}

func TestPlainVarToInoutAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_test(inout integer p_val) {}
		testcase tc() runs on C {
			var integer v_val := 1;
			f_test(v_val);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "lazy-fuzzy-var-to-out-inout") {
		t.Fatalf("unexpected diag on plain var, got %v", codes(diags))
	}
}

func TestLazyArgWithPureCallAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_pure(in integer p_val) return integer {
			return p_val + 1;
		}
		function f_test(@lazy integer p_val) {}
		testcase tc() runs on C {
			f_test(1 + f_pure(5));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "lazy-fuzzy-arg-side-effect") {
		t.Fatalf("unexpected lazy-fuzzy-arg-side-effect, got %v", codes(diags))
	}
}

func TestActualParamOmitOnOmitParamAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f_test (omit integer p_val) {}
		testcase tc() runs on C {
			f_test(omit);
			f_test(7);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "actual-parameter-restriction-violation") {
		t.Fatalf("unexpected violation on legal omit / value, got %v", codes(diags))
	}
}
