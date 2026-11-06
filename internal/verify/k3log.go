package verify

import (
	"os"
	"regexp"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// The following expressions mirror the remove-variable-content filter of
// build-aux/difftool-k3log so that produced k3logs can be compared against a
// golden file without spurious differences from run-to-run variable content.
var (
	reTimestamp = regexp.MustCompile(`timestamp=[0-9.]+`)
	reDateTime  = regexp.MustCompile(`\b[0-9]{8}T[0-9]{6}\.[0-9]{6}\b`)
	reLeadDate  = regexp.MustCompile(`^xxxxxxxxTxxxxxx.xxxxxx\|`)
	reErrDesc   = regexp.MustCompile(`^(.{4})(=[^|]+)?->[^|]+`)
	reT3xfOfs   = regexp.MustCompile(`=t3xf:[0-9]+`)
	reAddr      = regexp.MustCompile(`\$[0-9a-f]+=?`)
	reT3xfFile  = regexp.MustCompile(`[a-zA-Z0-9_/.-]+/([a-zA-Z0-9_.-]+\.t3xf)`)
	reTtcn3File = regexp.MustCompile(`[a-zA-Z0-9_/.-]+/([a-zA-Z0-9_.-]+\.ttcn3)`)
)

// NormalizeK3logLine removes variable content (timestamps, addresses, t3xf
// offsets, error descriptions and directory prefixes) from a single k3log line.
// It mirrors build-aux/difftool-k3log's remove-variable-content sed filter.
func NormalizeK3logLine(line string) string {
	line = reTimestamp.ReplaceAllString(line, "timestamp=x.xxxxxx")
	line = reDateTime.ReplaceAllString(line, "xxxxxxxxTxxxxxx.xxxxxx")
	line = reLeadDate.ReplaceAllString(line, "")
	line = reErrDesc.ReplaceAllString(line, "$1")
	line = reT3xfOfs.ReplaceAllString(line, "")
	line = reAddr.ReplaceAllString(line, "")
	// The two filename filters are not global in difftool-k3log: only the first
	// match on a line is rewritten.
	line = replaceFirst(reT3xfFile, line, "$1")
	line = replaceFirst(reTtcn3File, line, "$1")
	return line
}

// NormalizeK3log normalizes every line of a k3log and returns the result as a
// single newline-terminated string (empty input yields an empty string).
func NormalizeK3log(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = NormalizeK3logLine(l)
	}
	return strings.Join(out, "\n") + "\n"
}

// DiffK3log compares the normalized actual log against the golden file at
// goldenPath and returns a unified diff. An empty diff means they match. A
// missing or unreadable golden file is reported as an error.
func DiffK3log(goldenPath string, actual []string) (string, error) {
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		return "", err
	}
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(golden)),
		B:        difflib.SplitLines(NormalizeK3log(actual)),
		FromFile: goldenPath,
		ToFile:   "actual",
		Context:  3,
	})
}

// WriteGoldenK3log writes the normalized actual log to goldenPath, creating or
// overwriting the golden file (the equivalent of difftool-k3log's
// K3_CREATE_GOLDEN=1 mode).
func WriteGoldenK3log(goldenPath string, actual []string) error {
	return os.WriteFile(goldenPath, []byte(NormalizeK3log(actual)), 0644)
}

// replaceFirst rewrites only the first match of re in s, expanding repl (so $1
// references work), mirroring sed's default non-global substitution.
func replaceFirst(re *regexp.Regexp, s, repl string) string {
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return s
	}
	expanded := re.ExpandString(nil, repl, s, loc)
	return s[:loc[0]] + string(expanded) + s[loc[1]:]
}
