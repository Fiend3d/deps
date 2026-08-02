package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateStart()
		return m, nil

	case tea.MouseClickMsg:
		data := msg.Mouse()
		switch data.Button {
		case tea.MouseLeft:
			m.click = newClick(data.X, data.Y, &m.click)
			if m.click.y > 0 && m.click.y < m.height-1 &&
				m.click.y-1 < m.length()-m.start {
				m.cursor = m.click.y - 1 + m.start

				if m.click.doubleClick {
					switch m.mode {
					case importMode:
						if dep, ok := m.selectedDep(); ok && dep.found {
							return m.right()
						}
					case treeMode:
						// In the tree a double click opens the node rather than
						// re-rooting, matching space.
						m.toggleNode()
						return m, nil
					}
				}
			}
		}

	case tea.MouseWheelMsg:
		data := msg.Mouse()
		switch data.Button {
		case tea.MouseWheelUp:
			return m.handleWheel(-3)
		case tea.MouseWheelDown:
			return m.handleWheel(3)
		}

	case tea.KeyMsg:
		// Any keypress supersedes the last transient message.
		m.status = ""

		switch msg.String() {
		case "q":
			return m, tea.Quit

		case "tab":
			m.cursor = 0
			m.start = 0
			switch m.mode {
			case importMode:
				m.mode = exportMode
			case exportMode:
				m.mode = importMode
			}
			return m, nil

		case "r":
			switch m.mode {
			case treeMode:
				m.mode = importMode
			default:
				m.mode = treeMode
			}
			m.cursor = 0
			m.start = 0
			return m, nil

		case "d":
			switch m.mode {
			case importMode, treeMode:
				m.setHideDelayed(!m.hideDelayed)
			}
			return m, nil

		case "enter":
			switch m.mode {
			case importMode, treeMode:
				return m.right()
			}

		case "l", "right":
			switch m.mode {
			case importMode:
				return m.right()
			case treeMode:
				m.toggleNode()
				return m, nil
			}

		case "h", "left":
			if m.mode == treeMode {
				m.collapseNode()
				return m, nil
			}
			if len(m.history) > 1 {
				return m.left()
			}

		case "space":
			switch m.mode {
			case treeMode:
				m.toggleNode()
				return m, nil
			case importMode:
				if m.length() == 0 {
					return m, nil
				}
				mappedCursor, function := m.mapIndex(m.cursor)
				if function != -1 {
					m.cursor = m.mapFrom(mappedCursor, -1)
				}
				item := m.visibleImports[mappedCursor]
				if item.found {
					item.showFunctions = !item.showFunctions
					m.updateStart()
				}
				return m, nil
			}

		case "c":
			if m.length() == 0 {
				return m, nil
			}
			switch m.mode {
			case importMode, treeMode:
				if dep, ok := m.selectedDep(); ok {
					m.copy("name", dep.name)
				}
				return m, nil
			case exportMode:
				mappedCursor, _ := m.mapIndex(m.cursor)
				item := m.exports[mappedCursor]
				m.copy("export", item.String())
				return m, nil
			}

		case "p":
			if m.length() == 0 {
				return m, nil
			}
			switch m.mode {
			case importMode, treeMode:
				if dep, ok := m.selectedDep(); ok && dep.found {
					m.copy("path", dep.path)
					return m, nil
				}
			}

		case "a":
			if m.length() == 0 {
				return m, nil
			}
			switch m.mode {
			case importMode:
				names := make([]string, len(m.visibleImports))
				for i := range m.visibleImports {
					item := m.visibleImports[i]
					names[i] = item.name
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
			}

		case "f":
			if m.length() == 0 {
				return m, nil
			}
			switch m.mode {
			case importMode, treeMode:
				if dep, ok := m.selectedDep(); ok && dep.found {
					m.copy("functions", strings.Join(dep.functions, "\n"))
				}
			}

		case "j", "down":
			return m.moveCursor(1)

		case "k", "up":
			return m.moveCursor(-1)

		case "pgdown":
			return m.moveCursor((m.height - 2) / 2)

		case "pgup":
			return m.moveCursor(-(m.height - 2) / 2)

		case "home":
			m.cursor = 0
			m.start = 0
			return m, nil

		case "end":
			m.cursor = m.length() - 1
			m.updateStart()
			return m, nil
		}
	}
	return m, nil
}
