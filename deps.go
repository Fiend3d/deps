package main

import (
	"fmt"
	"strings"

	"github.com/saferwall/pe"
)

// dependency is one resolved import of an image.
type dependency struct {
	name    string
	path    string
	found   bool
	delayed bool
	// virtual marks an API set contract that has no file on disk; the loader
	// still satisfies it, so it is not a missing dependency.
	virtual   bool
	functions []string
}

// resolver yields the direct dependencies of the image at path. The tree takes
// its resolver as a value so tests can supply a graph without touching disk.
type resolver func(path string) ([]dependency, error)

// searchPath resolves dependency names against a fixed loader search path,
// remembering answers. A recursive walk asks for the same few dozen system DLLs
// thousands of times over, and every miss costs a stat in each directory.
type searchPath struct {
	dirs []string
	// systemCount marks how many of dirs are the image's own directory and the
	// Windows system directories, the rest being PATH.
	systemCount int
	cache       map[string]cachedLookup
}

type cachedLookup struct {
	path  string
	found bool
}

func newSearchPath(dirs []string, systemCount int) *searchPath {
	return &searchPath{
		dirs:        dirs,
		systemCount: systemCount,
		cache:       make(map[string]cachedLookup),
	}
}

func (s *searchPath) find(name string) (string, bool) {
	key := strings.ToLower(name)
	if hit, ok := s.cache[key]; ok {
		return hit.path, hit.found
	}

	dirs := s.dirs
	if isAPISet(name) {
		// API set contracts are resolved through the loader's schema, not by
		// scanning PATH. Some do exist in the system directories as downlevel
		// shims, but a same-named copy sitting in an unrelated PATH entry (an
		// SDK install, say) is not what would ever be loaded — and treating it
		// as a hit would attribute a whole wrong subtree to it.
		dirs = s.dirs[:s.systemCount]
	}

	path, found := findDependency(name, dirs)
	s.cache[key] = cachedLookup{path: path, found: found}

	return path, found
}

// resolveDeps parses the image at path and resolves its imports, delay-load
// included, against dirs. The image is closed before returning; every string
// handed back is a copy and outlives the mapping.
func resolveDeps(path string, search *searchPath) ([]dependency, error) {
	f, err := parseImports(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return depsFromFile(f, search), nil
}

// depsFromFile resolves the imports of an already-parsed image, for callers
// that need the *pe.File for other things too and should not parse it twice.
func depsFromFile(f *pe.File, search *searchPath) []dependency {
	var deps []dependency

	add := func(name string, functions []pe.ImportFunction, delayed bool) {
		resolved, found := search.find(name)
		dep := dependency{
			name:      name,
			path:      resolved,
			found:     found,
			delayed:   delayed,
			virtual:   !found && isAPISet(name),
			functions: make([]string, len(functions)),
		}
		for i, fn := range functions {
			if fn.ByOrdinal {
				dep.functions[i] = fmt.Sprintf("ordinal:%d", fn.Ordinal)
			} else {
				dep.functions[i] = fn.Name
			}
		}
		deps = append(deps, dep)
	}

	if f.HasImport {
		for _, imp := range f.Imports {
			add(imp.Name, imp.Functions, false)
		}
	}
	if f.HasDelayImp {
		for _, imp := range f.DelayImports {
			add(imp.Name, imp.Functions, true)
		}
	}

	return deps
}

// label renders a dependency the way both the tree and the printed output name
// it, so the two always agree.
func (d dependency) label() string {
	label := d.name
	if d.delayed {
		label += " (delay)"
	}
	if d.virtual {
		label += " (api-set)"
	}
	return label
}
