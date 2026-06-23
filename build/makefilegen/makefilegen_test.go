package makefilegen_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/build/makefilegen"
	"github.com/nokia/ntt/project"
)

func TestGenerate_Basic(t *testing.T) {
	cfg := &project.Config{
		Manifest: project.Manifest{
			Name:    "demo",
			Sources: []string{"a.ttcn3", "b.ttcn3"},
			Imports: []string{"include"},
		},
	}
	var buf bytes.Buffer
	if err := makefilegen.Generate(&buf, cfg, makefilegen.Options{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mk := buf.String()
	for _, want := range []string{
		"NAME    := demo",
		"a.ttcn3",
		"b.ttcn3",
		"INCLUDE_DIRS",
		"include",
		"$(NTT) check $(TTCN3_SOURCES)",
		"$(NTT) exec $(TTCN3_SOURCES)",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("missing %q in:\n%s", want, mk)
		}
	}
}

func TestGenerate_LegacyTargets(t *testing.T) {
	cfg := &project.Config{Manifest: project.Manifest{Name: "p"}}
	var buf bytes.Buffer
	if err := makefilegen.Generate(&buf, cfg, makefilegen.Options{LegacyTargets: true}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mk := buf.String()
	for _, want := range []string{"compile:", "run:"} {
		if !strings.Contains(mk, want) {
			t.Errorf("missing legacy target %q", want)
		}
	}
}

func TestGenerate_NilConfig(t *testing.T) {
	var buf bytes.Buffer
	if err := makefilegen.Generate(&buf, nil, makefilegen.Options{}); err == nil {
		t.Error("expected error for nil config")
	}
}
