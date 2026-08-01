package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saferwall/pe"
)

// newImportModel builds an import-mode model with one entry per element of
// fns, where the element is that import's function count. Indices listed in
// expanded have their function list shown.
func newImportModel(fns []int, expanded ...int) model {
	m := model{mode: importMode}
	for i, n := range fns {
		m.imports = append(m.imports, &importItem{
			dllName:   fmt.Sprintf("dll%d.dll", i),
			found:     true,
			functions: make([]string, n),
		})
	}
	for _, i := range expanded {
		m.imports[i].showFunctions = true
	}
	return m
}

func TestLength(t *testing.T) {
	tests := []struct {
		name     string
		fns      []int
		expanded []int
		want     int
	}{
		{"empty", nil, nil, 0},
		{"all collapsed", []int{2, 3}, nil, 2},
		{"no functions", []int{0, 0, 0}, nil, 3},
		{"first expanded", []int{2, 3}, []int{0}, 4},
		{"both expanded", []int{2, 3}, []int{0, 1}, 7},
		{"expanded but empty", []int{0, 3}, []int{0}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newImportModel(tt.fns, tt.expanded...)
			if got := m.length(); got != tt.want {
				t.Errorf("length() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLengthExportMode(t *testing.T) {
	m := model{mode: exportMode, exports: make([]exportItem, 7)}
	if got := m.length(); got != 7 {
		t.Errorf("length() = %d, want 7", got)
	}
}

// Every visible row must map to an item/function pair that maps back to the
// same row.
func TestMapIndexRoundTrip(t *testing.T) {
	m := newImportModel([]int{2, 0, 3, 1}, 0, 2)

	length := m.length()
	if length != 4+2+3 {
		t.Fatalf("unexpected length %d", length)
	}

	for index := range length {
		item, function := m.mapIndex(index)
		if item < 0 || item >= len(m.imports) {
			t.Fatalf("mapIndex(%d) = (%d, %d), item out of range", index, item, function)
		}
		if got := m.mapFrom(item, function); got != index {
			t.Errorf("mapFrom(mapIndex(%d)) = %d, want %d", index, got, index)
		}
	}
}

func TestMapIndexOutOfRange(t *testing.T) {
	m := newImportModel([]int{2, 3})
	if item, function := m.mapIndex(99); item != -1 || function != -1 {
		t.Errorf("mapIndex(99) = (%d, %d), want (-1, -1)", item, function)
	}
}

// A collapsed function row maps to its parent import, which is what `space`
// relies on to move the cursor before hiding the list.
func TestMapFromFunctionToParent(t *testing.T) {
	m := newImportModel([]int{2, 3}, 0)

	// Rows: 0 dll0, 1 fn, 2 fn, 3 dll1.
	item, function := m.mapIndex(2)
	if item != 0 || function != 1 {
		t.Fatalf("mapIndex(2) = (%d, %d), want (0, 1)", item, function)
	}
	if got := m.mapFrom(item, -1); got != 0 {
		t.Errorf("mapFrom(0, -1) = %d, want 0", got)
	}
}

func TestUpdateStart(t *testing.T) {
	tests := []struct {
		name   string
		count  int
		height int
		cursor int
		start  int
		want   int
	}{
		{"cursor above view scrolls up", 50, 10, 5, 20, 5},
		{"cursor below view scrolls down", 50, 10, 45, 0, 38},
		{"cursor already visible", 50, 10, 40, 38, 38},
		{"list shorter than view", 3, 20, 0, 0, 0},
		{"degenerate height", 50, 1, 10, 7, 0},

		// Growing the window must release a stale offset instead of leaving a
		// blank region below the last row.
		{"resize taller clamps to zero", 50, 60, 45, 41, 0},
		{"resize taller clamps to end", 100, 30, 95, 88, 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newImportModel(make([]int, tt.count))
			m.height = tt.height
			m.cursor = tt.cursor
			m.start = tt.start

			m.updateStart()

			if m.start != tt.want {
				t.Errorf("start = %d, want %d", m.start, tt.want)
			}
		})
	}
}

// The regression this guards: wheel-scroll, then grow the window.
func TestUpdateStartAfterWheelAndResize(t *testing.T) {
	m := newImportModel(make([]int, 50))
	m.height = 10

	m.cursor = 45
	m.updateStart()
	m.handleWheel(3)

	m.height = 60
	m.updateStart()

	if m.start != 0 {
		t.Errorf("start = %d, want 0: the whole list fits, nothing should be scrolled off", m.start)
	}
}

func TestHandleWheel(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		height     int
		cursor     int
		steps      int
		wantStart  int
		wantCursor int
	}{
		{"scroll down drags cursor", 50, 10, 0, 3, 3, 3},
		{"scroll up leaves cursor visible", 50, 10, 5, -3, 0, 5},
		{"clamped at top", 50, 10, 0, -3, 0, 0},
		{"clamped at end", 50, 10, 0, 100, 42, 42},
		{"list fits, no scroll", 5, 10, 0, 3, 0, 0},
		{"degenerate height", 50, 1, 0, 3, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newImportModel(make([]int, tt.count))
			m.height = tt.height
			m.cursor = tt.cursor

			m.handleWheel(tt.steps)

			if m.start != tt.wantStart {
				t.Errorf("start = %d, want %d", m.start, tt.wantStart)
			}
			if m.cursor != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.wantCursor)
			}
		})
	}
}

// After a wheel event the cursor must name a row that is actually drawn,
// otherwise the header counter lies.
func TestHandleWheelKeepsCursorVisible(t *testing.T) {
	m := newImportModel(make([]int, 200))
	m.height = 12
	visible := m.visibleRows()

	for _, steps := range []int{3, 3, 3, -1, 50, -20, 500, -500} {
		m.handleWheel(steps)
		if m.cursor < m.start || m.cursor > m.start+visible-1 {
			t.Fatalf("after %+d: cursor %d outside visible rows [%d, %d]",
				steps, m.cursor, m.start, m.start+visible-1)
		}
	}
}

// initModel unmaps the image before returning, so every string it copied out
// has to be independent of the mapping.
func TestInitModelSurvivesUnmap(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	m := initModel(exe, nil)
	if m.loadErr != "" {
		t.Fatalf("could not load the test binary: %s", m.loadErr)
	}
	if len(m.imports) == 0 {
		t.Fatal("the test binary is expected to import something")
	}

	// Read everything back now that the mapping is gone.
	for _, item := range m.imports {
		if item.dllName == "" {
			t.Error("import with an empty name")
		}
		for _, fn := range item.functions {
			if fn == "" {
				t.Errorf("%s: imported function with an empty name", item.dllName)
			}
		}
	}
}

// A live memory mapping locks the image on Windows, so a leaked one would stop
// anything else from replacing or deleting a DLL that deps had looked at.
func TestInitModelReleasesTheFile(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "copy.exe")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m := initModel(path, nil)
	if m.loadErr != "" {
		t.Fatalf("could not load the copy: %s", m.loadErr)
	}

	if err := os.Remove(path); err != nil {
		t.Errorf("image still locked after initModel returned: %v", err)
	}
}

// Before this was fixed, a file that failed to parse called log.Fatalf and took
// the process down — inside the TUI that left the terminal unusable.
func TestInitModelReportsErrorInsteadOfExiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bogus.dll")
	if err := os.WriteFile(path, []byte("not a PE"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := initModel(path, nil)

	if m.loadErr == "" {
		t.Fatal("expected loadErr to be set for a non-PE file")
	}
	if m.length() != 0 {
		t.Errorf("length() = %d, want 0", m.length())
	}
}

func TestRightStaysPutOnUnreadableTarget(t *testing.T) {
	bogus := filepath.Join(t.TempDir(), "bogus.dll")
	if err := os.WriteFile(bogus, []byte("not a PE"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newImportModel([]int{0})
	m.filePath = "root.exe"
	m.history = []string{"root.exe"}
	m.imports[0].path = bogus

	got, _ := m.right()

	next, ok := got.(*model)
	if !ok {
		t.Fatalf("right() returned %T, want *model", got)
	}
	if next.filePath != "root.exe" {
		t.Errorf("navigated to %q; should have stayed on the unreadable target", next.filePath)
	}
	if next.status == "" {
		t.Error("expected a status message explaining the failure")
	}
}

func TestFindDependencyOrder(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, dir := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(dir, "a.dll"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, found := findDependency("a.dll", []string{first, second})
	if !found {
		t.Fatal("a.dll not found")
	}
	if want := filepath.Join(first, "a.dll"); got != want {
		t.Errorf("resolved to %q, want the earlier directory %q", got, want)
	}
}

func TestFindDependencySkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	real := t.TempDir()

	if err := os.Mkdir(filepath.Join(dir, "trap.dll"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "trap.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found := findDependency("trap.dll", []string{dir, real})
	if !found {
		t.Fatal("trap.dll not found")
	}
	if want := filepath.Join(real, "trap.dll"); got != want {
		t.Errorf("resolved to %q, want %q: a directory must not shadow a real file", got, want)
	}
}

func TestFindDependencyMissing(t *testing.T) {
	if _, found := findDependency("nope.dll", []string{t.TempDir(), ""}); found {
		t.Error("nope.dll should not resolve")
	}
}

// View must survive any window size — in particular one too narrow for the
// right-hand column, where the available width goes negative.
func TestViewRenders(t *testing.T) {
	sizes := []struct{ width, height int }{
		{0, 0}, {1, 1}, {3, 3}, {8, 5}, {20, 10}, {200, 60},
	}

	build := func() model {
		m := newImportModel([]int{3, 0, 2}, 0)
		m.filePath = `C:\some\quite\long\path\app.exe`
		m.imports[0].path = `C:\Windows\System32\a\very\long\directory\kernel32.dll`
		m.imports[0].functions = []string{"CreateFileW", "ReadFile", "ordinal:12"}
		m.imports[1].found = false
		m.imports[1].virtual = true
		m.imports[2].delayed = true
		m.imports[2].path = `C:\Windows\System32\ole32.dll`
		m.exports = []exportItem{
			{name: "SomeExportedFunction", hasName: true, ordinal: 1, rva: 0x1000},
			{ordinal: 2, rva: 0x2000},
		}
		return m
	}

	for _, md := range []mode{importMode, exportMode} {
		for _, size := range sizes {
			m := build()
			m.mode = md
			m.width, m.height = size.width, size.height
			m.updateStart()

			view := m.View() // must not panic

			if size.height >= 3 {
				if got, want := strings.Count(view.Content, "\n"), size.height-1; got != want {
					t.Errorf("mode %d at %dx%d: %d newlines, want %d",
						md, size.width, size.height, got, want)
				}
			}
		}
	}
}

func TestViewShowsToolset(t *testing.T) {
	m := newImportModel([]int{0})
	m.filePath = `C:\app\app.exe`
	m.toolset = "MSVC 14.38.33145"
	m.width, m.height = 120, 24

	if view := m.View(); !strings.Contains(view.Content, "MSVC 14.38.33145") {
		t.Error("the toolset version should appear in the header")
	}
}

func TestViewShowsLoadError(t *testing.T) {
	m := model{mode: importMode, width: 80, height: 24, loadErr: "failed to parse PE: bad magic"}

	view := m.View()

	if !strings.Contains(view.Content, "bad magic") {
		t.Error("the load failure should be visible in the body")
	}
	if !strings.Contains(view.Content, "[error]") {
		t.Error("the header should mark the file as failed, not empty")
	}
}

func TestIsAPISet(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"api-ms-win-core-heap-l1-1-0.dll", true},
		{"API-MS-Win-Core-Synch-L1-1-0.dll", true},
		{"ext-ms-win-shell-shell32-l1-2-0.dll", true},
		{"kernel32.dll", false},
		{"apisetstub.dll", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isAPISet(tt.name); got != tt.want {
			t.Errorf("isAPISet(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSearchDirs(t *testing.T) {
	tests := []struct {
		name    string
		machine pe.ImageFileHeaderMachineType
		want    []string
	}{
		{"64-bit uses System32", pe.ImageFileMachineAMD64,
			[]string{`C:\app`, `C:\Win\System32`, `C:\Win`, `C:\extra`}},
		{"32-bit uses SysWOW64", pe.ImageFileMachineI386,
			[]string{`C:\app`, `C:\Win\SysWOW64`, `C:\Win`, `C:\extra`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SystemRoot", `C:\Win`)
			t.Setenv("PATH", `C:\extra`)

			got := searchDirs(`C:\app\app.exe`, tt.machine)
			if !slices.Equal(got, tt.want) {
				t.Errorf("searchDirs() = %v, want %v", got, tt.want)
			}
		})
	}
}
