package ttcn3

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// DB implements database for querying TTCN-3 source code bases.
type DB struct {
	// Names is a map from symbol name to a list of files that define the symbol.
	Names map[string]map[string]bool

	// Uses maps from symbol name to a list of files that use the symbol.
	Uses map[string]map[string]bool

	// Modules maps from module name to file path.
	Modules map[string]map[string]bool

	mu sync.Mutex
}

// Index parses TTCN-3 source files and adds names and dependencies to the database.
func (db *DB) Index(files ...string) {
	if db.Names == nil {
		db.Names = make(map[string]map[string]bool)
	}
	if db.Uses == nil {
		db.Uses = make(map[string]map[string]bool)
	}
	if db.Modules == nil {
		db.Modules = make(map[string]map[string]bool)
	}

	var (
		syms  int
		wg    sync.WaitGroup
		start = time.Now()
	)
	wg.Add(len(files))
	for _, path := range files {
		go func(path string) {
			defer wg.Done()
			if isASN1File(path) {
				// ASN.1 files don't go through the TTCN-3
				// parser. We index them as plain modules so
				// "import from XYZ all" from a TTCN-3 file can
				// resolve to the ASN.1 source.
				mod, err := asn1.ParseFile(path)
				if err != nil || mod == nil || mod.Name == "" {
					return
				}
				db.mu.Lock()
				db.addModule(path, mod.Name)
				for _, a := range mod.Assignments {
					syms++
					db.addDefinition(path, a.Name)
				}
				db.mu.Unlock()
				return
			}
			tree := ParseFile(path)
			if tree.Root != nil {
				db.mu.Lock()
				for _, n := range tree.Modules() {
					syms++
					db.addModule(path, syntax.Name(n.Node))
				}

				for k := range tree.Names {
					syms++
					db.addDefinition(path, k)
				}
				for k := range tree.Uses {
					syms++
					db.addRef(path, k)
				}
				db.mu.Unlock()
			}
		}(path)
	}
	wg.Wait()
	log.Debugf("Cache built in %v: %d symbols in %d files.\n", time.Since(start), syms, len(files))
}

// VisibleModules returns a list of modules that may contain the given
// symbol. First parameter id specifies the symbol to look for and second
// parameter module specifies where the imports come from.
func (db *DB) VisibleModules(id string, mod *syntax.Module) []*Node {
	importedModules := make(map[string]bool)
	importedFiles := make(map[string]bool)

	addImport := func(moduleName string) {
		importedModules[moduleName] = true
		for file := range db.Modules[moduleName] {
			importedFiles[file] = true
		}
	}

	// TTCN-3 standard requires, that all global definition may have a
	// module prefix. We handle this by "self-importing" the current
	// module.
	addImport(syntax.Name(mod))

	// Only use imports from the current module.
	mod.Inspect(func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.ImportDecl:
			addImport(syntax.Name(n.Module))
			return false
		case *syntax.Module, *syntax.ModuleDef:
			return true
		default:
			return false

		}
	})

	// Find all files that contain the symbol.
	var candidates []string
	for file := range db.Names[id] {
		if importedFiles[file] {

			candidates = append(candidates, file)
		}
	}

	// Parse all imported modules that contain the symbol.
	var mods []*Node
	for _, file := range candidates {
		tree := ParseFile(file)
		for _, m := range tree.Modules() {
			if importedModules[syntax.Name(m.Node)] {
				mods = append(mods, m)
			}
		}
	}
	return mods
}

func (db *DB) addModule(file string, name string) {
	if db.Modules[name] == nil {
		db.Modules[name] = make(map[string]bool)
	}
	db.Modules[name][file] = true
}

func (db *DB) addDefinition(file string, name string) {
	if db.Names[name] == nil {
		db.Names[name] = make(map[string]bool)
	}
	db.Names[name][file] = true
}

func (db *DB) addRef(file string, name string) {
	if db.Uses[name] == nil {
		db.Uses[name] = make(map[string]bool)
	}
	db.Uses[name][file] = true
}

// isASN1File reports whether path looks like an ASN.1 source file based
// on its extension. We deliberately keep this lookup case-insensitive
// because ASN.1 specs (and the editor extensions that ship them) are
// inconsistent about casing.
func isASN1File(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".asn" || ext == ".asn1"
}

// ASN1Location locates an ASN.1 assignment definition by name and
// returns its byte offset within the source file. The result is
// suitable for editor "go to definition" jumps from TTCN-3 callers
// that imported the ASN.1 module.
//
// We return (file, offset, ok). The caller is responsible for any
// LSP-specific line/column conversion via fs.Open(file) - this keeps
// db.go free of LSP imports.
func (db *DB) ASN1Location(symbol string) (file string, offset int, ok bool) {
	for path := range db.Names[symbol] {
		if !isASN1File(path) {
			continue
		}
		// Reparse the file using the full frontend so we get the
		// byte-precise position of the named assignment. The
		// underlying parser is cheap enough that re-parsing on every
		// jump is fine for editor workloads.
		mod, err := asn1.ParseFileFull(path)
		if err != nil || mod == nil {
			continue
		}
		for _, a := range mod.Assignments {
			if asn1.AssignmentName(a) == symbol {
				return path, a.Pos(), true
			}
		}
	}
	return "", 0, false
}
