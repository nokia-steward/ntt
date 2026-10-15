package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestReplyWithNotUsedOutViaNamedTemplateRejected covers the `Sig:tmplRef`
// COLON form: a call-safe template (out := `-`) referenced by name in a
// reply must be rejected — reply requires out/inout to be specific (ETSI
// 22.3.3 / 15.3 restriction e). Mirrors NegSem_1503_GlobalAndLocalTemplates_007.
func TestReplyWithNotUsedOutViaNamedTemplateRejected(t *testing.T) {
	tree := parse(t, `module M {
		signature S(in integer p_in, inout integer p_inout, out integer p_out);
		type port P procedure { inout S }
		type component C { port P pco }
		template S s_callSafe := { p_in := 1, p_inout := 2, p_out := - };
		function f() runs on C {
			pco.reply(S:s_callSafe);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "signature-template-matching-not-allowed") {
		t.Fatalf("expected signature-template-matching-not-allowed on reply(out:=-), got %v", codes(diags))
	}
}

// TestCallWithSetOutNotFlagged is the poisoned-well guard: a call template
// that sets its `out` param to a concrete value must NOT be flagged. ETSI
// 15.3 would forbid it, but the conformance suite contradicts itself
// (Sem_220304_getreply_operation_006 uses exactly this and expects accept),
// so a static rule must not reject it — otherwise it regresses that fixture
// and Syn_1502_DeclaringSignatureTemplates_003.
func TestCallWithSetOutNotFlagged(t *testing.T) {
	tree := parse(t, `module M {
		signature S(in integer p_in, out integer p_out, inout integer p_inout) return integer;
		type port P procedure { inout S }
		type component C { port P pco }
		template S m_all := { p_in := 1, p_out := 2, p_inout := 3 };
		function f() runs on C {
			pco.call(S:m_all, nowait);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "signature-template-matching-not-allowed") ||
		containsCode(diags, "signature-template-wrong-operation") {
		t.Fatalf("did not expect a signature-template diagnostic on call(out:=value), got %v", codes(diags))
	}
}

// TestReplyWithInSetNotFlagged guards the other poisoned-well direction: a
// reply template that sets an `in` param must NOT be flagged (reply requires
// only out/inout specific; Sem_220303_ReplyOperation_005 sets an in param in
// a reply and expects accept).
func TestReplyWithInSetNotFlagged(t *testing.T) {
	tree := parse(t, `module M {
		signature S(in integer par1);
		type port P procedure { inout S }
		type component C { port P pco }
		function f() runs on C {
			pco.reply(S:{ par1 := 7 });
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "signature-template-matching-not-allowed") ||
		containsCode(diags, "signature-template-wrong-operation") {
		t.Fatalf("did not expect a signature-template diagnostic on reply(in:=value), got %v", codes(diags))
	}
}
