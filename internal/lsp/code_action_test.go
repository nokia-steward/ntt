package lsp

import (
	"testing"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

func TestExtractAutofix(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
		want bool
	}{
		{name: "nil data", data: nil, want: false},
		{name: "non-map data", data: 42, want: false},
		{name: "no autofix key", data: map[string]interface{}{"foo": 1}, want: false},
		{
			name: "missing range",
			data: map[string]interface{}{"autofix": map[string]interface{}{
				"title": "t",
			}},
			want: false,
		},
		{
			name: "valid payload (float64 like JSON)",
			data: map[string]interface{}{"autofix": map[string]interface{}{
				"title":       "Remove unused import",
				"begin":       float64(10),
				"end":         float64(20),
				"replacement": "",
			}},
			want: true,
		},
		{
			name: "valid payload (int)",
			data: map[string]interface{}{"autofix": map[string]interface{}{
				"title":       "t",
				"begin":       1,
				"end":         5,
				"replacement": "x",
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := extractAutofix(tt.data)
			if ok != tt.want {
				t.Errorf("extractAutofix(%v) = %v, want %v", tt.data, ok, tt.want)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	r := func(sl, sc, el, ec uint32) protocol.Range {
		return protocol.Range{
			Start: protocol.Position{Line: sl, Character: sc},
			End:   protocol.Position{Line: el, Character: ec},
		}
	}
	cases := []struct {
		name string
		a, b protocol.Range
		want bool
	}{
		{name: "identical", a: r(0, 0, 1, 0), b: r(0, 0, 1, 0), want: true},
		{name: "before", a: r(0, 0, 0, 5), b: r(1, 0, 1, 5), want: false},
		{name: "after", a: r(1, 0, 1, 5), b: r(0, 0, 0, 5), want: false},
		{name: "overlap", a: r(0, 0, 1, 5), b: r(0, 3, 0, 7), want: true},
		{name: "touching", a: r(0, 0, 0, 5), b: r(0, 5, 0, 10), want: true},
		{name: "empty fallback", a: protocol.Range{}, b: r(0, 0, 0, 5), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlaps(tc.a, tc.b); got != tc.want {
				t.Errorf("overlaps(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
