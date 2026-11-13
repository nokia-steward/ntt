// Package env provides functionality to dynamically load the environment variables
//
// Note: This is just a modified copy and wrapper around the gotenv package by suboisto
package env

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/cache"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
)

var (
	ErrUnknownVariable = fmt.Errorf("unknown variable")
	ErrCyclicVariable  = fmt.Errorf("cyclic variable")
)

var knownVars = map[string]bool{
	"PATH":                true,
	"CFLAGS":              true,
	"CPPFLAGS":            true,
	"CXXFLAGS":            true,
	"K3CFLAGS":            true,
	"K3RFLAGS":            true,
	"LDFLAGS":             true,
	"LD_LIBRARY_PATH":     true,
	"NTT_IMPORTS":         true,
	"NTT_NAME":            true,
	"NTT_PARAMETERS_FILE": true,
	"NTT_SOURCES":         true,
	"NTT_SOURCE_DIR":      true,
	"NTT_TEST_HOOK":       true,
	"NTT_TIMEOUT":         true,
	"NTT_VARIABLES":       true,
}

// Slice returns a sorted string slice of the variables.
func (env Env) Slice() []string {
	var s []string
	for k, env := range env {
		s = append(s, fmt.Sprintf("%s=%s", k, env))
	}
	sort.Strings(s)
	return s
}

// Expand expands variable references recursively.
func Expand(v string, env Env) (string, error) {
	var (
		err     error
		expand  func(string) string
		visited = make(map[string]bool)
	)

	expand = func(name string) string {
		if s, ok := LookupEnv(name); ok {
			return s
		}

		v, ok := env[name]
		if !ok {
			if knownVars[name] || strings.HasPrefix(name, "NTT_") || strings.HasPrefix(name, "K3_") || strings.HasPrefix(name, "SCT_") {
				return ""
			}
			if err == nil {
				err = fmt.Errorf("%w: %s", ErrUnknownVariable, name)
			}
			return fmt.Sprintf("${%s}", name)
		}
		if !visited[name] {
			visited[name] = true
			v = os.Expand(v, expand)
			if err == nil {
				env[name] = v
			}
			return v

		}
		if err == nil {
			err = fmt.Errorf("%w: %s", ErrCyclicVariable, name)
		}
		return fmt.Sprintf("${%s}", name)
	}

	return os.Expand(v, expand), err
}

// Expand variable references recursively. Environment variables overwrite
// variables defined in environment files. Undefined variables won't
// be expanded and will return an error.
func (env Env) Expand() error {
	var gerr error
	for k, v := range env {
		v, err := Expand(v, env)
		if err == nil {
			env[k] = v
		}
		if gerr == nil {
			gerr = err
		}

	}
	return gerr
}

// EnvironMap returns the current process's environment as a string-map.
// It also replaces K3-prefixes with NTT-prefixes.
func EnvironMap() Env {
	env := make(Env)
	for _, kv := range os.Environ() {
		kv := strings.SplitN(kv, "=", 2)
		k, v := kv[0], kv[1]

		// Nokia uses environment variables with K3-Prefix all over the place.
		if strings.HasPrefix(k, "K3_") {
			k = strings.Replace(k, "K3_", "NTT_", 1)
			v = Getenv(k)
		}
		env[k] = v
	}
	return env
}

// Environ returns the current process's environment as a string-slice of the form key=value.
// It also replaces K3-prefixes with NTT-prefixes.
func Environ() []string {
	return EnvironMap().Slice()
}

var Files = []string{"ntt.env", "k3.env"}

// LoadFiles environment files ntt.env and k3.env
func LoadFiles(files ...string) {
	for k, v := range ParseFiles(files...) {
		if _, ok := os.LookupEnv(k); !ok {
			os.Setenv(k, v)
		}
	}
}

// ParseFiles environment files ntt.env and k3.env
func ParseFiles(files ...string) Env {
	if len(files) == 0 {
		files = Files
	}

	var env Env
	for _, path := range Files {
		f := fs.Open(cache.Lookup(path))
		b, err := f.Bytes()
		if err != nil {
			continue
		}
		log.Debugf("found environment file: %s\n", f.Path())
		e, err := StrictParse(bytes.NewReader(b))
		if err != nil {
			log.Verbosef("error parsing %q: %s\n", path, err.Error())
		}
		for k, v := range e {
			if _, ok := env[k]; !ok {
				if env == nil {
					env = make(map[string]string)
				}
				env[k] = v
			}
		}
	}
	return env
}

// Lookup key in process environment. Is key begins with "NTT_" also lookup key
// with "K3_" prefix.
func Getenv(key string) string {
	if env, ok := LookupEnv(key); ok {
		return env
	}
	return ""
}

// LookupEnv is like Getenv, but returns a true boolean when key exists
func LookupEnv(key string) (string, bool) {
	if env, ok := os.LookupEnv(key); ok {
		return env, ok
	}
	if strings.HasPrefix(key, "NTT") {
		return os.LookupEnv(strings.Replace(key, "NTT", "K3", 1))
	}
	return "", false
}

func ExpandAll(v interface{}, env Env) error {
	expandAll(reflect.ValueOf(v), env)
	return nil
}

func expandAll(v reflect.Value, env Env) {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsValid() {
			expandAll(v.Elem(), env)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			expandAll(v.Field(i), env)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			val := v.MapIndex(k)
			if s, ok := val.Interface().(string); ok {
				val := reflect.ValueOf(&s)
				expandAll(val, env)
				v.SetMapIndex(k, reflect.ValueOf(s))
			} else {
				expandAll(val, env)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			expandAll(v.Index(i), env)
		}
	case reflect.Interface:
		if !v.IsNil() {
			expandAll(v.Elem(), env)
		}
	case reflect.String:
		if v.CanSet() {
			s, _ := Expand(v.String(), env)
			v.SetString(s)
		}
	}
}
