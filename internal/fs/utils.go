package fs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
	"github.com/hashicorp/go-multierror"
)

// FindTTCN3Files returns a list of TTCN-3 source files (.ttcn3, .ttcn,
// .ttcnpp) in the given directory.
func FindTTCN3Files(dir string) []string {
	return findFiles(dir, HasTTCN3Extension)
}

// FindTTCN3FilesRecursive returns a list TTCN-3 source files available in directory sub-tree.
func FindTTCN3FilesRecursive(dir string) []string {
	var ret []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && HasTTCN3Extension(path) {
			ret = append(ret, path)
		}
		return nil
	})
	return ret
}

// FindTTCN3DirectoriesRecursive returns a list of directories containing TTCN-3 source files.
func FindTTCN3DirectoriesRecursive(dir string) []string {
	var ret []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && IsDir(path) && len(FindTTCN3Files(path)) > 0 {
			ret = append(ret, path)
		}
		return nil
	})
	return ret
}

// TTCN3Files takes a list of paths and replaces all paths describing a
// directory with the TTCN-3 files contained in that directory.
//
// A error is returned for each path which is not accessable or for each
// (non-directory), which does not have a TTCN-3 extension.
//
// This function a conventient way to "flatten" the Sources of a project.Config
// struct, for example.
func TTCN3Files(paths ...string) ([]string, error) {
	var (
		errs *multierror.Error
		ret  []string
	)
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case err != nil && !IsURI(path):
			ret = append(ret, path)
			errs = multierror.Append(errs, err)
		case err == nil && info.IsDir():
			files, err := ioutil.ReadDir(path)
			if err != nil {
				errs = multierror.Append(errs, err)
			}
			for _, file := range files {
				if !file.Mode().IsDir() && HasTTCN3Extension(file.Name()) {
					path := filepath.Join(path, file.Name())
					f, err := TTCN3Files(path)
					ret = append(ret, f...)
					errs = multierror.Append(errs, err)
				}
			}
		default:
			if !HasTTCN3Extension(path) {
				err := fmt.Errorf("%s: %w", path, ErrInvalidFileExtension)
				errs = multierror.Append(errs, err)
			}
			ret = append(ret, path)
		}

	}
	return ret, errs.ErrorOrNil()
}

// HasTTCN3Extension returns true if file has suffix .ttcn3 or .ttcn
func HasTTCN3Extension(file string) bool {
	return withExtension(".ttcn3", ".ttcn", ".ttcnpp")(file)
}

// FindASN1Files returns a list of ASN.1 files (.asn, asn1).
func FindASN1Files(dir string) []string {
	return findFiles(dir, HasASN1Extension)
}

// HasASN1Extension returns true if file has suffix .asn or .asn1
func HasASN1Extension(file string) bool {
	return withExtension(".asn", ".asn1")(file)
}

// FindCFiles returns a list of C/C++ files (.c, .cc, .cxx, .cpp).
func FindCFiles(dir string) []string {
	return findFiles(dir, HasCExtension)
}

// FindFilesRecursive returns a list files from the whole directory subtree.
func FindFilesRecursive(dir string) []string {
	var sources []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			sources = append(sources, path)
		}
		return nil
	})
	return sources
}

// HasCExtension returns true if file has suffix .c, .cc, .cxx or .cpp
func HasCExtension(file string) bool {
	return withExtension(".c", ".cc", ".cxx", ".cpp")(file)
}

// FindEnvFileInCurrPath returns the path of the directory containing an
// environment file
func FindEnvFileInCurrPath(dir string) string {
	path := dir
	for !IsFsRoot(path) {
		for _, file := range []string{"k3.env", "ntt.env"} {
			info, _ := os.Stat(filepath.Join(path, file))
			if info != nil {
				return path
			}

		}
		path = filepath.Dir(path)
	}
	return ""
}

// IsFsRoot returns true if the supplied path coincides with
// the filesystem root
func IsFsRoot(path string) bool {
	root := "/"
	if vol := filepath.VolumeName(path); vol != "" {
		root = vol + "\\"
	}
	return path == root
}

// Abs makes paths absolute, when not absolute already.
func Abs(paths ...string) []string {
	if len(paths) == 0 {
		return nil
	}
	ret := make([]string, len(paths))
	for i, path := range paths {
		ret[i], _ = filepath.Abs(path)
	}
	return ret
}

// Rel makes paths relative to base, when not absolute already. Use it when
// when you want to make a path relative to a test suite manifest.
func Rel(base string, paths ...string) []string {
	if len(paths) == 0 {
		return nil
	}
	ret := make([]string, len(paths))
	for i, path := range paths {
		if r, err := filepath.Rel(base, path); err == nil {
			ret[i] = r
		} else {
			ret[i] = path
		}
	}
	return ret
}

// Real makes a path, which is relativ to base, to a real path.
func Real(base, path string) string {
	if path == "" {
		return ""
	}
	if IsURI(path) || filepath.IsAbs(path) || path[0] == '$' {
		return path
	}
	return filepath.Join(base, path)
}

// Glob is a wrapper for filepath.Glob, but ignoring any errors.
func Glob(s string) []string {
	found, _ := filepath.Glob(s)
	return found
}

// Slugify generates a slug from unicode string.
func Slugify(s string) string {
	slug.Lowercase = false
	return strings.ReplaceAll(slug.Make(s), "-", "_")
}

// Stems strips directory and extension from a string.
func Stem(s string) string {
	base := filepath.Base(s)
	ext := filepath.Ext(s)
	return strings.TrimSuffix(base, ext)
}

// WalkUp traverses a path towards file system root.
func WalkUp(path string, f func(path string) bool) {
	for {
		if !f(path) {
			break
		}
		abs, _ := filepath.Abs(path)
		if IsFsRoot(abs) {
			break
		}
		path = filepath.Clean(filepath.Join(path, ".."))
	}
}

// IsRegulart returns true if path exists and is a regular file.
func IsRegular(path string) bool {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if info, err := os.Stat(path); err == nil {
		return info.Mode().IsRegular()
	}
	return false
}

// IsDir returns true if path exists and is a directory.
func IsDir(path string) bool {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if info, err := os.Stat(path); err == nil {
		return info.IsDir()
	}
	return false
}

func ReplaceExt(path string, newExt string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".log"
}

func findFiles(dir string, matcher func(name string) bool) []string {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sources []string
	for _, file := range files {
		if file.Mode().IsRegular() && matcher(file.Name()) {
			sources = append(sources, filepath.Join(dir, file.Name()))
		}
	}
	return sources
}

func withExtension(exts ...string) func(string) bool {
	return func(file string) bool {
		for _, ext := range exts {
			if strings.HasSuffix(file, ext) {
				return true
			}
		}
		return false
	}
}
