package k3log

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/yaml"
)

func TestGeneratedTables(t *testing.T) {
	b, err := os.ReadFile("events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var events map[string]Descriptor
	if err := yaml.Unmarshal(b, &events); err != nil {
		t.Fatal(err)
	}
	if len(Descriptions) != len(events) {
		t.Fatalf("len(Descriptions) = %d, want %d", len(Descriptions), len(events))
	}
	if len(Categories) != len(events) {
		t.Fatalf("len(Categories) = %d, want %d", len(Categories), len(events))
	}
	if len(tokens) != len(events) {
		t.Fatalf("len(tokens) = %d, want %d", len(tokens), len(events))
	}
	for id, want := range events {
		if got, ok := Descriptions[id]; !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("Descriptions[%q] = %+v, want %+v", id, got, want)
		}
		wantCat := Category(strings.Join(append([]string{id, want.Description}, want.Fields...), "|"))
		if got := Categories[id]; got != wantCat {
			t.Errorf("Categories[%q] = %q, want %q", id, got, wantCat)
		}
		if got := tokens[id]; got != wantCat {
			t.Errorf("tokens[%q] = %q, want %q", id, got, wantCat)
		}
		if got := wantCat.IsError(); got != want.IsError {
			t.Errorf("Category(%q).IsError() = %v, want %v", id, got, want.IsError)
		}
	}
	if !DIV0.IsError() {
		t.Errorf("DIV0.IsError() = false, want true")
	}
	if !ErrDEAD.IsError() {
		t.Errorf("ErrDEAD.IsError() = false, want true")
	}
}

type lines map[int]string

func (l lines) Pos(pc int) string { return l[pc] }

func TestWriteEventTimestampWidth(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEvent(&buf, nil, COCR, "k3r", NoPos, "MCC", "once"); err != nil {
		t.Fatal(err)
	}
	e, err := ParseEvent(buf.String())
	if err != nil {
		t.Fatal(err)
	}
	const width = len("20060102T150405.000000")
	if got := e.Field(0); len(got) != width {
		t.Fatalf("timestamp %q has length %d, want %d", got, len(got), width)
	}
	if _, err := e.Stamp(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteEventSource(t *testing.T) {
	loc := lines{0x10: "a.ttcn3:4"}
	for _, tt := range []struct {
		pc   int
		want string
	}{
		{NoPos, "MTC"},
		{0x10, "MTC=a.ttcn3:4"},
		{0x20, "MTC"},
	} {
		var buf bytes.Buffer
		if err := WriteEvent(&buf, loc, ULOG, "MTC", tt.pc, "hi"); err != nil {
			t.Fatal(err)
		}
		e, err := ParseEvent(buf.String())
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Field(2); got != tt.want {
			t.Errorf("pc=%#x: got %q, want %q", tt.pc, got, tt.want)
		}
	}
}
