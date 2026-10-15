package main

import (
	"os"
	"path/filepath"
	"testing"

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
