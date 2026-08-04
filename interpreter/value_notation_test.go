package interpreter_test

import "testing"

// Assignment notation over named fields leaves an unmentioned field
// with the value it already held (ETSI 6.2, Sem_0602_TopLevel_015).
func TestAssignmentNotation_KeepsUnmentionedFields(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer field1, charstring field2 optional, float field3 }
		testcase tc() runs on C {
			var R v := { field1 := 5, field2 := "hi", field3 := 3.14 };
			v := { field1 := 3, field3 := 2.0 };
			if (match(v, {3, "hi", 2.0})) { setverdict(pass); }
			else { setverdict(fail, v); }
		}
	}`)
}

// A union carries exactly one alternative, so re-assigning it selects a
// new alternative instead of merging into the old one.
func TestAssignmentNotation_UnionReplacesAlternative(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type union U { integer u1, float u2 }
		testcase tc() runs on C {
			var U v := { u1 := 1 };
			v := { u2 := 2.0 };
			if (ischosen(v.u2) and not ischosen(v.u1)) { setverdict(pass); }
			else { setverdict(fail, v); }
		}
	}`)
}

// Referencing an alternative of a union template assigned AnyValue
// returns AnyValue (ETSI 15.6.5 restriction b,
// Sem_150605_Referencing_union_alternatives_002).
func TestTemplateReference_AnyValueUnionAlternative(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type union U { integer u1, float u2 }
		type record R { integer a, U b optional }
		testcase tc() runs on C {
			var template R m;
			m.a := 10;
			m.b := ?;
			var template integer m2 := m.b.u1;
			if (ispresent(m2)) { setverdict(pass); }
			else { setverdict(fail, m2); }
		}
	}`)
}

// An OPTIONAL field of a record template assigned AnyValue admits
// absence, so referencing it yields `*` rather than `?`
// (Sem_150602_ReferencingRecordAndSetFields_003).
func TestTemplateReference_AnyValueOptionalFieldIsAnyOrNone(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer g1, integer g2 optional }
		testcase tc() runs on C {
			var template R m := ?;
			var R v := { g1 := 5, g2 := omit };
			if (match(v, R:{m.g1, m.g2})) { setverdict(pass); }
			else { setverdict(fail, v); }
		}
	}`)
}

// ischosen answers about the alternative the union carries, not about
// the value the reference yields: `{f2 := ?}` has f2 chosen even though
// its value is a wildcard, while a union template that is itself `?`
// has chosen nothing (ETSI 16.1.2,
// Sem_160102_predefined_functions_022).
func TestIschosen_WildcardTemplateChoosesNothing(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type union U { integer f1, octetstring f2 }
		testcase tc() runs on C {
			template U t_any := ?;
			template U t_f2 := { f2 := ? };
			if (not ischosen(t_any.f1) and ischosen(t_f2.f2) and not ischosen(t_f2.f1)) {
				setverdict(pass);
			} else { setverdict(fail); }
		}
	}`)
}

// Concatenating a fixed-length wildcard onto a binary string
// contributes that many unknown units, so `'ABCD'O & ? length(2)`
// spans four octets (ETSI 15.11,
// Sem_1511_ConcatenatingTemplatesOfStringAndListTypes_013).
func TestConcat_FixedLengthWildcardSpansItsUnits(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template octetstring v := ('ABCD'O & ? length(2)) length (1..6);
			if (match('ABCD1234'O, v) and not match('ABCD12'O, v)) { setverdict(pass); }
			else { setverdict(fail, v); }
		}
	}`)
}
