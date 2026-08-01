package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/saferwall/pe"
)

type mode int

const (
	importMode mode = iota
	exportMode
)

type importItem struct {
	dllName string
	found   bool
	delayed bool
	// virtual marks an API set contract that has no file on disk; the loader
	// still satisfies it, so it is not a missing dependency.
	virtual       bool
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

	click mouseClick

	filePath string

	// toolset names the toolchain that built filePath, e.g. "MSVC 14.38.33145".
	toolset string

	imports []*importItem
	exports []exportItem

	cursor int
	start  int

	width  int
	height int

	history []string

	// loadErr is set when this model's own file could not be read; the list is
	// empty and the body shows the failure instead.
	loadErr string

	// status is a transient message shown in the help line, cleared on the next
	// keypress.
	status string
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
	result := model{
		filePath: filePath,
		history:  append(history, filePath),
		mode:     importMode,
	}

	f, err := parseFile(filePath)
	if err != nil {
		result.loadErr = err.Error()
		return result
	}
	// Every name below is copied into the model, so the mapping can go as soon
	// as we are done reading it.
	defer f.Close()

	result.toolset = describeToolset(f)

	// The loader resolves dependencies relative to the image the walk started
	// from, not to the file currently being inspected.
	dirs := searchDirs(result.history[0], f.NtHeader.FileHeader.Machine)

	addImport := func(name string, functions []pe.ImportFunction, delayed bool) {
		path, found := findDependency(name, dirs)
		item := &importItem{
			dllName:   name,
			found:     found,
			delayed:   delayed,
			virtual:   !found && isAPISet(name),
			path:      path,
			functions: make([]string, len(functions)),
		}
		for i, fn := range functions {
			if fn.ByOrdinal {
				item.functions[i] = fmt.Sprintf("ordinal:%d", fn.Ordinal)
			} else {
				item.functions[i] = fn.Name
			}
		}
		result.imports = append(result.imports, item)
	}

	if f.HasImport {
		for _, imp := range f.Imports {
			addImport(imp.Name, imp.Functions, false)
		}
	}
	if f.HasDelayImp {
		for _, imp := range f.DelayImports {
			addImport(imp.Name, imp.Functions, true)
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

// visibleRows is the number of list rows on screen; the header and the help
// line take one each.
func (m *model) visibleRows() int {
	return m.height - 2
}

func (m *model) updateStart() {
	visible := m.visibleRows()
	if visible < 1 {
		m.start = 0
		return
	}

	if m.cursor < m.start {
		m.start = m.cursor
	} else if m.cursor > m.start+visible-1 {
		m.start = m.cursor - visible + 1
	}

	// Never scroll past the end: without this the list keeps a stale offset
	// when the window grows and renders a blank region below the last row.
	m.start = max(0, min(m.start, m.length()-visible))
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
	if !item.found {
		return m, nil
	}

	newModel := initModel(item.path, m.history)
	if newModel.loadErr != "" {
		// Stay put and report it rather than descending into a file we could
		// not read.
		m.status = newModel.loadErr
		return m, nil
	}
	newModel.width = m.width
	newModel.height = m.height
	return newModel, nil
}

func (m *model) left() (tea.Model, tea.Cmd) {
	last := len(m.history) - 2
	newModel := initModel(m.history[last], m.history[:last])
	newModel.width = m.width
	newModel.height = m.height
	return newModel, nil
}

// copy puts text on the clipboard and reports the outcome in the help line, so
// a clipboard that refuses to initialise is not a silent no-op.
func (m *model) copy(label, text string) {
	if err := clipboardWrite(text); err != nil {
		m.status = "clipboard failed: " + err.Error()
		return
	}
	m.status = "copied " + label
}

func (m *model) handleWheel(steps int) (tea.Model, tea.Cmd) {
	visible := m.visibleRows()
	if visible < 1 || m.length() <= visible {
		return m, nil
	}

	m.start += steps
	m.start = max(0, min(m.start, m.length()-visible))
	// Drag the cursor along so the header counter keeps naming a visible row
	// and the next j/k continues from what the user is looking at.
	m.cursor = max(m.start, min(m.cursor, m.start+visible-1))
	return m, nil
}
