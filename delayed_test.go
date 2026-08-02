package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestFilterDelayed(t *testing.T) {
	deps := []dependency{
		{name: "a.dll"},
		{name: "b.dll", delayed: true},
		{name: "c.dll"},
	}

	// Not hiding must hand back the input untouched, so the common path does no
	// work and callers keep the same backing array.
	if got := filterDelayed(deps, false); len(got) != 3 || &got[0] != &deps[0] {
		t.Errorf("filterDelayed(_, false) copied or altered the slice: %+v", got)
	}

	got := filterDelayed(deps, true)
	if len(got) != 2 || got[0].name != "a.dll" || got[1].name != "c.dll" {
		t.Errorf("filterDelayed(_, true) = %+v, want a.dll and c.dll", got)
	}
}

// The delayed module and everything under it goes: the subtree is reachable
// only through that edge.
func TestFlattenTreeHidesDelayedSubtree(t *testing.T) {
	g := newFakeGraph(map[string][]string{})
	g.deps["root.exe"] = []dependency{
		{name: "a.dll", path: "a.dll", found: true},
		{name: "b.dll", path: "b.dll", found: true, delayed: true},
	}
	g.edges["b.dll"] = []string{"deep.dll"}

	roots := newTree(g.deps["root.exe"], "root.exe")
	roots[1].toggle(g.resolve)

	if got := names(flattenTree(roots, false)); !equal(got, []string{"a.dll", "b.dll", "  deep.dll"}) {
		t.Fatalf("shown = %v", got)
	}
	if got := names(flattenTree(roots, true)); !equal(got, []string{"a.dll"}) {
		t.Errorf("hidden = %v, want just a.dll", got)
	}
}

// The closing glyph belongs to the last row actually drawn, or the tree ends on
// a branch that leads nowhere.
func TestPrefixClosesOnLastVisibleSibling(t *testing.T) {
	g := newFakeGraph(map[string][]string{})
	g.deps["a.dll"] = []dependency{
		{name: "c.dll", path: "c.dll", found: true},
		{name: "d.dll", path: "d.dll", found: true, delayed: true},
	}
	roots := g.roots("root.exe", "a.dll")
	roots[0].toggle(g.resolve)

	c := roots[0].children[0]
	if got := c.prefix(false); got != "├─ " {
		t.Errorf("with delayed shown, c.dll prefix = %q, want %q", got, "├─ ")
	}
	if got := c.prefix(true); got != "└─ " {
		t.Errorf("with delayed hidden, c.dll prefix = %q, want %q", got, "└─ ")
	}
}

func TestRefreshImportsFiltersAndMapsIndices(t *testing.T) {
	m := newImportModel([]int{2, 1, 1})
	m.imports[1].delayed = true
	m.imports[1].name = "delayed.dll"
	m.imports[0].showFunctions = true
	m.refreshImports()

	if got := m.length(); got != 5 {
		t.Fatalf("length() = %d, want 5 (3 imports + 2 shown functions)", got)
	}

	m.setHideDelayed(true)

	if got := len(m.visibleImports); got != 2 {
		t.Fatalf("visibleImports = %d, want 2", got)
	}
	if got := m.length(); got != 4 {
		t.Errorf("length() = %d, want 4 (2 imports + 2 shown functions)", got)
	}

	// Row 3 is the second surviving import, which must map to index 1 of the
	// filtered slice and back again.
	item, function := m.mapIndex(3)
	if item != 1 || function != -1 {
		t.Errorf("mapIndex(3) = (%d, %d), want (1, -1)", item, function)
	}
	if got := m.mapFrom(1, -1); got != 3 {
		t.Errorf("mapFrom(1, -1) = %d, want 3", got)
	}
	if got := m.visibleImports[item].name; got == "delayed.dll" {
		t.Error("a delayed import survived the filter")
	}

	m.setHideDelayed(false)
	if got := m.length(); got != 5 {
		t.Errorf("length() = %d after unhiding, want 5", got)
	}
}

// Toggling through the filtered view must still reach the real item, or the
// expansion is lost the moment the filter changes.
func TestFilteredViewSharesItems(t *testing.T) {
	m := newImportModel([]int{1, 1})
	m.imports[0].delayed = true
	m.setHideDelayed(true)

	m.visibleImports[0].showFunctions = true

	if !m.imports[1].showFunctions {
		t.Error("toggling through visibleImports did not reach the underlying import")
	}
}

// Hiding while the cursor sits past the end of the shortened list must not
// leave it out of range.
func TestSetHideDelayedKeepsCursorInRange(t *testing.T) {
	m := newImportModel([]int{0, 0, 0})
	m.height = 10
	m.imports[2].delayed = true
	m.cursor = 2

	m.setHideDelayed(true)

	if m.cursor >= m.length() {
		t.Errorf("cursor %d out of range for length %d", m.cursor, m.length())
	}
}

// The filter is a view preference, not a property of the file, so it has to
// survive descending into a dependency and coming back.
func TestHideDelayedSurvivesNavigation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	m := initModel(exe, nil)
	if m.loadErr != "" {
		t.Fatalf("could not load the test binary: %s", m.loadErr)
	}
	m.setHideDelayed(true)

	target := -1
	for i, item := range m.visibleImports {
		if item.found {
			target = i
			break
		}
	}
	if target < 0 {
		t.Skip("the test binary resolved none of its imports")
	}
	m.cursor = m.mapFrom(target, -1)

	next, _ := m.right()
	// right() hands back a fresh model by value; only the stay-put path returns
	// the pointer receiver.
	child, ok := next.(model)
	if !ok {
		t.Skipf("could not descend into a dependency: right() returned %T", next)
	}
	if !child.hideDelayed {
		t.Fatal("descending into a dependency reset the filter")
	}

	back, _ := child.left()
	if parent, ok := back.(model); !ok || !parent.hideDelayed {
		t.Error("coming back up reset the filter")
	}
}

// The d key must reach the filter in both dependency views, and leave the
// export list alone.
func TestDKeyTogglesFilter(t *testing.T) {
	press := tea.KeyPressMsg{Code: 'd', Text: "d"}

	for _, md := range []mode{importMode, treeMode} {
		m := newImportModel([]int{0, 0})
		m.mode = md
		m.height = 10

		next, _ := m.Update(press)
		if got := next.(model); !got.hideDelayed {
			t.Errorf("mode %d: d did not turn the filter on", md)
		}
	}

	m := newImportModel([]int{0, 0})
	m.mode = exportMode
	m.height = 10
	next, _ := m.Update(press)
	if got := next.(model); got.hideDelayed {
		t.Error("d should do nothing in the export list")
	}
}

// The whole path the app actually takes: a real image loaded by initModel, the
// key delivered through Update, and the rows the view would draw.
func TestDKeyOnARealImage(t *testing.T) {
	const image = `C:\Windows\System32\mstsc.exe`
	if _, err := os.Stat(image); err != nil {
		t.Skipf("%s not available: %v", image, err)
	}

	m := initModel(image, nil)
	if m.loadErr != "" {
		t.Skipf("could not load %s: %s", image, m.loadErr)
	}
	m.width, m.height = 120, 40

	delayed := 0
	for _, item := range m.imports {
		if item.delayed {
			delayed++
		}
	}
	if delayed == 0 {
		t.Skipf("%s has no delay-loaded imports to hide", image)
	}

	before := m.length()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	hidden, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", next)
	}

	if !hidden.hideDelayed {
		t.Fatal("d did not set the filter")
	}
	if got := hidden.length(); got != before-delayed {
		t.Errorf("length() = %d after hiding, want %d (%d - %d delayed)",
			got, before-delayed, before, delayed)
	}
	for _, item := range hidden.visibleImports {
		if item.delayed {
			t.Fatalf("%s survived the filter", item.name)
		}
	}
	if !strings.Contains(hidden.View().Content, "-delay") {
		t.Error("the header does not show the filter is on")
	}

	// And back again.
	back, _ := hidden.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if got := back.(model); got.length() != before {
		t.Errorf("length() = %d after unhiding, want %d", got.length(), before)
	}
}

// The header has to admit that rows are being withheld, or the counter quietly
// understates what the image imports.
func TestHeaderAndHelpAnnounceTheFilter(t *testing.T) {
	m := newImportModel([]int{0, 0})
	m.width, m.height = 120, 10
	m.filePath = `C:\app\app.exe`

	if got := m.View().Content; strings.Contains(got, "-delay") {
		t.Error("the header claimed rows were hidden while showing everything")
	} else if !strings.Contains(got, "Hide delayed") {
		t.Error("the help line should offer to hide delayed modules")
	}

	m.setHideDelayed(true)

	got := m.View().Content
	if !strings.Contains(got, "-delay") {
		t.Errorf("header does not mention the filter:\n%s", got)
	}
	if !strings.Contains(got, "Show delayed") {
		t.Errorf("help line does not offer to unhide:\n%s", got)
	}
}

// A delay-loaded api-set used to lose its (delay) marker in the tree, where the
// api-set case rebuilt the label from the bare name.
func TestTreeRowKeepsBothMarkers(t *testing.T) {
	m := model{mode: treeMode, width: 80, height: 10}
	node := &treeNode{dep: dependency{
		name:    "api-ms-win-core-x-l1-1-0.dll",
		delayed: true,
		virtual: true,
	}}

	got := m.renderTreeRow(node, false)
	want := "api-ms-win-core-x-l1-1-0.dll (delay) (api-set)"
	if !strings.Contains(got, want) {
		t.Errorf("renderTreeRow = %q, want it to contain %q", got, want)
	}
}

// -d turns the unresolved report into a load-time-only check: the yellow
// delay-only findings go, the red ones and the exit code stay.
func TestFilterDelayedFindings(t *testing.T) {
	missing := []unresolved{
		{name: "delay-only.dll", importers: []string{"a.dll"}},
		{name: "broken.dll", importers: []string{"a.dll"}, hard: true},
	}

	if got := filterDelayedFindings(missing, false); len(got) != 2 {
		t.Errorf("not hiding changed the findings: %+v", got)
	}

	got := filterDelayedFindings(missing, true)
	if len(got) != 1 || got[0].name != "broken.dll" {
		t.Fatalf("filtered = %+v, want just broken.dll", got)
	}

	captureOut(t, colorprofile.NoTTY)
	if exit := writeUnresolvedReport(got); exit != 1 {
		t.Errorf("exit = %d, want 1: a hard miss still fails the check", exit)
	}

	// Everything delay-only leaves nothing to report, and nothing to fail on.
	only := filterDelayedFindings(missing[:1], true)
	if len(only) != 0 {
		t.Errorf("filtered = %+v, want empty", only)
	}
}

func TestPrintTreeHidesDelayed(t *testing.T) {
	buf := captureOut(t, colorprofile.NoTTY)

	g := newFakeGraph(map[string][]string{"a.dll": {}, "c.dll": {}})
	g.deps["a.dll"] = []dependency{
		{name: "deep.dll", path: "deep.dll", found: true, delayed: true},
	}

	printTree("root.exe", []dependency{
		{name: "a.dll", path: "a.dll", found: true},
		{name: "b.dll", path: "b.dll", found: true, delayed: true},
		{name: "c.dll", path: "c.dll", found: true},
	}, g.resolve, lg.NewStyle(), true)

	want := strings.Join([]string{
		"├─ a.dll",
		"└─ c.dll",
		"",
	}, "\n")

	if got := buf.String(); got != want {
		t.Errorf("printTree wrote:\n%s\nwant:\n%s", got, want)
	}
}
