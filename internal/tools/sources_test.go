package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
