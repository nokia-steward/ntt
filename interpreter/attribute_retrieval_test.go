package interpreter_test

import "testing"

// Attributes retrieved (ETSI 27.8): a definition's own with clause over
// its type's; a field's through a value as through its type (27.1.2);
// display and optional as a universal charstring, empty when not
// defined; variant without a parameter leaves out those bound to an
// encoding.
func TestAttributeRetrieval(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of universal charstring RUC;
		type integer MyInt with { display "type"; encode "RuleInt" }
		type integer MyInt2 with { encode "Codec1"; variant "Rule0"; variant "Codec1"."Rule1" }
		type record MyRec { MyInt field1, integer field2 } with { encode (field1) "RuleDirect" }
		const MyInt c_int := 1 with { display "constant" }
		template MyRec m_r := { field1 := 1, field2 := 2 } with { optional "implicit omit" }
		testcase tc() runs on C {
			var MyRec v_rec;
			var RUC v_enc := v_rec.field1.encode;
			var RUC v_var := MyInt2.variant;
			if (c_int.display == "constant" and MyInt.display == "type" and
			    v_enc == { "RuleDirect" } and m_r.optional == "implicit omit" and
			    MyRec.display == "" and v_rec.field2.extension == {} and
			    v_var == { "Rule0" }) { setverdict(pass); }
			else { setverdict(fail, v_enc, v_var); }
		}
	}`)
}
