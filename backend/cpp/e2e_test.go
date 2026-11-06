package cpp_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/backend/cpp"
	"github.com/nokia/ntt/ir/lower"
	"github.com/nokia/ntt/ttcn3"
)

// repoRoot walks up from the working directory until it finds the
// repository's go.mod. The C++ build needs an absolute path to the
// runtime/cpp/include directory and the test is run from inside the
// package under test.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// TestE2E_TTCN3_To_Cpp_To_BinaryMatchesExpected is the closest
// thing to a real Titan-style "compile and run" we have on the C++
// side: parse TTCN-3 source, lower to IR, emit C++, compile against
// the real runtime/cpp header (no stubs), run the binary, and
// compare its printed verdict against the expected outcome. This is
// the existence proof that backend/cpp + runtime/cpp form a usable
// pair, not just two pieces of scaffolding.
func TestE2E_TTCN3_To_Cpp_To_BinaryMatchesExpected(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("no g++ on PATH")
	}
	root, err := repoRoot()
	if err != nil {
		t.Skipf("cannot locate repository root: %v", err)
	}
	include := filepath.Join(root, "runtime/cpp/include")

	cases := []struct {
		name string
		src  string
		want string // expected stdout, full line including verdict label
	}{
		{
			name: "pass",
			src: `module M {
                testcase tc() runs on C { setverdict(pass); }
            }`,
			want: "verdict=pass",
		},
		{
			name: "fail",
			src: `module M {
                testcase tc() runs on C { setverdict(fail); }
            }`,
			want: "verdict=fail",
		},
		{
			name: "if-pass-branch",
			src: `module M {
                testcase tc() runs on C {
                    if (1 == 1) { setverdict(pass); } else { setverdict(fail); }
                }
            }`,
			want: "verdict=pass",
		},
		{
			name: "if-fail-branch",
			src: `module M {
                testcase tc() runs on C {
                    if (1 == 2) { setverdict(pass); } else { setverdict(fail); }
                }
            }`,
			want: "verdict=fail",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := ttcn3.Parse(tc.src)
			if tree == nil || tree.Err != nil {
				t.Fatalf("parse: %v", tree.Err)
			}
			m, _ := lower.Module(tree)
			if m == nil {
				t.Fatalf("lower returned nil")
			}

			dir := t.TempDir()
			var buf bytes.Buffer
			if err := cpp.Generate(&buf, m); err != nil {
				t.Fatalf("Generate: %v", err)
			}

			// Wrap the generated translation unit with a tiny
			// main() that drives the auto-generated run_suite()
			// and prints the first case's verdict.
			main := fmt.Sprintf(`%s
#include <iostream>

int main() {
    auto cases = ntt_tests::run_suite();
    if (cases.empty()) {
        std::cerr << "no cases\n";
        return 1;
    }
    std::cout << "verdict=" << ntt::to_string(cases.front().verdict) << std::endl;
    return 0;
}
`, buf.String())

			src := filepath.Join(dir, "test.cpp")
			if err := os.WriteFile(src, []byte(main), 0o644); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "test")
			out, err := exec.Command("g++", "-std=c++17", "-I", include, "-o", binary, src).CombinedOutput()
			if err != nil {
				t.Fatalf("g++ failed: %v\n%s\n---\n%s", err, out, main)
			}
			runOut, err := exec.Command(binary).Output()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, runOut)
			}
			got := strings.TrimSpace(string(runOut))
			if got != tc.want {
				t.Errorf("verdict mismatch: got %q, want %q\nC++ source:\n%s", got, tc.want, main)
			}
		})
	}
}
