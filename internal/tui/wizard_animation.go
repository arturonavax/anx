package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/arturonavax/anx/internal/theme"
)

const wizardFPS = 60

type wizardAnimation struct {
	lastFrame time.Time
	age       time.Duration
	progress  float64
}

func (a *wizardAnimation) advance(now time.Time, percent int) {
	if a.lastFrame.IsZero() {
		a.lastFrame = now
		return
	}
	delta := now.Sub(a.lastFrame)
	if delta <= 0 {
		return
	}
	a.lastFrame = now
	a.age += delta
	goal := float64(percent)
	// Ease toward reported progress; don't simulate work between events.
	a.progress += (goal - a.progress) * (1 - math.Exp(-delta.Seconds()/0.13))
	if math.Abs(goal-a.progress) < 0.1 {
		a.progress = goal
	}
}

func (a wizardAnimation) wave(periodSeconds, offset float64) float64 {
	return (1 - math.Cos(2*math.Pi*(a.age.Seconds()/periodSeconds+offset))) / 2
}

const foxWaveShades = 64

var wizardFoxColors = func() [foxWaveShades]lipgloss.Style {
	var styles [foxWaveShades]lipgloss.Style
	for i := range styles {
		t := float64(i) / (foxWaveShades - 1)
		styles[i] = lipgloss.NewStyle().Bold(true).Foreground(color.RGBA{
			R: uint8(232 + 23*t), G: uint8(94 + 119*t), B: uint8(105 - 25*t), A: 255,
		})
	}
	return styles
}()

func (a wizardAnimation) foxBadge(noColor bool) string {
	const mark = "◢◤▲◥◣"
	if noColor {
		return mark
	}
	var b strings.Builder
	for i, r := range []rune(mark) {
		shade := int(math.Round(a.wave(2.4, float64(i)*0.14) * (foxWaveShades - 1)))
		b.WriteString(wizardFoxColors[shade].Render(string(r)))
	}
	return b.String()
}

func (w WizardModel) renderLoadingGlyph(id int) string {
	const dots = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	if w.styles.Palette.Name == theme.NoColorPalette.Name {
		frames := []rune(dots)
		return string(frames[(int(w.animation.age/(85*time.Millisecond))+id)%len(frames)])
	}
	// Keep the silhouette and position fixed; animate its luminance.
	shade := int(math.Round(w.animation.wave(1.35, float64(id)*0.07) * (foxWaveShades - 1)))
	return wizardFoxColors[shade].Render("⠿")
}

func (w WizardModel) renderProgressBar(width int) string {
	units := int(math.Round(w.animation.progress / 100 * float64(width*8)))
	cells, remainder := units/8, units%8
	filled := strings.Repeat("█", cells)
	if remainder > 0 {
		filled += string([]rune(" ▏▎▍▌▋▊▉")[remainder])
		cells++
	}
	return fmt.Sprintf("[%s%s] %3d%%", w.styles.Header.Render(filled),
		w.styles.Muted.Render(strings.Repeat("░", width-cells)), w.progressPercent)
}

func (w WizardModel) renderWorkerIndividualBar(slot activeWorkerSlot, width int) string {
	now := w.animation.lastFrame
	if now.IsZero() {
		now = time.Now()
	}
	elapsed := max(time.Duration(0), now.Sub(slot.StartedAt))
	if slot.StartedAt.IsZero() {
		elapsed = 0
	}

	pos := w.animation.wave(3.2, float64(slot.StepID)*0.07) * float64(width-1)
	shades := [...]string{"░", "▒", "▓", "█"}
	var bar strings.Builder
	bar.WriteByte('[')
	for i := range width {
		intensity := max(0, 1-math.Abs(float64(i)-pos)/2.3)
		rank := int(math.Round(intensity * 3))
		if rank == 0 {
			bar.WriteString(w.styles.Muted.Render(shades[rank]))
		} else {
			bar.WriteString(w.styles.Header.Render(shades[rank]))
		}
	}
	bar.WriteByte(']')
	return fmt.Sprintf("%s %s", bar.String(), w.styles.Muted.Render(fmt.Sprintf("%4.1fs", elapsed.Seconds())))
}
