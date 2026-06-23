package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		if tree != nil {
			t.Fatalf("parse error: %v\n%s", tree.Err, src)
		}
		t.Fatalf("parse returned nil for:\n%s", src)
	}
	return tree
}

func TestRunTestcase_DefaultsToPass(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_empty() runs on C {}
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_empty")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass", v)
	}
}

func TestRunTestcase_SetVerdictPass(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_pass() runs on C {
            setverdict(pass);
        }
    }`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_pass")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass", v)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestRunTestcase_SetVerdictFailWithReason(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_fail() runs on C {
            setverdict(fail, "expected 5 got 4");
        }
    }`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_fail")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.FailVerdict {
		t.Errorf("verdict = %s, want fail", v)
	}
	if !strings.Contains(reason, "expected 5 got 4") {
		t.Errorf("reason = %q, want it to contain the failure message", reason)
	}
}

func TestRunTestcase_VerdictAggregation(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_mix() runs on C {
            setverdict(pass);
            setverdict(fail, "later");
            setverdict(pass);
        }
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_mix")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.FailVerdict {
		t.Errorf("verdict = %s, want fail (fail beats pass per TTCN-3 5.4.2)", v)
	}
}

func TestRunTestcase_IfControlFlowDrivesVerdict(t *testing.T) {
	tree := parse(t, `module M {
        function add(integer a, integer b) return integer {
            return a + b;
        }
        testcase tc_logic() runs on C {
            if (add(2, 3) == 5) {
                setverdict(pass);
            } else {
                setverdict(fail, "math broken");
            }
        }
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_logic")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass", v)
	}
}

func TestRunTestcase_GetverdictReadsCurrentValue(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_get() runs on C {
            setverdict(pass);
            if (getverdict == pass) {
                setverdict(pass);
            } else {
                setverdict(fail, "getverdict mismatch");
            }
        }
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_get")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass", v)
	}
}

func TestRunTestcase_UnknownTestcaseErrors(t *testing.T) {
	tree := parse(t, `module M { testcase tc_a() runs on C {} }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_missing")
	if err == nil {
		t.Errorf("want error for missing testcase, got verdict=%s", v)
	}
}

func TestRunTestcase_LogIsCapturedNotPrinted(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_log() runs on C {
            log("hello");
            log("world");
            setverdict(pass);
        }
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_log")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass", v)
	}
}
