package verify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDirectives(t *testing.T) {
	tests := []struct {
		name string
		tags [][]string
		want Directives
	}{
		{"nil", nil, Directives{}},
		{
			"k3rflags",
			[][]string{{"@k3", "k3rflags -D -a ulog"}},
			Directives{K3RFLAGS: "-D -a ulog"},
		},
		{
			"escaped pipe unescaped",
			[][]string{{"@k3", `k3rflags -D -a setv\|ulog`}},
			Directives{K3RFLAGS: "-D -a setv|ulog"},
		},
		{
			"multiple escaped pipes",
			[][]string{{"@k3", `k3rflags -D -a cpen\|cplv\|ulog\|vach`}},
			Directives{K3RFLAGS: "-D -a cpen|cplv|ulog|vach"},
		},
		{
			"quoted flag list strips surrounding quotes",
			[][]string{{"@k3", `k3rflags "-D -a tcst -a tcfi --enable-constraints-at-receive"`}},
			Directives{K3RFLAGS: "-D -a tcst -a tcfi --enable-constraints-at-receive"},
		},
		{
			"accumulate across tags",
			[][]string{
				{"@k3", "k3rflags -D"},
				{"@k3", "k3rflags -a ulog"},
			},
			Directives{K3RFLAGS: "-D -a ulog"},
		},
		{
			"k3cflags",
			[][]string{{"@k3", "k3cflags --format=t3xf"}},
			Directives{K3CFLAGS: "--format=t3xf"},
		},
		{
			"env",
			[][]string{{"@k3", "env FOO=bar BAZ=qux"}},
			Directives{Env: []string{"FOO=bar", "BAZ=qux"}},
		},
		{
			"failure",
			[][]string{{"@k3", "failure known breakage"}},
			Directives{Failure: true},
		},
		{
			"mixed with verdict and tool ignored",
			[][]string{
				{"@k3", "tool k3log"},
				{"@k3", "verdict pass"},
				{"@k3", "k3rflags -D -a ulog"},
			},
			Directives{K3RFLAGS: "-D -a ulog"},
		},
		{"non-k3 tags ignored", [][]string{{"@verdict", "pass"}}, Directives{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseDirectives(tt.tags))
		})
	}
}

func TestDirectivesMerge(t *testing.T) {
	a := Directives{K3RFLAGS: "-D", K3CFLAGS: "--format=t3xf", Env: []string{"A=1"}}
	b := Directives{K3RFLAGS: "-a ulog", Env: []string{"B=2"}, Failure: true}
	got := a.Merge(b)
	assert.Equal(t, "-D -a ulog", got.K3RFLAGS)
	assert.Equal(t, "--format=t3xf", got.K3CFLAGS)
	assert.Equal(t, []string{"A=1", "B=2"}, got.Env)
	assert.True(t, got.Failure)
}
