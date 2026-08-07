package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// foundDep builds a resolvable dependency; missingDep and delayedDep build the
// two kinds of unresolved one, and apiSetDep a contract that only looks missing.
func foundDep(name string) dependency {
	return dependency{name: name, path: name, found: true}
}

func missingDep(name string) dependency {
	return dependency{name: name}
}

func delayedDep(name string) dependency {
	return dependency{name: name, delayed: true}
}

func apiSetDep(name string) dependency {
	return dependency{name: name, virtual: true}
}

func TestUnresolvedDepsCollectsMissing(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{missingDep("gone.dll")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll")}, g.resolve)

	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if got[0].name != "gone.dll" {
		t.Errorf("name = %q, want gone.dll", got[0].name)
	}
	if !got[0].hard {
		t.Error("a non-delay miss should be hard")
	}
	if !slices.Equal(got[0].importers, []string{"a.dll"}) {
		t.Errorf("importers = %v, want [a.dll]", got[0].importers)
	}
}

// The same missing module named by several importers is one finding.
func TestUnresolvedDepsDeduplicates(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{delayedDep("gone.dll")}
	g.deps["b.dll"] = []dependency{delayedDep("GONE.DLL")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll"), foundDep("b.dll")}, g.resolve)

	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !slices.Equal(got[0].importers, []string{"a.dll", "b.dll"}) {
		t.Errorf("importers = %v, want [a.dll b.dll]", got[0].importers)
	}
}

// An api-set has no file on disk by design; the loader resolves it through its
// schema, so it is not a missing dependency.
func TestUnresolvedDepsIgnoresAPISets(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{apiSetDep("api-ms-win-core-heap-l1-1-0.dll")}

	if got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll")}, g.resolve); len(got) != 0 {
		t.Errorf("got %+v, want no findings", got)
	}
}

func TestUnresolvedDepsDelayOnlyIsNotHard(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{delayedDep("gone.dll")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll")}, g.resolve)

	if len(got) != 1 || got[0].hard {
		t.Fatalf("delay-only miss should not be hard: %+v", got)
	}
	if want := "gone.dll (delay)"; got[0].label() != want {
		t.Errorf("label() = %q, want %q", got[0].label(), want)
	}
}

// The subtle case: delay-loaded by one importer, required at load time by
// another. The strictest edge decides, because that is the one that stops the
// image from starting.
func TestUnresolvedDepsMixedDelayIsHard(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{delayedDep("gone.dll")}
	g.deps["b.dll"] = []dependency{missingDep("gone.dll")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll"), foundDep("b.dll")}, g.resolve)

	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if !got[0].hard {
		t.Error("a module required at load time by any importer must be hard")
	}
	if got[0].label() != "gone.dll" {
		t.Errorf("label() = %q, want no (delay) marker", got[0].label())
	}
}

func TestUnresolvedDepsTerminatesOnCycle(t *testing.T) {
	g := newFakeGraph(map[string][]string{
		"a.dll": {"b.dll"},
		"b.dll": {"a.dll"},
	})
	g.deps["c.dll"] = []dependency{missingDep("gone.dll")}
	g.edges["b.dll"] = []string{"a.dll", "c.dll"}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll")}, g.resolve)

	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if !slices.Equal(got[0].importers, []string{"c.dll"}) {
		t.Errorf("importers = %v, want [c.dll] with no repeats", got[0].importers)
	}
}

// Map iteration is random, so both findings and importers must be sorted for
// the report to be reproducible.
func TestUnresolvedDepsIsSorted(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["z.dll"] = []dependency{missingDep("zeta.dll"), missingDep("alpha.dll")}
	g.deps["a.dll"] = []dependency{missingDep("alpha.dll")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("z.dll"), foundDep("a.dll")}, g.resolve)

	names := make([]string, len(got))
	for i, hit := range got {
		names[i] = hit.name
	}
	if !slices.Equal(names, []string{"alpha.dll", "zeta.dll"}) {
		t.Errorf("findings = %v, want sorted", names)
	}
	if !slices.Equal(got[0].importers, []string{"a.dll", "z.dll"}) {
		t.Errorf("importers = %v, want sorted", got[0].importers)
	}
}

// An unreadable module was resolved: it is a different problem and is not
// reported as unresolved, and it must not stop the walk.
func TestUnresolvedDepsSkipsUnreadable(t *testing.T) {
	g := newFakeGraph(nil)
	g.fail["a.dll"] = true
	g.deps["b.dll"] = []dependency{missingDep("gone.dll")}

	got := unresolvedDeps("root.exe", []dependency{foundDep("a.dll"), foundDep("b.dll")}, g.resolve)

	if len(got) != 1 || got[0].name != "gone.dll" {
		t.Errorf("got %+v, want just gone.dll from the module after the failure", got)
	}
}

func TestImporterListCollapsesLongLists(t *testing.T) {
	hit := unresolved{
		name:      "gone.dll",
		importers: []string{"a.dll", "b.dll", "c.dll", "d.dll", "e.dll"},
	}

	if want := "a.dll, b.dll, c.dll, +2 more"; hit.importerList() != want {
		t.Errorf("importerList() = %q, want %q", hit.importerList(), want)
	}
}

func TestImporterListShort(t *testing.T) {
	hit := unresolved{name: "gone.dll", importers: []string{"a.dll", "b.dll"}}

	if want := "a.dll, b.dll"; hit.importerList() != want {
		t.Errorf("importerList() = %q, want %q", hit.importerList(), want)
	}
}

// printUnresolved's exit code is the flag's whole point: it must ignore
// delay-load misses, which every healthy Windows binary has.
func TestUnresolvedExitCode(t *testing.T) {
	tests := []struct {
		name string
		deps []dependency
		want int
	}{
		{"nothing missing", []dependency{foundDep("a.dll")}, 0},
		{"delay miss only", []dependency{delayedDep("gone.dll")}, 0},
		{"hard miss", []dependency{missingDep("gone.dll")}, 1},
		{"api-set only", []dependency{apiSetDep("api-ms-win-core-heap-l1-1-0.dll")}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureOut(t, colorprofile.NoTTY)

			g := newFakeGraph(nil)
			got := unresolvedDeps("root.exe", tt.deps, g.resolve)

			exit := 0
			if len(got) > 0 {
				exit = writeUnresolvedReport(got)
			}

			if exit != tt.want {
				t.Errorf("exit = %d, want %d (findings %+v)", exit, tt.want, got)
			}
		})
	}
}

func TestPrintUnresolvedReportShape(t *testing.T) {
	buf := captureOut(t, colorprofile.NoTTY)

	missing := []unresolved{
		{name: "AzureAttestManager.dll", importers: []string{"DMCmnUtils.dll"}},
		{name: "broken.dll", importers: []string{"a.dll", "b.dll"}, hard: true},
	}
	writeUnresolvedReport(missing)

	want := strings.Join([]string{
		"AzureAttestManager.dll (delay)  <- DMCmnUtils.dll",
		"broken.dll                      <- a.dll, b.dll",
		"",
		"2 unresolved",
		"",
	}, "\n")

	if got := buf.String(); got != want {
		t.Errorf("report:\n%q\nwant:\n%q", got, want)
	}
}

func TestUnresolvedText(t *testing.T) {
	missing := []unresolved{
		{name: "AzureAttestManager.dll", importers: []string{"DMCmnUtils.dll"}},
		{name: "broken.dll", importers: []string{"a.dll", "b.dll"}, hard: true},
	}

	want := strings.Join([]string{
		"AzureAttestManager.dll (delay)  <- DMCmnUtils.dll",
		"broken.dll                      <- a.dll, b.dll",
		"",
	}, "\n")

	if got := unresolvedText(missing); got != want {
		t.Errorf("unresolvedText():\n%q\nwant:\n%q", got, want)
	}
}

// newUnresolvedModel wires a model to a fake graph the way initModel wires one
// to the real resolver, with the named modules as the root's imports.
func newUnresolvedModel(g *fakeGraph, deps ...dependency) model {
	m := model{
		mode:     importMode,
		filePath: "root.exe",
		width:    80,
		height:   12,
		resolve:  g.resolve,
	}
	for _, dep := range deps {
		m.imports = append(m.imports, &importItem{dependency: dep})
	}
	m.refreshImports()
	return m
}

func pressU(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	got, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", next)
	}
	return got
}

// u is a toggle: into the findings and straight back to the import list.
func TestUKeyTogglesUnresolvedMode(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{missingDep("gone.dll")}

	m := pressU(t, newUnresolvedModel(g, foundDep("a.dll")))

	if m.mode != unresolvedMode {
		t.Fatalf("mode = %d after u, want unresolvedMode", m.mode)
	}
	if m.length() != 1 || m.visibleMissing[0].name != "gone.dll" {
		t.Errorf("visibleMissing = %+v, want just gone.dll", m.visibleMissing)
	}

	if back := pressU(t, m); back.mode != importMode {
		t.Errorf("mode = %d after a second u, want importMode", back.mode)
	}
}

// The walk resolves the whole graph, so it must happen once and be cached —
// this is the only place the TUI blocks.
func TestUKeyScansOnlyOnce(t *testing.T) {
	g := newFakeGraph(map[string][]string{"a.dll": {"b.dll"}})
	g.deps["b.dll"] = []dependency{missingDep("gone.dll")}

	m := pressU(t, newUnresolvedModel(g, foundDep("a.dll")))
	if m.length() != 1 {
		t.Fatalf("length() = %d after the first scan, want 1", m.length())
	}

	calls := g.calls["a.dll"]
	if calls == 0 {
		t.Fatal("the first u did not walk the graph")
	}

	// Out of the mode and back in.
	m = pressU(t, pressU(t, m))

	if g.calls["a.dll"] != calls {
		t.Errorf("a.dll resolved %d times, want %d: the scan was not cached",
			g.calls["a.dll"], calls)
	}
	if m.length() != 1 {
		t.Errorf("length() = %d on the second visit, want the cached 1", m.length())
	}
}

// A file that failed to load has no resolver; u must not walk with it.
func TestUKeyOnUnloadableFile(t *testing.T) {
	m := model{mode: importMode, width: 80, height: 12, loadErr: "failed to parse PE"}

	got := pressU(t, m)

	if got.scanned {
		t.Error("scanned a file that never loaded")
	}
	if got.length() != 0 {
		t.Errorf("length() = %d, want 0", got.length())
	}
	if !strings.Contains(got.View().Content, "failed to parse PE") {
		t.Error("the body should show the load failure")
	}
}

// d in the unresolved view drops the findings no importer needs at load time,
// exactly as -u -d does.
func TestDKeyFiltersFindings(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{missingDep("hard.dll"), delayedDep("soft.dll")}

	m := pressU(t, newUnresolvedModel(g, foundDep("a.dll")))
	if m.length() != 2 {
		t.Fatalf("length() = %d, want both findings", m.length())
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	hidden := next.(model)

	if hidden.length() != 1 || hidden.visibleMissing[0].name != "hard.dll" {
		t.Errorf("visibleMissing = %+v, want just hard.dll", hidden.visibleMissing)
	}
	// The walk's own findings must survive the filter, or unhiding cannot
	// restore them and hard would have been decided on a pruned graph.
	if len(hidden.missing) != 2 {
		t.Errorf("missing = %+v, want the filter to leave the findings alone", hidden.missing)
	}
	if !strings.Contains(hidden.View().Content, "-delay") {
		t.Error("the header does not show the filter is on")
	}

	back, _ := hidden.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if got := back.(model); got.length() != 2 {
		t.Errorf("length() = %d after unhiding, want 2", got.length())
	}
}

func TestUnresolvedModeRendersWithoutPanic(t *testing.T) {
	g := newFakeGraph(nil)
	g.deps["a.dll"] = []dependency{missingDep("gone.dll"), delayedDep("other.dll")}

	m := pressU(t, newUnresolvedModel(g, foundDep("a.dll")))
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

	// And with nothing to report.
	empty := pressU(t, newUnresolvedModel(newFakeGraph(nil), foundDep("a.dll")))
	empty.width, empty.height = 80, 12
	if got := empty.View().Content; !strings.Contains(got, "No unresolved dependencies") {
		t.Errorf("empty body:\n%s", got)
	}
}
