package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
	case exportMode:
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
				line += yellow.Render(item.name)
				leftLength := lipgloss.Width(line)
				rightStyle := style.Align(lipgloss.Right).Width(m.width - leftLength)
				if !current {
					rightStyle = rightStyle.Foreground(lipgloss.BrightBlack)
				}
				rightStr := fmt.Sprintf(" (ordinal %d, RVA 0x%08X)", item.ordinal, item.rva)
				if len(rightStr) <= m.width-leftLength {
					line += rightStyle.Render(rightStr)
				}
			} else {
				line += fmt.Sprintf("(ordinal %d only, RVA 0x%08X)", item.ordinal, item.rva)
			}

			s.WriteString(truncate(line, m.width))
			s.WriteRune('\n')
			lineCount++
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
