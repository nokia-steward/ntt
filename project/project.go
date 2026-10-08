// Package project provides a tool-independent interface for working with
// various TTCN-3 project layouts and configurations. The interface is
// intended to be uniform across all project layouts. Supported layouts will
// be extended over time.
//
// Here is a simple example, opening a project configuration:
//
//	conf, err := project.Open("/path/to/project")
//	if err != nil {
//		log.Fatal(err)
//	}
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/internal/cache"
	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/results"
	"github.com/nokia/ntt/internal/yaml"
	"github.com/nokia/ntt/project/internal/titan"
)

var (
	ErrNoSources = errors.New("no sources")
	ErrNotFound  = errors.New("not found")
)

// Config describes a single project configuration. It aggregates various
// sources, such as manifest files, parameters files, environment files, ...
type Config struct {
	// The Manifest pulls in most of the project configuration.
	Manifest `json:",inline"`

	// ManifestFile is the path to the manifest file.
	ManifestFile string `json:"manifest_file"`

	// Root is the root directory of the project. Usually this is the
	// directory of the manifest file.
	Root string

	// SourceDir is the directory containing source files, data files or
	// additional configuration files. SourceDir and Root are usually
	// identical, unless manifest was generated into a dedicated build
	// directory.
	SourceDir string `json:"source_dir"`

	// EnvFile is the path to the environment file. The content of the
	// environment file will overwrite variables defined in the manifest.
	// Default:
	//
	// 	${NTT_CACHE}/ntt.env
	EnvFile string `json:"env_file"`

	// ResultsFile is the path to the results file.
	ResultsFile string `json:"results_file"`

	// what toolchain to use.
	toolchain string
}

// The Manifest file (package.yml).
type Manifest struct {
	// Name is a short name of the project. It is recommended to use TTCN-3
	// identifiers (A-Za-z0-9_).
	Name string

	Version     string
	Description string
	Author      string
	License     string
	Homepage    string
	Repository  string
	Bugs        string
	Keywords    []string

	// Sources is a list of source files which provide the TTCN-3
	// test-cases.
	Sources []string

	// Imports is list of dependencies, which are required to run TTCN-3
	// test-cases. E.g. common code, adapters, codecs, ...
	Imports []string

	// BeforeBuild is a list of shell commands to be executed before
	// building. An exit code unequal to 0 will cancel any further
	// execution.
	BeforeBuild []string `json:"before_build"`

	// AfterBuild is a list of shell commands to be executed after
	// building. An exit code unequal to 0 will cancel any further
	// execution.
	AfterBuild []string `json:"after_build"`

	// BeforeRun is execute before any tests are run. An exit code unequal
	// to 0 will cancel any further test execution.
	BeforeRun []string `json:"before_run"`

	// AfterRun is executed after all tests are run.
	AfterRun []string `json:"after_run"`

	// BeforeTest is executed before each test. An exit code unequal to 0
	// will prevent test execution.
	BeforeTest []string `json:"before_test"`

	// AfterTest is executed after each test. An exit code unequal to 0
	// will result in an error verdict.
	AfterTest []string `json:"after_test"`

	// Variables is a list of variables that can be used in the
	// configuration files. Environment variables and variables defined in
	// the environment file (ntt.env) will overwrite variables defined in
	// this variables section.
	Variables env.Env

	// Diagnostics is a list of diagnostics flags used by compilator
	Diagnostics []string `json:"diagnostics"`

	// Tools holds per-tool configuration sections so individual
	// editors (and CI jobs) can override the defaults without
	// resorting to per-project flags. See the docs on Tools for the
	// full schema.
	Tools Tools `yaml:"tools,omitempty" json:"tools,omitempty"`

	// Parameters is an embedded parameters file.
	Parameters `json:",inline"`

	// ParametersFile is the path to the parameters file. Default:
	//
	// 	${NTT_SOURCE_DIR}/${NTT_NAME}.parameters
	ParametersFile string `json:"parameters_file"`

	// HooksFile is the path to the hooks file. Default:
	//
	// 	${NTT_SOURCE_DIR}/${NTT_NAME}.hooks
	HooksFile string `json:"hooks_file"`

	// LintFile is the path to the lint configuration file. Default:
	//
	// ${NTT_SOURCE_DIR}/ntt-lint.yml
	LintFile string `json:"lint_file"`
}

// The Parameters file provide runtime configuration for a project (e.g. parameters files)
type Parameters struct {
	// Global test configuration
	TestConfig `json:",inline"`

	// Presets provides configuration presets, which can be used for global
	// and test specific configuration.
	Presets map[string]TestConfig

	// Execute provides a list of test specific configuration. Each entry
	// specifies how and when a test should be executed.
	Execute []TestConfig
}

// A TestConfig specifies how and when a testcase should be executed.
type TestConfig struct {
	// A pattern describing a test. Optional testcase parameters are allowed.
	Test string `json:",omitempty"`

	// Description is an optional human-readable label for this test
	// configuration (variant). It mirrors the "description" field used by
	// k3 in .parameters execute entries.
	Description string `json:",omitempty"`

	// Presets is a list of preset configurations to be used to execute
	// the test.
	Preset []string `json:",omitempty"`

	// Timeout in seconds.
	Timeout yaml.Duration `json:",omitempty"`

	// Module parameters
	Parameters map[string]string `json:",omitempty"`

	// Rules describe when a configuration should be used.
	Rules `json:",inline"`
}

type Rules struct {
	// Only execute testcase if the given conditions are met.
	Only *ExecuteCondition `json:",omitempty"`

	// Do not execute testcase if the given conditions are met.
	Except *ExecuteCondition `json:",omitempty"`
}

// ExecuteCondition specifies conditions for executing a testcase.
type ExecuteCondition struct {
	Presets []string
}

// Index specifies where to look for project configuration files.
type Index struct {
	// SourceDir is the top level source directory.
	SourceDir string `json:"source_dir"`

	// BinaryDir is the top level binary directory.
	BinaryDir string `json:"binary_dir"`

	// Suite is a list of directories to search for project configuration files.
	Suites []Suite `json:"suites"`
}

// Suite specifies the root and source directories of a project.
type Suite struct {
	// RootDir is the root directory of the project. This is usually where the package.yml is located.
	RootDir string `json:"root_dir"`

	// SourceDir is the directory where the test suite source files are located.
	SourceDir string `json:"source_dir"`

	// Target is an optional build target.
	Target string `json:"target,omitempty"`
}

var (
	ManifestFile = "package.yml"
	IndexFile    = "ttcn3_suites.json"
)

// Discover walks towards the file system root and collects
// known test suite layouts.
//
// Discover returns a list of potential test suite root directories.
func Discover(path string) []Suite {

	// Convert possible URIs to proper file system paths.
	path = fs.Path(path)

	var list []Suite

	// Return index, ignoring errors.
	readIndices := func(file string) []Suite {
		b, err := fs.Content(file)
		if err != nil {
			log.Debugf("Failed to read %s: %s", file, err.Error())
			return nil
		}
		var idx Index
		if err := json.Unmarshal(b, &idx); err != nil {
			log.Debugf("%s: %s", file, err.Error())
		}

		var list []Suite
		for _, s := range idx.Suites {
			if s.RootDir != "" {
				root := fs.Real(filepath.Dir(file), s.RootDir)
				log.Debugf("using root_dir: %q\n", root)
				list = append(list, s)
			}
		}
		return list
	}

	fs.WalkUp(path, func(path string) bool {
		// Check source directories
		if file := fs.JoinPath(path, ManifestFile); fs.IsRegular(file) {
			log.Debugf("discovered manifest: %q\n", file)
			list = append(list, Suite{RootDir: path, SourceDir: path})
		}
		// Discover Titan project descriptors next to package.yml.
		// A directory may contain several .tpd files (one per
		// configuration); we report each of them so the caller can
		// pick.
		for _, tpd := range fs.Glob(fs.JoinPath(path, "*.tpd")) {
			log.Debugf("discovered titan descriptor: %q\n", tpd)
			list = append(list, Suite{RootDir: path, SourceDir: path})
		}
		list = append(list, readIndices(fs.JoinPath(path, IndexFile))...)

		// Check parallel build directories
		for _, file := range fs.Glob(path + "/*build*/" + IndexFile) {
			list = append(list, readIndices(file)...)
		}
		for _, file := range fs.Glob(path + "/build/native/*/sct/" + IndexFile) {
			list = append(list, readIndices(file)...)
		}
		return true
	})

	// If we could not find any manifest, try guess a root directory based on known naming schemes.
	if len(list) == 0 {
		fs.WalkUp(path, func(path string) bool {
			if isRoot(path) {
				log.Debugf("discovered potential project root folder in %q\n", path)
				list = append(list, Suite{RootDir: path, SourceDir: path})
				return false
			}
			return true
		})
	}

	// If we still could not find any coherent test suite, just add the
	// current directory in the hopes it contains TTCN-3 files.
	if len(list) == 0 {
		return []Suite{{RootDir: path, SourceDir: path}}
	}

	// Remove duplicate entries
	result := make([]Suite, 0, len(list))
	visited := make(map[Suite]bool)
	for _, v := range list {
		if !visited[v] {
			visited[v] = true
			result = append(result, v)
		}
	}
	return result
}

func match(path string, patterns ...string) bool {
	for _, pattern := range patterns {
		if files := fs.Glob(filepath.Join(path, pattern)); len(files) > 0 {
			return true
		}
	}
	return false
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

// NameFromURI derives a TTCN-3 compatible name from a path or URI.
func NameFromURI(uri string) (string, error) {
	uri = fs.Path(uri)
	if strings.HasPrefix(uri, ".") {
		abs, err := filepath.Abs(uri)
		if err != nil {
			return "", fmt.Errorf("%s: %w", uri, err)
		}
		uri = abs
	}
	return fs.Slugify(fs.Stem(uri)), nil
}

// Encoding returns the ASN.1 encoding for a given URI. Current implementation
// always returns "per", unless the URI contains the string "rrc"
func EncodingFromURI(uri string) (string, error) {
	name, err := NameFromURI(uri)
	if err != nil {
		return "", err
	}
	if strings.Contains(strings.ToLower(name), "rrc") {
		return "uper", nil
	}
	return "per", nil
}

// Files returns the list of TTCN-3 source files. Genereated files are
// excluded for now, but might be added in the future.
func Files(c *Config) ([]string, error) {
	var files []string
	files = append(files, c.Sources...)
	files = append(files, c.Imports...)
	return fs.TTCN3Files(files...)
}

// GlobalCOnfig returns the global test configuration with applied presets
func (p *Parameters) GlobalConfig(presets ...string) (TestConfig, error) {
	gc := p.TestConfig
	for _, preset := range presets {
		tc, ok := p.Presets[preset]
		if !ok {
			return TestConfig{}, fmt.Errorf("preset %q: %w", preset, ErrNotFound)
		}
		gc = MergeTestConfig(gc, tc)
	}
	return gc, nil
}

// TestConfigs returns the test configurations matching the given name and
// applied presets.
func (p *Parameters) TestConfigs(name string, presets ...string) ([]TestConfig, error) {
	gc, err := p.GlobalConfig(presets...)
	if err != nil {
		return nil, err
	}

	var list []TestConfig
	for _, tc := range p.Execute {
		tc = MergeTestConfig(gc, tc)
		if tc, ok := matchTestConfig(name, tc, presets...); ok {
			list = append(list, tc)
		}
	}

	// Our job is done if we have at least one match.
	if len(list) == 0 {
		if tc, ok := matchTestConfig(name, p.TestConfig, presets...); ok {
			list = append(list, tc)
		}
	}

	return list, nil
}

func matchTestConfig(name string, tc TestConfig, presets ...string) (TestConfig, bool) {
	pattern, params := split(tc.Test)
	if pattern != "" {
		ok, err := filepath.Match(pattern, name)
		if err != nil {
			log.Verbosef("%s: %s\n", name, err.Error())
		}
		if !ok {
			return tc, false
		}
	}

	tc.Test = name
	if params != "" {
		tc.Test += "(" + params + ")"
	}

	return tc, matchRules(tc.Rules, presets...)
}

// matchRules returns true if presets match given rules
func matchRules(r Rules, presets ...string) bool {
	if r.Except != nil && matchPresets(r.Except, presets...) {
		return false
	}
	if r.Only != nil && len(r.Only.Presets) > 0 && !matchPresets(r.Only, presets...) {
		return false
	}
	return true
}

// matchPresets returns true the given execute condition matches any of the given presets.
func matchPresets(c *ExecuteCondition, presets ...string) bool {
	for _, p := range presets {
		for _, q := range c.Presets {
			if p == q {
				return true
			}
		}
	}
	return false
}

// mergeParameters merges the given parameters. Scalar values from b override
// values from a. Maps are merged and arrays are appended.
func mergeParameters(a, b Parameters) Parameters {
	result := Parameters{}
	result.TestConfig = MergeTestConfig(a.TestConfig, b.TestConfig)
	result.Presets = make(map[string]TestConfig)
	for k, v := range a.Presets {
		result.Presets[k] = v
	}
	for k, v := range b.Presets {
		result.Presets[k] = MergeTestConfig(result.Presets[k], v)
	}
	if len(result.Presets) == 0 {
		result.Presets = nil
	}
	result.Execute = append(a.Execute, b.Execute...)
	return result
}

// MergeTestConfig merges two test configurations. Scalar values from b override
// values from a. Maps are merged. Arrays are appended.
func MergeTestConfig(a, b TestConfig) TestConfig {
	result := TestConfig{}
	result.Test = a.Test
	if b.Test != "" {
		result.Test = b.Test
	}
	result.Description = a.Description
	if b.Description != "" {
		result.Description = b.Description
	}
	result.Timeout = a.Timeout
	if b.Timeout.Duration > 0 {
		result.Timeout = b.Timeout
	}
	result.Preset = append(a.Preset, b.Preset...)
	result.Parameters = make(map[string]string)
	for k, v := range a.Parameters {
		result.Parameters[k] = v
	}
	for k, v := range b.Parameters {
		result.Parameters[k] = v
	}
	if len(result.Parameters) == 0 {
		result.Parameters = nil
	}
	// Should we return an error if a and b have conflicting execute conditions?
	result.Only = b.Only
	result.Except = b.Except
	return result
}

// split splits a function call into its name and parameters.
func split(name string) (string, string) {
	if i := strings.Index(name, "("); i > 0 {
		return name[:i], name[i+1 : len(name)-1]
	}
	return name, ""
}

// Open returns the best possible configuration using the given arguments.
//
// Without any arguments Open will open the current working directory, unless
// environment variable NTT_SOURCE_DIR is set.
//
// If you pass a manifest file as single argument, Open will use it directly.
//
// If you pass a directory as single argument, Open will first look for a
// package.yml and use it.
//
// If no package.yml is found Open will look for TTCN-3 source files. It will
// look recursively if directory contains typical project root files (i.e.
// build.sh, testcases-folder, ...). Open will also recursively load TTCN-3
// source files from typical import-directories (i.e ../common).
func Open(args ...string) (*Config, error) {
	defaults := configOptions(
		AutomaticEnv(),
		WithIndex(cache.Lookup(IndexFile)),
		WithDefaults(),
	)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Use default arguments if none are given.
	if len(args) == 0 {
		args = []string{cwd}
		if source_dir := env.Getenv("NTT_SOURCE_DIR"); source_dir != "" {
			args = []string{source_dir}
		}
	}

	// Multiple arguments are always treated as sources.
	if len(args) > 1 {
		return NewConfig(WithSources(args...), defaults)
	}

	// Treat a single file argument as source, unless it is a directory or
	// a recognised project descriptor.
	if file := args[0]; fs.IsRegular(file) {
		if filepath.Base(file) == ManifestFile {
			return NewConfig(WithManifest(file), defaults)
		}
		if strings.EqualFold(filepath.Ext(file), ".tpd") {
			return NewConfig(WithTPD(file), defaults)
		}
		return NewConfig(WithSources(file), defaults)
	}

	return NewConfig(AutomaticRoot(args[0]), defaults)
}

func NewConfig(opts ...ConfigOption) (*Config, error) {
	c := &Config{}
	if err := configOptions(opts...)(c); err != nil {
		return nil, err
	}

	if c.ParametersFile != "" {
		var pf Parameters
		b, err := fs.Content(c.ParametersFile)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(b, &pf); err != nil {
			return nil, err
		}
		c.Parameters = mergeParameters(c.Parameters, pf)
	}

	return c, nil
}

type ConfigOption func(*Config) error

// configOptions returns an options, which applies the given configuration options.
func configOptions(opts ...ConfigOption) ConfigOption {
	return func(c *Config) error {
		var gerr *multierror.Error
		for _, opt := range opts {
			if err := opt(c); err != nil {
				gerr = multierror.Append(gerr, err)
			}
		}
		return gerr.ErrorOrNil()
	}
}

// WithIndex uses hints from given index file to configure source directories.
func WithIndex(file string) ConfigOption {
	return func(c *Config) error {
		if c.Root == "" {
			return nil
		}
		b, err := fs.Content(file)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		var idx Index
		if len(b) > 0 {
			if err := json.Unmarshal(b, &idx); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}

		base := filepath.Dir(file)
		info, err := os.Stat(c.Root)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Root, err)
		}
		for _, s := range idx.Suites {
			if s.SourceDir != "" && s.RootDir != "" {
				root := fs.Real(base, s.RootDir)
				info2, err := os.Stat(root)
				if err != nil {
					return fmt.Errorf("%s: %w", root, err)
				}
				if os.SameFile(info, info2) {
					log.Debugf("project: using source dir %s\n", s.SourceDir)
					c.SourceDir = fs.Real(base, s.SourceDir)
				}
			}
		}
		return nil
	}
}

// WithManifest reads a manifest file (package.yml).
//
// expand variable references recursively. Environment variables overwrite
// variables defined in environment files. Variables defined in environment
// files overwrite variables defined in the manifest. Undefined variables won't
// be expanded and will return an error.
func WithManifest(file string) ConfigOption {
	return func(c *Config) error {
		c.ManifestFile = file
		c.Root = filepath.Dir(file)
		b, err := fs.Content(file)
		if err != nil {
			return err
		}
		if len(b) > 0 {
			if err := yaml.Unmarshal(b, &c.Manifest); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}
		c.updateVariables()
		if err := c.Variables.Expand(); err != nil {
			return err
		}
		env.ExpandAll(&c.Manifest, c.Variables)
		c.Manifest.expandPaths(c.Root)
		log.Debugf("project: using manifest %s\n", file)
		return nil
	}
}

// WithTPD reads a Titan Project Descriptor (.tpd) and uses it to seed
// the project configuration. We translate the XML into the same fields
// WithManifest would have populated, so downstream code (lookup, lint,
// LSP) doesn't need to know which format the user actually has on disk.
//
// Referenced sub-projects are loaded transitively and their sources +
// import directories are merged into the top-level Config. We dedupe
// paths so a diamond reference graph still produces a sane Sources
// list.
//
// The descriptor's root (the directory containing the .tpd file)
// becomes Config.Root, mirroring WithManifest's behaviour.
func WithTPD(file string) ConfigOption {
	return func(c *Config) error {
		root, sources, imports, err := loadTPD(file)
		if err != nil {
			return err
		}
		c.Root = root
		c.Sources = append(c.Sources, sources...)
		c.Imports = append(c.Imports, imports...)
		log.Debugf("project: loaded titan descriptor %s (%d sources, %d imports)\n",
			file, len(sources), len(imports))
		return nil
	}
}

// loadTPD parses descriptor (and any descriptors it references) and
// returns the resolved root + flattened sources + imports.
func loadTPD(descriptor string) (root string, sources, imports []string, err error) {
	seen := map[string]bool{}
	var visit func(path string) error
	visit = func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if seen[abs] {
			return nil
		}
		seen[abs] = true

		proj, err := titan.Load(abs)
		if err != nil {
			return err
		}
		// The first descriptor visited defines the project root.
		if root == "" {
			root = proj.Root
		}
		sources = append(sources, proj.Sources...)
		imports = append(imports, proj.ImportDirs...)
		for _, ref := range proj.References {
			// Resolve relative refs against the *current*
			// descriptor's root, not the top-level project.
			refPath := ref.Path
			if !filepath.IsAbs(refPath) {
				refPath = filepath.Join(proj.Root, refPath)
			}
			if err := visit(refPath); err != nil {
				log.Debugf("project: skipping unresolvable .tpd reference %q: %s\n", refPath, err.Error())
				continue
			}
		}
		return nil
	}
	if err := visit(descriptor); err != nil {
		return "", nil, nil, err
	}
	sources = dedupePaths(sources)
	imports = dedupePaths(imports)
	return root, sources, imports, nil
}

func dedupePaths(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, p := range in {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// WithRoot sets the root directory of the project.
func WithRoot(root string) ConfigOption {
	return func(c *Config) error {
		c.Root = root
		return nil
	}
}

// WithSourceDir sets the source directory of the project.
func WithSourceDir(dir string) ConfigOption {
	return func(c *Config) error {
		c.SourceDir = dir
		return nil
	}
}

// WithSources sets the sources of the project.
func WithSources(srcs ...string) ConfigOption {
	return func(c *Config) error {
		c.Sources = srcs
		return nil
	}
}

// WithImports sets the imports of the project.
func WithImports(dirs ...string) ConfigOption {
	return func(c *Config) error {
		c.Imports = dirs
		return nil
	}
}

// AutomaticRoot sets the root directory and automatically sets the source and
// imports directories.
//
// If the root directory contains a package.yml, sources and imports are set
// from the manifest exclusively.
//
// AutomaticRoot will load TTCN-3 source files recursively, if the root
// directory contains typical project root files (e.g. build.sh, testcases/,
// ...).
//
// Additionally AutomaticRoot finds common import directories relative to the root folder
// (i.e ../library or ../common) and recursively adds all folders containing
// TTCN-3 source files.
func AutomaticRoot(root string) ConfigOption {
	return func(c *Config) error {
		c.Root = root
		log.Debugf("project: root %s\n", root)
		if manifest := fs.JoinPath(root, ManifestFile); fs.IsRegular(manifest) {
			return WithManifest(manifest)(c)
		}
		// Fall back to Titan project descriptors when there is no
		// package.yml. We deliberately pick the first .tpd in
		// alphabetical order so the discovery is deterministic.
		if tpds := fs.Glob(fs.JoinPath(root, "*.tpd")); len(tpds) > 0 {
			return WithTPD(tpds[0])(c)
		}

		if isRoot(c.Root) {
			log.Debugln("project: scanning recursively...")
			c.Sources = fs.FindTTCN3FilesRecursive(c.Root)
		} else {
			c.Sources = fs.FindTTCN3Files(c.Root)
		}
		s := fmt.Sprintf("%v", c.Sources)
		if len(s) > 200 {
			s = s[:200] + fmt.Sprintf("...] (%d files/directories)", len(c.Sources))
		}
		log.Debugf("project: use sources: %s\n", s)

		commonDirs := []string{
			"../../../sct",
			"../../../../sct",
			"../common",
			"../Common",
			"../library",
			"../../../../Common",
		}
		for _, dir := range commonDirs {
			path := fs.JoinPath(c.Root, dir)
			if eval, err := filepath.EvalSymlinks(path); err == nil {
				path = eval
			}

			c.Imports = append(c.Imports, fs.FindTTCN3DirectoriesRecursive(path)...)
		}
		s = fmt.Sprintf("%v", c.Imports)
		if len(s) > 200 {
			s = s[:200] + fmt.Sprintf("...] (%d directories)", len(c.Imports))
		}
		log.Debugf("project: use imports: %s\n", s)

		return nil
	}
}

// AutomaticEnv let environment variables with NTT_ prefix overwrite
// configuration. Currently supported are: NTT_NAME, NTT_SOURCES, NTT_IMPORTS,
// NTT_PARAMETERS_FILE, NTT_HOOKS_FILE, NTT_LINT_FILE, NTT_TIMEOUT.
func AutomaticEnv() ConfigOption {
	return func(c *Config) error {
		for k, v := range env.EnvironMap() {
			used := true
			switch k {
			case "NTT_NAME":
				c.Name = v
			case "NTT_SOURCES":
				c.Sources = strings.Split(v, string(os.PathListSeparator))
			case "NTT_IMPORTS":
				c.Imports = strings.Split(v, string(os.PathListSeparator))
			case "NTT_PARAMETERS_FILE":
				c.ParametersFile = v
			case "NTT_HOOKS_FILE":
				c.HooksFile = v
			case "NTT_LINT_FILE":
				c.LintFile = v
			case "NTT_TIMEOUT":
				if err := c.Timeout.UnmarshalText([]byte(v)); err != nil {
					return fmt.Errorf("environment variable %s: %w", k, err)
				}
			default:
				used = false
			}
			if used {
				log.Debugf("project: using environment variable %s=%s\n", k, v)
			}
		}
		return nil
	}
}

// WithDefaults initializes Root, SourceDir, Name, ParametersFile and HooksFile.
func WithDefaults() ConfigOption {
	return func(c *Config) error {
		if c.Root == "" {
			switch {
			case c.SourceDir != "":
				c.Root = c.SourceDir
			case len(c.Sources) > 0:
				c.Root = filepath.Dir(c.Sources[0])
				// When there's no root, but only source, we want the suite to be named after the source.
				n, err := NameFromURI(c.Sources[0])
				if err != nil {
					return err
				}
				c.Name = n
			default:
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				c.Root = cwd
			}
			log.Debugf("project: using default root %s\n", c.Root)
		}
		if c.SourceDir == "" {
			c.SourceDir = c.Root
			log.Debugf("project: using default source dir %s\n", c.SourceDir)
		}
		if c.Name == "" {
			n, err := NameFromURI(c.Root)
			if err != nil {
				return err
			}
			c.Name = n
			log.Debugf("project: using default name %s\n", c.Name)
		}
		defaultFile := func(name string) string {
			if path := cache.Lookup(name); fs.IsRegular(path) {
				return path
			}
			if path := fs.JoinPath(c.Root, name); fs.IsRegular(path) {
				return path
			}
			if path := fs.JoinPath(c.SourceDir, name); fs.IsRegular(path) {
				return path
			}
			return ""
		}
		if c.ParametersFile == "" {
			if path := defaultFile(fmt.Sprintf("%s.parameters", c.Name)); path != "" {
				c.ParametersFile = path
				log.Debugf("project: using parameters file %s\n", c.ParametersFile)
			}
		}
		if c.HooksFile == "" {
			if path := defaultFile(fmt.Sprintf("%s.hooks", c.Name)); path != "" {
				c.HooksFile = path
				log.Debugf("project: using hooks file %s\n", c.HooksFile)
			}
		}
		if c.LintFile == "" {
			if path := defaultFile("ntt-lint.yml"); path != "" {
				c.LintFile = path
				log.Debugf("project: using lint file %s\n", c.LintFile)
			}
		}

		// Use existing test results file if available. Alternatively,
		// use the directory of the index file (usually the
		// CMAKE_BINARY_DIR). Otherwise, use the default name.
		if path := cache.Lookup(results.Filename); fs.IsRegular(path) {
			c.ResultsFile = path
		} else if index := cache.Lookup(IndexFile); fs.IsRegular(index) {
			c.ResultsFile = fs.JoinPath(filepath.Dir(index), results.Filename)
		} else {
			c.ResultsFile = results.Filename
		}

		c.updateVariables()
		return nil
	}
}

// isRoot returns true if the given path contains typical project root files.
func isRoot(root string) bool {
	patterns := []string{
		"testcases/*",
		"*_Testsuite_*.ttcn*",
		"test_purposes/*.tplan2",
		"PicsPixit/*.ttcn*",
		ManifestFile,
		"build.sh",
		"project.xml",
		"*.cfg",
		"*.parameters",
		".git",
		"sct",
	}
	return match(root, patterns...)
}

// updateVariables updates the given variable with the variables from
// environment files. Environment variables override environment files.
// Environment files overwrite manifest variables.
func (m *Manifest) updateVariables() {
	if m.Variables == nil {
		m.Variables = make(map[string]string)
	}
	for k, v := range env.ParseFiles() {
		if s, ok := env.LookupEnv(k); ok {
			v = s
		}
		m.Variables[k] = v
	}
	for k, v := range env.EnvironMap() {
		if strings.HasPrefix(k, "NTT_") || strings.HasPrefix(k, "K3_") || strings.HasPrefix(k, "SCT_") {
			m.Variables[k] = v
		}
	}

	if len(m.Variables) == 0 {
		m.Variables = nil
	}
}

func (m *Manifest) expandPaths(base string) {
	for i, src := range m.Sources {
		m.Sources[i] = fs.Real(base, src)
	}
	for i, imp := range m.Imports {
		m.Imports[i] = fs.Real(base, imp)
	}
	m.HooksFile = fs.Real(base, m.HooksFile)
	m.ParametersFile = fs.Real(base, m.ParametersFile)
	m.LintFile = fs.Real(base, m.LintFile)
}
