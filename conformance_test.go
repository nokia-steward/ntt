package main

import "testing"

func TestParseVerdictAnnotation(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"// @verdict pass accept, ttcn3verdict:pass", "pass"},
		{"// @verdict pass accept, ttcn3verdict:fail", "fail"},
		{"// @verdict pass accept, ttcn3verdict:inconc", "inconc"},
		{"// @verdict  pass accept", "pass"},
		{"// @verdict  pass reject", "reject"},
		{"// @verdict inconclusive", ""},
		{"// nothing here", ""},
		{`/** @verdict  pass accept, ttcn3verdict:pass */`, "pass"},
	}
	for _, tc := range cases {
		got := parseVerdictAnnotation(tc.src)
		if got != tc.want {
			t.Errorf("parseVerdictAnnotation(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}
