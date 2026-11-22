package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/arturonavax/anx/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

const logoTickInterval = 140 * time.Millisecond

// LogoTier selects the responsive size variant of the animated Fox logo.
type LogoTier int

const (
	// LogoTierMinimal renders a single-line emblem for narrow or short terminals.
	LogoTierMinimal LogoTier = iota
	// LogoTierCompact renders a 3-line fox crest for medium terminals.
	LogoTierCompact
	// LogoTierFull renders the full 7-line hero fox + ANX wordmark for large terminals.
	LogoTierFull
)

type logoTickMsg time.Time

func tickLogoCmd() tea.Cmd {
	return tea.Tick(logoTickInterval, func(t time.Time) tea.Msg {
		return logoTickMsg(t)
	})
}

var (
	foxGradientStyles = buildFoxGradientStyles()
	foxEyeGlintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FCD34D")).Bold(true)
	foxEyeSteadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	foxNoseStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9B112E")).Bold(true)
	foxTaglineStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4A5AE"))
)

func buildFoxGradientStyles() []lipgloss.Style {
	stops := theme.FoxGradientHex()
	// Ping-pong wave (0..N-1..1) for seamless looping without a harsh seam.
	wave := make([]lipgloss.Style, 0, len(stops)*2-2)
	for _, hex := range stops {
		wave = append(wave, lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Bold(true))
	}
	for i := len(stops) - 2; i >= 1; i-- {
		wave = append(wave, lipgloss.NewStyle().Foreground(lipgloss.Color(stops[i])).Bold(true))
	}
	return wave
}

// SelectLogoTier chooses the responsive Fox logo tier from terminal dimensions.
func SelectLogoTier(width, height int) LogoTier {
	if width >= 88 && height >= 26 {
		return LogoTierFull
	}
	if width >= 60 && height >= 18 {
		return LogoTierCompact
	}
	return LogoTierMinimal
}

// RenderFoxLogo renders the animated burgundy-crimson-gold Fox logo adapted to (width, height).
// Every returned line is strictly bounded to width cells; when noColor or NO_COLOR is active,
// it emits clean plain text with zero ANSI escape sequences.
func RenderFoxLogo(width, height, frame int, noColor bool) string {
	if width <= 0 {
		return ""
	}
	plain := noColor || theme.IsNoColor()
	tier := SelectLogoTier(width, height)

	var lines []string
	switch tier {
	case LogoTierFull:
		lines = renderFullFoxHero(frame, plain)
	case LogoTierCompact:
		lines = renderCompactFoxCrest(frame, plain)
	default:
		lines = []string{renderMinimalFoxEmblem(frame, plain)}
	}

	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// RenderFoxBadge renders a sleek single-line animated Fox emblem for headers.
func RenderFoxBadge(frame int, noColor bool) string {
	plain := noColor || theme.IsNoColor()
	mark := `◢◤▲◥◣`
	if plain {
		return mark
	}
	return renderFoxGradientLine(mark, 0, frame, false, false)
}

func renderFullFoxHero(frame int, plain bool) []string {
	eyeRune := '◈'
	if !plain && (frame%8 == 0 || frame%8 == 1) {
		eyeRune = '✦'
	}

	// Low-poly geometric Fox silhouette (8 lines x 21 cells).
	foxArt := []string{
		`  ◢◣             ◢◣  `,
		` ◢█▀◣           ◢▀█◣ `,
		`◢█▀ ▀◣ ▄▄▄▄▄▄▄ ◢▀ ▀█◣`,
		`█▌  ◢███████████◣  ▐█`,
		`◥█◣ ▐█ ` + string(eyeRune) + `  ▄  ` + string(eyeRune) + ` █▌ ◢█◤`,
		` ◥██◣◥████▀████◤◢██◤ `,
		`   ◥██◣  ◥▼◤  ◢██◤   `,
		`     ◥████▄████◤     `,
	}

	wordmark := []string{
		` █████╗ ███╗   ██╗██╗  ██╗`,
		`██╔══██╗████╗  ██║╚██╗██╔╝`,
		`███████║██╔██╗ ██║ ╚███╔╝ `,
		`██╔══██║██║╚██╗██║ ██╔██╗ `,
		`██║  ██║██║ ╚████║██╔╝ ██╗`,
		`╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝`,
		`━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`,
		`ANX · DEVELOPER ENVIRONMENT ORCHESTRATOR`,
	}

	out := make([]string, len(foxArt))
	for row := range foxArt {
		left := renderFoxGradientLine(foxArt[row], row, frame, plain, true)
		var right string
		if row == len(wordmark)-1 {
			if plain {
				right = wordmark[row]
			} else {
				right = foxTaglineStyle.Render(wordmark[row])
			}
		} else {
			right = renderFoxGradientLine(wordmark[row], row, frame+2, plain, false)
		}
		out[row] = left + "   " + right
	}
	return out
}

func renderCompactFoxCrest(frame int, plain bool) []string {
	eyeRune := '◈'
	if !plain && (frame%8 == 0 || frame%8 == 1) {
		eyeRune = '✦'
	}

	// Compact low-poly Fox crest (4 lines x 13 cells).
	foxLines := []string{
		` ◢◣       ◢◣ `,
		`◢█▀◣▄▄▄▄▄◢▀█◣`,
		`◥█ ` + string(eyeRune) + `  ▄  ` + string(eyeRune) + ` █◤`,
		`  ◥██◥▼◤██◤  `,
	}
	rightLines := []string{
		`▄▀█ █▄░█ ▀▄▀  ANX`,
		`█▀█ █░▀█ █░█  Environment Orchestrator`,
		`━━━━━━━━━━━━  Burgundy · Crimson · Gold`,
		`Ready · Cross-Platform · Concurrent`,
	}

	out := make([]string, len(foxLines))
	for row := range foxLines {
		left := renderFoxGradientLine(foxLines[row], row, frame, plain, true)
		var right string
		switch {
		case (row == 1 || row == 2) && !plain:
			parts := strings.SplitN(rightLines[row], "  ", 2)
			right = renderFoxGradientLine(parts[0], row, frame+2, false, false) + "  " + foxTaglineStyle.Render(parts[1])
		case row == 3 && !plain:
			right = foxTaglineStyle.Render(rightLines[row])
		default:
			right = renderFoxGradientLine(rightLines[row], row, frame+2, plain, false)
		}
		out[row] = left + "  " + right
	}
	return out
}

func renderMinimalFoxEmblem(frame int, plain bool) string {
	emblem := `◢◤▲◥◣ ANX`
	if plain {
		return emblem
	}
	return renderFoxGradientLine(emblem, 0, frame, false, false)
}

func renderFoxGradientLine(line string, row, frame int, plain, accentFace bool) string {
	if plain {
		return line
	}
	waveLen := len(foxGradientStyles)
	if waveLen == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) * 4)
	col := 0
	for _, r := range line {
		if r == ' ' {
			sb.WriteRune(r)
			col++
			continue
		}
		if accentFace {
			switch r {
			case '✦', '◆':
				sb.WriteString(foxEyeGlintStyle.Render(string(r)))
				col++
				continue
			case '◈':
				sb.WriteString(foxEyeSteadyStyle.Render(string(r)))
				col++
				continue
			case '▼':
				sb.WriteString(foxNoseStyle.Render(string(r)))
				col++
				continue
			}
		}
		idx := (col/2 + row + frame) % waveLen
		if idx < 0 {
			idx += waveLen
		}
		sb.WriteString(foxGradientStyles[idx].Render(string(r)))
		col++
	}
	return sb.String()
}
