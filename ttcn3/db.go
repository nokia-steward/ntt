package ttcn3

import (
	"sync"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// DB implements database for querying TTCN-3 source code bases.
type DB struct {
	// Names maps from symbol name to the set of files that define the symbol.
	Names map[string]map[string]bool

	// Uses maps from symbol name to the set of files that use the symbol.
	Uses map[string]map[string]bool

	// Modules maps from module name to the set of files that define the module.
	Modules map[string]map[string]bool

	mu sync.Mutex
}

// Index parses TTCN-3 source files and adds names and dependencies to the database.
func (db *DB) Index(files ...string) {
	var (
		syms  int
		wg    sync.WaitGroup
		start = time.Now()
	)
	wg.Add(len(files))
	for _, path := range files {
		go func(path string) {
			defer wg.Done()
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
// parameter mod specifies where the imports come from.
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
	if db.Modules == nil {
		db.Modules = make(map[string]map[string]bool)
	}
	if db.Modules[name] == nil {
		db.Modules[name] = make(map[string]bool)
	}
	db.Modules[name][file] = true
}

func (db *DB) addDefinition(file string, name string) {
	if db.Names == nil {
		db.Names = make(map[string]map[string]bool)
	}
	if db.Names[name] == nil {
		db.Names[name] = make(map[string]bool)
	}
	db.Names[name][file] = true
}

func (db *DB) addRef(file string, name string) {
	if db.Uses == nil {
		db.Uses = make(map[string]map[string]bool)
	}
	if db.Uses[name] == nil {
		db.Uses[name] = make(map[string]bool)
	}
	db.Uses[name][file] = true
}
