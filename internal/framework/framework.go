// Package framework provides convenience functions for handling SCT framework
// tools, such as locating the k3 compiler and runtime, and building TTCN-3
// libraries and plugins.
package framework

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/trace"
	"strings"

	"github.com/nokia/ntt/internal/cache"
	"github.com/nokia/ntt/internal/cmake"
	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/proc"
	"github.com/nokia/ntt/internal/t3xf"

	"github.com/bmatcuk/doublestar"
	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/project"
)

var (
	// ErrNotFound is returned when k3 or part of k3 is not found.
	ErrNotFound = errors.New("not found")

	// ErrNoSources is returned when no sources are available.
	ErrNoSources = fmt.Errorf("no sources available")
)

// Instance provides available information about the SCT framework. Such as the
// paths to compiler, runtime, plugins, includes, compile flags, ...
type Instance struct {

	// Root is the root directory of the k3 installation. For example 5Gs
	// root is usually the $prefix_root of the SDK.
	Root string

	// Compiler is the path to the TTCN-3 compiler. If no compiler is
	// found, the default is "mtc".
	Compiler string

	// Runtime is the path to the TTCN-3 runtime. If no runtime is found,
	// the default is "k3r".
	Runtime string

	// Plugin is a list of paths that contain k3 plugins.
	Plugins []string

	// Includes is a list of paths that contain standard TTCN-3 modules.
	Includes []string

	// CLibDirs contains paths to C libraries required to compile plugins.
	CLibDirs []string

	// CIncludes contains paths to C include directories required to
	// compile plugins.
	CIncludes []string

	// OssInfo is the path to the OSS Nokalva license file.
	OssInfo string

	// ossDefaults is the path to the machine dependent ASN.1 files.
	ossDefaults string
}

var k3 *Instance

type searchFunc func(ctx context.Context) (*Instance, error)

func init() {
	var err error
	k3, err = Find()
	if err != nil {
		k3 = &Instance{
			Runtime:  "k3r",
			Compiler: "mtc",
		}
		log.Debugf("k3: %s\n", err.Error())
	}
}

func Find() (*Instance, error) {
	ctx, task := trace.NewTask(context.Background(), "framework.find")
	defer task.End()

	// Search for k3 in the following order:
	searchers := []searchFunc{
		searchEnvionment("K3ROOT"),
		searchEnvionment("NTTROOT"),
		searchRepo(),
		searchPath("k3r"),
		searchPath("k3s"),
		searchPath("ntt"),
	}

	for _, s := range searchers {
		k, err := s(ctx)
		if err != nil {
			return nil, err
		}
		if k != nil {
			return k, nil
		}
	}
	return nil, ErrNotFound
}

// findExecutableRelativeToPath searches for an executable located at a relative path to directories in $PATH.
// returns the absolute path of the executable if found, or the input path otherwise
func findExecutableRelativeToPath(relativePath string) string {
	// Get the PATH environment variable
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return relativePath
	}

	// Split PATH into individual directories
	pathDirs := strings.Split(pathEnv, string(os.PathListSeparator))

	// Iterate through each directory in PATH
	for _, dir := range pathDirs {
		// Resolve the relative path with respect to the current directory
		absPath := filepath.Join(dir, relativePath)

		// Check if the file exists and is executable
		if fileInfo, err := os.Stat(absPath); err == nil && !fileInfo.IsDir() && (fileInfo.Mode()&0111 != 0) {
			return absPath
		}
	}

	return relativePath
}

func searchEnvionment(v string) searchFunc {
	return func(ctx context.Context) (*Instance, error) {
		region := trace.StartRegion(ctx, "search environment")
		defer region.End()
		if root := env.Getenv(v); root != "" {
			return getInstall(ctx, root)
		}
		return nil, nil
	}
}

func searchRepo() searchFunc {
	return func(ctx context.Context) (*Instance, error) {
		region := trace.StartRegion(ctx, "search repo")
		defer region.End()

		cwd, err := os.Getwd()
		if err != nil {
			return nil, err

		}

		cache := cmake.FindCache(cwd)
		if cache == nil {
			// Out-of-source builds keep CMakeCache.txt in build/, which is not
			// a parent of the source tree. go test runs from the package
			// directory, so this is how the in-tree k3r is found.
			cache = cmake.FindBuildCache(cwd)
		}
		if cache == nil {
			return nil, nil
		}
		log.Tracef(ctx, "k3", "found CMakeCache.txt: %+v\n", cache)

		k3BinaryDir, err := cache.Get("k3_BINARY_DIR")
		if err != nil {
			if errors.Is(err, cmake.ErrNotFound) {
				log.Tracef(ctx, "k3", "k3_BINARY_DIR not found in CMakeCache.txt\n")
				return nil, nil
			}
			return nil, err
		}
		k3SourceDir, err := cache.Get("k3_SOURCE_DIR")
		if err != nil {
			return nil, err
		}
		mtcBinaryDir, err := cache.Get("mtc_BINARY_DIR")
		if err != nil && !errors.Is(err, cmake.ErrNotFound) {
			return nil, err
		}
		return getRepo(ctx, k3SourceDir, k3BinaryDir, mtcBinaryDir)
	}
}

func searchPath(exe string) searchFunc {
	return func(ctx context.Context) (*Instance, error) {
		region := trace.StartRegion(ctx, "search path")
		defer region.End()

		path, err := exec.LookPath(exe)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				// Clear error to allow other searchers to run.
				err = nil
			}
			return nil, err
		}

		prefix := filepath.Clean(filepath.Join(filepath.Dir(path), ".."))

		// Symlinks make checking for directories harder. We resolve them, when possible.
		if target, err := filepath.EvalSymlinks(prefix); err == nil {
			log.Tracef(ctx, "k3", "resolved symlink: %s -> %s\n", prefix, target)
			prefix = target
		}

		k, err := getInstall(ctx, prefix)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// Clear error to allow other searchers to run.
				err = nil
			}
			return nil, err
		}
		return k, nil
	}
}

func getInstall(ctx context.Context, prefix string) (*Instance, error) {

	region := trace.StartRegion(ctx, "get install")
	defer region.End()
	log.Tracef(ctx, "k3", "get install: %s\n", prefix)

	k := Instance{Root: prefix}

	if k3c := env.Getenv("K3C"); k3c != "" {
		k.Compiler = k3c
	} else {
		k3c, err := glob(ctx, prefix+"/{libexec,bin}/{mtc,k3c}")
		if err != nil {
			return nil, fmt.Errorf("%s: mtc: %w", prefix, err)
		}
		// glob unfortunately sorts the results, so we need to take the last one.
		k.Compiler = k3c[len(k3c)-1]
	}

	if k3r := env.Getenv("K3R"); k3r != "" {
		k.Runtime = k3r
	} else {
		k3r, err := glob(ctx, prefix+"/{libexec,bin}/k3r")
		if err != nil {
			return nil, fmt.Errorf("k3r: %w", err)
		}
		// glob unfortunately sorts the results, so we need to take the last one.
		k.Runtime = k3r[len(k3r)-1]
	}

	pluginLib, err := glob(ctx, prefix+"/{lib64,lib}/libk3-plugin.so")
	if err != nil {
		return nil, fmt.Errorf("libk3-plugin.so: %w", err)
	}
	k.CLibDirs = append(k.CLibDirs, filepath.Dir(pluginLib[0]))

	cIncludes, err := glob(ctx, prefix+"/include/k3")
	if err != nil {
		return nil, fmt.Errorf("k3 includes: %w", err)
	}
	k.CIncludes = append(k.CIncludes, filepath.Dir(cIncludes[0]))

	plugins, err := glob(ctx, prefix+"/{lib64,lib}/k3/plugins/")
	if err != nil {
		return nil, fmt.Errorf("k3 plugins: %w", err)
	}
	k.Plugins = append(k.Plugins, plugins[0])

	includes, err := glob(ctx,
		k.Plugins[0]+"/ttcn3",
		prefix+"/share/k3/ttcn3",
		prefix+"/share/github.com/nokia/ntt/ttcn3")
	if err != nil {
		return nil, fmt.Errorf("k3 includes: %w", err)
	}
	k.Includes = includes

	if ossInfo, err := glob(ctx, prefix+"/share/k3/asn1/ossinfo"); err == nil {
		k.OssInfo = filepath.Dir(ossInfo[0])
	}
	if ossDefaults, err := glob(ctx, prefix+"/share/k3/asn1/asn1dflt.*"); err == nil {
		k.ossDefaults = ossDefaults[0]
	}

	return &k, nil
}

// installedCompiler returns the mtc binary on PATH. bin/mtc is a wrapper
// that warns and execs libexec/mtc, so prefer that binary.
func installedCompiler() (string, error) {
	path, err := exec.LookPath("mtc")
	if err != nil {
		return "", err
	}
	libexec := filepath.Join(filepath.Dir(path), "..", "libexec", "mtc")
	if abs, err := filepath.Abs(libexec); err == nil {
		libexec = abs
	}
	if fs.IsRegular(libexec) {
		return libexec, nil
	}
	return path, nil
}

func getRepo(ctx context.Context, k3SourceDir, k3BinaryDir, mtcBinaryDir string) (*Instance, error) {
	region := trace.StartRegion(ctx, "get repo")
	defer region.End()
	log.Tracef(ctx, "k3", "get repo: %s %s %s\n", k3SourceDir, k3BinaryDir, mtcBinaryDir)

	k := Instance{Root: k3BinaryDir}

	if k3c := env.Getenv("K3C"); k3c != "" {
		k.Compiler = k3c
	} else if mtcBinaryDir != "" {
		k3c, err := glob(ctx, mtcBinaryDir+"/source/mtc/mtc")
		if err != nil {
			return nil, fmt.Errorf("mtc: %w", err)
		}
		k.Compiler = k3c[0]
	} else if k3c, err := installedCompiler(); err == nil {
		// A runtime-only build has no mtc. Use the installed compiler so
		// tests still run against this tree's k3r.
		k.Compiler = k3c
	} else {
		k.Compiler = "mtc"
	}

	if k3r := env.Getenv("K3R"); k3r != "" {
		k.Runtime = k3r
	} else {
		k3r, err := glob(ctx, k3BinaryDir+"/src/k3r/k3r")
		if err != nil {
			return nil, fmt.Errorf("k3r: %w", err)
		}
		k.Runtime = k3r[0]
	}

	pluginLib, err := glob(ctx, k3BinaryDir+"/src/libk3-plugin/libk3-plugin.so")
	if err != nil {
		return nil, fmt.Errorf("libk3-plugin.so: %w", err)
	}
	k.CLibDirs = append(k.CLibDirs, filepath.Dir(pluginLib[0]))

	ossLibs, err := glob(ctx, k3SourceDir+"/lib/ossasn1/lib64")
	if err != nil {
		return nil, fmt.Errorf("oss libs: %w", err)
	}
	k.CLibDirs = append(k.CLibDirs, ossLibs...)

	cIncludes, err := glob(ctx, k3SourceDir+"/include/k3")
	if err != nil {
		return nil, fmt.Errorf("k3 includes: %w", err)
	}
	k.CIncludes = append(k.CIncludes, filepath.Dir(cIncludes[0]))

	ossIncludes, err := glob(ctx, k3SourceDir+"/lib/ossasn1/include")
	if err != nil {
		return nil, fmt.Errorf("oss includes: %w", err)
	}
	k.CIncludes = append(k.CIncludes, ossIncludes...)

	plugins, err := glob(ctx, k3BinaryDir+"/src/k3r-*-plugin")
	if err != nil {
		return nil, fmt.Errorf("k3 plugins: %w", err)
	}
	k.Plugins = append(k.Plugins, plugins...)

	includes, err := glob(ctx,
		k3SourceDir+"/src/k3r-*-plugin",
		k3SourceDir+"/src/libzmq",
		k3SourceDir+"/src/ttcn3",
	)
	if err != nil {
		return nil, fmt.Errorf("k3 includes: %w", err)
	}
	k.Includes = includes

	if ossInfo, err := glob(ctx, k3BinaryDir+"/ossinfo"); err == nil {
		k.OssInfo = filepath.Dir(ossInfo[0])
	}
	if ossDefaults, err := glob(ctx, k3SourceDir+"/lib/ossasn1/asn1dflt.*"); err == nil {
		k.ossDefaults = ossDefaults[0]
	}

	return &k, nil

}

func glob(ctx context.Context, patterns ...string) ([]string, error) {
	region := trace.StartRegion(ctx, "glob")
	defer region.End()

	var ret []string
	for _, p := range patterns {
		matches, err := doublestar.Glob(p)
		log.Tracef(ctx, "k3", "glob: %s -> %v\n", p, matches)
		if err != nil {
			return nil, err
		}
		ret = append(ret, matches...)
	}
	if len(ret) == 0 {
		return nil, ErrNotFound
	}
	return ret, nil
}

// DefaultEnv is the default environment for k3-based test suites.
var DefaultEnv = map[string]string{
	"CXX":        "g++",
	"CC":         "gcc",
	"ASN1C":      "asn1",
	"ASN1CFLAGS": "-reservedWords ffs -c -charIntegers -listingFile -messageFormat emacs -noDefines -valuerefs -debug -root -soed -compat decoderUpdatesInputAddress",
	"ASN2TTCN3":  findExecutableRelativeToPath("../libexec/asn1tottcn3"),
}

// OssInfo returns the path to the ossinfo file.
func OssInfo() string {
	return k3.OssInfo
}

// Compiler returns the path to the TTCN-3 compiler. Compiler will return "mtc"
// if no compiler is found.
func Compiler() string {
	return k3.Compiler
}

// Runtime returns the path to the TTCN-3 runtime. Runtime will return "k3r" if
// no runtime is found.
func Runtime() string {
	return k3.Runtime
}

func CLibDirs() []string {
	return k3.CLibDirs
}

// Plugins returns a list of k3 plugins.
func Plugins() []string {
	return k3.Plugins
}

// Includes returns a list of TTCN-3 include directories required by the k3 compiler.
func Includes() []string {
	return k3.Includes
}

// NewASN1Codec returns the commands required to compile ASN.1 files.
func NewASN1Codec(vars map[string]string, name string, encoding string, srcs ...string) []*proc.Cmd {
	if vars == nil {
		vars = make(map[string]string)
		for k, v := range DefaultEnv {
			vars[k] = v
		}
	}

	if _, ok := vars["OSSINFO"]; !ok && k3.OssInfo != "" {
		vars["OSSINFO"] = k3.OssInfo
	}

	var cFlags []string
	if f, ok := vars["CFLAGS"]; ok {
		cFlags = append(cFlags, f)
	}
	for _, inc := range k3.CIncludes {
		cFlags = append(cFlags, "-I"+inc)
	}
	vars["CFLAGS"] = strings.Join(cFlags, " ")

	var ldFlags []string
	if f, ok := vars["LDFLAGS"]; ok {
		ldFlags = append(ldFlags, f)
	}
	for _, dir := range k3.CLibDirs {
		ldFlags = append(ldFlags, "-L"+dir)
	}
	vars["LDFLAGS"] = strings.Join(ldFlags, " ")

	vars["name"] = name
	vars["encoding"] = encoding

	asn1 := proc.Task("$ASN1C -${encoding} $ASN1CFLAGS -output ${name}.enc -prefix ${name} ${srcs}")
	asn1.Env = vars
	asn1.Sources = []string{k3.ossDefaults}
	asn1.Sources = append(asn1.Sources, srcs...)
	asn1.Targets = []string{
		pathf("%s.enc.c", name),
		pathf("%s.enc.h", name),
	}

	lib := proc.Task("$CC -fPIC -shared -D_OSSGETHEADER -DOSSPRINT $CPPFLAGS $CFLAGS $LDFLAGS $EXTRA_LDFLAGS ${srcs} -l:libasn1code.a -Wl,-Bdynamic -o ${tgts}")
	lib.Env = vars
	lib.Sources = asn1.Targets
	lib.Targets = []string{
		pathf("%slib.so", name),
	}

	mod := proc.Task("$ASN2TTCN3 $ASN1TOTTCN3FLAGS -o ${tgts} ${srcs} ${name}mod  ${encoding}")
	mod.Env = vars
	mod.Sources = lib.Targets
	mod.Targets = []string{
		pathf("%smod.ttcn3", name),
	}

	return []*proc.Cmd{asn1, lib, mod}
}

// NewPlugin returns the commands for building a k3 plugin.
func NewPlugin(vars map[string]string, name string, srcs ...string) []*proc.Cmd {
	if vars == nil {
		vars = make(map[string]string)
		for k, v := range DefaultEnv {
			vars[k] = v
		}
	}

	var cFlags []string
	if f, ok := vars["CFLAGS"]; ok {
		cFlags = append(cFlags, f)
	}
	for _, inc := range k3.CIncludes {
		cFlags = append(cFlags, "-I"+inc)
	}
	vars["CFLAGS"] = strings.Join(cFlags, " ")

	var ldFlags []string
	if f, ok := vars["LDFLAGS"]; ok {
		ldFlags = append(ldFlags, f)
	}
	for _, dir := range k3.CLibDirs {
		ldFlags = append(ldFlags, "-L"+dir)
	}
	vars["LDFLAGS"] = strings.Join(ldFlags, " ")

	p := proc.Task("$CXX $CPPFLAGS $CFLAGS $CXXFLAGS -shared -fPIC -o ${tgts} ${srcs} $LDFLAGS $EXTRA_LDFLAGS -lk3-plugin")
	p.Env = vars
	p.Sources = srcs
	p.Targets = []string{pathf("k3r-%s-plugin.so", name)}
	return []*proc.Cmd{p}
}

// T3XF compiles TTCN-3 sources into a T3XF image with the in-process compiler.
type T3XF struct {
	srcs   []string
	target string
}

func (t *T3XF) Inputs() []string  { return t.srcs }
func (t *T3XF) Outputs() []string { return []string{t.target} }
func (t *T3XF) String() string {
	return fmt.Sprintf("t3xf -o %s %s", t.target, strings.Join(t.srcs, " "))
}

func (t *T3XF) Run() error {
	older, err := proc.IsOlder(t.target, t.srcs...)
	if err != nil {
		return err
	}
	if !older {
		return nil
	}
	if err := os.Remove(t.target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	log.Verboseln("+", t.String())
	b, err := t3xf.CompileImage(t.srcs...)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(t.target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(t.target, b, 0o644)
}

// NewT3XF returns the task that builds a T3XF image from TTCN-3 sources.
func NewT3XF(vars map[string]string, name string, srcs []string, imports ...string) []*T3XF {
	files := append(append([]string{}, imports...), removeStdlib(srcs)...)
	return []*T3XF{{srcs: files, target: name}}
}

func removeStdlib(srcs []string) []string {

	// There are multiple installations of the stdlib, so we cannot compare
	// for identity but use a hash instead.
	sum := func(file string) ([]byte, error) {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}

	// We build a map of hashes of the stdlib files. The key is the base
	// name of the file.
	stdlib := make(map[string][]byte)
	for _, dir := range Includes() {
		for _, file := range fs.FindTTCN3Files(dir) {
			if s, err := sum(file); err == nil {
				stdlib[fs.Stem(file)] = s
			}
		}
	}

	isStdlib := func(file string) bool {
		if s1 := stdlib[fs.Stem(file)]; s1 != nil {
			if s2, err := sum(file); err == nil {
				return bytes.Equal(s1, s2)
			}
		}
		return false
	}

	var ret []string
	for _, src := range srcs {
		if !isStdlib(src) {
			ret = append(ret, src)
		}
	}
	return ret
}

type TTCN3Library struct{ srcs []string }

func (l *TTCN3Library) String() string    { return fmt.Sprintf("%s", l.srcs) }
func (l *TTCN3Library) Outputs() []string { return l.srcs }
func (l *TTCN3Library) Inputs() []string  { return l.srcs }
func (l *TTCN3Library) Run() error        { return nil }

// NewTTCN3Library returns the commands for building a TTCN-3 library.
func NewTTCN3Library(vars map[string]string, name string, srcs ...string) []*TTCN3Library {
	return []*TTCN3Library{{srcs}}
}

// pathf is like fmt.Sprintf but searches the K3_CACHE environment variable first.
func pathf(f string, v ...any) string {
	return cache.Lookup(fmt.Sprintf(f, v...))
}

// Task is a build task.
type Task interface {
	// Inputs returns the list of input files.
	Inputs() []string

	// Outputs returns the list of output files.
	Outputs() []string

	// Run executes the task.
	Run() error

	// String returns a string representation of the task.
	String() string
}

// BuildTasks returns the build tasks required to generate and build the test
// executable and its dependencies.
func BuildTasks(c *project.Config) ([]Task, error) {
	var (
		ret  []Task
		merr *multierror.Error
	)

	for _, imp := range c.Imports {
		tasks, err := ImportTasks(c, imp)
		if err != nil {
			merr = multierror.Append(merr, err)
			continue
		}
		ret = append(ret, tasks...)
	}

	srcs, err := fs.TTCN3Files(c.Sources...)
	if err != nil {
		merr = multierror.Append(merr, err)
	}

	var imports []string
	for _, t := range ret {
		for _, output := range t.Outputs() {
			if fs.HasTTCN3Extension(output) {
				imports = append(imports, output)
			}
		}
	}

	t3xf := cache.Lookup(fmt.Sprintf("%s.t3xf", c.Name))
	for _, t := range NewT3XF(c.Variables, t3xf, srcs, imports...) {
		ret = append(ret, t)
	}

	return ret, merr.ErrorOrNil()
}

// ImportTasks returns the build tasks required to generate and build a given
// test suite dependency.
func ImportTasks(c *project.Config, path string) ([]Task, error) {
	dir := path
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var (
		tasks                         []Task
		asn1Files, ttcn3Files, cFiles []string
		processed                     int
	)

	// Collect and categorize all the files!
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		switch path := filepath.Join(dir, f.Name()); filepath.Ext(path) {
		case ".asn1", ".asn":
			asn1Files = append(asn1Files, path)
			processed++
		case ".ttcn3", ".ttcn":
			ttcn3Files = append(ttcn3Files, path)
			processed++
		case ".c", ".cxx", ".cpp", ".cc":
			// Skip ASN1 codecs
			if strings.HasSuffix(path, ".enc.c") {
				continue
			}
			cFiles = append(cFiles, path)
			processed++
		}
	}
	if processed == 0 {
		return nil, fmt.Errorf("%s: %w", dir, ErrNoSources)
	}

	name, err := project.NameFromURI(dir)
	if err != nil {
		return nil, err
	}

	if len(asn1Files) > 0 {
		encoding, err := project.EncodingFromURI(dir)
		if err != nil {
			return nil, err
		}
		for _, t := range NewASN1Codec(c.Variables, name, encoding, asn1Files...) {
			tasks = append(tasks, t)
		}
	}
	if len(cFiles) > 0 {
		for _, t := range NewPlugin(c.Variables, name, cFiles...) {
			tasks = append(tasks, t)
		}
	}
	if len(ttcn3Files) > 0 {
		for _, t := range NewTTCN3Library(c.Variables, name, ttcn3Files...) {
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

func WithK3() project.ConfigOption {
	return func(c *project.Config) error {
		if c.Variables == nil {
			c.Variables = make(map[string]string)
		}

		for k, v := range DefaultEnv {
			if _, ok := c.Variables[k]; !ok {
				c.Variables[k] = v
			}
		}

		// Locating required tools (k3c, k3r, asn1c, ...) has an
		// issue with order of initialization.
		// To fix this knot without rewriting everything, we take
		// a short for asn1tottcn3 and figure out details, later.
		//
		// When ASN2TTCN3 is not found we'll workaround.
		_, err := exec.LookPath(c.Variables["ASN2TTCN3"])
		if err == nil {
			return nil
		}

		cache := cmake.FindCache(".")
		if cache == nil {
			return nil
		}

		k3BinaryDir, err := cache.Get("k3_BINARY_DIR")
		if err != nil {
			return nil
		}

		c.Variables["ASN2TTCN3"] = filepath.Join(k3BinaryDir, "src", "asn1tottcn3", "asn1tottcn3")

		return nil
	}
}
