package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
)

func writeFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestCheckOne_PassAndFail(t *testing.T) {
	dir := t.TempDir()
	good := writeFile(t, dir, "good.ttcn3", `module M { type integer T }`)
	bad := writeFile(t, dir, "bad.ttcn3", `module M {
		function f() return integer { return; }
	}`)

	db := &ttcn3.DB{}
	db.Index(good, bad)
	r := checkOne(good, db)
	if !r.Passed {
		t.Errorf("good.ttcn3 should pass, got errors %v", r.Errors)
	}
	r = checkOne(bad, db)
	if r.Passed {
		t.Errorf("bad.ttcn3 should fail (return-missing-value)")
	}
}

func TestCollectTTCN3Files(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.ttcn3", `module A {}`)
	writeFile(t, dir, "b.ttcn", `module B {}`)
	writeFile(t, dir, "c.txt", "not ttcn3")
	hidden := filepath.Join(dir, ".hidden")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, hidden, "skip.ttcn3", `module S {}`)

	files := collectTTCN3Files([]string{dir})
	if got, want := len(files), 2; got != want {
		t.Fatalf("got %d files, want %d (%v)", got, want, files)
	}
}

func TestCheckFiles_Summary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.ttcn3", `module G { type integer T }`)
	writeFile(t, dir, "bad.ttcn3", `module B { function f() return integer { return; } }`)

	files := collectTTCN3Files([]string{dir})
	db := &ttcn3.DB{}
	db.Index(files...)
	summary := checkFiles(files, db)
	if summary.Total != 2 {
		t.Fatalf("Total = %d, want 2", summary.Total)
	}
	if summary.Passed != 1 || summary.Failed != 1 {
		t.Fatalf("Passed=%d Failed=%d, want 1/1", summary.Passed, summary.Failed)
	}
	if summary.PassRate <= 49 || summary.PassRate >= 51 {
		t.Fatalf("PassRate = %.2f, want ~50%%", summary.PassRate)
	}
}

func TestCheckOne_Positions(t *testing.T) {
	dir := t.TempDir()
	bad := writeFile(t, dir, "bad.ttcn3", `module M {
  type record of integer RI;
  function f() {
    const integer k := 1;
    k := 2;
    var RI l := {1};
    var integer n := sizeof(l);
  }
}`)
	syntaxErr := writeFile(t, dir, "syntax.ttcn3", `module S {
  function f() { var integer x := ; var integer y := +; }
}`)

	db := &ttcn3.DB{}
	db.Index(bad, syntaxErr)

	r := checkOne(bad, db)
	if want := bad + `:5:5-5:11: const-reassign: cannot reassign constant "k" (ETSI 10)`; len(r.Errors) != 1 || r.Errors[0] != want {
		t.Errorf("errors = %q, want [%q]", r.Errors, want)
	}
	if r.Warnings != 1 || len(r.WarningMessages) != 1 ||
		!strings.HasPrefix(r.WarningMessages[0], bad+":7:22-7:31: sizeof-on-list-deprecated: ") {
		t.Errorf("warnings = %d %q, want one sizeof-on-list-deprecated at 7:22", r.Warnings, r.WarningMessages)
	}

	// One line per syntax error, each with its position.
	r = checkOne(syntaxErr, db)
	want := []string{
		syntaxErr + ":2:35-2:36: expected operand, found ;",
		syntaxErr + ":2:55-2:56: expected operand, found ;",
	}
	if !slices.Equal(r.Errors, want) {
		t.Errorf("errors = %q, want %q", r.Errors, want)
	}
}

func TestCheckInputs_Project(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"src", "lib"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A file named twice, a missing directory and a non-TTCN-3 file.
	writeFile(t, dir, "package.yml", "sources: [src, src/A.ttcn3, src/X.asn]\nimports: [lib, nolib]\n")
	writeFile(t, dir, "src/X.asn", "X DEFINITIONS ::= BEGIN END")
	writeFile(t, dir, "src/A.ttcn3", `module A { import from L all; const RL c := {1}; }`)
	writeFile(t, dir, "lib/L.ttcn3", `module L { type record of integer RL; }`)
	writeFile(t, dir, "lib/Broken.ttcn3", `module Broken { const integer k := 1; function f() { k := 2; } }`)
	t.Chdir(dir)

	// The manifest alone: no NTT_* (or legacy) environment overrides.
	p, err := project.NewConfig(project.AutomaticRoot(dir))
	if err != nil {
		t.Fatal(err)
	}
	files, index := checkInputs(nil, p)
	// The sources are checked, relative to the current directory; the
	// imports only resolve names.
	if want := []string{filepath.Join("src", "A.ttcn3")}; !slices.Equal(files, want) {
		t.Errorf("files = %q, want %q", files, want)
	}
	if len(index) != 3 {
		t.Errorf("index = %q, want the 3 files of sources and imports", index)
	}
	db := &ttcn3.DB{}
	db.Index(index...)
	if s := checkFiles(files, db); s.Total != 1 || s.Passed != 1 {
		t.Errorf("summary = %+v, want the one source passing", s)
	}

	// Paths on the command line are checked as given, and resolve only
	// against each other.
	files, index = checkInputs([]string{"src"}, p)
	if !slices.Equal(files, index) || len(files) != 1 {
		t.Errorf("files = %q, index = %q, want the one file in src for both", files, index)
	}
}
