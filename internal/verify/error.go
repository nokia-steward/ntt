package verify

import (
	"strings"

	"github.com/nokia/ntt/internal/k3log"
)

// runnerErrors are the error categories k3 run writes itself. No runtime
// event has them, so an @error tag cannot expect them.
var runnerErrors = map[string]bool{
	k3log.ErrVMIS.ID(): true,
	k3log.ErrRMIS.ID(): true,
	k3log.ErrOMIS.ID(): true,
	k3log.ErrEMIS.ID(): true,
}

// ParseExpectedError returns the category of a `// @error` or
// `// @error: <category>` line comment. comment is the comment as written in
// the source, including the leading "//". The category is a k3log error
// category code such as CONV or DOME, and "" for a bare @error, which accepts
// any error. The second return value reports whether comment is an @error
// comment.
//
// Only line comments that start with @error count. A block comment, a comment
// that mentions @error later, or a longer tag such as @errors is not an
// expectation. An empty or unknown category is an error, and so is anything
// after the category or after a bare @error.
func ParseExpectedError(comment string) (string, bool, error) {
	body, ok := strings.CutPrefix(comment, "//")
	if !ok {
		return "", false, nil
	}
	body = strings.TrimSpace(body)
	rest, ok := strings.CutPrefix(body, "@error")
	if !ok {
		return "", false, nil
	}
	if rest == "" {
		return "", true, nil
	}
	category, ok := strings.CutPrefix(rest, ":")
	if !ok {
		if rest[0] != ' ' && rest[0] != '\t' {
			return "", false, nil
		}
		return "", false, &DirectiveError{Tag: "@error", Value: body}
	}
	category = strings.TrimSpace(category)
	if c, ok := k3log.Categories[category]; !ok || !c.IsError() || runnerErrors[category] {
		return "", false, &DirectiveError{Tag: "@error", Value: body}
	}
	return category, true, nil
}
