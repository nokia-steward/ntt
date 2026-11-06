package verify

import "strings"

// ParseExpectedOutput returns the text of a `// @output: <text>` line comment.
// comment is the comment as written in the source, including the leading
// "//". The text is everything after "@output:", without surrounding white
// space. It is compared verbatim with the text of a ulog event, so it may
// contain spaces, quotes, and '|'. The second return value reports whether
// comment is an @output comment.
//
// Only line comments that start with @output: count. A block comment or a
// comment that mentions @output: later is not an expectation. An empty text
// is an error, because a log statement never logs an empty text, and so is a
// second @output: in the same comment, because a line has one expectation.
func ParseExpectedOutput(comment string) (string, bool, error) {
	body, ok := strings.CutPrefix(comment, "//")
	if !ok {
		return "", false, nil
	}
	text, ok := strings.CutPrefix(strings.TrimSpace(body), "@output:")
	if !ok {
		return "", false, nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "@output:") {
		return "", false, &DirectiveError{Tag: "@output", Value: strings.TrimSpace(body)}
	}
	return text, true, nil
}
