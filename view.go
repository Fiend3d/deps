package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// counter renders the header's position indicator.
func (m *model) counter(length int) string {
	switch {
	case m.loadErr != "":
		return "[error]"
	case length == 0:
		return "[empty]"
	default:
		return fmt.Sprintf("[%d/%d]", m.cursor+1, length)
	}
}

// delayedHelp names what the d key would do next, the way the function list
// help names its own toggle.
func (m *model) delayedHelp() string {
	if m.hideDelayed {
		return " - Show delayed "
	}
	return " - Hide delayed "
}

// emptyBody renders the body of an empty list: the load failure if there was
// one, otherwise the given reassurance.
func (m *model) emptyBody(empty string) string {
	style := lg.NewStyle()
	if m.loadErr != "" {
		return style.Foreground(lg.BrightRed).Render(m.loadErr)
	}
	return style.Foreground(lg.Green).Render(empty)
}

// renderTreeRow draws one row of the dependency tree: cursor, branch glyph,
// module name coloured by how it resolved, state marker, and the directory
// right-aligned when there is room for it.
func (m *model) renderTreeRow(node *treeNode, current bool) string {
	style := lg.NewStyle()

	line := "   "
	if current {
		line = " > "
	}
	line += style.Foreground(lg.BrightBlack).Render(node.prefix(m.hideDelayed))

	label := node.dep.label() + node.marker()

	var colour ansi.BasicColor
	switch {
	case node.cycle:
		colour = lg.Yellow
		if current {
			colour = lg.BrightYellow
		}
	case node.dep.found:
		colour = lg.Green
		if current {
			colour = lg.BrightGreen
		}
	case node.dep.virtual:
		colour = lg.Cyan
		if current {
			colour = lg.BrightCyan
		}
	default:
		colour = lg.Red
		if current {
			colour = lg.BrightRed
		}
	}
	line += style.Foreground(colour).Render(label)

	if !node.dep.found {
		return line
	}

	// Drop the directory column rather than asking for a negative width.
	rightSize := m.width - lg.Width(line) - 1
	if rightSize <= 0 {
		return line
	}
	rightStyle := style.Width(rightSize).Align(lg.Right)
	if !current {
		rightStyle = rightStyle.Foreground(lg.BrightBlack)
	}

	return line + " " + rightStyle.Render(truncate(filepath.Dir(node.dep.path), rightSize))
}

func (m model) View() tea.View {
	var result tea.View
	result.AltScreen = true
	result.MouseMode = tea.MouseModeCellMotion

	switch m.mode {
	case importMode:
		result.WindowTitle = "Deps - Import"
	case exportMode:
		result.WindowTitle = "Deps - Export"
	case treeMode:
		result.WindowTitle = "Deps - Tree"
	}

	var s strings.Builder

	style := lg.NewStyle()

	header := m.filePath
	if m.toolset != "" {
		header += style.Foreground(lg.BrightBlack).Render(" " + m.toolset)
	}
	length := m.length()

	switch m.mode {
	case importMode:
		header += style.Foreground(lg.Yellow).Render(" IMPORT ")
		header += m.counter(length)
	case exportMode:
		header += style.Foreground(lg.BrightBlue).Render(" EXPORT ")
		header += m.counter(len(m.exports))
	case treeMode:
		header += style.Foreground(lg.BrightMagenta).Render(" TREE ")
		header += m.counter(length)
	}

	// Say so when rows are being withheld, or the counter quietly understates
	// what the image imports.
	if m.hideDelayed && m.mode != exportMode {
		header += style.Foreground(lg.BrightBlack).Render(" -delay")
	}

	s.WriteString(truncate(header, m.width))
	s.WriteRune('\n')

	lineCount := 1
	switch m.mode {
	case importMode:
		if length == 0 {
			s.WriteString(truncate(m.emptyBody("No imports"), m.width))
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
				item := m.visibleImports[mappedIndex]

				if function == -1 {
					dllName := item.label()

					if item.found {
						if current {
							line += style.Foreground(lg.BrightGreen).Render(dllName)
						} else {
							line += style.Foreground(lg.Green).Render(dllName)
						}
						// Drop the directory column entirely rather than
						// asking for a negative width when the name alone
						// fills the window.
						rightSize := m.width - lg.Width(line) - 1
						if rightSize > 0 {
							rightDir := filepath.Dir(item.path)
							rightStr := truncate(rightDir, rightSize)
							rightStyle := style.Width(rightSize).Align(lg.Right)
							if current {
								line += " " + rightStyle.Render(rightStr)
							} else {
								line += " " + rightStyle.Foreground(lg.BrightBlack).Render(rightStr)
							}
						}
					} else if item.virtual {
						if current {
							line += style.Foreground(lg.BrightCyan).Render(dllName)
						} else {
							line += style.Foreground(lg.Cyan).Render(dllName)
						}
					} else {
						if current {
							line += style.Foreground(lg.BrightRed).Render(dllName)
						} else {
							line += style.Foreground(lg.Red).Render(dllName)
						}
					}
				} else {
					functionStr := item.functions[function]
					if function != len(item.functions)-1 {
						line += style.Foreground(lg.BrightBlack).Render("├─") + functionStr
					} else {
						line += style.Foreground(lg.BrightBlack).Render("└─") + functionStr
					}
				}

				s.WriteString(truncate(line, m.width))
				s.WriteRune('\n')
				lineCount++
			}
		}
	case treeMode:
		if length == 0 {
			s.WriteString(truncate(m.emptyBody("No dependencies"), m.width))
			s.WriteRune('\n')
			lineCount++
		} else {
			for i := range length {
				if i+1 > m.height-2 || i+m.start >= length {
					break
				}

				index := i + m.start
				s.WriteString(truncate(
					m.renderTreeRow(m.visible[index], index == m.cursor),
					m.width,
				))
				s.WriteRune('\n')
				lineCount++
			}
		}
	case exportMode:
		if length == 0 {
			s.WriteString(truncate(m.emptyBody("No exports"), m.width))
			s.WriteRune('\n')
			lineCount++
		} else {
			for i := range length {
				if i+1 > m.height-2 || i+m.start >= length {
					break
				}

				index := i + m.start
				current := index == m.cursor
				cursor := "   "

				if current {
					cursor = " > "
				}

				line := cursor

				item := m.exports[index]
				if item.hasName {
					if current {
						line += style.Foreground(lg.BrightYellow).Render(item.name)
					} else {
						line += style.Foreground(lg.Yellow).Render(item.name)
					}
					rightSize := m.width - lg.Width(line)
					if rightSize > 0 {
						rightStr := fmt.Sprintf(" (ordinal %d, RVA 0x%08X)", item.ordinal, item.rva)
						rightStr = truncate(rightStr, rightSize)
						rightStyle := style.Width(rightSize).Align(lg.Right)
						if !current {
							rightStyle = rightStyle.Foreground(lg.BrightBlack)
						}
						line += rightStyle.Render(rightStr)
					}
				} else {
					if current {
						line += fmt.Sprintf("(ordinal %d only, RVA 0x%08X)", item.ordinal, item.rva)
					} else {
						line += style.Foreground(lg.BrightBlack).Render(fmt.Sprintf("(ordinal %d only, RVA 0x%08X)", item.ordinal, item.rva))
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

	help := style.Foreground(lg.BrightBlue).Render("Keys: ")

	// Only the flat import list uses the two-level mapping; in tree mode the
	// cursor indexes m.visible directly.
	mappedCursor, function := -1, -1
	if m.mode == importMode {
		mappedCursor, function = m.mapIndex(m.cursor)
	}

	switch m.mode {
	case treeMode:
		help += "r"
		help += style.Foreground(lg.BrightBlue).Render(" - IMPORT ")
		if node := m.selectedNode(); node != nil && node.expandable() {
			help += "Space"
			if node.expanded {
				help += style.Foreground(lg.BrightBlue).Render(" - Collapse ")
			} else {
				help += style.Foreground(lg.BrightBlue).Render(" - Expand ")
			}
			help += "Enter"
			help += style.Foreground(lg.BrightBlue).Render(" - Open ")
		}
		help += "d" + style.Foreground(lg.BrightBlue).Render(m.delayedHelp())

	case importMode:
		help += "Tab"
		help += style.Foreground(lg.BrightBlue).Render(" - EXPORT ")
		help += "r"
		help += style.Foreground(lg.BrightBlue).Render(" - TREE ")

		if length > 0 {
			item := m.visibleImports[mappedCursor]
			if item.found {
				help += "Space"
				if function == -1 {
					if !item.showFunctions {
						help += style.Foreground(lg.BrightBlue).Render(" - Show functions ")
					} else {
						help += style.Foreground(lg.BrightBlue).Render(" - Hide functions ")
					}
				} else {
					help += style.Foreground(lg.BrightBlue).Render(" - Hide functions ")
				}
			}
		}
		help += "d" + style.Foreground(lg.BrightBlue).Render(m.delayedHelp())

	case exportMode:
		help += "Tab"
		help += style.Foreground(lg.BrightBlue).Render(" - IMPORT ")
	}

	if length > 0 {
		help += "["
		help += style.Foreground(lg.BrightBlue).Render("Copy: ")

		help += "a"
		help += style.Foreground(lg.BrightBlue).Render(" - All ")

		switch m.mode {
		case importMode, treeMode:
			help += "c"
			help += style.Foreground(lg.BrightBlue).Render(" - Selected")

			if dep, ok := m.selectedDep(); ok && dep.found {
				help += " p"
				help += style.Foreground(lg.BrightBlue).Render(" - Path ")
				help += "f"
				help += style.Foreground(lg.BrightBlue).Render(" - Functions")
			}
			help += "] "

		case exportMode:
			help += "c"
			help += style.Foreground(lg.BrightBlue).Render(" - Copy selected ")
		}
	}

	if m.status != "" {
		s.WriteString(truncate(style.Foreground(lg.BrightYellow).Render(m.status), m.width))
	} else {
		s.WriteString(truncate(help, m.width))
	}

	result.Content = s.String()
	return result
}
