package cache_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/cache"
	"github.com/stretchr/testify/assert"
)

func init() {
	os.Unsetenv("NTT_CACHE")
	os.Unsetenv("K3_CACHE")
}

func TestLookup(t *testing.T) {
	// The cache directory is joined with filepath.Join in the
	// implementation, so on Windows the expected separator is "\".
	cacheDir := filepath.FromSlash("testdata/cache")
	os.Setenv("NTT_CACHE", cacheDir)
	assert.Equal(t, "./file", cache.Lookup("./file"))
	assert.Equal(t, "./cache.go", cache.Lookup("./cache.go"))

	assert.Equal(t, "file", cache.Lookup("file"))
	assert.Equal(t, "file://cache.go", cache.Lookup("file://cache.go"))
	assert.Equal(t, ".", cache.Lookup("."))
	assert.Equal(t, "..", cache.Lookup(".."))
	assert.Equal(t, "cache.go", cache.Lookup("cache.go"))
	assert.Equal(t, filepath.Join(cacheDir, "other.go"), cache.Lookup("other.go"))
}
