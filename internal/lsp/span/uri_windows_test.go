// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the NOTICE file.

// +build windows

package span_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/lsp/span"
)

// currentDrive returns the upper-case drive letter of the current
// working directory (e.g. "C"). We use this to build expectations in
// TestURIFromPath dynamically, because GitHub Actions Windows runners
// expose D: as the default drive whereas developers typically run on
// C: - either way the test should still pass.
func currentDrive(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if len(cwd) < 2 || cwd[1] != ':' {
		t.Fatalf("cwd %q has no drive letter", cwd)
	}
	return strings.ToUpper(string(cwd[0]))
}

// TestURI tests the conversion between URIs and filenames. The test cases
// include Windows-style URIs and filepaths, but we avoid having OS-specific
// tests by using only forward slashes, assuming that the standard library
// functions filepath.ToSlash and filepath.FromSlash do not need testing.
func TestURIFromPath(t *testing.T) {
	drive := currentDrive(t)
	for _, test := range []struct {
		path, wantFile string
		wantURI        span.URI
	}{
		{
			path:     ``,
			wantFile: ``,
			wantURI:  span.URI(""),
		},
		{
			path:     `C:\Windows\System32`,
			wantFile: `C:\Windows\System32`,
			wantURI:  span.URI("file:///C:/Windows/System32"),
		},
		{
			path:     `C:\Go\src\bob.go`,
			wantFile: `C:\Go\src\bob.go`,
			wantURI:  span.URI("file:///C:/Go/src/bob.go"),
		},
		{
			path:     `c:\Go\src\bob.go`,
			wantFile: `C:\Go\src\bob.go`,
			wantURI:  span.URI("file:///C:/Go/src/bob.go"),
		},
		// Paths without a drive letter are resolved against the
		// current drive by filepath.Abs(), so build the expectation
		// from currentDrive() rather than hard-coding "C:".
		{
			path:     `\path\to\dir`,
			wantFile: drive + `:\path\to\dir`,
			wantURI:  span.URI("file:///" + drive + ":/path/to/dir"),
		},
		{
			path:     `\a\b\c\src\bob.go`,
			wantFile: drive + `:\a\b\c\src\bob.go`,
			wantURI:  span.URI("file:///" + drive + ":/a/b/c/src/bob.go"),
		},
		{
			path:     `c:\Go\src\bob george\george\george.go`,
			wantFile: `C:\Go\src\bob george\george\george.go`,
			wantURI:  span.URI("file:///C:/Go/src/bob%20george/george/george.go"),
		},
	} {
		got := span.URIFromPath(test.path)
		if got != test.wantURI {
			t.Errorf("URIFromPath(%q): got %q, expected %q", test.path, got, test.wantURI)
		}
		gotFilename := got.Filename()
		if gotFilename != test.wantFile {
			t.Errorf("Filename(%q): got %q, expected %q", got, gotFilename, test.wantFile)
		}
	}
}

func TestURIFromURI(t *testing.T) {
	for _, test := range []struct {
		inputURI, wantFile string
		wantURI            span.URI
	}{
		{
			inputURI: `file:///c:/Go/src/bob%20george/george/george.go`,
			wantFile: `C:\Go\src\bob george\george\george.go`,
			wantURI:  span.URI("file:///C:/Go/src/bob%20george/george/george.go"),
		},
		{
			inputURI: `file:///C%3A/Go/src/bob%20george/george/george.go`,
			wantFile: `C:\Go\src\bob george\george\george.go`,
			wantURI:  span.URI("file:///C:/Go/src/bob%20george/george/george.go"),
		},
		{
			inputURI: `file:///c:/path/to/%25p%25ercent%25/per%25cent.go`,
			wantFile: `C:\path\to\%p%ercent%\per%cent.go`,
			wantURI:  span.URI(`file:///C:/path/to/%25p%25ercent%25/per%25cent.go`),
		},
		{
			inputURI: `file:///C%3A/`,
			wantFile: `C:\`,
			wantURI:  span.URI(`file:///C:/`),
		},
		{
			inputURI: `file:///`,
			wantFile: `\`,
			wantURI:  span.URI(`file:///`),
		},
	} {
		got := span.URIFromURI(test.inputURI)
		if got != test.wantURI {
			t.Errorf("NewURI(%q): got %q, expected %q", test.inputURI, got, test.wantURI)
		}
		gotFilename := got.Filename()
		if gotFilename != test.wantFile {
			t.Errorf("Filename(%q): got %q, expected %q", got, gotFilename, test.wantFile)
		}
	}
}
