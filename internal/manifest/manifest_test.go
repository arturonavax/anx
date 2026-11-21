package manifest_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/arturonavax/anx/internal/manifest"
)

func TestEmbeddedDefaultsLoadAndValidate(t *testing.T) {
	cat, err := manifest.LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() failed: %v", err)
	}

	for _, id := range []string{"anx", "git", "curl"} {
		if _, ok := cat.GetPackageSpec(id); !ok {
			t.Fatalf("expected universal default package %q in embedded catalog", id)
		}
	}
	if _, ok := cat.GetProfile("minimal"); !ok {
		t.Fatalf("expected universal default profile 'minimal' in embedded catalog")
	}
}

func TestExternalManifestOverlayAndProfileInheritance(t *testing.T) {
	repoDir := t.TempDir()
	userDir := t.TempDir()

	repoYAML := `
packages:
  - id: base-tool
    name: Base Tool
    category: core
    strategies:
      - type: native
        packages:
          apt: [base-tool]
  - id: app-tool
    name: App Tool
    category: cli-tools
    depends_on: [base-tool]
    strategies:
      - type: native
        packages:
          apt: [app-tool]
configurations:
  - id: shell-rc
    group: shell
    source: files/shell/rc
    target: ~/.shellrc
profiles:
  - name: developer
    extends: minimal
    packages: [app-tool]
    config_groups: [shell]
`
	if err := os.WriteFile(filepath.Join(repoDir, "repo.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	userYAML := `
packages:
  - id: user-extra
    name: User Extra
    category: custom
    strategies:
      - type: native
        packages:
          apt: [user-extra]
`
	if err := os.WriteFile(filepath.Join(userDir, "user.yaml"), []byte(userYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := manifest.LoadCatalog(
		manifest.WithRepoManifestDir(repoDir),
		manifest.WithUserDropInDir(userDir),
	)
	if err != nil {
		t.Fatalf("LoadCatalog with overlays failed: %v", err)
	}

	if _, ok := cat.GetPackageSpec("user-extra"); !ok {
		t.Fatalf("expected user drop-in package 'user-extra'")
	}

	devPkgs, err := cat.ResolveProfilePackages("developer")
	if err != nil {
		t.Fatalf("ResolveProfilePackages(developer) failed: %v", err)
	}
	for _, want := range []string{"git", "curl", "app-tool"} {
		if !slices.Contains(devPkgs, want) {
			t.Errorf("expected %q in developer profile, got %v", want, devPkgs)
		}
	}

	order, err := cat.ResolveInstallOrder([]string{"app-tool"})
	if err != nil {
		t.Fatalf("ResolveInstallOrder failed: %v", err)
	}
	baseIdx := slices.IndexFunc(order, func(p manifest.PackageSpec) bool { return p.ID == "base-tool" })
	appIdx := slices.IndexFunc(order, func(p manifest.PackageSpec) bool { return p.ID == "app-tool" })
	if baseIdx < 0 || appIdx < 0 || baseIdx >= appIdx {
		t.Fatalf("expected base-tool before app-tool, got %v", order)
	}
}

func TestInvalidManifestsRejected(t *testing.T) {
	t.Run("missing package strategy", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("packages:\n  - id: broken\n    name: Broken\n    strategies: []\n"), 0o644)
		if _, err := manifest.LoadCatalog(manifest.WithRepoManifestDir(dir)); err == nil {
			t.Fatalf("expected error for package without strategies")
		}
	})

	t.Run("cyclic profile inheritance", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "cycle.yaml"), []byte(`
profiles:
  - name: a
    extends: b
  - name: b
    extends: a
`), 0o644)
		cat, err := manifest.LoadCatalog(manifest.WithRepoManifestDir(dir))
		if err != nil {
			t.Fatalf("LoadCatalog failed: %v", err)
		}
		if _, err := cat.ResolveProfilePackages("a"); err == nil {
			t.Fatalf("expected cyclic profile inheritance error")
		}
	})
}
