package theme

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette holds the semantic color tokens for a theme.
type Palette struct {
	Name       string
	Background color.Color
	Surface    color.Color
	SurfaceAlt color.Color
	Foreground color.Color
	Accent     color.Color
	Gold       color.Color
	Cyan       color.Color
	Purple     color.Color
	Green      color.Color
	Yellow     color.Color
	Red        color.Color
	Muted      color.Color
	Border     color.Color
}

var foxGradientHex = []string{
	"#4A0718",
	"#6E0D25",
	"#9B112E",
	"#C81E3A",
	"#E11D48",
	"#EA580C",
	"#D97706",
	"#F59E0B",
	"#FCD34D",
}

// FoxGradientHex returns a copy of the burgundy-crimson-gold brand gradient stops.
func FoxGradientHex() []string {
	out := make([]string, len(foxGradientHex))
	copy(out, foxGradientHex)
	return out
}

// FoxPalette is the flagship burgundy-crimson-gold palette on dark obsidian surfaces.
var FoxPalette = Palette{
	Name:       "fox",
	Background: lipgloss.Color("#11090C"),
	Surface:    lipgloss.Color("#1B0F14"),
	SurfaceAlt: lipgloss.Color("#29161D"),
	Foreground: lipgloss.Color("#F5E9EC"),
	Accent:     lipgloss.Color("#E11D48"),
	Gold:       lipgloss.Color("#F59E0B"),
	Cyan:       lipgloss.Color("#FCD34D"),
	Purple:     lipgloss.Color("#9B112E"),
	Green:      lipgloss.Color("#34D399"),
	Yellow:     lipgloss.Color("#F59E0B"),
	Red:        lipgloss.Color("#E11D48"),
	Muted:      lipgloss.Color("#9A7B84"),
	Border:     lipgloss.Color("#4A1D29"),
}

// TokyoNightPalette is the standard Tokyo Night dark palette with red accent.
var TokyoNightPalette = Palette{
	Name:       "tokyo-night",
	Background: lipgloss.Color("#1a1b26"),
	Surface:    lipgloss.Color("#1f2335"),
	SurfaceAlt: lipgloss.Color("#292e42"),
	Foreground: lipgloss.Color("#c0caf5"),
	Accent:     lipgloss.Color("#f7768e"),
	Gold:       lipgloss.Color("#e0af68"),
	Cyan:       lipgloss.Color("#7dcfff"),
	Purple:     lipgloss.Color("#bb9af7"),
	Green:      lipgloss.Color("#9ece6a"),
	Yellow:     lipgloss.Color("#e0af68"),
	Red:        lipgloss.Color("#f7768e"),
	Muted:      lipgloss.Color("#565f89"),
	Border:     lipgloss.Color("#3b4261"),
}

// TokyoNightStormPalette is the slightly darker Tokyo Night Storm palette with red accent.
var TokyoNightStormPalette = Palette{
	Name:       "tokyo-night-storm",
	Background: lipgloss.Color("#24283b"),
	Surface:    lipgloss.Color("#292e42"),
	SurfaceAlt: lipgloss.Color("#343a55"),
	Foreground: lipgloss.Color("#c0caf5"),
	Accent:     lipgloss.Color("#f7768e"),
	Gold:       lipgloss.Color("#e0af68"),
	Cyan:       lipgloss.Color("#7dcfff"),
	Purple:     lipgloss.Color("#bb9af7"),
	Green:      lipgloss.Color("#9ece6a"),
	Yellow:     lipgloss.Color("#e0af68"),
	Red:        lipgloss.Color("#f7768e"),
	Muted:      lipgloss.Color("#565f89"),
	Border:     lipgloss.Color("#414868"),
}

// CatppuccinMochaPalette is the Catppuccin Mocha dark palette.
var CatppuccinMochaPalette = Palette{
	Name:       "catppuccin",
	Background: lipgloss.Color("#1e1e2e"),
	Surface:    lipgloss.Color("#262637"),
	SurfaceAlt: lipgloss.Color("#313244"),
	Foreground: lipgloss.Color("#cdd6f4"),
	Accent:     lipgloss.Color("#89b4fa"),
	Gold:       lipgloss.Color("#f9e2af"),
	Cyan:       lipgloss.Color("#89dceb"),
	Purple:     lipgloss.Color("#cba6f7"),
	Green:      lipgloss.Color("#a6e3a1"),
	Yellow:     lipgloss.Color("#f9e2af"),
	Red:        lipgloss.Color("#f38ba8"),
	Muted:      lipgloss.Color("#6c7086"),
	Border:     lipgloss.Color("#45475a"),
}

// GruvboxPalette is the Gruvbox dark palette.
var GruvboxPalette = Palette{
	Name:       "gruvbox",
	Background: lipgloss.Color("#282828"),
	Surface:    lipgloss.Color("#32302f"),
	SurfaceAlt: lipgloss.Color("#3c3836"),
	Foreground: lipgloss.Color("#ebdbb2"),
	Accent:     lipgloss.Color("#83a598"),
	Gold:       lipgloss.Color("#fabd2f"),
	Cyan:       lipgloss.Color("#8ec07c"),
	Purple:     lipgloss.Color("#d3869b"),
	Green:      lipgloss.Color("#b8bb26"),
	Yellow:     lipgloss.Color("#fabd2f"),
	Red:        lipgloss.Color("#fb4934"),
	Muted:      lipgloss.Color("#928374"),
	Border:     lipgloss.Color("#504945"),
}

// NordPalette is the Nord palette.
var NordPalette = Palette{
	Name:       "nord",
	Background: lipgloss.Color("#2e3440"),
	Surface:    lipgloss.Color("#3b4252"),
	SurfaceAlt: lipgloss.Color("#434c5e"),
	Foreground: lipgloss.Color("#d8dee9"),
	Accent:     lipgloss.Color("#81a1c1"),
	Gold:       lipgloss.Color("#ebcb8b"),
	Cyan:       lipgloss.Color("#88c0d0"),
	Purple:     lipgloss.Color("#b48ead"),
	Green:      lipgloss.Color("#a3be8c"),
	Yellow:     lipgloss.Color("#ebcb8b"),
	Red:        lipgloss.Color("#bf616a"),
	Muted:      lipgloss.Color("#616e88"),
	Border:     lipgloss.Color("#434c5e"),
}

// NoColorPalette renders everything with the terminal default foreground (monochrome).
var NoColorPalette = Palette{
	Name:       "no-color",
	Background: lipgloss.NoColor{},
	Surface:    lipgloss.NoColor{},
	SurfaceAlt: lipgloss.NoColor{},
	Foreground: lipgloss.NoColor{},
	Accent:     lipgloss.NoColor{},
	Gold:       lipgloss.NoColor{},
	Cyan:       lipgloss.NoColor{},
	Purple:     lipgloss.NoColor{},
	Green:      lipgloss.NoColor{},
	Yellow:     lipgloss.NoColor{},
	Red:        lipgloss.NoColor{},
	Muted:      lipgloss.NoColor{},
	Border:     lipgloss.NoColor{},
}

var registry = []Palette{
	FoxPalette,
	TokyoNightPalette,
	TokyoNightStormPalette,
	CatppuccinMochaPalette,
	GruvboxPalette,
	NordPalette,
	NoColorPalette,
}

// Names lists every available theme name in stable order.
func Names() []string {
	names := make([]string, 0, len(registry))
	for _, p := range registry {
		names = append(names, p.Name)
	}
	return names
}

// ByName resolves a palette by name; unknown names and NO_COLOR fall back safely.
func ByName(name string) Palette {
	if IsNoColor() {
		return NoColorPalette
	}
	if name == "crimson-fox" {
		return FoxPalette
	}
	for _, p := range registry {
		if p.Name == name {
			return p
		}
	}
	return TokyoNightPalette
}

// Styles encapsulates Lipgloss v2 styles derived from a Palette.
type Styles struct {
	Palette        Palette
	Header         lipgloss.Style
	TabActive      lipgloss.Style
	TabInactive    lipgloss.Style
	BorderActive   lipgloss.Style
	BorderInactive lipgloss.Style
	Success        lipgloss.Style
	Warning        lipgloss.Style
	Error          lipgloss.Style
	Muted          lipgloss.Style
	Highlight      lipgloss.Style
	SelectedRow    lipgloss.Style
	BadgeAccent    lipgloss.Style
	BadgeGold      lipgloss.Style
	BadgeMuted     lipgloss.Style
	KeyCap         lipgloss.Style
	CardTitle      lipgloss.Style
}

// NewStyles creates a Styles instance from the specified palette.
func NewStyles(p Palette) *Styles {
	noColor := p.Name == NoColorPalette.Name || IsNoColor()
	if noColor {
		p = NoColorPalette
	}

	selectedFg := color.Color(lipgloss.Color("#11090C"))
	if noColor {
		selectedFg = lipgloss.NoColor{}
	}

	surfaceAlt := p.SurfaceAlt
	if surfaceAlt == nil {
		surfaceAlt = p.Border
	}
	gold := p.Gold
	if gold == nil {
		gold = p.Yellow
	}

	return &Styles{
		Palette: p,
		Header: lipgloss.NewStyle().
			Foreground(p.Accent).
			Bold(true),
		TabActive: lipgloss.NewStyle().
			Foreground(selectedFg).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		TabInactive: lipgloss.NewStyle().
			Foreground(p.Muted).
			Background(p.Surface).
			Padding(0, 1),
		BorderActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Accent),
		BorderInactive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Border),
		Success: lipgloss.NewStyle().
			Foreground(p.Green).
			Bold(true),
		Warning: lipgloss.NewStyle().
			Foreground(p.Yellow).
			Bold(true),
		Error: lipgloss.NewStyle().
			Foreground(p.Red).
			Bold(true),
		Muted: lipgloss.NewStyle().
			Foreground(p.Muted),
		Highlight: lipgloss.NewStyle().
			Foreground(p.Cyan).
			Bold(true),
		SelectedRow: lipgloss.NewStyle().
			Foreground(selectedFg).
			Background(p.Accent).
			Bold(true),
		BadgeAccent: lipgloss.NewStyle().
			Foreground(selectedFg).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		BadgeGold: lipgloss.NewStyle().
			Foreground(gold).
			Bold(true),
		BadgeMuted: lipgloss.NewStyle().
			Foreground(p.Muted).
			Background(surfaceAlt).
			Padding(0, 1),
		KeyCap: lipgloss.NewStyle().
			Foreground(gold).
			Background(surfaceAlt).
			Bold(true),
		CardTitle: lipgloss.NewStyle().
			Foreground(gold).
			Bold(true),
	}
}

// RenderSelectedRow renders text on the focused/accent background, stripping inner ANSI color codes
// to prevent color resets (\x1b[0m) or nested light colors that make text light and unreadable on red accents.
func (s *Styles) RenderSelectedRow(text string) string {
	clean := ansi.Strip(text)
	return s.SelectedRow.Render(clean)
}

// DefaultStyles returns the default Tokyo Night styles.
func DefaultStyles() *Styles {
	return NewStyles(TokyoNightPalette)
}

// IsNoColor checks if NO_COLOR environment variable is set.
func IsNoColor() bool {
	return os.Getenv("NO_COLOR") != ""
}
