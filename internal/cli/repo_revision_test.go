package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arturonavax/anx/internal/cli"
)

func TestRepoUpdateChecksAndReportsEnvironmentRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	home := t.TempDir()
	t.Chdir(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Setenv("ANX_CONFIG_REF", "")
	t.Setenv("ANX_SSH_KEY", "")
	t.Setenv("ANX_DOTFILES_URL", "")
	t.Setenv("ANX_REPO", "")
	t.Setenv("REPO_ROOT", "")

	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	remote := filepath.Join(root, "remote.git")
	clone := filepath.Join(root, "checkout")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(seed, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(root, "init", "--bare", "-q", "-b", "main", remote)
	git(root, "init", "-q", "-b", "main", seed)
	git(seed, "config", "user.name", "Fixture")
	git(seed, "config", "user.email", "fixture@example.invalid")
	write("manifests/profile.yaml", "profiles: []\n")
	write("files/environment.conf", "old\n")
	git(seed, "add", ".")
	git(seed, "commit", "-qm", "fixture base")
	git(seed, "remote", "add", "origin", remote)
	git(seed, "push", "-q", "-u", "origin", "main")
	git(root, "clone", "-q", remote, clone)
	git(clone, "remote", "rename", "origin", "upstream")
	git(clone, "config", "--add", "remote.upstream.fetch", "+refs/heads/side:refs/heads/received-side")
	before := git(clone, "rev-parse", "HEAD")
	write("files/environment.conf", "new\n")
	write("scripts/configure.sh", "#!/bin/sh\n")
	write("notes.md", "not an install asset\n")
	git(seed, "add", ".")
	git(seed, "commit", "-qm", "change an environment file")
	git(seed, "push", "-q")
	git(seed, "branch", "side")
	git(seed, "tag", "fixture-unrelated")
	git(seed, "push", "-q", "origin", "side", "refs/tags/fixture-unrelated")
	after := git(seed, "rev-parse", "HEAD")

	t.Setenv("ANX_CONFIG_REPO", clone)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		rc := cli.ExecuteCLI(args, &out, &errOut, "test", "", "")
		return rc, out.String(), errOut.String()
	}
	type report struct {
		Revision string `json:"revision"`
		Tracking *struct {
			UpstreamRevision string   `json:"upstream_revision"`
			Ahead            int      `json:"ahead"`
			Behind           int      `json:"behind"`
			IncomingPaths    []string `json:"incoming_paths"`
		} `json:"tracking"`
	}
	status := func(args ...string) report {
		t.Helper()
		rc, stdout, stderr := run(append(args, "--format", "json")...)
		if rc != 0 {
			t.Fatalf("anx %v: exit %d, stdout=%q stderr=%q", args, rc, stdout, stderr)
		}
		var result report
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("parse output %q: %v", stdout, err)
		}
		return result
	}

	if initial := status("repo", "status"); initial.Revision != before || initial.Tracking == nil ||
		initial.Tracking.UpstreamRevision != before || initial.Tracking.Behind != 0 {
		t.Errorf("status unexpectedly fetched or omitted the cached revision: %+v", initial)
	}
	if rc, _, errOut := run("repo", "update", "--offline", "--check"); rc == 0 || !strings.Contains(errOut, "offline") {
		t.Errorf("offline check accepted or not explained: exit=%d stderr=%q", rc, errOut)
	}
	if rc, out, stderr := run("repo", "update", "--check", "--dry-run"); rc != 0 ||
		!strings.Contains(out, "without changing the checkout") || git(clone, "rev-parse", "@{upstream}") != before {
		t.Errorf("dry-run check: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	dirtyPath := filepath.Join(clone, "files", "environment.conf")
	stagedPath := filepath.Join(clone, "local-staged.txt")
	if err := os.WriteFile(dirtyPath, []byte("local dirty content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("local staged content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(clone, "add", "local-staged.txt")
	indexPath := filepath.Join(clone, ".git", "index")
	currentIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if preview := status("repo", "update", "--check"); preview.Revision != before || preview.Tracking == nil ||
		preview.Tracking.UpstreamRevision != after || preview.Tracking.Ahead != 0 ||
		preview.Tracking.Behind != 1 || len(preview.Tracking.IncomingPaths) != 2 ||
		preview.Tracking.IncomingPaths[0] != "files/environment.conf" ||
		preview.Tracking.IncomingPaths[1] != "scripts/configure.sh" {
		t.Errorf("unexpected check result: %+v", preview)
	}
	if got := git(clone, "show", "HEAD:files/environment.conf"); got != "old" {
		t.Fatalf("check changed the checkout: %q", got)
	}
	if next, err := os.ReadFile(indexPath); err != nil || !bytes.Equal(next, currentIndex) {
		t.Fatalf("check changed the Git index: %v", err)
	}
	if got, err := os.ReadFile(dirtyPath); err != nil || string(got) != "local dirty content\n" {
		t.Fatalf("check changed the dirty file: %q, %v", got, err)
	}
	if got, err := os.ReadFile(stagedPath); err != nil || string(got) != "local staged content\n" {
		t.Fatalf("check changed the staged file: %q, %v", got, err)
	}
	if refs := git(clone, "for-each-ref", "--format=%(refname)",
		"refs/heads/received-side", "refs/tags/fixture-unrelated"); refs != "" {
		t.Fatalf("check fetched unrelated refs: %q", refs)
	}

	git(clone, "restore", "--", "files/environment.conf")
	git(clone, "restore", "--staged", "--", "local-staged.txt")
	rc, out, stderr := run("repo", "update")
	if rc != 0 || !strings.Contains(out, after) || !strings.Contains(out, "files/environment.conf") ||
		!strings.Contains(out, "scripts/configure.sh") || strings.Contains(out, "notes.md") {
		t.Errorf("update: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if final := status("repo", "status"); final.Revision != after || final.Tracking == nil || final.Tracking.Behind != 0 {
		t.Errorf("unexpected final status: %+v", final)
	}
	git(clone, "switch", "--detach", "-q", after)
	if rc, _, stderr := run("repo", "update", "--check"); rc == 0 || !strings.Contains(stderr, "no current Git branch") {
		t.Errorf("detached check: exit=%d stderr=%q", rc, stderr)
	}
	if detached := status("repo", "status"); detached.Revision != after || detached.Tracking != nil {
		t.Errorf("unexpected detached status: %+v", detached)
	}
}
