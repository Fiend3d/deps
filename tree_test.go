package main

import (
	"errors"
	"strings"
	"testing"
)

// fakeGraph is a dependency graph held in memory, so tree tests never touch
// System32. Keys and dependency paths are the module "paths".
type fakeGraph struct {
	edges map[string][]string
	// deps overrides edges for a module, for tests that need dependencies which
	// are missing, delay-loaded, or api-set contracts rather than plain
	// resolvable ones.
	deps  map[string][]dependency
	fail  map[string]bool
	calls map[string]int
}

func newFakeGraph(edges map[string][]string) *fakeGraph {
	return &fakeGraph{
		edges: edges,
		deps:  map[string][]dependency{},
		fail:  map[string]bool{},
		calls: map[string]int{},
	}
}

func (g *fakeGraph) resolve(path string) ([]dependency, error) {
	g.calls[path]++
	if g.fail[path] {
		return nil, errors.New("failed to parse PE: fake")
	}

	if deps, ok := g.deps[path]; ok {
		return deps, nil
	}

	deps := make([]dependency, 0, len(g.edges[path]))
	for _, child := range g.edges[path] {
		deps = append(deps, dependency{name: child, path: child, found: true})
	}
	return deps, nil
}

// roots builds tree roots for the named modules, as newTree would.
func (g *fakeGraph) roots(rootPath string, names ...string) []*treeNode {
	deps := make([]dependency, 0, len(names))
	for _, name := range names {
		deps = append(deps, dependency{name: name, path: name, found: true})
	}
	return newTree(deps, rootPath)
}

// names renders the flattened rows as "indent+name", the shape the view draws.
func names(visible []*treeNode) []string {
	out := make([]string, 0, len(visible))
	for _, node := range visible {
		out = append(out, strings.Repeat("  ", node.depth)+node.dep.name)
	}
	return out
}

func TestFlattenTree(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"c.dll", "d.dll"},
	})
	roots := g.roots("root.exe", "a.dll", "b.dll")

	if got := names(flattenTree(roots)); !equal(got, []string{"a.dll", "b.dll"}) {
		t.Fatalf("collapsed = %v", got)
	}

	roots[0].toggle(g.resolve)
	want := []string{"a.dll", "  c.dll", "  d.dll", "b.dll"}
	if got := names(flattenTree(roots)); !equal(got, want) {
		t.Errorf("expanded = %v, want %v", got, want)
	}

	roots[0].toggle(g.resolve)
	if got := names(flattenTree(roots)); !equal(got, []string{"a.dll", "b.dll"}) {
		t.Errorf("collapsed again = %v", got)
	}
}

// Nothing below an unopened node may be parsed; that is the whole point of the
// tree being lazy.
func TestExpansionIsLazy(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"c.dll"},
		"b.dll": {"d.dll"},
		"c.dll": {"e.dll"},
	})
	roots := g.roots("root.exe", "a.dll", "b.dll")

	if len(g.calls) != 0 {
		t.Fatalf("building the tree resolved %d modules, want 0", len(g.calls))
	}

	roots[0].toggle(g.resolve)

	if g.calls["a.dll"] != 1 {
		t.Errorf("a.dll resolved %d times, want 1", g.calls["a.dll"])
	}
	for _, unopened := range []string{"b.dll", "c.dll"} {
		if g.calls[unopened] != 0 {
			t.Errorf("%s was resolved but never opened", unopened)
		}
	}

	// Re-opening a node must reuse the children it already has.
	roots[0].toggle(g.resolve)
	roots[0].toggle(g.resolve)
	if g.calls["a.dll"] != 1 {
		t.Errorf("a.dll re-resolved on reopen: %d calls", g.calls["a.dll"])
	}
}

// a -> b -> a must terminate: the repeat is marked and cannot be opened.
func TestCycleIsMarkedAndNotExpandable(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"b.dll"},
		"b.dll": {"a.dll"},
	})
	roots := g.roots("root.exe", "a.dll")

	roots[0].toggle(g.resolve)
	b := roots[0].children[0]
	b.toggle(g.resolve)

	backToA := b.children[0]
	if !backToA.cycle {
		t.Fatal("a.dll under b.dll should be marked as a cycle")
	}
	if backToA.expandable() {
		t.Error("a cycle must not be expandable")
	}

	backToA.toggle(g.resolve)
	if backToA.expanded {
		t.Error("toggling a cycle expanded it anyway")
	}
}

// A dependency pointing back at the image the walk started from is a cycle too.
func TestCycleBackToRootImage(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"root.exe"}})
	roots := g.roots("root.exe", "a.dll")

	roots[0].toggle(g.resolve)

	if !roots[0].children[0].cycle {
		t.Error("a dependency on the root image should be marked as a cycle")
	}
}

func TestCycleDetectionIsCaseInsensitive(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"B.DLL"},
		"B.DLL": {"b.dll"},
	})
	roots := g.roots("root.exe", "a.dll")

	roots[0].toggle(g.resolve)
	upper := roots[0].children[0]
	upper.toggle(g.resolve)

	if !upper.children[0].cycle {
		t.Error("b.dll under B.DLL should be a cycle: Windows paths are case-insensitive")
	}
}

func TestExpandRecordsLoadError(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"b.dll"}})
	g.fail["a.dll"] = true
	roots := g.roots("root.exe", "a.dll")

	roots[0].toggle(g.resolve)

	if roots[0].loadErr == "" {
		t.Error("a module that failed to parse should record the failure")
	}
	if roots[0].expanded {
		t.Error("a module that failed to parse must not appear expanded")
	}
	if got := roots[0].marker(); got != " (unreadable)" {
		t.Errorf("marker = %q, want \" (unreadable)\"", got)
	}
}

// A module with no file on disk has nothing to open.
func TestUnresolvedIsNotExpandable(t *testing.T) {
	node := &treeNode{dep: dependency{name: "api-ms-win-x.dll", virtual: true}}
	if node.expandable() {
		t.Error("a module with no path must not be expandable")
	}
}

func TestPrefixDrawsBranches(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"c.dll", "d.dll"},
	})
	roots := g.roots("root.exe", "a.dll", "b.dll")
	roots[0].toggle(g.resolve)

	if got := roots[0].prefix(); got != "" {
		t.Errorf("top-level prefix = %q, want empty", got)
	}
	if got := roots[0].children[0].prefix(); got != "├─ " {
		t.Errorf("first child prefix = %q, want %q", got, "├─ ")
	}
	if got := roots[0].children[1].prefix(); got != "└─ " {
		t.Errorf("last child prefix = %q, want %q", got, "└─ ")
	}
}

func TestTreeText(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll"}})
	roots := g.roots("root.exe", "a.dll", "b.dll")
	roots[0].toggle(g.resolve)

	want := "a.dll\n  c.dll\nb.dll"
	if got := treeText(flattenTree(roots)); got != want {
		t.Errorf("treeText() = %q, want %q", got, want)
	}
}

// newTreeModel wires a model to a fake graph, as initModel would to the real
// resolver.
func newTreeModel(g *fakeGraph, rootNames ...string) model {
	m := model{mode: treeMode, width: 80, height: 12, resolve: g.resolve}
	m.roots = g.roots("root.exe", rootNames...)
	m.visible = flattenTree(m.roots)
	return m
}

func TestModelToggleNode(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll", "d.dll"}})
	m := newTreeModel(g, "a.dll", "b.dll")

	if m.length() != 2 {
		t.Fatalf("length() = %d, want 2", m.length())
	}

	m.toggleNode() // cursor is on a.dll

	if m.length() != 4 {
		t.Errorf("length() = %d after expanding, want 4", m.length())
	}
	if got := names(m.visible); !equal(got, []string{"a.dll", "  c.dll", "  d.dll", "b.dll"}) {
		t.Errorf("visible = %v", got)
	}
}

// Collapsing a node the cursor sits below must not leave the cursor past the
// end of the shortened list.
func TestModelCollapseKeepsCursorInRange(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll", "d.dll", "e.dll"}})
	m := newTreeModel(g, "a.dll")

	m.toggleNode()
	m.cursor = m.length() - 1 // on the last child

	// Collapsing the root removes every child under the cursor.
	m.cursor = 0
	m.toggleNode()

	if m.cursor >= m.length() {
		t.Errorf("cursor %d out of range for length %d", m.cursor, m.length())
	}
	if m.length() != 1 {
		t.Errorf("length() = %d, want 1", m.length())
	}
}

func TestModelCollapseNodeMovesToParent(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll"}})
	m := newTreeModel(g, "a.dll")

	m.toggleNode()
	m.cursor = 1 // on c.dll, which is collapsed already

	m.collapseNode()

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0: collapsing a closed node moves to its parent", m.cursor)
	}
}

func TestModelSelectedDepInTreeMode(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll"}})
	m := newTreeModel(g, "a.dll")
	m.toggleNode()
	m.cursor = 1

	dep, ok := m.selectedDep()
	if !ok || dep.name != "c.dll" {
		t.Errorf("selectedDep() = %+v, %v; want c.dll", dep, ok)
	}
}

// A failed expansion surfaces in the status line rather than killing the app.
func TestModelToggleReportsLoadError(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {}})
	g.fail["a.dll"] = true
	m := newTreeModel(g, "a.dll")

	m.toggleNode()

	if m.status == "" {
		t.Error("a failed expansion should leave a status message")
	}
	if m.length() != 1 {
		t.Errorf("length() = %d, want 1: nothing should have been added", m.length())
	}
}

func TestTreeModeRendersWithoutPanic(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"c.dll", "d.dll"}})
	m := newTreeModel(g, "a.dll", "b.dll")
	m.toggleNode()
	m.filePath = `C:\app\app.exe`

	for _, size := range []struct{ width, height int }{{0, 0}, {3, 3}, {20, 10}, {200, 60}} {
		m.width, m.height = size.width, size.height
		m.updateStart()

		view := m.View() // must not panic

		if size.height >= 3 {
			if got, want := strings.Count(view.Content, "\n"), size.height-1; got != want {
				t.Errorf("at %dx%d: %d newlines, want %d", size.width, size.height, got, want)
			}
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
