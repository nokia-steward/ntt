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

// In the context of an enumerated type an imported enumerated value
// takes precedence over a same-named definition in the importing module
// (ETSI 8.2.3.1, Sem_08020301_GeneralFormatOfImport_004): `enumX` means
// the enum member, not the local integer variable.
func TestEnumPrecedence_MemberBeatsLocalDefinition(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type enumerated EnumType { enumX, enumY, enumZ }
		const EnumType c_enumVal := enumY;
		testcase tc() runs on C {
			var integer enumY := 1;
			if (c_enumVal == enumY) { setverdict(pass); }
			else { setverdict(fail, enumY); }
		}
	}`)
}

// Precedence only applies to a name the enumerated type actually
// declares: an ordinary variable compared against an enum value keeps
// its own binding.
func TestEnumPrecedence_LeavesOrdinaryNamesAlone(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type enumerated EnumType { enumX, enumY, enumZ }
		testcase tc() runs on C {
			var EnumType v_other := enumZ;
			var EnumType v_same := enumX;
			if (v_same == enumX and not (v_other == enumX) and v_other == enumZ) {
				setverdict(pass);
			} else { setverdict(fail, v_other); }
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

// A value assigned whole replaces the target: what it leaves
// uninitialised is uninitialised there too (ETSI 19.1, EXAMPLE 3). Only a
// notation keeps what it does not mention.
func TestAssignment_WholeValueReplacesTarget(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f1, integer f2 }
		type record of integer RoI;
		testcase tc() runs on C {
			var R r := { f1 := 1, f2 := 2 };
			var R s;
			s.f1 := 11;
			r := s;
			var RoI l := { 1, 2, 3 };
			var RoI m := { 9, - };
			l := m;
			if (r.f1 == 11 and not isbound(r.f2) and lengthof(l) == 1 and not isbound(l[1])) { setverdict(pass); }
			else { setverdict(fail, r, l); }
		}
	}`)
}

// A notation assigned to a field or an element merges into what is there,
// whether the record is held by name or positionally (ETSI 6.2, 6.2.3.2).
func TestAssignmentNotation_MergesIntoFieldsAndElements(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f1, integer f2, integer f3 }
		type record Q { R r, integer g }
		type record of R RR;
		testcase tc() runs on C {
			var R a := { 1, 2, 3 };
			a := { f2 := 9 };
			var R b := { f1 := 1, f2 := 2, f3 := 3 };
			b := { 5, -, - };
			var Q q := { r := { 1, 2, 3 }, g := 4 };
			q.r := { f2 := 9 };
			var RR rr := { { 1, 2, 3 }, { 4, 5, 6 } };
			rr[1] := { -, 9, - };
			if (a == { 1, 9, 3 } and b == { 5, 2, 3 } and q == { { 1, 9, 3 }, 4 } and rr[1] == { 4, 9, 6 }) { setverdict(pass); }
			else { setverdict(fail, a, b, q, rr); }
		}
	}`)
}

// A notation assigned to a union chooses its alternative anew, whether the
// union is reached through a synonym or is anytype (ETSI 6.2.5.1); the
// alternative there before is not kept.
func TestAssignmentNotation_UnionThroughSynonymIsReplaced(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type union U { integer a, charstring b }
		type U U2;
		type record WA { U2 u }
		type record of U2 RU2;
		testcase tc() runs on C {
			var U2 u := { a := 1 };
			u := { b := "x" };
			var WA w := { u := { a := 1 } };
			w.u := { b := "x" };
			var RU2 l := { { a := 1 } };
			l[0] := { b := "x" };
			var record { anytype at } y := { at := { integer := 1 } };
			y.at := { charstring := "a" };
			if (ischosen(u.b) and not ischosen(u.a) and ischosen(w.u.b) and not ischosen(w.u.a) and
			    ischosen(l[0].b) and not ischosen(l[0].a) and
			    ischosen(y.at.charstring) and not ischosen(y.at.integer)) { setverdict(pass); }
			else { setverdict(fail, u, w, l, y); }
		}
	}`)
}
