package cli

import (
	"bytes"
	"github.com/arturonavax/anx/internal/catalog"
	"github.com/arturonavax/anx/internal/platform"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncScriptUsesConfigurationHostSelectors(t *testing.T) {
	d := t.TempDir()
	err := os.WriteFile(filepath.Join(d, "probe.yaml"), []byte(`configurations:
  - id: yes-host
    group: independent
    source: config/yes-host
    target: ~/.yes-host
    distros: [uBuNtU]
    desktops: [gnome]
    display_servers: [wayland]
  - id: no-distro
    group: independent
    source: config/no-distro
    target: ~/.no-distro
    distros: [fedora]
  - id: no-display
    group: independent
    source: config/no-display
    target: ~/.no-display
    display_servers: [x11]
  - id: no-desktop
    group: independent
    source: config/no-desktop
    target: ~/.no-desktop
    desktops: [kde]
  - id: portable-host
    group: independent
    source: config/portable-host
    target: ~/.portable-host
`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewRegistryWithOverlays(d, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &AppContext{Catalog: cat, Platform: platform.NewTestInfo(platform.TestInfoParams{OS: "linux", Distribution: "ubuntu", DesktopEnv: "gnome", DisplayServer: "wayland", HomeDir: t.TempDir()})}
	for _, push := range []bool{false, true} {
		var b bytes.Buffer
		if err := renderSyncScript(&b, ctx, []string{"independent"}, push); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"yes-host", "portable-host"} {
			if !strings.Contains(b.String(), "config/"+id) {
				t.Errorf("missing %s push=%v", id, push)
			}
		}
		for _, id := range []string{"no-distro", "no-display", "no-desktop"} {
			if strings.Contains(b.String(), "config/"+id) {
				t.Errorf("inapplicable %s push=%v", id, push)
			}
		}
	}
}
