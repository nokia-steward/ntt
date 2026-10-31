package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/ttcn3"
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
		switch e.Kind {
		case "add":
			got = append(got, e.Text)
			assert.Equal(t, e.Text, input[e.Offs:e.Offs+len(e.Text)])
		case "open":
			if assert.Less(t, i, e.End) {
				assert.Equal(t, "close", src.Events[e.End].Kind)
			}
		case "close":
		default:
			t.Errorf("unexpected event kind %q", e.Kind)
		}
	}
	assert.Equal(t, want, got)
	assert.Contains(t, got, "// header")
	assert.Contains(t, got, "/* trailer */")
	assert.Contains(t, got, ",")
	assert.Contains(t, got, ";")
	assert.Equal(t, "open", src.Events[0].Kind)
	assert.Equal(t, "Root", src.Events[0].Node)
	assert.Equal(t, len(src.Events)-1, src.Events[0].End)
	assert.Equal(t, "close", src.Events[len(src.Events)-1].Kind)
}

func TestNodeEventJSON(t *testing.T) {
	add, err := json.Marshal(ttcn3.NodeEvent{Kind: "add", Text: "module", Offs: 0})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"kind":"add","text":"module","offs":0}`, string(add))

	open, err := json.Marshal(ttcn3.NodeEvent{Kind: "open", Node: "Module", End: 5})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"kind":"open","node":"Module","end":5}`, string(open))

	closeEv, err := json.Marshal(ttcn3.NodeEvent{Kind: "close", Node: "Module", End: 1, Text: "module", Offs: 0})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"kind":"close"}`, string(closeEv))
}
