package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
			dependency: dependency{
				name:      fmt.Sprintf("dll%d.dll", i),
				found:     true,
				functions: make([]string, n),
			},
		})
	}
	for _, i := range expanded {
		m.imports[i].showFunctions = true
	}
	m.refreshImports()
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

// The scroll offset is settled by rendering, because only then is the height
// inside the box known. These cases are the old updateStart table, driven
// through a real render.
func TestListOffsetFollowsCursor(t *testing.T) {
	tests := []struct {
		name   string
		count  int
		rows   int
		cursor int
		offset int
		want   int
	}{
		{"cursor above view scrolls up", 50, 8, 5, 20, 5},
		{"cursor below view scrolls down", 50, 8, 45, 0, 38},
		{"cursor already visible", 50, 8, 40, 38, 38},
		{"list shorter than view", 3, 18, 0, 0, 0},

		// Growing the window must release a stale offset instead of leaving a
		// blank region below the last row.
		{"resize taller clamps to zero", 50, 58, 45, 41, 0},
		{"resize taller clamps to end", 100, 28, 95, 88, 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newImportModel(make([]int, tt.count))
			m.cursor = tt.cursor
			m.list.SetOffset(tt.offset)

			drawRows(&m, 80, tt.rows)

			if got := m.list.Offset(); got != tt.want {
				t.Errorf("offset = %d, want %d", got, tt.want)
			}
		})
	}
}

// A window too small to hold a single row must not disturb the offset.
func TestListOffsetSurvivesDegenerateHeight(t *testing.T) {
	m := newImportModel(make([]int, 50))
	m.cursor = 10
	m.list.SetOffset(7)

	draw(&m, 80, 1)

	if got := m.list.Offset(); got != 7 {
		t.Errorf("offset = %d, want 7: nothing was drawn, so nothing should have moved", got)
	}
}

// The regression this guards: wheel-scroll, then grow the window.
func TestListOffsetAfterWheelAndResize(t *testing.T) {
	m := newImportModel(make([]int, 50))
	m.cursor = 45

	drawRows(&m, 80, 8)
	m.handleWheel(3)
	drawRows(&m, 80, 58)

	if got := m.list.Offset(); got != 0 {
		t.Errorf("offset = %d, want 0: the whole list fits, nothing should be scrolled off", got)
	}
}

func TestHandleWheel(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		rows       int
		cursor     int
		steps      int
		wantOffset int
		wantCursor int
	}{
		{"scroll down leaves cursor behind", 50, 8, 0, 3, 3, 0},
		{"scroll up keeps cursor unchanged", 50, 8, 5, -3, 0, 5},
		{"clamped at top", 50, 8, 0, -3, 0, 0},
		{"clamped at end", 50, 8, 0, 100, 42, 0},
		{"list fits, no scroll", 5, 8, 0, 3, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newImportModel(make([]int, tt.count))
			m.cursor = tt.cursor

			// The wheel scrolls by the height of the last frame.
			drawRows(&m, 80, tt.rows)
			m.handleWheel(tt.steps)

			if got := m.list.Offset(); got != tt.wantOffset {
				t.Errorf("offset = %d, want %d", got, tt.wantOffset)
			}
			if m.cursor != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.wantCursor)
			}
		})
	}
}

// A wheel event before the first frame has nothing to scroll by and must not
// move anything.
func TestHandleWheelBeforeFirstFrame(t *testing.T) {
	m := newImportModel(make([]int, 50))

	m.handleWheel(3)

	if m.list.Offset() != 0 || m.cursor != 0 {
		t.Errorf("offset %d cursor %d, want 0 and 0", m.list.Offset(), m.cursor)
	}
}

func TestHandleWheelLetsCursorLeaveView(t *testing.T) {
	m := newImportModel(make([]int, 50))
	drawRows(&m, 80, 8)
	m.handleWheel(3)
	drawRows(&m, 80, 8)

	if m.cursor != 0 || m.list.Offset() != 3 {
		t.Fatalf("cursor %d offset %d, want unchanged cursor 0 and offset 3", m.cursor, m.list.Offset())
	}
	if _, selected := m.list.Selected(); selected {
		t.Fatal("off-screen cursor should not be rendered as a list selection")
	}

	// Keyboard navigation reattaches the cursor and brings it back into view.
	m.handleKey(keyEvent('j'))
	drawRows(&m, 80, 8)
	if selected, ok := m.list.Selected(); !ok || selected != 1 {
		t.Fatalf("selection = (%d, %v), want (1, true)", selected, ok)
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

			got, systemCount := searchDirs(`C:\app\app.exe`, tt.machine)
			if !slices.Equal(got, tt.want) {
				t.Errorf("searchDirs() = %v, want %v", got, tt.want)
			}
			// The image directory and the two Windows directories; C:\extra
			// came from PATH and must fall outside the system range.
			if systemCount != 3 {
				t.Errorf("systemCount = %d, want 3", systemCount)
			}
		})
	}
}

// An API set contract must not resolve to a same-named file sitting in some
// unrelated PATH directory: the loader would never load it.
func TestAPISetIgnoresPathDirectories(t *testing.T) {
	system, stray := t.TempDir(), t.TempDir()

	const contract = "api-ms-win-core-processthreads-l1-1-1.dll"
	if err := os.WriteFile(filepath.Join(stray, contract), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// system dirs first, then the PATH entry holding the stray copy.
	search := newSearchPath([]string{system, stray}, 1)

	if path, found := search.find(contract); found {
		t.Errorf("resolved the contract to %q; a PATH copy is not what loads", path)
	}

	// A real DLL still resolves from PATH as usual.
	if err := os.WriteFile(filepath.Join(stray, "helper.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found := newSearchPath([]string{system, stray}, 1).find("helper.dll"); !found {
		t.Error("an ordinary DLL should still resolve from PATH")
	}
}

// The same name is looked up thousands of times in a recursive walk.
func TestSearchPathCachesLookups(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	search := newSearchPath([]string{dir}, 1)
	first, _ := search.find("a.dll")

	// Removing the file must not change the answer: it came from the cache.
	if err := os.Remove(filepath.Join(dir, "a.dll")); err != nil {
		t.Fatal(err)
	}
	second, found := search.find("A.DLL")

	if !found || second != first {
		t.Errorf("second lookup = %q/%v, want the cached %q", second, found, first)
	}
}
