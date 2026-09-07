package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Fiend3d/catatui"
	"github.com/Fiend3d/catatui/widgets"
)

// The palette. Every row colour is chosen once here so the list, the header and
// the printed reports cannot drift apart.
var (
	dimStyle    = catatui.NewStyle().Fg(catatui.ColorDarkGray)
	greenStyle  = catatui.NewStyle().Fg(catatui.ColorGreen)
	redStyle    = catatui.NewStyle().Fg(catatui.ColorRed)
	cyanStyle   = catatui.NewStyle().Fg(catatui.ColorCyan)
	yellowStyle = catatui.NewStyle().Fg(catatui.ColorYellow)
	labelStyle  = catatui.NewStyle().Fg(catatui.ColorLightBlue)
	statusStyle = catatui.NewStyle().Fg(catatui.ColorLightYellow)
	errorStyle  = catatui.NewStyle().Fg(catatui.ColorLightRed)

	// The selected row keeps its own colour and is picked out with weight
	// alone: Cell.SetStyle layers, so bold is added over whatever the row
	// already is.
	highlightStyle = catatui.NewStyle().AddModifier(catatui.ModifierBold)
)

// highlightSymbol is the cursor, and its width is the gutter every row carries
// whether or not it is selected.
const highlightSymbol = " > "

var tabLabels = []string{"Import", "Export", "Tree", "Unresolved"}

// modeAccent is the colour of the tab for each mode.
func modeAccent(m mode) catatui.Style {
	return catatui.NewStyle().Fg(modeColor(m))
}

func modeColor(m mode) catatui.Color {
	switch m {
	case exportMode:
		return catatui.ColorLightBlue
	case treeMode:
		return catatui.ColorLightMagenta
	case unresolvedMode:
		return catatui.ColorLightRed
	default:
		return catatui.ColorYellow
	}
}

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

// twoColumnLine lays left out at the start of width columns and right at the
// end. The right column is dropped rather than squeezed when there is no room
// for it, which is what keeps a narrow window showing the name that matters.
func twoColumnLine(left []catatui.Span, right string, rightStyle catatui.Style, width int) catatui.Line {
	used := 0
	for _, s := range left {
		used += catatui.StringWidth(s.GetContent())
	}

	// One column of gap is the minimum, so the two never run together.
	room := width - used - 1
	if right == "" || room <= 0 {
		return catatui.NewLine(left...)
	}

	right = truncate(right, room)
	pad := room - catatui.StringWidth(right) + 1

	spans := make([]catatui.Span, 0, len(left)+2)
	spans = append(spans, left...)
	spans = append(spans,
		catatui.NewSpan(strings.Repeat(" ", pad)),
		catatui.NewStyledSpan(right, rightStyle))
	return catatui.NewLine(spans...)
}

// depSpan colours a dependency name by how it resolved.
func depSpan(dep dependency, name string) catatui.Span {
	switch {
	case dep.found:
		return catatui.NewStyledSpan(name, greenStyle)
	case dep.virtual:
		return catatui.NewStyledSpan(name, cyanStyle)
	default:
		return catatui.NewStyledSpan(name, redStyle)
	}
}

// rows builds one list item per visible row of the active mode. width is the
// room a row's content has: the list area less the cursor gutter.
func (m *model) rows(width int) []widgets.ListItem {
	items := make([]widgets.ListItem, 0, m.length())
	line := func(l catatui.Line) {
		items = append(items, widgets.NewListItemFromLine(l))
	}

	switch m.mode {
	case importMode:
		for i := range m.length() {
			index, function := m.mapIndex(i)
			item := m.visibleImports[index]

			if function == -1 {
				dir := ""
				if item.found {
					dir = filepath.Dir(item.path)
				}
				line(twoColumnLine(
					[]catatui.Span{depSpan(item.dependency, item.label())},
					dir, dimStyle, width))
				continue
			}

			branch := "├─"
			if function == len(item.functions)-1 {
				branch = "└─"
			}
			line(catatui.NewLine(
				catatui.NewStyledSpan(branch, dimStyle),
				catatui.NewSpan(item.functions[function]),
			))
		}

	case treeMode:
		for _, node := range m.visible {
			var style catatui.Style
			switch {
			case node.cycle:
				style = yellowStyle
			case node.dep.found:
				style = greenStyle
			case node.dep.virtual:
				style = cyanStyle
			default:
				style = redStyle
			}

			left := []catatui.Span{
				catatui.NewStyledSpan(node.prefix(m.hideDelayed), dimStyle),
				catatui.NewStyledSpan(node.dep.label()+node.marker(), style),
			}

			dir := ""
			if node.dep.found {
				dir = filepath.Dir(node.dep.path)
			}
			line(twoColumnLine(left, dir, dimStyle, width))
		}

	case unresolvedMode:
		for _, hit := range m.visibleMissing {
			// Red is a module the image needs to start; yellow only fails if
			// the delay-loaded path is ever called. Same split as the printed
			// report.
			style := yellowStyle
			if hit.hard {
				style = redStyle
			}
			line(twoColumnLine(
				[]catatui.Span{catatui.NewStyledSpan(hit.label(), style)},
				"<- "+hit.importerList(), dimStyle, width))
		}

	case exportMode:
		for _, item := range m.exports {
			if !item.hasName {
				line(catatui.LineFromStyledString(
					fmt.Sprintf("(ordinal %d only, RVA 0x%08X)", item.ordinal, item.rva),
					dimStyle))
				continue
			}
			line(twoColumnLine(
				[]catatui.Span{catatui.NewStyledSpan(item.name, yellowStyle)},
				fmt.Sprintf("(ordinal %d, RVA 0x%08X)", item.ordinal, item.rva),
				dimStyle, width))
		}
	}

	return items
}

// emptyBody is what stands in for the list when there is nothing to show: the
// load failure if there was one, otherwise the given reassurance.
func (m *model) emptyBody(empty string) widgets.Paragraph {
	if m.loadErr != "" {
		return widgets.NewParagraph(m.loadErr).
			Style(errorStyle).
			Wrap(widgets.Wrap{Trim: false})
	}
	return widgets.NewParagraph(empty).Style(greenStyle)
}

// gutter insets an area by the width of the cursor column, or leaves it alone
// when there is not room to spare.
func gutter(area catatui.Rect) catatui.Rect {
	width := uint16(catatui.StringWidth(highlightSymbol))
	if area.Width <= width {
		return area
	}
	area.X += width
	area.Width -= width
	return area
}

// emptyMessage names what is missing, per mode.
func (m *model) emptyMessage() string {
	switch m.mode {
	case exportMode:
		return "No exports"
	case treeMode:
		return "No dependencies"
	case unresolvedMode:
		return "No unresolved dependencies"
	default:
		return "No imports"
	}
}

// Render draws the whole screen: the mode tabs, the bordered list with the file
// it is describing in its title, and the help line. The model is itself the
// widget, so a test can draw it into a plain buffer with no terminal involved.
func (m *model) Render(area catatui.Rect, buf *catatui.Buffer) {
	rows := catatui.VerticalLayout(
		catatui.Length(1),
		catatui.Fill(1),
		catatui.Length(1),
	).Split(area)

	length := m.length()
	m.tabsArea = rows[0]

	m.renderTabs(rows[0], buf)
	m.renderBody(rows[1], buf, length)
	m.renderHelp(rows[2], buf, length)
}

func (m *model) renderTabs(area catatui.Rect, buf *catatui.Buffer) {
	catatui.RenderWidget(
		widgets.NewTabs(tabLabels...).
			Select(int(m.mode)).
			Style(dimStyle).
			HighlightStyle(modeAccent(m.mode).AddModifier(catatui.ModifierBold)).
			Divider("│"),
		area, buf)
}

func (m *model) renderBody(area catatui.Rect, buf *catatui.Buffer, length int) {
	title := []catatui.Span{catatui.NewSpan(" " + m.filePath)}
	if m.toolset != "" {
		title = append(title, catatui.NewStyledSpan(" "+m.toolset, dimStyle))
	}
	title = append(title, catatui.NewSpan(" "))

	counter := []catatui.Span{catatui.NewSpan(" " + m.counter(length))}
	// Say so when rows are being withheld, or the counter quietly understates
	// what the image imports.
	if m.hideDelayed && m.mode != exportMode {
		counter = append(counter, catatui.NewStyledSpan(" -delay", dimStyle))
	}
	counter = append(counter, catatui.NewSpan(" "))

	block := widgets.Bordered().
		BorderStyle(dimStyle).
		TitleTop(catatui.NewLine(title...)).
		TitleTop(catatui.NewLine(counter...).Right())

	// The list area is remembered because a mouse click arrives later with
	// nothing but a row and column to say what it hit.
	m.listArea = block.Inner(area)

	if length == 0 {
		catatui.RenderWidget(block, area, buf)
		// Indented by the cursor gutter, so the message starts where the rows
		// it stands in for would have.
		catatui.RenderWidget(m.emptyBody(m.emptyMessage()), gutter(m.listArea), buf)
		return
	}

	visible := int(m.listArea.Height)
	if visible > 0 {
		// Never leave the view scrolled past the end. The list widget keeps
		// whatever offset it is given as long as the selection is visible, so
		// growing the window would otherwise strand the rows at the top and
		// leave a blank region below the last one.
		m.list.SetOffset(max(0, min(m.list.Offset(), length-visible)))
	}

	start := m.list.Offset()
	if !m.cursorDetached || (m.cursor >= start && m.cursor < start+visible) {
		m.list.Select(m.cursor)
	} else {
		// CatatUI follows ratatui and resets the offset when selection is
		// cleared, so restore the independently scrolled viewport afterward.
		m.list.SelectNone()
		m.list.SetOffset(start)
	}

	rowWidth := int(m.listArea.Width) - catatui.StringWidth(highlightSymbol)
	if length > visible {
		// Leave one blank cell between the right-aligned directory and the
		// scrollbar drawn over the block's right border.
		rowWidth--
	}
	list := widgets.NewList(m.rows(max(0, rowWidth))...).
		Block(block).
		HighlightSymbol(highlightSymbol).
		HighlightSpacing(widgets.HighlightSpacingAlways).
		HighlightStyle(highlightStyle)
	list.RenderStateful(area, buf, &m.list)

	// The offset is only settled by the render above, so the thumb follows it
	// here rather than in the event handler.
	if length > visible {
		// ScrollbarState positions an item within the full content range, while
		// ListState exposes the first visible row (whose maximum is
		// length-visible). Scale between those ranges so the thumb reaches both
		// ends of the track without changing its viewport-sized length.
		position := m.list.Offset() * (length - 1) / (length - visible)
		m.scroll = m.scroll.
			ContentLength(length).
			ViewportContentLength(visible).
			Position(position)
		widgets.NewScrollbar(widgets.ScrollbarVerticalRight).
			BeginSymbolNone().
			EndSymbolNone().
			// Keep the block's single-line border as the track. The scrollbar's
			// default double-line track clashes with it visually; a transparent
			// track replaces only the cells occupied by the thumb.
			TrackSymbolNone().
			RenderStateful(area.Inner(catatui.Margin{Vertical: 1}), buf, &m.scroll)
	}
}

// helpLine is the key list for the active mode, or the transient status message
// when there is one.
func (m *model) helpLine(length int) catatui.Line {
	if m.status != "" {
		return catatui.NewLine(
			catatui.NewStyledSpan(" ◆ ", statusStyle.AddModifier(catatui.ModifierBold)),
			catatui.NewStyledSpan(m.status, statusStyle),
		)
	}

	modeName := strings.ToUpper(tabLabels[int(m.mode)])
	spans := []catatui.Span{catatui.NewStyledSpan(
		" "+modeName+" ",
		catatui.NewStyle().Fg(catatui.ColorBlack).Bg(modeColor(m.mode)).AddModifier(catatui.ModifierBold),
	)}
	first := true
	command := func(key, label string) {
		if first {
			spans = append(spans, catatui.NewSpan("  "))
			first = false
		} else {
			spans = append(spans, catatui.NewStyledSpan("  •  ", dimStyle))
		}
		spans = append(spans,
			catatui.NewStyledSpan("[", dimStyle),
			catatui.NewStyledSpan(key, modeAccent(m.mode).AddModifier(catatui.ModifierBold)),
			catatui.NewStyledSpan("] ", dimStyle),
			catatui.NewStyledSpan(label, labelStyle),
		)
	}

	// Only the flat import list uses the two-level mapping; everywhere else the
	// cursor indexes the visible rows directly.
	mappedCursor, function := -1, -1
	if m.mode == importMode && length > 0 {
		mappedCursor, function = m.mapIndex(m.cursor)
	}
	delayedLabel := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.delayedHelp()), "-"))

	switch m.mode {
	case treeMode:
		command("r", "imports")
		if node := m.selectedNode(); node != nil && node.expandable() {
			if node.expanded {
				command("Space", "collapse")
			} else {
				command("Space", "expand")
			}
			command("Enter", "open")
		}
		command("u", "unresolved")
		command("d", delayedLabel)

	case unresolvedMode:
		command("u", "imports")
		command("r", "tree")
		command("d", delayedLabel)

	case importMode:
		command("Tab", "exports")
		command("r", "tree")
		command("u", "unresolved")

		if length > 0 && m.visibleImports[mappedCursor].found {
			if function == -1 && !m.visibleImports[mappedCursor].showFunctions {
				command("Space", "show functions")
			} else {
				command("Space", "hide functions")
			}
		}
		command("d", delayedLabel)

	case exportMode:
		command("Tab", "imports")
	}

	if length > 0 {
		command("a", "copy all")

		switch m.mode {
		case importMode, treeMode:
			command("c", "copy selected")

			if dep, ok := m.selectedDep(); ok && dep.found {
				command("p", "copy path")
				command("f", "copy functions")
			}

		case unresolvedMode:
			// No path and no functions to copy: the module resolved to nothing.
			command("c", "copy selected")

		case exportMode:
			command("c", "copy selected")
		}
	}

	return catatui.NewLine(spans...)
}

func (m *model) renderHelp(area catatui.Rect, buf *catatui.Buffer, length int) {
	if area.IsEmpty() {
		return
	}
	buf.SetLine(area.X, area.Y, m.helpLine(length), area.Width)
}
