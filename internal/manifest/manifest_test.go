package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogToolSourcesParsingAndLayeredMerging(t *testing.T) {
	repoDir := t.TempDir()
	userDir := t.TempDir()

	repoYAML := `
tool_sources:
  - id: personal-vault
    name: "Personal Vault"
    url: "git@github.com:example-user/vault.git"
    type: collection
    auth: ssh
    ssh_key: "~/.ssh/id_ed25519"
    branch: main
    optional: true
    verify_ssh: true
    description: "Private tools and vault recipes"
  - id: community-tools
    name: "Community Tools"
    url: "https://github.com/example-org/tools.git"
    type: collection
    auth: auto
`
	if err := os.WriteFile(filepath.Join(repoDir, "tool_sources.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// User drop-in overrides personal-vault ssh_key & branch, and adds a third source.
	userYAML := `
tool_sources:
  - id: personal-vault
    name: "Personal Vault Override"
    url: "git@github.com:example-user/vault.git"
    type: collection
    auth: ssh
    ssh_key: "~/.ssh/id_vault_custom"
    branch: develop
    optional: true
    verify_ssh: true
    description: "Overridden in user drop-in"
  - id: alpha-tool
    name: "Alpha Tool"
    url: "https://github.com/example-org/alpha.git"
    type: single
    auth: https
`
	if err := os.WriteFile(filepath.Join(userDir, "personal.yaml"), []byte(userYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := LoadCatalog(
		WithRepoManifestDir(repoDir),
		WithUserDropInDir(userDir),
	)
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	sources := cat.ListToolSources()
	if len(sources) != 3 {
		t.Fatalf("expected 3 merged tool sources, got %d: %+v", len(sources), sources)
	}

	// Verify deterministic sorted order by ID: alpha-tool, community-tools, personal-vault
	wantIDs := []string{"alpha-tool", "community-tools", "personal-vault"}
	for i, want := range wantIDs {
		if sources[i].ID != want {
			t.Fatalf("index %d: expected ID %q, got %q", i, want, sources[i].ID)
		}
	}

	vault, ok := cat.GetToolSource("personal-vault")
	if !ok {
		t.Fatalf("expected personal-vault tool source to exist")
	}
	if vault.SSHKey != "~/.ssh/id_vault_custom" || vault.Branch != "develop" {
		t.Fatalf("expected user drop-in to win on personal-vault, got %+v", vault)
	}
	if !vault.Optional || !vault.VerifySSH {
		t.Fatalf("expected Optional=true and VerifySSH=true, got %+v", vault)
	}
}

func TestCatalogToolSourcesValidation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing id",
			yaml: "tool_sources:\n  - url: https://github.com/org/repo.git\n",
		},
		{
			name: "missing url for non-builtin",
			yaml: "tool_sources:\n  - id: empty-url\n    type: single\n",
		},
		{
			name: "invalid type",
			yaml: "tool_sources:\n  - id: bad-type\n    url: https://github.com/org/repo.git\n    type: unknown\n",
		},
		{
			name: "invalid auth",
			yaml: "tool_sources:\n  - id: bad-auth\n    url: https://github.com/org/repo.git\n    auth: kerberos\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(tc.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadCatalog(WithUserDropInDir(dir)); err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.name)
			}
		})
	}
}
