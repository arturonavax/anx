package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arturonavax/anx/internal/catalog"
)

func TestCatalogRegistry(t *testing.T) {
	// 1. Standalone default registry works with zero external env checkout
	reg := catalog.NewDefaultRegistry()

	packages := reg.ListPackages()
	if len(packages) == 0 {
		t.Fatalf("expected universal default packages in catalog")
	}
	if _, ok := reg.GetPackage("anx"); !ok {
		t.Fatalf("expected anx package in universal default catalog")
	}
	minimalPkgs, err := reg.ResolveProfilePackages("minimal")
	if err != nil {
		t.Fatalf("expected universal minimal profile: %v", err)
	}
	if len(minimalPkgs) == 0 {
		t.Fatalf("expected packages in minimal profile")
	}

	// 2. Overlay external configuration repository manifests
	repoManifests := t.TempDir()
	overlayYAML := `
packages:
  - id: ripgrep
    name: Ripgrep
    category: cli-tools
    binary_name: rg
    strategies:
      - type: native
        packages:
          apt: [ripgrep]
configurations:
  - id: sample-cfg
    group: shell
    source: files/sample
    target: ~/.sample
profiles:
  - name: workstation
    extends: minimal
    packages: [ripgrep]
`
	if err := os.WriteFile(filepath.Join(repoManifests, "overlay.yaml"), []byte(overlayYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	overlaid, err := catalog.NewRegistryWithOverlays(repoManifests, "")
	if err != nil {
		t.Fatalf("NewRegistryWithOverlays failed: %v", err)
	}
	rg, ok := overlaid.GetPackage("ripgrep")
	if !ok || rg.BinaryName != "rg" {
		t.Fatalf("expected overlaid ripgrep package with binary rg, got ok=%v pkg=%+v", ok, rg)
	}
	if len(overlaid.ListConfigurations()) == 0 {
		t.Fatalf("expected overlaid configurations")
	}
	wsPkgs, err := overlaid.ResolveProfilePackages("workstation")
	if err != nil || len(wsPkgs) < 3 {
		t.Fatalf("expected inherited workstation packages, err=%v pkgs=%v", err, wsPkgs)
	}
}
