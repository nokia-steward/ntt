// Package cmake provides basic support for reading CMake cache files.
package cmake

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
)

var ErrNotFound = errors.New("not found")

type Cache struct {
	path string
}

func (c *Cache) Get(name string) (string, error) {

	f, err := os.Open(c.path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if v := strings.SplitN(s.Text(), ":", 2); v[0] == name {
			w := strings.SplitN(v[1], "=", 2)
			log.Debugln("cmake: get:", name, "=", w[1])
			return w[1], nil
		}
	}
	return "", ErrNotFound
}

// FindCache finds the CMakeCache.txt file by walking up the given path, and
// returns a CMake Cache or nil if not found.
func FindCache(path string) *Cache {
	var c *Cache
	fs.WalkUp(path, func(path string) bool {
		if file := filepath.Join(path, "CMakeCache.txt"); fs.IsRegular(file) {
			c = &Cache{path: file}
		}
		return true
	})
	return c
}

// FindBuildCache finds an out-of-source cache at build/CMakeCache.txt by
// walking up from path. The nearest match wins.
func FindBuildCache(path string) *Cache {
	var c *Cache
	fs.WalkUp(path, func(path string) bool {
		if file := filepath.Join(path, "build", "CMakeCache.txt"); fs.IsRegular(file) {
			c = &Cache{path: file}
			return false
		}
		return true
	})
	return c
}
