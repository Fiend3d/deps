package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/saferwall/pe"
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
	name string
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

func initModel(filePath string, f *pe.File) model {
	result := model{filePath: filePath}

	if f.Imports != nil {
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

func (m model) View() tea.View {
	var result tea.View
	result.AltScreen = true
	result.MouseMode = tea.MouseModeCellMotion

	var s strings.Builder

	style := lipgloss.NewStyle()
	red := style.Foreground(lipgloss.Red)
	green := style.Foreground(lipgloss.Green)
	yellow := style.Foreground(lipgloss.Yellow)
	gray := style.Foreground(lipgloss.BrightBlack)
	sky := style.Foreground(lipgloss.BrightBlue)
	blue := style.Foreground(lipgloss.Blue)

	header := m.filePath
	length := m.length()

	switch m.mode {
	case importMode:
		header += yellow.Render(" IMPORT ")
		if length == 0 {
			header += "[empty]"
		} else {
			header += fmt.Sprintf("[%d/%d]", m.cursor+1, length)
		}
	case exportMode:
		header += sky.Render(" EXPORT ")
		if len(m.exports) == 0 {
			header += "[empty]"
		} else {
			header += fmt.Sprintf("[%d/%d]", m.cursor+1, len(m.exports))
		}
	}

	s.WriteString(truncate(header, m.width))
	s.WriteRune('\n')

	lineCount := 1
	switch m.mode {
	case importMode:
		if length == 0 {
			s.WriteString(truncate(
				red.Render("No imports"),
				m.width,
			))
			s.WriteRune('\n')
			lineCount++
		} else {
			for i := range length {
				if i+1 > m.height-2 || i+m.start >= length {
					break
				}

				index := i + m.start
				mappedIndex, function := m.mapIndex(index)
				current := index == m.cursor
				cursor := "   "

				if current {
					cursor = " > "
				}

				line := cursor
				item := m.imports[mappedIndex]

				if function == -1 {
					dllName := item.dllName

					if item.found {
						line += green.Render(dllName)
						rightSize := m.width - lipgloss.Width(line) - 1
						rightStr := truncate(item.path, rightSize)
						rightStyle := style.Width(rightSize).Align(lipgloss.Right)
						if current {
							line += " " + rightStyle.Render(rightStr)
						} else {
							line += " " + rightStyle.Foreground(lipgloss.BrightBlack).Render(rightStr)
						}
					} else {
						line += red.Render(dllName)
					}
				} else {
					functionStr := item.functions[function]
					if function != len(item.functions)-1 {
						line += gray.Render("├─") + functionStr
					} else {
						line += gray.Render("└─") + functionStr
					}
				}

				s.WriteString(truncate(line, m.width))
				s.WriteRune('\n')
				lineCount++
			}
		}
	}

	for i := lineCount; i < m.height-1; i++ {
		s.WriteRune('\n')
	}

	help := blue.Render("Keys: ")

	mappedCursor, function := m.mapIndex(m.cursor)

	switch m.mode {
	case importMode:
		help += "Tab"
		help += blue.Render(" - EXPORT ")
		help += "Space"
		if function == -1 {
			item := m.imports[mappedCursor]
			if item.found {
				if !item.showFunctions {
					help += blue.Render(" - Show functions ")
				} else {
					help += blue.Render(" - Hide functions ")
				}
			}
		} else {
			help += blue.Render(" - Hide functions ")
		}
	case exportMode:
		help += "Tab"
		help += blue.Render(" - IMPORT ")
	}

	help += "a"
	help += blue.Render(" - Copy all ")
	help += "c"
	help += blue.Render(" - Copy selected")

	s.WriteString(truncate(help, m.width))

	result.Content = s.String()
	return result
}

var (
	// Set by build flags
	Version   = "dev"
	GitCommit = ""
)

func main() {
	version := flag.Bool("v", false, "print version")

	flag.Parse()

	args := flag.Args()

	if *version {
		fmt.Printf("version:%s (%s)\n", Version, GitCommit)
		return
	}

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: deps [flags] <filepath>\n")
		fmt.Fprintf(os.Stderr, "  -v    print version\n")
		os.Exit(1)
	}

	filePath, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("failed to convert (%s) to absulte path: %v", args[0], err)
	}

	f, err := pe.New(filePath, &pe.Options{})
	if err != nil {
		log.Fatalf("failed to open PE file: %v", err)
	}

	err = f.Parse()
	if err != nil {
		log.Fatalf("failed to parse PE: %v", err)
	}

	p := tea.NewProgram(initModel(filePath, f))

	_, err = p.Run()
	if err != nil {
		log.Fatalf("failed to launch the program: %s", err)
	}
}
