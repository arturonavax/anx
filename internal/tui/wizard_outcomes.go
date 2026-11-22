package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (w WizardModel) renderOutcomeCounts() string {
	success := w.styles.Success.Render(fmt.Sprintf("✔ %d succeeded", len(w.succeededItems)))
	failed := w.styles.Error.Render(fmt.Sprintf("✖ %d failed", len(w.failedItems)))
	skipped := w.styles.Warning.Render(fmt.Sprintf("– %d skipped", len(w.skippedItems)))
	return "  " + success + "  " + failed + "  " + skipped
}

func (w WizardModel) renderLiveOutcomes(width int) []string {
	success := []string{w.styles.Success.Render(fmt.Sprintf("SUCCEEDED (%d)", len(w.succeededItems)))}
	for _, item := range slices.Backward(w.succeededItems) {
		success = append(success, w.styles.Success.Render("✔ "+item))
	}
	notInstalled := []string{
		w.styles.Error.Render(fmt.Sprintf("NOT INSTALLED (%d) · %d failed, %d skipped",
			len(w.failedItems)+len(w.skippedItems), len(w.failedItems), len(w.skippedItems))),
	}
	for _, item := range slices.Backward(w.failedItems) {
		notInstalled = append(notInstalled, w.styles.Error.Render("✖ "+item))
	}
	for _, item := range slices.Backward(w.skippedItems) {
		notInstalled = append(notInstalled, w.styles.Warning.Render("– "+item+" (skipped)"))
	}

	if width < 80 {
		lines := append(slimOutcomeColumn(success, width-2), "")
		return append(lines, slimOutcomeColumn(notInstalled, width-2)...)
	}

	leftWidth := (width - 6) / 2
	rightWidth := width - 6 - leftWidth
	left, right := slimOutcomeColumn(success, leftWidth), slimOutcomeColumn(notInstalled, rightWidth)
	var lines []string
	for i := range max(len(left), len(right)) {
		var a, b string
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		lines = append(lines, "  "+a+strings.Repeat(" ", leftWidth-ansi.StringWidth(a)+2)+b)
	}
	return lines
}

func slimOutcomeColumn(lines []string, width int) []string {
	for i, line := range lines {
		lines[i] = AnsiTruncate(line, width)
	}
	return lines
}
