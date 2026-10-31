// Package goldenfile implement simplistic goldenfile parsing in a format
// similar to TreeSitter tests.
//
// Unlike TreeSitter we do not support any additional test-data (such as
// skipping tests, or multi-file input).
//
// Each file contains one or more tests. The name musst be enclosed by at least
// three equal-signs (`===`). The input text and expected text musst be
// separated by at least three minuses (`---`):
//
// ```
// ===
// Test 1
// ===
// input of test 1
// ---
// wanted output of test 1
//
// ===
// Test 2
// ===
// ---
// ```
//
// # Example
//
// tests, _ := goldenfile.Parse(os.Stdin)
//
//	for _, test := range tests {
//		fmt.Printf("Test: %+n")
//	}
//
// # Updating files
//
// The package add a new command line flag `-update` to go test.
// This flags can be checked by the user and let update the goldenfiles.
// See the example in the repository for details.
package goldenfile

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	head = regexp.MustCompile(`^===+\s*$`)
	sep  = regexp.MustCompile(`^---s*$`)
)

// Test is a single test
type Test struct {
	Name string // Testname
	Text string // Given input
	Want string // Expected output
}

// Parse content and return a slice of tests.
func Parse(r io.Reader) ([]Test, error) {
	return newParser(r).Parse()
}

type parser struct {
	*bufio.Scanner
	Line int
	err  error
}

func newParser(r io.Reader) *parser {
	return &parser{
		Scanner: bufio.NewScanner(r),
		Line:    1,
	}
}

func (p *parser) Next() bool {
	if p.err != nil {
		return false
	}
	for p.Scan() {
		if strings.TrimSpace(p.Text()) != "" {
			return true
		}
	}
	return false
}

func (p *parser) Parse() ([]Test, error) {
	tests := []Test{}

	p.Next()
	if !head.MatchString(p.Text()) {
		return nil, fmt.Errorf("line %d: expected separator `===`", p.Line)
	}

	for p.Next() {
		tests = append(tests, p.parseTest())
	}
	return tests, p.err
}

func (p *parser) errorf(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf(format, args...)
	}
}

func (p *parser) parseTest() Test {
	return Test{
		Name: p.parseName(),
		Text: p.parseText(),
		Want: p.parseWant(),
	}
}

func (p *parser) parseName() string {
	name := strings.TrimSpace(p.Text())
	if !p.Next() {
		p.errorf("unexpected EOF")
	}

	if !head.MatchString(p.Text()) {
		p.errorf("line %d: expected separator `===`", p.Line)
	}
	return name
}

func (p *parser) parseText() string {
	text := ""
	for p.Next() && !sep.MatchString(p.Text()) {
		text += p.Text() + "\n"
	}
	return strings.TrimSpace(text)
}

func (p *parser) parseWant() string {
	want := ""
	for p.Next() && !head.MatchString(p.Text()) {
		want += p.Text() + "\n"
	}
	return strings.TrimSpace(want)
}

var (
	Update = flag.Bool("update", false, "update golden files")
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}
