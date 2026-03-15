package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type mode int

const (
	importMode mode = iota
	exportMode
)

type importItem struct {
	dllName       string
	found         bool
	path          string
	functions     []string
	showFunctions bool
}

type exportItem struct {
	name    string
	hasName bool
	ordinal uint32
	rva     uint32
}

func (i *exportItem) String() string {
	if i.hasName {
		return fmt.Sprintf(
			"%s (ordinal %d, RVA 0x%08X)",
			i.name,
			i.ordinal,
			i.rva,
		)
	} else {
		return fmt.Sprintf(
			"Ordinal %d only (RVA 0x%08X)",
			i.ordinal,
			i.rva,
		)
	}
}

type model struct {
	mode mode

	filePath string

	imports []*importItem
	exports []exportItem

	cursor int
	start  int

	width  int
	height int

	history []string
}

func (m *model) length() int {
	switch m.mode {
	case importMode:
		count := 0
		for i := range m.imports {
			count++
			if m.imports[i].showFunctions {
				for range m.imports[i].functions {
					count++
				}
			}
		}
		return count
	case exportMode:
		return len(m.exports)
	}
	return 0
}

func (m *model) mapFrom(item, function int) int {
	switch m.mode {
	case importMode:
		count := 0
		for i := range m.imports {
			if i == item && function == -1 {
				return count
			}
			count++
			if m.imports[i].showFunctions {
				for j := range m.imports[i].functions {
					if i == item && j == function {
						return count
					}
					count++
				}
			}
		}
	case exportMode:
		return item
	}

	return -1

}

func (m *model) mapIndex(index int) (int, int) {
	switch m.mode {
	case importMode:
		count := 0
		for i := range m.imports {
			if count == index {
				return i, -1
			}
			count++
			if m.imports[i].showFunctions {
				for j := range m.imports[i].functions {
					if count == index {
						return i, j
					}
					count++
				}
			}
		}
	case exportMode:
		return index, -1
	}

	return -1, -1
}

func initModel(filePath string, history []string) model {
	f := parseFile(filePath)

	result := model{
		filePath: filePath,
		history:  append(history, filePath),
		mode:     importMode,
	}

	if f.HasImport {
		for _, imp := range f.Imports {
			path, found := findDependency(imp.Name, filePath)
			item := &importItem{
				dllName:   imp.Name,
				found:     found,
				path:      path,
				functions: make([]string, len(imp.Functions)),
			}
			for i, fn := range imp.Functions {
				if fn.ByOrdinal {
					item.functions[i] = fmt.Sprintf("ordinal:%d", fn.Ordinal)
				} else {
					item.functions[i] = fn.Name
				}
			}
			result.imports = append(result.imports, item)
		}
	}
	if f.HasExport {
		for _, fn := range f.Export.Functions {
			var item exportItem
			if fn.Name != "" {
				item.name = fn.Name
				item.hasName = true
			}
			item.ordinal = fn.Ordinal
			item.rva = fn.FunctionRVA
			result.exports = append(result.exports, item)
		}
	}

	return result
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m *model) updateStart() {
	if m.cursor < m.start {
		m.start = m.cursor
		return
	}
	actualHeight := m.height - 3
	if m.cursor > m.start+actualHeight {
		m.start = m.cursor - actualHeight
	}
}

func (m *model) moveCursor(move int) (tea.Model, tea.Cmd) {
	m.cursor += move
	m.cursor = max(0, min(m.cursor, m.length()-1))
	m.updateStart()
	return m, nil
}

func (m *model) right() (tea.Model, tea.Cmd) {
	if m.length() == 0 {
		return m, nil
	}
	mappedCursor, _ := m.mapIndex(m.cursor)
	item := m.imports[mappedCursor]
	if item.found {
		newModel := initModel(item.path, m.history)
		newModel.width = m.width
		newModel.height = m.height
		return newModel, nil
	}

	return m, nil
}

func (m *model) left() (tea.Model, tea.Cmd) {
	last := len(m.history) - 2
	newModel := initModel(m.history[last], m.history[:last])
	newModel.width = m.width
	newModel.height = m.height
	return newModel, nil
}
