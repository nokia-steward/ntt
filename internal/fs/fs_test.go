package fs_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/span"
	"github.com/stretchr/testify/assert"
)

// TestBytesFromURL verifies, that files specified by an URL can be read.
func TestBytesFromURL(t *testing.T) {
	path, err := filepath.Abs("fs_test.go")
	if err != nil {
		panic(err)
	}

	expected, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	// Constructing the URL by literal concatenation produces broken
	// URIs on Windows ("file://D:\\..."). Defer to URIFromPath which
	// knows how to encode drive letters and convert backslashes.
	f := fs.Open(string(span.URIFromPath(path)))
	b, err := f.Bytes()
	assert.Nil(t, err)
	assert.Equal(t, expected, b)
}

func TestCaching(t *testing.T) {
	assert.Equal(t, "package.yml", fs.Open("package.yml").Path())

	cacheDir := filepath.FromSlash("testdata/cache")
	os.Setenv("NTT_CACHE", cacheDir)
	assert.Equal(t, filepath.Join(cacheDir, "package.yml"), fs.Open("package.yml").Path())
}

func TestJoinPath(t *testing.T) {
	// JoinPath returns OS-native file paths but keeps URLs untouched.
	// We mark URL expectations explicitly so we don't accidentally
	// run them through filepath.FromSlash.
	type joinCase struct {
		first, second string
		want          string
		isURL         bool
	}
	tests := []joinCase{
		{"", "", "", false},
		{".", "", ".", false},
		{".", "a", "a", false},
		{"/", "b", "/b", false},
		// A "//" base is left out: its cleaned form is OS-specific
		// (Unix "/c" vs Windows UNC "\\c"), which is filepath.Clean's
		// behaviour rather than JoinPath's path-vs-URL routing.
		{"/", "/d", "/d", false},
		{"e", "f", "e/f", false},
		{"/g", "h", "/g/h", false},
		{"/i", "../j", "/j", false},
		{"file://k", "l", "file://k/l", true},
		{"file:///m", "n", "file:///m/n", true},
		{"file:///o", "../p", "file:///p", true},
	}

	for _, test := range tests {
		want := test.want
		if !test.isURL {
			want = filepath.FromSlash(want)
		}
		got := fs.JoinPath(test.first, test.second)
		assert.Equal(t, want, got)
	}

}

func TestTTCN3Files(t *testing.T) {
	// fromSlash converts the slash-style literals we keep in this
	// test to whatever path separator the host OS uses, so the
	// suite runs on Windows as well as Unix.
	fromSlash := func(paths []string) []string {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = filepath.FromSlash(p)
		}
		return out
	}

	t.Run("empty", func(t *testing.T) {
		got, err := fs.TTCN3Files()
		assert.Nil(t, err)
		assert.Nil(t, got)
	})
	t.Run("dir", func(t *testing.T) {
		got, err := fs.TTCN3Files(filepath.FromSlash("testdata/TestTTCN3Files"))
		assert.Nil(t, err)
		assert.Nil(t, got)
	})
	t.Run("dir", func(t *testing.T) {
		got, err := fs.TTCN3Files(filepath.FromSlash("testdata/TestTTCN3Files/some-dir"))
		assert.Nil(t, err)
		assert.Nil(t, got)
	})
	t.Run("dir", func(t *testing.T) {
		want := fromSlash([]string{
			"testdata/TestTTCN3Files/ttcn3-dir/a.ttcn3",
			"testdata/TestTTCN3Files/ttcn3-dir/b.ttcn",
			"testdata/TestTTCN3Files/ttcn3-dir/c.ttcnpp",
		})
		got, err := fs.TTCN3Files(filepath.FromSlash("testdata/TestTTCN3Files/ttcn3-dir"))
		assert.Nil(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("errors", func(t *testing.T) {
		want := fromSlash([]string{
			"testdata/TestTTCN3Files/xxx-dir/a.ttcn3",
		})
		got, err := fs.TTCN3Files(filepath.FromSlash("testdata/TestTTCN3Files/xxx-dir/a.ttcn3"))
		assert.True(t, errors.Is(err, os.ErrNotExist))
		assert.Equal(t, want, got)
	})
	t.Run("file", func(t *testing.T) {
		want := fromSlash([]string{
			"testdata/TestTTCN3Files/ttcn3-dir/a.ttcn3",
			"testdata/TestTTCN3Files/ttcn3-dir/a.ttcn3",
		})
		got, err := fs.TTCN3Files(
			filepath.FromSlash("testdata/TestTTCN3Files/ttcn3-dir/a.ttcn3"),
			filepath.FromSlash("testdata/TestTTCN3Files/ttcn3-dir/a.ttcn3"),
		)
		assert.Nil(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("URI", func(t *testing.T) {
		// URIs always use forward slashes regardless of host OS,
		// so no conversion here.
		want := []string{"foo://a.ttcn3"}
		got, err := fs.TTCN3Files("foo://a.ttcn3")
		assert.Nil(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("URI", func(t *testing.T) {
		want := []string{"foo://a.ttcn3?bar"}
		got, err := fs.TTCN3Files("foo://a.ttcn3?bar")
		assert.True(t, errors.Is(err, fs.ErrInvalidFileExtension))
		assert.Equal(t, want, got)
	})

}
