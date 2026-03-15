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
		return m, nil

	case tea.KeyMsg:
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

		case "space":
			switch m.mode {
			case importMode:
				mappedCursor, function := m.mapIndex(m.cursor)
				if function != -1 {
					m.cursor = m.mapFrom(mappedCursor, -1)
				}
				item := m.imports[mappedCursor]
				if item.found {
					item.showFunctions = !item.showFunctions
					m.updateStart()
				}
				return m, nil
			}

		case "c":
			switch m.mode {
			case importMode:
				mappedCursor, _ := m.mapIndex(m.cursor)
				item := m.imports[mappedCursor]
				if item.found {
					clipboardWrite(item.path)
				}
			case exportMode:
				mappedCursor, _ := m.mapIndex(m.cursor)
				item := m.exports[mappedCursor]
				clipboardWrite(item.String())
			}

		case "a":
			switch m.mode {
			case importMode:
				names := make([]string, len(m.imports))
				for i := range m.imports {
					item := m.imports[i]
					names[i] = item.dllName
				}
				clipboardWrite(strings.Join(names, "\n"))
			case exportMode:
				names := make([]string, len(m.exports))
				for i := range m.exports {
					names[i] = m.exports[i].String()
				}
				clipboardWrite(strings.Join(names, "\n"))
			}

		case "f":
			switch m.mode {
			case importMode:
				mappedCursor, _ := m.mapIndex(m.cursor)
				item := m.imports[mappedCursor]
				if item.found {
					clipboardWrite(strings.Join(item.functions, "\n"))
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
