package tpdmigrate_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/project/tpdmigrate"
	"github.com/nokia/ntt/project/internal/titan"
)

func TestConvert_SourcesAndImports(t *testing.T) {
	p := &titan.Project{
		Name:       "demo",
		Sources:    []string{"src/b.ttcn", "src/a.ttcn"},
		ImportDirs: []string{"inc"},
	}
	var buf bytes.Buffer
	if err := tpdmigrate.Convert(&buf, p); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"name: demo",
		"sources:",
		"- src/a.ttcn",
		"- src/b.ttcn",
		"imports:",
		"- inc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestConvert_ReferencedProjectsAsTodos(t *testing.T) {
	p := &titan.Project{
		Name: "demo",
		References: []titan.Reference{
			{Name: "common", Path: "/abs/path/common.tpd"},
		},
	}
	var buf bytes.Buffer
	if err := tpdmigrate.Convert(&buf, p); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# TODO: translate ReferencedProjects") {
		t.Errorf("missing TODO comment in:\n%s", out)
	}
	if !strings.Contains(out, "common") {
		t.Errorf("missing referenced project listing in:\n%s", out)
	}
}

func TestConvert_NilProject(t *testing.T) {
	if err := tpdmigrate.Convert(&bytes.Buffer{}, nil); err == nil {
		t.Error("expected error for nil project")
	}
}
