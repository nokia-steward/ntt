package main

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/nokia/ntt/project"
)

// TestListModulePars: `ntt list modulepars` lists a module's
// parameters, also those of a modulepar group. It listed none, and a
// group's made it panic.
func TestListModulePars(t *testing.T) {
	file := writeFile(t, t.TempDir(), "m.ttcn3", `module M {
		modulepar integer a := 1, b := 2;
		modulepar { charstring g1 := "x"; integer g2 }
		const integer c := 3;
	}`)
	saved, savedW := Project, w
	defer func() { Project, w = saved, savedW }()
	Project = &project.Config{}
	Project.Sources = []string{file}
	var buf bytes.Buffer
	w = bufio.NewWriter(&buf)
	for _, c := range ListCommand.Commands() {
		if c.Use != "modulepars" {
			continue
		}
		if err := c.ParseFlags(nil); err != nil {
			t.Fatal(err)
		}
		if err := c.RunE(c, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := buf.String(), "M.a\nM.b\nM.g1\nM.g2\n"; got != want {
		t.Errorf("listed %q, want %q", got, want)
	}
}
