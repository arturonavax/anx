package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arturonavax/anx/internal/cli"
	"github.com/arturonavax/anx/internal/gittransport"
)

func TestANXStandaloneIndependenceAndExternalManifestVerification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("ANX_CONFIG_REPO", "")
	t.Setenv("ANX_REPO", "")

	// 1. Help, version, doctor, tools source list, tools doctor work with ZERO env checkout
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "Usage:"},
		{[]string{"version"}, "anx"},
		{[]string{"doctor"}, "ANX Health Doctor"},
		{[]string{"tools", "source", "list"}, "bundled"},
		{[]string{"tools", "doctor"}, "ANX Tools Diagnostics"},
	} {
		var out, errOut bytes.Buffer
		code := cli.ExecuteCLI(tc.args, &out, &errOut, "2.0.0-test", "abc1234", "2026-11-21")
		if code != 0 {
			t.Fatalf("anx %v failed with exit %d: %s", tc.args, code, errOut.String())
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("anx %v output missing %q:\n%s", tc.args, tc.want, out.String())
		}
	}

	// 2. Valid external configuration repository passes `anx repo set` + `anx repo verify`
	extRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(extRepo, "manifests", "packages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(extRepo, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(extRepo, "files", "sample.conf"), []byte("ok=true\n"), 0o644)
	_ = os.WriteFile(filepath.Join(extRepo, "manifests", "configs.yaml"), []byte(`
configurations:
  - id: sample-conf
    group: core
    source: files/sample.conf
    target: ~/.sample.conf
`), 0o644)
	_ = os.WriteFile(filepath.Join(extRepo, "manifests", "profiles.yaml"), []byte(`
profiles:
  - name: custom
    packages: [git]
    configurations: [sample-conf]
`), 0o644)

	var out, errOut bytes.Buffer
	if code := cli.ExecuteCLI([]string{"repo", "set", extRepo}, &out, &errOut, "2.0.0-test", "", ""); code != 0 {
		t.Fatalf("repo set failed (%d): %s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	t.Setenv("ANX_CONFIG_REPO", extRepo)
	if code := cli.ExecuteCLI([]string{"repo", "verify"}, &out, &errOut, "2.0.0-test", "", ""); code != 0 {
		t.Fatalf("repo verify valid failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "[VALID]") {
		t.Fatalf("expected [VALID], got:\n%s", out.String())
	}

	// 3. Invalid external manifest produces clear error
	badRepo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(badRepo, "manifests"), 0o755)
	_ = os.WriteFile(filepath.Join(badRepo, "manifests", "broken.yaml"), []byte("packages:\n  - id: bad\n    strategies: []\n"), 0o644)
	t.Setenv("ANX_CONFIG_REPO", badRepo)
	out.Reset()
	errOut.Reset()
	if code := cli.ExecuteCLI([]string{"repo", "verify"}, &out, &errOut, "2.0.0-test", "", ""); code == 0 {
		t.Fatalf("expected repo verify to fail for invalid manifest, got exit 0:\n%s", out.String())
	}
}

type fakeCLIGitRunner struct {
	fn func(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error)
}

func TestCLIManagedEnvironmentCheckout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("ANX_CONFIG_REPO", "")
	t.Setenv("ANX_REPO", "")
	t.Setenv("REPO_ROOT", "")
	t.Setenv("ANX_SSH_KEY", "")

	dest := filepath.Join(home, ".local", "share", "anx", "env")
	marker := filepath.Join(dest, "manifests", "uncommitted.yaml")
	engine := filepath.Join(home, ".local", "share", "anx", "repo", "engine-marker")
	if err := os.MkdirAll(filepath.Dir(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(engine, []byte("engine"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoURL := "https://github.com/example-org/different-name"
	var clones int
	restore := cli.SetGitAdapterFactoryForTest(func() *gittransport.Adapter {
		return &gittransport.Adapter{Runner: fakeCLIGitRunner{fn: func(
			_ context.Context, _ []string, _, _ string, args ...string,
		) (string, string, error) {
			switch args[0] {
			case "ls-remote":
				return "cafebabe\tHEAD\n", "", nil
			case "clone":
				clones++
				if args[len(args)-2] != "git@github.com:example-org/different-name.git" {
					t.Errorf("expected SSH clone, got %v", args)
				}
				if got := args[len(args)-1]; got != dest {
					return "", "", errors.New("clone targeted " + got + " instead of " + dest)
				}
				return "", "", os.MkdirAll(filepath.Dir(marker), 0o755)
			default:
				return "", "", errors.New("unexpected Git command " + args[0])
			}
		}}}
	})
	defer restore()

	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := cli.ExecuteCLI(args, &out, &errOut, "2.0.0-test", "", "")
		return code, out.String(), errOut.String()
	}
	code, out, stderr := run("repo", "clone", repoURL, "--dry-run")
	if code != 0 || !strings.Contains(out, dest) || clones != 0 {
		t.Errorf("dry-run: exit=%d clones=%d stdout=%q stderr=%q", code, clones, out, stderr)
	}
	code, out, stderr = run("repo", "clone", repoURL)
	if code != 0 || !strings.Contains(out, "Linked configuration repository: "+dest) || clones != 1 {
		t.Errorf("clone: exit=%d clones=%d stdout=%q stderr=%q", code, clones, out, stderr)
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("# local changes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	otherURL := "https://github.com/example-org/other"
	for _, action := range []string{"clone", "set"} {
		code, out, stderr = run("repo", action, otherURL)
		if code == 0 || !strings.Contains(stderr, dest) || clones != 1 {
			t.Errorf("%s: exit=%d clones=%d stdout=%q stderr=%q", action, code, clones, out, stderr)
		}
	}
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			target := filepath.Join(home, kind)
			if kind == "symlink" {
				if err := os.Symlink(dest, target); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("cannot create a symlink on this Windows host: %v", err)
					}
					t.Fatal(err)
				}
			} else if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := run("repo", "clone", otherURL, "--dir", target)
			if code != 2 || !strings.Contains(stderr, "not an empty directory") || clones != 1 {
				t.Errorf("exit=%d clones=%d stdout=%q stderr=%q", code, clones, out, stderr)
			}
			if kind == "symlink" {
				if destination, err := os.Readlink(target); err != nil || destination != dest {
					t.Errorf("symlink changed: destination=%q err=%v", destination, err)
				}
			} else if data, err := os.ReadFile(target); err != nil || string(data) != "unrelated" {
				t.Errorf("file changed: contents=%q err=%v", data, err)
			}
		})
	}
	for path, want := range map[string]string{engine: "engine", marker: "# local changes\n"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Errorf("%s changed: contents=%q err=%v", path, data, err)
		}
	}
	code, out, stderr = run("repo", "status", "--format", "json")
	if code != 0 || !strings.Contains(out, "different-name") || strings.Contains(out, "example-org/other") {
		t.Errorf("status after attempted switch: exit=%d stdout=%q stderr=%q", code, out, stderr)
	}
}

func (f fakeCLIGitRunner) Run(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error) {
	return f.fn(ctx, env, dir, name, args...)
}

func TestCLIToolsSourceSSHKeyAndVerifyAndRepoSSHKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("ANX_CONFIG_REPO", "")
	t.Setenv("ANX_REPO", "")
	t.Setenv("ANX_SSH_KEY", "")

	keyPath := filepath.Join(home, ".ssh", "id_vault")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("fake-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	var seenSSHCmds []string
	failAuth := false
	restore := cli.SetGitAdapterFactoryForTest(func() *gittransport.Adapter {
		return &gittransport.Adapter{
			Runner: fakeCLIGitRunner{fn: func(_ context.Context, env []string, _ string, _ string, args ...string) (string, string, error) {
				for _, kv := range env {
					if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
						seenSSHCmds = append(seenSSHCmds, kv)
					}
				}
				if failAuth {
					return "", "git@github.com: Permission denied (publickey).", errors.New("exit 128")
				}
				if len(args) > 0 && args[0] == "ls-remote" {
					return "cafebabe\tHEAD\n", "", nil
				}
				if len(args) > 0 && args[0] == "clone" {
					dest := args[len(args)-1]
					if err := os.MkdirAll(filepath.Join(dest, "bin"), 0o755); err != nil {
						return "", err.Error(), err
					}
					if err := os.MkdirAll(filepath.Join(dest, "manifests"), 0o755); err != nil {
						return "", err.Error(), err
					}
					if err := os.WriteFile(filepath.Join(dest, "tool.yaml"), []byte(`
schema_version: 1
id: vault-cli
name: "Vault CLI"
executable: vault-cli
entrypoint: bin/vault.sh
language: bash
`), 0o644); err != nil {
						return "", err.Error(), err
					}
					if err := os.WriteFile(filepath.Join(dest, "bin", "vault.sh"), []byte("#!/bin/sh\necho vault\n"), 0o755); err != nil {
						return "", err.Error(), err
					}
					return "", "", nil
				}
				return "", "", nil
			}},
		}
	})
	defer restore()

	// 1. Direct SSH URL verification via `anx tools source verify <url> --ssh-key <key>`
	var out, errOut bytes.Buffer
	code := cli.ExecuteCLI(
		[]string{"tools", "source", "verify", "git@github.com:example-org/vault.git", "--ssh-key", keyPath},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("tools source verify URL failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "SSH access verified") {
		t.Fatalf("expected SSH access verified in stdout, got:\n%s", out.String())
	}
	if len(seenSSHCmds) == 0 || !strings.Contains(seenSSHCmds[0], "id_vault") {
		t.Fatalf("expected GIT_SSH_COMMAND with id_vault, got %v", seenSSHCmds)
	}

	// 2. Auth failure on verify returns non-zero with clear error
	failAuth = true
	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCLI(
		[]string{"tools", "source", "verify", "git@github.com:example-org/vault.git", "--ssh-key", keyPath},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code == 0 {
		t.Fatalf("expected verify to fail on SSH auth error, got exit 0")
	}
	failAuth = false

	// 3. `anx tools source add` with `--ssh-key` and `--optional`
	seenSSHCmds = nil
	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCLI(
		[]string{"tools", "source", "add", "git@github.com:example-org/vault.git", "--id", "vault", "--ssh-key", keyPath, "--optional"},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("tools source add --ssh-key failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "SSH access verified") || !strings.Contains(out.String(), `Added tool source "vault"`) {
		t.Fatalf("unexpected add output:\n%s", out.String())
	}

	// 4. Verify registered source by ID uses its stored SSH key
	seenSSHCmds = nil
	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCLI(
		[]string{"tools", "source", "verify", "vault"},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("tools source verify vault failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "SSH access verified") || !strings.Contains(out.String(), `Verified tool source "vault"`) {
		t.Fatalf("unexpected verify output:\n%s", out.String())
	}

	// 5. `anx repo clone` defaults to https://github.com/arturonavax/env and supports --ssh-key
	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCLI(
		[]string{"repo", "clone", "--dry-run"},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("repo clone --dry-run failed (%d): %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), cli.DefaultConfigRepoURL) {
		t.Fatalf("expected default config repo %q in dry-run output, got:\n%s", cli.DefaultConfigRepoURL, out.String())
	}

	seenSSHCmds = nil
	out.Reset()
	errOut.Reset()
	repoDest := filepath.Join(home, "private-env-checkout")
	code = cli.ExecuteCLI(
		[]string{"repo", "clone", "git@github.com:example-org/private-env.git", "--dir", repoDest, "--auth", "ssh", "--ssh-key", keyPath},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("repo clone --ssh-key failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if len(seenSSHCmds) == 0 || !strings.Contains(seenSSHCmds[0], "id_vault") {
		t.Fatalf("expected repo clone to forward --ssh-key in GIT_SSH_COMMAND, got %v", seenSSHCmds)
	}

	// 6. Credentials embedded in ssh:// URLs must be redacted in stdout/stderr
	out.Reset()
	errOut.Reset()
	code = cli.ExecuteCLI(
		[]string{"tools", "source", "verify", "ssh://user:secret-token@github.com/example-org/vault.git", "--ssh-key", keyPath},
		&out,
		&errOut,
		"2.0.0-test",
		"",
		"",
	)
	if code != 0 {
		t.Fatalf("tools source verify ssh:// URL failed (%d): stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "secret-token") || strings.Contains(errOut.String(), "secret-token") {
		t.Fatalf("expected embedded credentials to be redacted from output, got stdout=%q stderr=%q", out.String(), errOut.String())
	}
}
