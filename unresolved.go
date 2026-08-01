package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// unresolved is one module that resolved to nothing, and the modules importing
// it.
type unresolved struct {
	name      string
	importers []string
	// hard is set when at least one importer needs the module at load time. A
	// module imported normally by one module and delay-loaded by another counts
	// as hard: the strictest edge decides, because that is the one that stops
	// the image from starting.
	hard bool
}

// label names the finding the way the rest of the tool names dependencies.
// Only a module every importer delay-loads is marked (delay).
func (u unresolved) label() string {
	if u.hard {
		return u.name
	}
	return u.name + " (delay)"
}

// unresolvedDeps walks the dependency graph to full depth, expanding each
// resolvable module once, and collects everything that resolved to nothing.
//
// API set contracts are not collected: they have no file on disk by design and
// the loader satisfies them through its schema, so reporting them would bury
// the real findings.
func unresolvedDeps(rootPath string, deps []dependency, resolve resolver) []unresolved {
	// Keyed on the resolved path, so each module is expanded once and cycles
	// terminate — the same rule the printed tree uses.
	seen := map[string]bool{strings.ToLower(rootPath): true}

	// Findings are keyed by name: an unresolved module has no path to key on.
	found := map[string]*unresolved{}

	var walk func(deps []dependency, importer string)
	walk = func(deps []dependency, importer string) {
		for _, dep := range deps {
			if dep.found {
				key := strings.ToLower(dep.path)
				if seen[key] {
					continue
				}
				seen[key] = true

				children, err := resolve(dep.path)
				if err != nil {
					// An unreadable module is a different problem, and it was
					// resolved: it is not reported here.
					continue
				}
				walk(children, filepath.Base(dep.path))
				continue
			}

			if dep.virtual {
				continue
			}

			key := strings.ToLower(dep.name)
			hit := found[key]
			if hit == nil {
				hit = &unresolved{name: dep.name}
				found[key] = hit
			}
			if !dep.delayed {
				hit.hard = true
			}
			if !slices.Contains(hit.importers, importer) {
				hit.importers = append(hit.importers, importer)
			}
		}
	}

	walk(deps, filepath.Base(rootPath))

	result := make([]unresolved, 0, len(found))
	for _, hit := range found {
		slices.Sort(hit.importers)
		result = append(result, *hit)
	}
	// Map iteration is random; sort so the report is stable run to run.
	slices.SortFunc(result, func(a, b unresolved) int {
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})

	return result
}

// importerList names the modules that import a finding, keeping the line short
// when many of them do.
func (u unresolved) importerList() string {
	const max = 3
	if len(u.importers) <= max {
		return strings.Join(u.importers, ", ")
	}
	return fmt.Sprintf("%s, +%d more",
		strings.Join(u.importers[:max], ", "), len(u.importers)-max)
}
