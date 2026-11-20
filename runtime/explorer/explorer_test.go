package explorer_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/explorer"
)

func TestWriteTree_SchemaInjected(t *testing.T) {
	var buf bytes.Buffer
	tr := explorer.Tree{
		Project: "demo",
		Modules: []explorer.Module{{Name: "M", Testcases: []explorer.Testcase{
			{Name: "tc_a", FullName: "M.tc_a", File: "M.ttcn3", Line: 5},
		}}},
	}
	if err := explorer.WriteTree(&buf, tr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"schema": "`+explorer.SchemaVersion+`"`)) {
		t.Fatalf("schema not injected:\n%s", buf.String())
	}
}

func TestStream_Emit_DefaultsApplied(t *testing.T) {
	var buf bytes.Buffer
	s := explorer.NewStream(&buf)
	if err := s.Emit(explorer.Event{Kind: explorer.EventTestEnd, Testcase: "M.tc_a", Verdict: "pass"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(explorer.Event{Kind: explorer.EventFinished}); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 events, got %d:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var ev explorer.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not JSON: %v", i, err)
		}
		if ev.Schema != explorer.SchemaVersion {
			t.Errorf("line %d schema=%q want %q", i, ev.Schema, explorer.SchemaVersion)
		}
		if ev.Time.IsZero() {
			t.Errorf("line %d has zero timestamp", i)
		}
	}
}
