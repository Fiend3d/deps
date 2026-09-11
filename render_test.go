package main

import (
	"strings"
	"testing"

	"github.com/Fiend3d/catatui"
	"github.com/Fiend3d/catatui/term"
)

// chromeRows is how many rows the screen spends on something other than the
// list: the mode tabs, the top and bottom border of the box, and the help line.
const chromeRows = 4

// heightFor is the terminal height that leaves room for rows list rows.
func heightFor(rows int) int { return rows + chromeRows }

// draw renders m into a buffer of the given size. Rendering is what settles the
// scroll offset and the area a mouse click is resolved against, so a test that
// cares about either has to draw first.
func draw(m *model, width, height int) *catatui.Buffer {
	buf := catatui.NewBuffer(catatui.NewRect(0, 0, uint16(width), uint16(height)))
	m.Render(buf.Area, buf)
	return buf
}

// drawRows renders m with room for the given number of list rows.
func drawRows(m *model, width, rows int) *catatui.Buffer {
	return draw(m, width, heightFor(rows))
}

// renderString renders m and returns what is on screen, one row per line with
// trailing blanks removed, so a test can look for text without caring where the
// chrome put it.
func renderString(m *model, width, height int) string {
	buf := draw(m, width, height)

	lines := make([]string, 0, buf.Area.Height)
	for y := range buf.Area.Height {
		var row strings.Builder
		for x := range buf.Area.Width {
			row.WriteString(buf.Get(x, y).GetSymbol())
		}
		lines = append(lines, strings.TrimRight(row.String(), " "))
	}
	return strings.Join(lines, "\n")
}

// keyEvent is a press of a printable character with no modifiers.
func keyEvent(r rune) term.Event {
	return term.Event{Kind: term.EventKey, Key: term.KeyRune, Rune: r}
}

func TestRenderAtEverySize(t *testing.T) {
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

	for _, md := range []mode{importMode, exportMode, treeMode, unresolvedMode} {
		for _, size := range sizes {
			m := build()
			m.mode = md

			got := renderString(&m, size.width, size.height) // must not panic

			if size.height == 0 {
				continue
			}
			if want := size.height - 1; strings.Count(got, "\n") != want {
				t.Errorf("mode %d at %dx%d: %d newlines, want %d",
					md, size.width, size.height, strings.Count(got, "\n"), want)
			}
		}
	}
}

func TestRenderShowsToolset(t *testing.T) {
	m := newImportModel([]int{0})
	m.filePath = `C:\app\app.exe`
	m.toolset = "MSVC 14.38.33145"

	if !strings.Contains(renderString(&m, 120, 24), "MSVC 14.38.33145") {
		t.Error("the toolset version should appear in the header")
	}
}

func TestRenderShowsLoadError(t *testing.T) {
	m := model{mode: importMode, loadErr: "failed to parse PE: bad magic"}

	got := renderString(&m, 80, 24)

	if !strings.Contains(got, "bad magic") {
		t.Error("the load failure should be visible in the body")
	}
	if !strings.Contains(got, "[error]") {
		t.Error("the header should mark the file as failed, not empty")
	}
}

// The tabs name every mode, and the one on screen is the one selected.
func TestRenderShowsTabs(t *testing.T) {
	m := newImportModel([]int{0})

	got := renderString(&m, 120, 24)
	for _, tab := range []string{"Import", "Export", "Tree", "Unresolved"} {
		if !strings.Contains(got, tab) {
			t.Errorf("the tab row should name %q, got:\n%s", tab, got)
		}
	}
}

func TestClickingTabsSelectsMode(t *testing.T) {
	m := newImportModel(make([]int, 20))
	m.scanned = true // keep the unresolved tab click independent of filesystem I/O
	drawRows(&m, 80, 4)

	click := func(x uint16) {
		m.handleMouse(term.Event{
			Kind: term.EventMouse, MouseKind: term.MouseDown,
			Button: term.MouseButtonLeft, X: x, Y: m.tabsArea.Y,
		})
	}
	for _, tt := range []struct {
		x    uint16
		want mode
	}{
		{3, importMode}, {12, exportMode}, {20, treeMode}, {29, unresolvedMode},
	} {
		m.cursor = 5
		m.list.SetOffset(3)
		click(tt.x)
		if m.mode != tt.want {
			t.Fatalf("click at x=%d selected mode %d, want %d", tt.x, m.mode, tt.want)
		}
		if m.cursor != 0 || m.list.Offset() != 0 {
			t.Fatalf("tab click did not reset cursor and viewport")
		}
	}

	m.mode = treeMode
	click(24) // divider between Tree and Unresolved
	if m.mode != treeMode {
		t.Fatal("clicking a divider should not select a tab")
	}
}

func TestScrollbarPreservesBlockBorder(t *testing.T) {
	m := newImportModel([]int{0, 0, 0, 0, 0, 0, 0, 0})
	const width = 40

	for _, tt := range []struct {
		name      string
		cursor    int
		thumbEdge func(catatui.Rect) uint16
	}{
		{"top", 0, func(area catatui.Rect) uint16 { return area.Y }},
		{"bottom", 7, func(area catatui.Rect) uint16 { return area.Y + area.Height - 1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m.cursor = tt.cursor
			buf := drawRows(&m, width, 4)

			// The scrollbar is drawn over the block's right edge. Its transparent
			// track must leave that single-line border intact instead of replacing
			// it with the widget's default double-line track.
			thumbs := 0
			for y := m.listArea.Y; y < m.listArea.Y+m.listArea.Height; y++ {
				symbol := buf.Get(width-1, y).GetSymbol()
				switch symbol {
				case "│":
				case "█":
					thumbs++
				default:
					t.Fatalf("right border at row %d = %q, want border or thumb", y, symbol)
				}
			}
			if thumbs == 0 {
				t.Fatal("scrollbar thumb was not rendered")
			}
			if y := tt.thumbEdge(m.listArea); buf.Get(width-1, y).GetSymbol() != "█" {
				t.Fatalf("thumb does not reach %s of track", tt.name)
			}
		})
	}
}

func TestDirectoryHasPaddingBeforeScrollbar(t *testing.T) {
	m := newImportModel([]int{0, 0, 0, 0, 0, 0, 0, 0})
	for i := range m.imports {
		m.imports[i].path = `C:\dir\module.dll`
	}
	m.refreshImports()

	const width = 40
	buf := drawRows(&m, width, 4)
	y := m.listArea.Y
	if got := buf.Get(width-2, y).GetSymbol(); got != " " {
		t.Fatalf("cell before scrollbar = %q, want one column of padding", got)
	}
	if got := buf.Get(width-3, y).GetSymbol(); got != "r" {
		t.Fatalf("directory should end before padding, got %q", got)
	}
}

// A row wider than the window keeps the name and drops the directory column,
// rather than pushing the name off screen.
func TestRenderDropsDirectoryWhenNarrow(t *testing.T) {
	m := newImportModel([]int{0})
	m.imports[0].name = "kernel32.dll"
	m.imports[0].path = `C:\Windows\System32\kernel32.dll`
	m.refreshImports()

	if got := renderString(&m, 24, heightFor(4)); !strings.Contains(got, "kernel32.dll") {
		t.Errorf("the module name should survive a narrow window, got:\n%s", got)
	}
}

// The help line leaves keys off whole when the window is too narrow for all of
// them, rather than cutting the last one off halfway through its label.
func TestHelpLineDropsWholeKeys(t *testing.T) {
	m := newImportModel([]int{2, 0})
	m.imports[0].path = `C:\dir\module.dll`
	m.refreshImports()

	full := m.helpLine(m.length(), 1000).String()
	if !strings.Contains(full, "f functions") {
		t.Fatalf("a wide window should fit every key, got %q", full)
	}

	for width := 20; width <= len(full); width++ {
		got := m.helpLine(m.length(), width).String()
		if w := catatui.StringWidth(got); w > width {
			t.Fatalf("width %d: help line is %d columns wide", width, w)
		}
		if !strings.HasPrefix(full, got) {
			t.Fatalf("width %d: %q is not a prefix of the full line", width, got)
		}
		if rest := full[len(got):]; rest != "" && !strings.HasPrefix(rest, "  ") && !strings.HasPrefix(rest, " │") {
			t.Fatalf("width %d: %q stops partway through a key", width, got)
		}
	}
}

// truncate measures in terminal columns and marks the cut.
func TestTruncate(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"kernel32.dll", 20, "kernel32.dll"},
		{"kernel32.dll", 12, "kernel32.dll"},
		{"kernel32.dll", 5, "kern…"},
		{"kernel32.dll", 1, "…"},
		{"kernel32.dll", 0, ""},
		{"kernel32.dll", -3, ""},
		{"", 5, ""},
	}

	for _, tt := range tests {
		if got := truncate(tt.s, tt.width); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
		if got := truncate(tt.s, tt.width); tt.width > 0 && catatui.StringWidth(got) > tt.width {
			t.Errorf("truncate(%q, %d) = %q, wider than %d columns",
				tt.s, tt.width, got, tt.width)
		}
	}
}

// twoColumnLine never overruns the width it is given, whatever it is handed.
func TestTwoColumnLineFitsWidth(t *testing.T) {
	left := []catatui.Span{catatui.NewSpan("kernel32.dll")}

	for _, width := range []int{0, 1, 5, 13, 14, 20, 60} {
		line := twoColumnLine(left, `C:\Windows\System32`, dimStyle, width)
		if got := line.Width(); width >= 12 && got > width {
			t.Errorf("width %d: line is %d columns wide", width, got)
		}
	}
}
