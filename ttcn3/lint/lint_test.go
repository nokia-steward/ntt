package lint

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree.Err != nil {
		t.Fatalf("parse error: %v\nsource:\n%s", tree.Err, src)
	}
	return tree
}

func TestUnusedImport(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantCode string
	}{
		{
			name: "unused all import",
			src: `module M {
				import from Other all;
			}`,
			wantCode: "unused-import",
		},
		{
			name: "used all import",
			src: `module M {
				import from Other all;
				function f() { Other.foo(); }
			}`,
		},
		{
			name: "used kind import",
			src: `module M {
				import from Other { type Bar };
				function f() { var Bar b; }
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, tt.src)
			problems := NewLinter(&UnusedImportRule{}).Lint(tree)
			switch {
			case tt.wantCode == "" && len(problems) > 0:
				t.Fatalf("expected no problems, got %v", problems)
			case tt.wantCode != "" && len(problems) == 0:
				t.Fatalf("expected problem with code %q, got none", tt.wantCode)
			case tt.wantCode != "" && problems[0].Code != tt.wantCode:
				t.Fatalf("got code %q, want %q", problems[0].Code, tt.wantCode)
			}
			if tt.wantCode != "" {
				if fix := problems[0].Fix; fix == nil {
					t.Fatalf("expected an autofix, got none")
				} else if fix.End <= fix.Begin {
					t.Fatalf("autofix has empty range [%d, %d)", fix.Begin, fix.End)
				}
			}
		})
	}
}

func TestEmptyBlock(t *testing.T) {
	src := `module M {
		function f() { }
		function g() { log("hi"); }
	}`
	tree := parse(t, src)
	problems := NewLinter(&EmptyBlockRule{}).Lint(tree)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem, got %d: %v", len(problems), problems)
	}
	if problems[0].Code != "empty-block" {
		t.Fatalf("got code %q, want %q", problems[0].Code, "empty-block")
	}
}

func TestDefaultLinter_NoPanicOnEmpty(t *testing.T) {
	tree := parse(t, "module M { }")
	got := DefaultLinter().Lint(tree)
	for _, p := range got {
		if strings.TrimSpace(p.Message) == "" {
			t.Errorf("problem with empty message: %+v", p)
		}
	}
}

func TestLint_NilTreeIsSafe(t *testing.T) {
	if got := DefaultLinter().Lint(nil); got != nil {
		t.Fatalf("expected nil for nil tree, got %v", got)
	}
}
