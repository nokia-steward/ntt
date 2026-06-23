package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func TestModuleParamOverrideCharstring(t *testing.T) {
	src := `module M {
		modulepar charstring PX_HOST := "default";
		testcase tc_check() runs on C {
			if (PX_HOST == "OVERRIDDEN") {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse: %v", tree.Err)
	}
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{
				"M.PX_HOST": `"OVERRIDDEN"`,
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (override should win)", v)
	}
}

func TestModuleParamOverrideInteger(t *testing.T) {
	src := `module M {
		modulepar integer PX_PORT := 80;
		testcase tc_check() runs on C {
			if (PX_PORT == 8443) {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse: %v", tree.Err)
	}
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{
				"M.PX_PORT": "8443",
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (integer override should win)", v)
	}
}

func TestModuleParamOverrideBoolean(t *testing.T) {
	src := `module M {
		modulepar boolean PX_ON := false;
		testcase tc_check() runs on C {
			if (PX_ON) {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse: %v", tree.Err)
	}
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{"M.PX_ON": "true"},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (boolean override should win)", v)
	}
}

func TestModuleParamOverrideWithTrailingSemicolon(t *testing.T) {
	src := `module M {
		modulepar charstring PX_TOKEN := "";
		testcase tc_check() runs on C {
			if (PX_TOKEN == "abc") {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{"M.PX_TOKEN": `"abc";`},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (trailing ; tolerated)", v)
	}
}

func TestModuleParamUnknownKeyWarns(t *testing.T) {
	src := `module M {
		modulepar charstring PX_HOST := "x";
		testcase tc_check() runs on C { setverdict(pass); }
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	var warnings []string
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{"M.PX_NOPE": `"y"`},
			ModuleParamWarning: func(msg string) {
				warnings = append(warnings, msg)
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (unknown override is non-fatal)", v)
	}
	if len(warnings) == 0 {
		t.Errorf("expected at least one warning about PX_NOPE")
	}
}

func TestModuleParamBareNameAccepted(t *testing.T) {
	src := `module M {
		modulepar charstring PX_NODE := "default";
		testcase tc_check() runs on C {
			if (PX_NODE == "OVERRIDDEN") {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
		type component C {}
	}`
	tree := ttcn3.Parse(src)
	v, _, err := interpreter.RunTestcaseWith(
		[]*ttcn3.Tree{tree},
		"M.tc_check",
		interpreter.TestcaseOptions{
			ModuleParameters: map[string]string{"PX_NODE": `"OVERRIDDEN"`},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (bare name should be accepted when unique)", v)
	}
}
