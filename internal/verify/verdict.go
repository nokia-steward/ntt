// Package verify interprets test directives (documentation tags) and verifies
// test runs against their expected outcome.
package verify

import (
	"fmt"
	"strings"
)

// Expectation is how a testcase verdict is judged.
type Expectation struct {
	Kind    ExpectationKind
	Verdict string
}

// ExpectationKind selects how an Expectation judges a testcase verdict.
type ExpectationKind int

const (
	// ExpectDefault fails a run for fail or error.
	ExpectDefault ExpectationKind = iota

	// ExpectExact requires Verdict exactly.
	ExpectExact

	// ExpectNotError accepts every verdict except error.
	ExpectNotError

	// ExpectReject requires error after a successful build.
	ExpectReject
)

// DirectiveError reports a malformed verdict, reason, output, or error
// directive.
type DirectiveError struct {
	Tag   string
	Value string
}

func (e *DirectiveError) Error() string {
	return fmt.Sprintf("invalid %s directive: %q", e.Tag, e.Value)
}

// ParseExpectedVerdict returns the expected TTCN-3 verdict declared by a test's
// documentation tags, if any. The second return value reports whether an
// explicit verdict was found.
//
// Three directive styles are understood (note how the tag tokenizer in
// github.com/nokia/ntt/ttcn3/doc splits on the first ':' only):
//
//	// @verdict: <verdict>            -> tag ["@verdict", "<verdict>"]
//	// @k3:verdict <verdict>          -> tag ["@k3", "verdict <verdict>"]
//	// @verdict ..., ttcn3verdict:<v> -> tag ["@verdict", "..., ttcn3verdict:<v>"]  (ETSI)
//
// A plain @verdict value is accepted when it is one of none, pass, inconc,
// fail, or error. The first matching tag wins. All verdict directives are
// validated before returning, so a later malformed tag still returns an error;
// unrelated documentation tags are ignored.
func ParseExpectedVerdict(tags [][]string) (string, bool, error) {
	var expected string
	var found bool
	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		key, value := tag[0], tag[1]
		switch key {
		case "@k3":
			// "@k3:verdict pass" tokenizes to key "@k3", value "verdict pass".
			fields := strings.Fields(value)
			if len(fields) == 0 || fields[0] != "verdict" {
				continue
			}
			if len(fields) != 2 || !isVerdict(fields[1]) {
				return "", false, &DirectiveError{Tag: "@k3:verdict", Value: value}
			}
			if !found {
				expected, found = fields[1], true
			}
		case "@verdict":
			// ETSI specs embed the runtime verdict as "ttcn3verdict:<v>", e.g.
			// "pass accept, ttcn3verdict:inconc".
			if v, hasTTCN3, err := etsiTTCN3Verdict(value); hasTTCN3 {
				if err != nil {
					return "", false, err
				}
				if !found {
					expected, found = v, true
				}
				continue
			}
			// "@verdict: error" tokenizes to key "@verdict", value "error".
			if v := strings.TrimSpace(value); isVerdict(v) {
				if !found {
					expected, found = v, true
				}
				continue
			}
			return "", false, &DirectiveError{Tag: "@verdict", Value: value}
		}
	}
	return expected, found, nil
}

// ParseExpectedReason returns the expected reason of a testcase's final
// verdict declared by a `// @k3:reason <reason>` tag, which tokenizes to
// ["@k3", "reason <reason>"]. The reason is the rest of the tag value, without
// surrounding white space. It is free text and may contain spaces and '|'.
// The second return value reports whether a reason was found.
//
// The first tag wins. All reason directives are validated before returning,
// so a later empty reason still returns an error.
func ParseExpectedReason(tags [][]string) (string, bool, error) {
	var expected string
	var found bool
	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != "@k3" {
			continue
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(tag[1]), "reason")
		if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		reason := strings.TrimSpace(rest)
		if reason == "" {
			return "", false, &DirectiveError{Tag: "@k3:reason", Value: tag[1]}
		}
		if !found {
			expected, found = reason, true
		}
	}
	return expected, found, nil
}

// isVerdict reports whether s is a TTCN-3 testcase verdict.
func isVerdict(s string) bool {
	switch s {
	case "none", "pass", "inconc", "fail", "error":
		return true
	default:
		return false
	}
}

// ParsePIXIT returns the expectation declared by a module's PIXIT-style
// @verdict tag. The second return value reports whether such a tag was found.
func ParsePIXIT(tags [][]string) (Expectation, bool, error) {
	var expected Expectation
	var found bool
	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != "@verdict" {
			continue
		}
		fields := strings.FieldsFunc(tag[1], func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
		if len(fields) < 2 || fields[0] != "pass" {
			return Expectation{}, false, &DirectiveError{Tag: "@verdict", Value: tag[1]}
		}
		switch fields[1] {
		case "reject":
			if _, hasTTCN3, err := etsiTTCN3Verdict(tag[1]); err != nil {
				return Expectation{}, false, err
			} else if hasTTCN3 {
				return Expectation{}, false, &DirectiveError{Tag: "@verdict", Value: tag[1]}
			}
			if !found {
				expected, found = Expectation{Kind: ExpectReject}, true
			}
		case "accept":
			v, hasTTCN3, err := etsiTTCN3Verdict(tag[1])
			if err != nil {
				return Expectation{}, false, err
			}
			if hasTTCN3 {
				if !found {
					expected, found = Expectation{Kind: ExpectExact, Verdict: v}, true
				}
				continue
			}
			if !found {
				expected, found = Expectation{Kind: ExpectNotError}, true
			}
		default:
			return Expectation{}, false, &DirectiveError{Tag: "@verdict", Value: tag[1]}
		}
	}
	return expected, found, nil
}

// etsiTTCN3Verdict extracts the ttcn3verdict:<verdict> token from an ETSI
// @verdict specification. found distinguishes no token from an invalid one.
func etsiTTCN3Verdict(spec string) (verdict string, found bool, err error) {
	for _, field := range strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		if field == "ttcn3verdict" || strings.HasPrefix(field, "ttcn3verdict:") {
			v, ok := strings.CutPrefix(field, "ttcn3verdict:")
			if !ok || !isVerdict(v) {
				return "", true, &DirectiveError{Tag: "@verdict", Value: spec}
			}
			return v, true, nil
		}
	}
	return "", false, nil
}
