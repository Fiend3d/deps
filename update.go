package main

import (
	"strings"

	"github.com/Fiend3d/catatui"
	"github.com/Fiend3d/catatui/term"
)

// handle applies one terminal event to the model. It is the whole of the
// program's input handling: main does nothing but read events and hand them
// here, so every key is decided in one place.
func (m *model) handle(ev term.Event) {
	// A resize needs no handling of its own: the terminal resizes its buffers
	// and the next Render is handed the new area.
	switch ev.Kind {
	case term.EventMouse:
		m.handleMouse(ev)

	case term.EventKey:
		m.handleKey(ev)
	}
}

func (m *model) handleMouse(ev term.Event) {
	switch ev.MouseKind {
	case term.MouseScrollUp:
		m.handleWheel(-3)

	case term.MouseScrollDown:
		m.handleWheel(3)

	case term.MouseDown:
		if ev.Button != term.MouseButtonLeft {
			return
		}
		if tab, ok := m.tabAt(catatui.Position{X: ev.X, Y: ev.Y}); ok {
			m.selectMode(tab)
			return
		}

		m.click = newClick(int(ev.X), int(ev.Y), &m.click)

		// Rows are only where the list was last drawn; the tabs, the border
		// and the help line are not clickable.
		if !m.listArea.Contains(catatui.Position{X: ev.X, Y: ev.Y}) {
			return
		}
		row := int(ev.Y) - int(m.listArea.Y) + m.list.Offset()
		if row >= m.length() {
			return
		}
		m.cursor = row
		m.cursorDetached = false

		if !m.click.doubleClick {
			return
		}
		switch m.mode {
		case importMode:
			if dep, ok := m.selectedDep(); ok && dep.found {
				m.right()
			}
		case treeMode:
			// In the tree a double click opens the node rather than
			// re-rooting, matching space.
			m.toggleNode()
		}
	}
}

// tabAt resolves a click against the default one-cell padding around each tab
// title. Divider cells deliberately do not activate either neighbour.
func (m *model) tabAt(pos catatui.Position) (mode, bool) {
	if !m.tabsArea.Contains(pos) {
		return 0, false
	}
	x := int(m.tabsArea.X)
	for i, label := range tabLabels {
		end := x + 1 + catatui.StringWidth(label) + 1
		if int(pos.X) >= x && int(pos.X) < end && int(pos.X) < int(m.tabsArea.Right()) {
			return mode(i), true
		}
		x = end + 1 // the divider is one terminal cell wide
	}
	return 0, false
}

func (m *model) selectMode(next mode) {
	if next < importMode || next > unresolvedMode {
		return
	}
	m.mode = next
	m.cursor = 0
	m.cursorDetached = false
	m.list.SetOffset(0)
	if next == unresolvedMode {
		m.scanUnresolved()
	}
}

func (m *model) handleKey(ev term.Event) {
	// Any keypress supersedes the last transient message.
	m.status = ""
	m.cursorDetached = false

	switch {
	case ev.IsRune('q'), ev.IsCtrl('c'):
		m.quit = true

	case ev.IsKey(term.KeyTab):
		m.cursor = 0
		m.list.SetOffset(0)
		switch m.mode {
		case importMode:
			m.mode = exportMode
		case exportMode:
			m.mode = importMode
		}

	case ev.IsRune('r'):
		switch m.mode {
		case treeMode:
			m.mode = importMode
		default:
			m.mode = treeMode
		}
		m.cursor = 0
		m.list.SetOffset(0)

	case ev.IsRune('u'):
		switch m.mode {
		case unresolvedMode:
			m.mode = importMode
		default:
			m.mode = unresolvedMode
			// Blocks the first time on a large image; cached after that.
			m.scanUnresolved()
		}
		m.cursor = 0
		m.list.SetOffset(0)

	case ev.IsRune('d'):
		switch m.mode {
		case importMode, treeMode, unresolvedMode:
			m.setHideDelayed(!m.hideDelayed)
		}

	case ev.IsKey(term.KeyEnter):
		switch m.mode {
		case importMode, treeMode:
			m.right()
		}

	case ev.IsRune('l'), ev.IsKey(term.KeyRight):
		switch m.mode {
		case importMode:
			m.right()
		case treeMode:
			m.toggleNode()
		}

	case ev.IsRune('h'), ev.IsKey(term.KeyLeft):
		if m.mode == treeMode {
			m.collapseNode()
			return
		}
		if len(m.history) > 1 {
			m.left()
		}

	case ev.IsRune(' '):
		switch m.mode {
		case treeMode:
			m.toggleNode()
		case importMode:
			if m.length() == 0 {
				return
			}
			mappedCursor, function := m.mapIndex(m.cursor)
			if function != -1 {
				m.cursor = m.mapFrom(mappedCursor, -1)
			}
			item := m.visibleImports[mappedCursor]
			if item.found {
				item.showFunctions = !item.showFunctions
				m.clampCursor()
			}
		}

	case ev.IsRune('c'):
		if m.length() == 0 {
			return
		}
		switch m.mode {
		case importMode, treeMode:
			if dep, ok := m.selectedDep(); ok {
				m.copy("name", dep.name)
			}
		case exportMode:
			mappedCursor, _ := m.mapIndex(m.cursor)
			m.copy("export", m.exports[mappedCursor].String())
		case unresolvedMode:
			m.copy("name", m.visibleMissing[m.cursor].name)
		}

	case ev.IsRune('p'):
		if m.length() == 0 {
			return
		}
		switch m.mode {
		case importMode, treeMode:
			if dep, ok := m.selectedDep(); ok && dep.found {
				m.copy("path", dep.path)
			}
		}

	case ev.IsRune('a'):
		if m.length() == 0 {
			return
		}
		switch m.mode {
		case importMode:
			names := make([]string, len(m.visibleImports))
			for i := range m.visibleImports {
				names[i] = m.visibleImports[i].name
			}
			m.copy("all names", strings.Join(names, "\n"))
		case exportMode:
			names := make([]string, len(m.exports))
			for i := range m.exports {
				names[i] = m.exports[i].String()
			}
			m.copy("all exports", strings.Join(names, "\n"))
		case treeMode:
			m.copy("tree", treeText(m.visible))
		case unresolvedMode:
			m.copy("unresolved", unresolvedText(m.visibleMissing))
		}

	case ev.IsRune('f'):
		if m.length() == 0 {
			return
		}
		switch m.mode {
		case importMode, treeMode:
			if dep, ok := m.selectedDep(); ok && dep.found {
				m.copy("functions", strings.Join(dep.functions, "\n"))
			}
		}

	case ev.IsRune('j'), ev.IsKey(term.KeyDown):
		m.moveCursor(1)

	case ev.IsRune('k'), ev.IsKey(term.KeyUp):
		m.moveCursor(-1)

	case ev.IsKey(term.KeyPageDown):
		m.moveCursor(m.visibleRows() / 2)

	case ev.IsKey(term.KeyPageUp):
		m.moveCursor(-m.visibleRows() / 2)

	case ev.IsKey(term.KeyHome):
		m.cursor = 0
		m.list.SetOffset(0)

	case ev.IsKey(term.KeyEnd):
		m.cursor = m.length() - 1
		m.clampCursor()
	}
}
