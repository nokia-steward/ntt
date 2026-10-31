package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
)

func TestBuildSource(t *testing.T) {
	input := "// header\nmodule M {\n\tconst integer x[2] := { 1, 2 }; /* trailer */\n}\n"
	file := filepath.Join(t.TempDir(), "test.ttcn3")
	if err := os.WriteFile(file, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	var want []string
	s := syntax.NewScanner([]byte(input))
	for {
		kind, begin, end := s.Scan()
		if kind == syntax.EOF {
			break
		}
		want = append(want, input[begin:end])
	}

	src := buildSource(file)

	var got []string
	for i, e := range src.Events {
		switch {
		case e.Kind == "AddToken":
			got = append(got, e.Text)
			assert.Equal(t, e.Text, input[e.Offs:e.Offs+e.Len])
		case strings.HasPrefix(e.Kind, "Open"):
			if assert.Less(t, i, e.Other) {
				close := src.Events[e.Other]
				assert.Equal(t, "Close"+strings.TrimPrefix(e.Kind, "Open"), close.Kind)
				assert.Equal(t, i, close.Other)
			}
		case strings.HasPrefix(e.Kind, "Close"):
			if assert.Less(t, e.Other, i) {
				assert.Equal(t, i, src.Events[e.Other].Other)
			}
		default:
			t.Errorf("unexpected event kind %q", e.Kind)
		}
	}
	assert.Equal(t, want, got)
	assert.Contains(t, got, "// header")
	assert.Contains(t, got, "/* trailer */")
	assert.Contains(t, got, ",")
	assert.Contains(t, got, ";")
	assert.Equal(t, "OpenRoot", src.Events[0].Kind)
	assert.Equal(t, "CloseRoot", src.Events[len(src.Events)-1].Kind)
}
