package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
)

func (m model) View() tea.View {
	var result tea.View
	result.AltScreen = true
	result.MouseMode = tea.MouseModeCellMotion

	switch m.mode {
	case importMode:
		result.WindowTitle = "Deps - Import"
	case exportMode:
		result.WindowTitle = "Deps - Export"
	}

	var s strings.Builder

	style := lg.NewStyle()

	header := m.filePath
	length := m.length()

	switch m.mode {
	case importMode:
		header += style.Foreground(lg.Yellow).Render(" IMPORT ")
		if length == 0 {
			header += "[empty]"
		} else {
			header += fmt.Sprintf("[%d/%d]", m.cursor+1, length)
		}
	case exportMode:
		header += style.Foreground(lg.BrightBlue).Render(" EXPORT ")
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
				style.Foreground(lg.Green).Render("No imports"),
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
						if current {
							line += style.Foreground(lg.BrightGreen).Render(dllName)
						} else {
							line += style.Foreground(lg.Green).Render(dllName)
						}
						rightSize := m.width - lg.Width(line) - 1
						rightStr := truncate(item.path, rightSize)
						rightStyle := style.Width(rightSize).Align(lg.Right)
						if current {
							line += " " + rightStyle.Render(rightStr)
						} else {
							line += " " + rightStyle.Foreground(lg.BrightBlack).Render(rightStr)
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
	case exportMode:
		if length == 0 {
			s.WriteString(truncate(
				style.Foreground(lg.Green).Render("No exports"),
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
					rightStr := fmt.Sprintf(" (ordinal %d, RVA 0x%08X)", item.ordinal, item.rva)
					rightStr = truncate(rightStr, rightSize)
					rightStyle := style.Width(rightSize).Align(lg.Right)
					if !current {
						rightStyle = rightStyle.Foreground(lg.BrightBlack)
					}
					line += rightStyle.Render(rightStr)
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

	mappedCursor, function := m.mapIndex(m.cursor)

	switch m.mode {
	case importMode:
		help += "Tab"
		help += style.Foreground(lg.BrightBlue).Render(" - EXPORT ")

		if length > 0 {
			item := m.imports[mappedCursor]
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

	case exportMode:
		help += "Tab"
		help += style.Foreground(lg.BrightBlue).Render(" - IMPORT ")
	}

	if length > 0 {
		help += "a"
		help += style.Foreground(lg.BrightBlue).Render(" - Copy all ")

		switch m.mode {
		case importMode:
			item := m.imports[mappedCursor]
			if item.found {
				help += "c"
				help += style.Foreground(lg.BrightBlue).Render(" - Copy selected path ")
				help += "f"
				help += style.Foreground(lg.BrightBlue).Render(" - Copy functions ")
			} else {
				help += "c"
				help += style.Foreground(lg.BrightBlue).Render(" - Copy selected ")
			}

		case exportMode:
			help += "c"
			help += style.Foreground(lg.BrightBlue).Render(" - Copy selected ")
		}
	}

	s.WriteString(truncate(help, m.width))

	result.Content = s.String()
	return result
}
