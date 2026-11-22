package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type tabRange struct {
	tab    TabID
	startX int
	endX   int
}

// handleMouse implements mouse handling adhering strictly to Golden Rule #3:
// - Uses dynamically computed X ranges for horizontal tab bar and dual-pane panels
// - Uses dynamically computed Y offsets for vertical list item selection
func (m Model) handleMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}

		// Tab bar row -> Use dynamically computed X ranges to switch tabs
		if msg.Y == m.tabBarY() {
			tabIdx := m.calculateTabFromX(msg.X)
			if tabIdx >= 0 && tabIdx < len(TabNames) {
				m.activeTab = TabID(tabIdx)
				m.selectedIdx = 0
				m.scrollOffset = 0
				return m, nil
			}
		}

		// Content rows below tab bar
		if msg.Y > m.tabBarY() && m.isWideLayout() {
			// Side-by-side mode: use X coordinates to focus panel
			leftWidth, _ := m.calculateDualPaneWidths()
			if msg.X < leftWidth {
				m.focusedPanel = "left"
				if m.activeTab == TabPackages {
					m.selectedIdx = m.toolsSelectedIdx
					m.scrollOffset = m.toolsScrollOffset
				} else if m.activeTab == TabConfigs {
					m.selectedIdx = m.intConfigsSelectedIdx
					m.scrollOffset = m.intConfigsScrollOffset
				}
			} else {
				m.focusedPanel = "right"
				if m.activeTab == TabPackages {
					m.selectedIdx = m.depsSelectedIdx
					m.scrollOffset = m.depsScrollOffset
				} else if m.activeTab == TabConfigs {
					m.selectedIdx = m.notIntConfigsSelectedIdx
					m.scrollOffset = m.notIntConfigsScrollOffset
				}
			}
		}

		// Vertical list selection: use relative Y coordinate + scrollOffset
		contentStartY := m.contentStartY()
		relY := msg.Y - contentStartY
		if relY >= 0 {
			targetIdx := m.scrollOffset + relY
			maxIdx := m.maxSelectedIndex()
			if targetIdx <= maxIdx {
				m.selectedIdx = targetIdx
				m.adjustScroll()
			}
		}

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
			m.adjustScroll()
		case tea.MouseWheelDown:
			if m.selectedIdx < m.maxSelectedIndex() {
				m.selectedIdx++
			}
			m.adjustScroll()
		}
	}

	return m, nil
}

// tabBarY computes the 0-based Y row of the tab bar from the actual rendered header height.
func (m Model) tabBarY() int {
	header := m.renderHeader()
	return len(strings.Split(header, "\n"))
}

// contentStartY computes the 0-based Y row where scrollable list items begin
// based on the actual rendered header, tab bar, top border, and active tab's inner header rows.
func (m Model) contentStartY() int {
	headerLines := len(strings.Split(m.renderHeader(), "\n"))
	tabsLines := len(strings.Split(m.renderTabs(), "\n"))
	const topBorderLine = 1
	return headerLines + tabsLines + topBorderLine + m.panelHeaderLineCount()
}

func (m Model) panelHeaderLineCount() int {
	switch m.activeTab {
	case TabPackages, TabConfigs, TabDoctor:
		return 1
	case TabHacks:
		return 3
	case TabSecurity:
		if m.isWideLayout() {
			return 6
		}
		return 2
	default:
		return 1
	}
}

// tabRanges computes the exact [startX, endX) column bounds for each rendered tab pill.
func (m Model) tabRanges() []tabRange {
	ranges := make([]tabRange, 0, len(TabNames))
	cursor := 1 // leading single space in renderTabs
	for i, name := range TabNames {
		var rendered string
		if TabID(i) == m.activeTab {
			rendered = m.styles.TabActive.Render(name)
		} else {
			rendered = m.styles.TabInactive.Render(name)
		}
		w := ansi.StringWidth(rendered)
		ranges = append(ranges, tabRange{
			tab:    TabID(i),
			startX: cursor,
			endX:   cursor + w,
		})
		cursor += w + 1 // trailing single space separator
	}
	return ranges
}

// calculateTabFromX matches horizontal tab clicks to TabID using actual rendered tab widths (Golden Rule #3).
func (m Model) calculateTabFromX(x int) int {
	for _, r := range m.tabRanges() {
		if x >= r.startX && x < r.endX {
			return int(r.tab)
		}
	}
	return -1
}

// isWideLayout checks if the terminal is wide enough for side-by-side panels.
func (m Model) isWideLayout() bool {
	return m.width >= 100
}

// calculateDualPaneWidths computes weights-based panel widths (Golden Rule #4).
func (m Model) calculateDualPaneWidths() (int, int) {
	availWidth := m.effectiveWidth()
	if availWidth < 40 {
		availWidth = 40
	}

	leftWeight, rightWeight := 1, 1
	if m.accordionMode {
		if m.focusedPanel == "left" {
			leftWeight = 2
		} else {
			rightWeight = 2
		}
	}

	totalWeight := leftWeight + rightWeight
	leftWidth := (availWidth * leftWeight) / totalWeight
	rightWidth := availWidth - leftWidth - 2 // 2 char horizontal gap

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	return leftWidth, rightWidth
}
