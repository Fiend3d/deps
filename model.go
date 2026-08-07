package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type mode int

const (
	importMode mode = iota
	exportMode
	treeMode
	unresolvedMode
)

// importItem is one row of the flat import list: a resolved dependency plus the
// list's own expand state.
type importItem struct {
	dependency
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

	// imports holds every import of the image; visibleImports is the filtered
	// view the list actually shows, rebuilt whenever the filter changes, so the
	// cursor can index it directly — the same split as roots and visible below.
	imports        []*importItem
	visibleImports []*importItem

	exports []exportItem

	// hideDelayed folds delay-loaded dependencies out of both dependency views.
	// Off by default: the list should agree with the image's import table until
	// the user asks otherwise.
	hideDelayed bool

	// Recursive tree state. roots holds the direct dependencies; visible is the
	// flattened list of on-screen rows, rebuilt whenever a node expands or
	// collapses, so the cursor can index it directly.
	roots   []*treeNode
	visible []*treeNode

	// missing holds every finding of the full-depth walk; visibleMissing is the
	// filtered view the list shows, the same split as the two pairs above. The
	// walk resolves the whole graph, so unlike the lazy tree it cannot run up
	// front: it runs on the first press of u, and scanned records that it has.
	missing        []unresolved
	visibleMissing []unresolved
	scanned        bool

	// search is the loader search path for this walk; resolve turns a module
	// path into its dependencies, and is swapped out in tests.
	search  *searchPath
	resolve resolver

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
		for i := range m.visibleImports {
			count++
			if m.visibleImports[i].showFunctions {
				for range m.visibleImports[i].functions {
					count++
				}
			}
		}
		return count
	case exportMode:
		return len(m.exports)
	case treeMode:
		return len(m.visible)
	case unresolvedMode:
		return len(m.visibleMissing)
	}
	return 0
}

// selectedNode returns the tree node under the cursor.
func (m *model) selectedNode() *treeNode {
	if m.mode != treeMode || m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor]
}

// selectedDep returns the dependency under the cursor, in whichever of the two
// dependency views is active.
func (m *model) selectedDep() (dependency, bool) {
	switch m.mode {
	case importMode:
		if m.length() == 0 {
			return dependency{}, false
		}
		item, _ := m.mapIndex(m.cursor)
		if item < 0 || item >= len(m.visibleImports) {
			return dependency{}, false
		}
		return m.visibleImports[item].dependency, true
	case treeMode:
		if node := m.selectedNode(); node != nil {
			return node.dep, true
		}
	}
	return dependency{}, false
}

// clampCursor keeps the cursor and scroll offset inside whichever list is on
// screen. Both refreshers rebuild a list the other mode may be showing, so the
// bound has to come from the active mode rather than the list just rebuilt.
func (m *model) clampCursor() {
	m.cursor = max(0, min(m.cursor, m.length()-1))
	m.updateStart()
}

// refreshVisible rebuilds the flattened tree after an expand or collapse and
// keeps the cursor and scroll offset in range.
func (m *model) refreshVisible() {
	m.visible = flattenTree(m.roots, m.hideDelayed)
	m.clampCursor()
}

// refreshImports rebuilds the filtered import list and keeps the cursor and
// scroll offset in range, the flat list's counterpart to refreshVisible.
func (m *model) refreshImports() {
	if !m.hideDelayed {
		m.visibleImports = m.imports
	} else {
		m.visibleImports = make([]*importItem, 0, len(m.imports))
		for _, item := range m.imports {
			if item.delayed {
				continue
			}
			m.visibleImports = append(m.visibleImports, item)
		}
	}
	m.clampCursor()
}

// refreshMissing rebuilds the filtered findings list, the unresolved view's
// counterpart to refreshVisible and refreshImports. Filtering the findings
// rather than the walk is deliberate — see filterDelayedFindings.
func (m *model) refreshMissing() {
	m.visibleMissing = filterDelayedFindings(m.missing, m.hideDelayed)
	m.clampCursor()
}

// setHideDelayed applies the filter and rebuilds every dependency view, so the
// flag, the key and navigation all go through one place.
func (m *model) setHideDelayed(hide bool) {
	m.hideDelayed = hide
	m.refreshImports()
	m.refreshVisible()
	m.refreshMissing()
}

// scanUnresolved walks the graph to full depth and caches the findings. It is
// the one blocking operation in the TUI, so it runs at most once per image —
// every other view is either already in memory or resolved lazily one node at a
// time.
func (m *model) scanUnresolved() {
	// A file we could not read has no resolver to walk with.
	if m.scanned || m.loadErr != "" {
		return
	}
	m.scanned = true

	// The unfiltered imports: hard is decided by seeing every edge to a module,
	// so the delay filter belongs after the walk, not before it.
	deps := make([]dependency, 0, len(m.imports))
	for _, item := range m.imports {
		deps = append(deps, item.dependency)
	}

	m.missing = unresolvedDeps(m.filePath, deps, m.resolve)
	m.refreshMissing()
}

// toggleNode opens or closes the node under the cursor, resolving its children
// on first open.
func (m *model) toggleNode() {
	node := m.selectedNode()
	if node == nil || !node.expandable() {
		return
	}

	node.toggle(m.resolve)
	if node.loadErr != "" {
		m.status = node.loadErr
	}
	m.refreshVisible()
}

// collapseNode closes the node under the cursor, or moves to its parent when it
// is already closed — the usual way out of a deep tree.
func (m *model) collapseNode() {
	node := m.selectedNode()
	if node == nil {
		return
	}

	if node.expanded {
		node.expanded = false
		m.refreshVisible()
		return
	}

	if node.parent == nil || node.parent.depth < 0 {
		return
	}
	for i, candidate := range m.visible {
		if candidate == node.parent {
			m.cursor = i
			m.updateStart()
			return
		}
	}
}

func (m *model) mapFrom(item, function int) int {
	switch m.mode {
	case importMode:
		count := 0
		for i := range m.visibleImports {
			if i == item && function == -1 {
				return count
			}
			count++
			if m.visibleImports[i].showFunctions {
				for j := range m.visibleImports[i].functions {
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
		for i := range m.visibleImports {
			if count == index {
				return i, -1
			}
			count++
			if m.visibleImports[i].showFunctions {
				for j := range m.visibleImports[i].functions {
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
	result.search = newSearchPath(searchDirs(result.history[0], f.NtHeader.FileHeader.Machine))

	result.resolve = func(path string) ([]dependency, error) {
		return resolveDeps(path, result.search)
	}

	deps := depsFromFile(f, result.search)
	for _, dep := range deps {
		result.imports = append(result.imports, &importItem{dependency: dep})
	}
	result.visibleImports = result.imports

	// The tree is only node structs until something is expanded, so building it
	// up front costs no I/O and leaves `r` instant.
	result.roots = newTree(deps, filePath)
	result.visible = flattenTree(result.roots, result.hideDelayed)

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
	dep, ok := m.selectedDep()
	if !ok || !dep.found {
		return m, nil
	}

	newModel := initModel(dep.path, m.history)
	if newModel.loadErr != "" {
		// Stay put and report it rather than descending into a file we could
		// not read.
		m.status = newModel.loadErr
		return m, nil
	}
	newModel.width = m.width
	newModel.height = m.height
	// Keep exploring the way the user already was.
	newModel.mode = m.mode
	newModel.setHideDelayed(m.hideDelayed)
	// The findings are per-image, so the cache does not carry over: descending
	// while in the unresolved view has to walk the module we landed on.
	if newModel.mode == unresolvedMode {
		newModel.scanUnresolved()
	}
	return newModel, nil
}

func (m *model) left() (tea.Model, tea.Cmd) {
	last := len(m.history) - 2
	newModel := initModel(m.history[last], m.history[:last])
	newModel.width = m.width
	newModel.height = m.height
	newModel.setHideDelayed(m.hideDelayed)
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
