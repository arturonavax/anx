package theme_test

import (
	"strings"
	"testing"

	"github.com/arturonavax/anx/internal/theme"
)

func TestTokyoNightTokens(t *testing.T) {
	styles := theme.DefaultStyles()
	if styles.Palette.Name != "tokyo-night" {
		t.Fatalf("expected tokyo-night, got %s", styles.Palette.Name)
	}

	// Verify Tokyo Night Dark uses red accent (#f7768e)
	tokyoNight := theme.ByName("tokyo-night")
	if tokyoNight.Accent != tokyoNight.Red {
		t.Fatalf("expected tokyo-night accent to be red (%v), got %v", tokyoNight.Red, tokyoNight.Accent)
	}

	tokyoNightStorm := theme.ByName("tokyo-night-storm")
	if tokyoNightStorm.Accent != tokyoNightStorm.Red {
		t.Fatalf("expected tokyo-night-storm accent to be red (%v), got %v", tokyoNightStorm.Red, tokyoNightStorm.Accent)
	}

	rendered := styles.Header.Render("anx dashboard")
	if rendered == "" {
		t.Fatalf("expected rendered header, got empty string")
	}

	successRender := styles.Success.Render("Installed")
	if successRender == "" {
		t.Fatalf("expected rendered success, got empty string")
	}
}

func TestFoxPaletteAndGradient(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	fox := theme.ByName("fox")
	if fox.Name != "fox" {
		t.Fatalf("expected fox palette, got %q", fox.Name)
	}

	alias := theme.ByName("crimson-fox")
	if alias.Name != "fox" {
		t.Fatalf("expected crimson-fox alias to resolve to fox palette, got %q", alias.Name)
	}

	names := theme.Names()
	foundFox := false
	for _, n := range names {
		if n == "fox" {
			foundFox = true
			break
		}
	}
	if !foundFox {
		t.Fatalf("expected theme.Names() to include \"fox\", got %v", names)
	}

	grad := theme.FoxGradientHex()
	if len(grad) != 9 {
		t.Fatalf("expected 9 burgundy-crimson-gold gradient stops, got %d", len(grad))
	}
	if grad[0] != "#4A0718" || grad[len(grad)-1] != "#FCD34D" {
		t.Fatalf("expected gradient from #4A0718 to #FCD34D, got %v", grad)
	}

	styles := theme.NewStyles(fox)
	if styles.KeyCap.Render("Tab") == "" {
		t.Fatalf("expected non-empty KeyCap render")
	}
	if styles.BadgeAccent.Render("FOX") == "" {
		t.Fatalf("expected non-empty BadgeAccent render")
	}
}

func TestNoColorCompliance(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	pal := theme.ByName("fox")
	if pal.Name != "no-color" {
		t.Fatalf("expected no-color palette when NO_COLOR=1, got %q", pal.Name)
	}

	styles := theme.NewStyles(pal)
	rendered := styles.RenderSelectedRow("\x1b[31mred text\x1b[0m")
	if strings.Contains(rendered, "\x1b[38;") || strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("expected zero ANSI color sequences under NO_COLOR, got %q", rendered)
	}
}

