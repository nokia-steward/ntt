package verify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExpectedVerdict(t *testing.T) {
	tests := []struct {
		name   string
		tags   [][]string
		want   string
		wantOk bool
	}{
		{"nil", nil, "", false},
		{"k3 pass", [][]string{{"@k3", "verdict pass"}}, "pass", true},
		{"k3 fail", [][]string{{"@k3", "verdict fail"}}, "fail", true},
		{"k3 error", [][]string{{"@k3", "verdict error"}}, "error", true},
		{"k3 inconc", [][]string{{"@k3", "verdict inconc"}}, "inconc", true},
		{"k3 none", [][]string{{"@k3", "verdict none"}}, "none", true},
		{"k3 verdict without value", [][]string{{"@k3", "verdict"}}, "", false},
		{"k3 verdict unknown", [][]string{{"@k3", "verdict bogus"}}, "", false},
		{"k3 unrelated directive", [][]string{{"@k3", "k3rflags -D -a ulog"}}, "", false},
		{"etsi ttcn3verdict pass", [][]string{{"@verdict", "pass accept, ttcn3verdict:pass"}}, "pass", true},
		{"etsi ttcn3verdict inconc", [][]string{{"@verdict", "pass accept, ttcn3verdict:inconc"}}, "inconc", true},
		{"etsi without ttcn3verdict", [][]string{{"@verdict", "pass accept"}}, "", false},
		{"etsi malformed ttcn3verdict", [][]string{{"@verdict", "pass accept, ttcn3verdict: pass"}}, "", false},
		{"plain pass", [][]string{{"@verdict", "pass"}}, "pass", true},
		{"plain error", [][]string{{"@verdict", "error"}}, "error", true},
		{"plain fail trimmed", [][]string{{"@verdict", " fail "}}, "fail", true},
		{"plain inconc", [][]string{{"@verdict", "inconc"}}, "inconc", true},
		{"plain none", [][]string{{"@verdict", "none"}}, "none", true},
		{"plain unknown", [][]string{{"@verdict", "bogus"}}, "", false},
		{"plain wins over later k3", [][]string{{"@verdict", "pass"}, {"@k3", "verdict fail"}}, "pass", true},
		{"later malformed tag fails", [][]string{{"@verdict", "pass"}, {"@k3", "verdict bogus"}}, "", false},
		{"unrelated tags", [][]string{{"@wip", ""}, {"@author", "STF 572"}}, "", false},
		{"first match wins", [][]string{{"@k3", "verdict fail"}, {"@k3", "verdict pass"}}, "fail", true},
		{"k3 wins over later etsi", [][]string{{"@k3", "verdict error"}, {"@verdict", "pass accept, ttcn3verdict:pass"}}, "error", true},
		{"short tag ignored", [][]string{{"@k3"}, {"@k3", "verdict pass"}}, "pass", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseExpectedVerdict(tt.tags)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
			if tt.wantOk || tt.name == "nil" || tt.name == "k3 unrelated directive" || tt.name == "unrelated tags" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestParseExpectedReason(t *testing.T) {
	tests := []struct {
		name    string
		tags    [][]string
		want    string
		wantOk  bool
		wantErr bool
	}{
		{name: "nil"},
		{name: "plain", tags: [][]string{{"@k3", "reason TIME->Testcase execution time limit exceeded"}}, want: "TIME->Testcase execution time limit exceeded", wantOk: true},
		{name: "fields and spaces", tags: [][]string{{"@k3", "reason ARRV->Array bounds violation|MTC=a.ttcn3:21|a.tc.str|1"}}, want: "ARRV->Array bounds violation|MTC=a.ttcn3:21|a.tc.str|1", wantOk: true},
		{name: "escaped bar kept", tags: [][]string{{"@k3", `reason a\u007cb`}}, want: `a\u007cb`, wantOk: true},
		{name: "surrounding space trimmed", tags: [][]string{{"@k3", " reason \t my|reason  "}}, want: "my|reason", wantOk: true},
		{name: "empty", tags: [][]string{{"@k3", "reason"}}, wantErr: true},
		{name: "blank", tags: [][]string{{"@k3", "reason   "}}, wantErr: true},
		{name: "first match wins", tags: [][]string{{"@k3", "reason first"}, {"@k3", "reason second"}}, want: "first", wantOk: true},
		{name: "later empty tag fails", tags: [][]string{{"@k3", "reason first"}, {"@k3", "reason"}}, wantErr: true},
		{name: "verdict ignored", tags: [][]string{{"@k3", "verdict fail"}, {"@k3", "reason my"}}, want: "my", wantOk: true},
		{name: "other k3 directive", tags: [][]string{{"@k3", "reasons x"}, {"@k3", "reasonable"}, {"@k3", "k3rflags -D"}}},
		{name: "other tag", tags: [][]string{{"@reason", "x"}, {"@k3"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseExpectedReason(tt.tags)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
			if tt.wantErr {
				var derr *DirectiveError
				assert.ErrorAs(t, err, &derr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParsePIXIT(t *testing.T) {
	tests := []struct {
		name   string
		tags   [][]string
		want   Expectation
		wantOK bool
	}{
		{"absent", nil, Expectation{}, false},
		{"reject", [][]string{{"@verdict", "pass reject"}}, Expectation{Kind: ExpectReject}, true},
		{"reject with todo", [][]string{{"@verdict", "pass reject, todo: compiler issue"}}, Expectation{Kind: ExpectReject}, true},
		{"accept", [][]string{{"@verdict", "pass accept"}}, Expectation{Kind: ExpectNotError}, true},
		{"accept with exact verdict", [][]string{{"@verdict", "pass accept, ttcn3verdict:fail"}}, Expectation{Kind: ExpectExact, Verdict: "fail"}, true},
		{"optional tag colon", [][]string{{"@verdict", "pass\taccept,\tttcn3verdict:pass"}}, Expectation{Kind: ExpectExact, Verdict: "pass"}, true},
		{"space around verdict colon", [][]string{{"@verdict", "pass accept, ttcn3verdict: pass"}}, Expectation{}, false},
		{"missing condition", [][]string{{"@verdict", "pass"}}, Expectation{}, false},
		{"unsupported condition", [][]string{{"@verdict", "pass skip"}}, Expectation{}, false},
		{"unknown ttcn3 verdict", [][]string{{"@verdict", "pass accept, ttcn3verdict:bogus"}}, Expectation{}, false},
		{"later malformed tag fails", [][]string{{"@verdict", "pass accept"}, {"@verdict", "pass skip"}}, Expectation{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParsePIXIT(tt.tags)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
			if tt.wantOK || tt.name == "absent" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
