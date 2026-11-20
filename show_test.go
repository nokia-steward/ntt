package main

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/project"
)

func TestShowGet(t *testing.T) {
	c := &project.Config{}
	c.Name = "suite"
	c.Sources = []string{"a.ttcn3", "b.ttcn3"}
	c.Variables = map[string]string{"B": "2", "A": "x y"}

	for _, tt := range []struct {
		key  string
		want []string
	}{
		// A list gives one value per element, as `$(ntt show -- sources)`
		// needs.
		{"sources", []string{"a.ttcn3", "b.ttcn3"}},
		{"name", []string{"suite"}},
		// An object gives one 'key=value' per entry, sorted.
		{"variables", []string{"'A=x y'", "'B=2'"}},
		{"variables.A", []string{"x y"}},
	} {
		got, err := get(c, tt.key)
		if err != nil {
			t.Errorf("get(%q): %v", tt.key, err)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("get(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
	if _, err := get(c, "nosuch"); err == nil {
		t.Error(`get("nosuch") succeeded, want an error`)
	}
}
