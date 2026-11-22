package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arturonavax/anx/internal/gittransport"
)

func TestMultiSourceDiscoveryCollisionAndAtomicUpdate(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	binDir := filepath.Join(tmp, "bin")
	bundledDir := filepath.Join(tmp, "bundled")

	// 1. Bundled tool
	mustWriteFile(t, filepath.Join(bundledDir, "hello", "tool.yaml"), `
schema_version: 1
id: hello
name: "Bundled Hello"
executable: hello
entrypoint: hello.sh
language: bash
`)
	mustWriteFile(t, filepath.Join(bundledDir, "hello", "hello.sh"), "#!/bin/sh\necho bundled-hello \"$@\"\n")

	// 2. Standalone single-tool repo (local dir)
	singleRepo := filepath.Join(tmp, "single-repo")
	mustWriteFile(t, filepath.Join(singleRepo, "tool.yaml"), `
schema_version: 1
id: solo
name: "Solo Tool"
executable: solo
entrypoint: bin/solo.sh
language: bash
`)
	mustWriteFile(t, filepath.Join(singleRepo, "bin", "solo.sh"), "#!/bin/sh\necho solo-v1 \"$@\"\n")

	// 3. Multi-tool collection repo (including a deliberate collision on "hello")
	collRepo := filepath.Join(tmp, "collection-repo")
	mustWriteFile(t, filepath.Join(collRepo, "tools", "alpha", "tool.yaml"), `
schema_version: 1
id: alpha
name: "Alpha Tool"
executable: alpha
entrypoint: alpha.sh
language: bash
`)
	mustWriteFile(t, filepath.Join(collRepo, "tools", "alpha", "alpha.sh"), "#!/bin/sh\necho alpha \"$@\"\n")
	mustWriteFile(t, filepath.Join(collRepo, "tools", "hello", "tool.yaml"), `
schema_version: 1
id: hello
name: "Collection Hello"
executable: hello
entrypoint: hello.sh
language: bash
`)
	mustWriteFile(t, filepath.Join(collRepo, "tools", "hello", "hello.sh"), "#!/bin/sh\necho collection-hello \"$@\"\n")

	mgr := NewSourceManager(cfgDir, dataDir, binDir, bundledDir)

	if _, found, err := mgr.AddSource(context.Background(), ToolSource{ID: "solo-src", Path: singleRepo}); err != nil {
		t.Fatalf("AddSource single failed: %v", err)
	} else if len(found) != 1 || found[0].Manifest.ID != "solo" {
		t.Fatalf("unexpected single discovery: %+v", found)
	}

	if _, found, err := mgr.AddSource(context.Background(), ToolSource{ID: "my-coll", Path: collRepo}); err != nil {
		t.Fatalf("AddSource collection failed: %v", err)
	} else if len(found) != 2 {
		t.Fatalf("expected 2 tools in collection, got %d", len(found))
	}

	byID, collisions, err := mgr.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	// Verify all three sources coexist and namespaced lookup resolves collisions cleanly
	if byID["solo"].SourceID != "solo-src" {
		t.Fatalf("solo missing or wrong source: %+v", byID["solo"])
	}
	if byID["alpha"].SourceID != "my-coll" {
		t.Fatalf("alpha missing or wrong source: %+v", byID["alpha"])
	}
	if byID["hello"].SourceID != "my-coll" {
		t.Fatalf("expected external source to override builtin on collision, got %s", byID["hello"].SourceID)
	}
	if byID["bundled/hello"].SourceID != "bundled" {
		t.Fatalf("namespaced bundled/hello lookup failed")
	}
	if len(collisions) != 1 || collisions[0].ToolID != "hello" {
		t.Fatalf("expected 1 collision on hello, got %+v", collisions)
	}

	// Verify installation and argument forwarding
	installed, err := mgr.InstallTool(context.Background(), byID["solo"])
	if err != nil {
		t.Fatalf("InstallTool solo failed: %v", err)
	}
	out, err := exec.Command(installed, "--flag", "arg1").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed solo failed: %v (%s)", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "solo-v1 --flag arg1" {
		t.Fatalf("unexpected output: %q", string(out))
	}

	// Verify failed update preserves last known working installation!
	broken := byID["solo"]
	broken.BuildCmd = []string{"sh", "-c", "exit 1"}
	if _, err := mgr.InstallTool(context.Background(), broken); err == nil {
		t.Fatalf("expected broken build to fail")
	}
	out2, err := exec.Command(installed, "still-works").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out2)) != "solo-v1 still-works" {
		t.Fatalf("previous working binary was NOT preserved after failed update: err=%v out=%q", err, string(out2))
	}
}

func TestInvalidSourceManifestRejected(t *testing.T) {
	tmp := t.TempDir()
	badRepo := filepath.Join(tmp, "bad")
	mustWriteFile(t, filepath.Join(badRepo, "tool.yaml"), "schema_version: 99\nid: bad\nname: Bad\nexecutable: bad\n")

	mgr := NewSourceManager(filepath.Join(tmp, "cfg"), filepath.Join(tmp, "data"), filepath.Join(tmp, "bin"), "")
	if _, _, err := mgr.AddSource(context.Background(), ToolSource{ID: "bad", Path: badRepo}); err == nil {
		t.Fatalf("expected invalid schema_version to be rejected")
	}
}

type fakeGitRunner struct {
	fn func(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error)
}

func (f fakeGitRunner) Run(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error) {
	return f.fn(ctx, env, dir, name, args...)
}

func TestSourceManagerSSHKeyAndOptionalPersistence(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_vault")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	var seenSSHCmds []string
	mgr := NewSourceManager(
		filepath.Join(tmp, "cfg"),
		filepath.Join(tmp, "data"),
		filepath.Join(tmp, "bin"),
		"",
	)
	mgr.Git = &gittransport.Adapter{
		Runner: fakeGitRunner{fn: func(_ context.Context, env []string, _ string, _ string, args ...string) (string, string, error) {
			for _, kv := range env {
				if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
					seenSSHCmds = append(seenSSHCmds, kv)
				}
			}
			if len(args) > 0 && args[0] == "ls-remote" {
				return "deadbeef\tHEAD\n", "", nil
			}
			if len(args) > 0 && args[0] == "clone" {
				dest := args[len(args)-1]
				mustWriteFile(t, filepath.Join(dest, "tool.yaml"), `
schema_version: 1
id: vault-helper
name: "Vault Helper"
executable: vault-helper
entrypoint: bin/helper.sh
language: bash
`)
				mustWriteFile(t, filepath.Join(dest, "bin", "helper.sh"), "#!/bin/sh\necho ok\n")
				return "", "", nil
			}
			return "", "", nil
		}},
	}

	saved, discovered, err := mgr.AddSource(context.Background(), ToolSource{
		ID:       "vault",
		URL:      "git@github.com:example-org/vault.git",
		Auth:     "ssh",
		SSHKey:   keyPath,
		Optional: true,
	})
	if err != nil {
		t.Fatalf("AddSource with SSHKey failed: %v", err)
	}
	if saved.SSHKey != keyPath || !saved.Optional {
		t.Fatalf("expected SSHKey and Optional preserved, got %+v", saved)
	}
	if len(discovered) != 1 || discovered[0].Manifest.ID != "vault-helper" {
		t.Fatalf("unexpected discovered tools: %+v", discovered)
	}
	if len(seenSSHCmds) == 0 {
		t.Fatalf("expected GIT_SSH_COMMAND to be passed to git runner")
	}
	for _, cmd := range seenSSHCmds {
		if !strings.Contains(cmd, "id_vault") || !strings.Contains(cmd, "IdentitiesOnly=yes") {
			t.Fatalf("expected SSH key in GIT_SSH_COMMAND, got %q", cmd)
		}
	}

	// Reload from disk and verify YAML persistence
	loaded, err := mgr.LoadSources()
	if err != nil {
		t.Fatalf("LoadSources failed: %v", err)
	}
	var foundVault *ToolSource
	for i := range loaded {
		if loaded[i].ID == "vault" {
			foundVault = &loaded[i]
			break
		}
	}
	if foundVault == nil || foundVault.SSHKey != keyPath || !foundVault.Optional {
		t.Fatalf("reloaded vault source missing SSHKey/Optional: %+v", loaded)
	}

	// Verify UpdateSource forwards SSHKey
	seenSSHCmds = nil
	if _, err := mgr.UpdateSource(context.Background(), "vault"); err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}
	if len(seenSSHCmds) == 0 {
		t.Fatalf("expected GIT_SSH_COMMAND on UpdateSource")
	}
	for _, cmd := range seenSSHCmds {
		if !strings.Contains(cmd, "id_vault") {
			t.Fatalf("expected SSH key in UpdateSource GIT_SSH_COMMAND, got %q", cmd)
		}
	}

	// Remove cached dir and confirm VerifySource probes with SSHKey
	if err := os.RemoveAll(filepath.Join(mgr.CacheDir, "vault")); err != nil {
		t.Fatal(err)
	}
	seenSSHCmds = nil
	_, err = mgr.VerifySource(context.Background(), "vault")
	if err == nil || !strings.Contains(err.Error(), "not cached locally") {
		t.Fatalf("expected not cached locally after successful probe, got %v", err)
	}
	if len(seenSSHCmds) == 0 || !strings.Contains(seenSSHCmds[0], "id_vault") {
		t.Fatalf("expected VerifySource remote probe to forward SSHKey, got %v", seenSSHCmds)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
