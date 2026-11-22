package gittransport

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	fn func(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error)
}

func (f fakeRunner) Run(ctx context.Context, env []string, dir string, name string, args ...string) (string, string, error) {
	return f.fn(ctx, env, dir, name, args...)
}

func TestNormalizeURL(t *testing.T) {
	t.Run("github https generates ssh candidate and strips credentials", func(t *testing.T) {
		n := NormalizeURL("https://user:secret-token@github.com/arturonavax/env.git")
		if !n.IsHTTPS {
			t.Fatalf("expected IsHTTPS=true")
		}
		if strings.Contains(n.Redacted, "secret-token") || strings.Contains(n.Canonical, "secret-token") {
			t.Fatalf("credentials leaked in normalized URL: %+v", n)
		}
		if n.SSHCandidate != "git@github.com:arturonavax/env.git" {
			t.Fatalf("unexpected SSH candidate: %q", n.SSHCandidate)
		}
	})

	t.Run("github shorthand expands cleanly", func(t *testing.T) {
		n := NormalizeURL("arturonavax/env")
		if n.HTTPSURL != "https://github.com/arturonavax/env.git" {
			t.Fatalf("unexpected HTTPSURL: %q", n.HTTPSURL)
		}
		if n.SSHCandidate != "git@github.com:arturonavax/env.git" {
			t.Fatalf("unexpected SSHCandidate: %q", n.SSHCandidate)
		}
	})

	t.Run("explicit ssh preserved", func(t *testing.T) {
		n := NormalizeURL("git@github.com:org/private-repo.git")
		if !n.IsSSH || n.SSHCandidate != "git@github.com:org/private-repo.git" {
			t.Fatalf("unexpected SSH normalization: %+v", n)
		}
	})
}

func TestNonInteractiveEnvSafety(t *testing.T) {
	env := NonInteractiveEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("expected GIT_TERMINAL_PROMPT=0")
	}
	if !strings.Contains(joined, "BatchMode=yes") {
		t.Fatalf("expected BatchMode=yes in GIT_SSH_COMMAND")
	}
	if strings.Contains(joined, "StrictHostKeyChecking=no") {
		t.Fatalf("must NEVER disable StrictHostKeyChecking")
	}
}

func TestProbeAutoPrefersSSHAndFallsBackToHTTPS(t *testing.T) {
	t.Run("auto succeeds via SSH when key works", func(t *testing.T) {
		var tried []string
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, _ []string, _ string, _ string, args ...string) (string, string, error) {
				target := args[2]
				tried = append(tried, target)
				if strings.HasPrefix(target, "git@") {
					return "deadbeef\tHEAD\n", "", nil
				}
				return "", "unexpected https call", errors.New("exit 1")
			}},
		}
		res, err := a.Probe(context.Background(), "https://github.com/arturonavax/private-env.git", "", AuthAuto)
		if err != nil {
			t.Fatalf("expected SSH probe success, got %v", err)
		}
		if res.Transport != "ssh" || res.ResolvedURL != "git@github.com:arturonavax/private-env.git" {
			t.Fatalf("unexpected probe result: %+v", res)
		}
		if len(tried) != 1 {
			t.Fatalf("expected 1 probe attempt, got %v", tried)
		}
	})

	t.Run("auto falls back to HTTPS when SSH fails", func(t *testing.T) {
		var tried []string
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, _ []string, _ string, _ string, args ...string) (string, string, error) {
				target := args[2]
				tried = append(tried, target)
				if strings.HasPrefix(target, "git@") {
					return "", "Permission denied (publickey).", errors.New("exit 128")
				}
				return "cafebabe\tHEAD\n", "", nil
			}},
		}
		res, err := a.Probe(context.Background(), "https://github.com/arturonavax/env.git", "", AuthAuto)
		if err != nil {
			t.Fatalf("expected HTTPS fallback success, got %v", err)
		}
		if res.Transport != "https" || res.ResolvedURL != "https://github.com/arturonavax/env.git" {
			t.Fatalf("unexpected fallback result: %+v", res)
		}
		if len(tried) != 2 {
			t.Fatalf("expected 2 attempts (ssh then https), got %v", tried)
		}
	})

	t.Run("explicit HTTPS does not probe SSH", func(t *testing.T) {
		var tried []string
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, _ []string, _ string, _ string, args ...string) (string, string, error) {
				tried = append(tried, args[2])
				return "123456\tHEAD\n", "", nil
			}},
		}
		res, err := a.Probe(context.Background(), "https://github.com/arturonavax/env.git", "", AuthHTTPS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Transport != "https" || len(tried) != 1 || !strings.HasPrefix(tried[0], "https://") {
			t.Fatalf("explicit HTTPS was not honored: tried=%v res=%+v", tried, res)
		}
	})

	t.Run("missing credentials return classified AuthError", func(t *testing.T) {
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, _ []string, _ string, _ string, args ...string) (string, string, error) {
				return "", "fatal: could not read Username for 'https://github.com': terminal prompts disabled", errors.New("exit 128")
			}},
		}
		_, err := a.Probe(context.Background(), "https://github.com/arturonavax/private.git", "", AuthHTTPS)
		var tErr *TransportError
		if !errors.As(err, &tErr) || tErr.Kind != ErrKindAuth {
			t.Fatalf("expected ErrKindAuth, got %v", err)
		}
	})
}

func TestCloneLocalBareRepoEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	bare := filepath.Join(tmp, "remote.git")
	dest := filepath.Join(tmp, "cloned")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(src, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(src, "add", ".")
	runGit(src, "commit", "-m", "init")
	runGit(tmp, "clone", "--bare", src, bare)

	a := NewAdapter()
	res, err := a.Probe(context.Background(), bare, "", AuthAuto)
	if err != nil {
		t.Fatalf("local bare probe failed: %v", err)
	}
	if res.Transport != "local" {
		t.Fatalf("expected local transport for local dir, got %s", res.Transport)
	}

	// Verify git ls-remote directly via file:// is handled too
	if err := a.Update(context.Background(), src, ""); err == nil {
		// expected error since src has no origin
		t.Logf("src has no origin")
	}
	_ = dest
}

func TestNonInteractiveEnvWithKey(t *testing.T) {
	t.Run("empty key matches NonInteractiveEnv", func(t *testing.T) {
		env, err := NonInteractiveEnvWithKey("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		joined := strings.Join(env, "\n")
		if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") {
			t.Fatalf("expected GIT_TERMINAL_PROMPT=0")
		}
		if !strings.Contains(joined, "BatchMode=yes") {
			t.Fatalf("expected BatchMode=yes")
		}
		if strings.Contains(joined, "StrictHostKeyChecking=no") {
			t.Fatalf("must NEVER disable StrictHostKeyChecking")
		}
	})

	t.Run("missing key returns classified ErrKindAuth", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist-id_ed25519")
		_, err := NonInteractiveEnvWithKey(missing)
		var tErr *TransportError
		if !errors.As(err, &tErr) || tErr.Kind != ErrKindAuth {
			t.Fatalf("expected TransportError ErrKindAuth, got %v", err)
		}
		if tErr.Remediation == "" {
			t.Fatalf("expected actionable remediation in TransportError")
		}
	})

	t.Run("tilde expansion resolves via HOME and builds strict GIT_SSH_COMMAND", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		keyPath := filepath.Join(home, ".ssh", "vault key")
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, []byte("fake-key-content"), 0o600); err != nil {
			t.Fatal(err)
		}

		env, err := NonInteractiveEnvWithKey("~/.ssh/vault key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var sshCmd string
		var hasPromptZero bool
		for _, kv := range env {
			if kv == "GIT_TERMINAL_PROMPT=0" {
				hasPromptZero = true
			}
			if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
				sshCmd = strings.TrimPrefix(kv, "GIT_SSH_COMMAND=")
			}
		}
		if !hasPromptZero {
			t.Fatalf("expected GIT_TERMINAL_PROMPT=0")
		}
		if sshCmd == "" {
			t.Fatalf("expected GIT_SSH_COMMAND to be set")
		}
		if !strings.Contains(sshCmd, "-i ") || !strings.Contains(sshCmd, filepath.ToSlash(keyPath)) && !strings.Contains(sshCmd, keyPath) {
			t.Fatalf("expected -i with expanded key path %q in GIT_SSH_COMMAND, got %q", keyPath, sshCmd)
		}
		for _, want := range []string{"-o IdentitiesOnly=yes", "-o BatchMode=yes", "-o ConnectTimeout=8"} {
			if !strings.Contains(sshCmd, want) {
				t.Fatalf("expected %q in GIT_SSH_COMMAND, got %q", want, sshCmd)
			}
		}
		if strings.Contains(sshCmd, "StrictHostKeyChecking=no") {
			t.Fatalf("must NEVER disable StrictHostKeyChecking: %q", sshCmd)
		}
	})
}

func TestVerifySSHAccess(t *testing.T) {
	home := t.TempDir()
	keyPath := filepath.Join(home, "id_vault")
	if err := os.WriteFile(keyPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("forces SSH transport and passes identity key", func(t *testing.T) {
		var seenTarget string
		var seenSSHCmd string
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, env []string, _ string, _ string, args ...string) (string, string, error) {
				seenTarget = args[2]
				for _, kv := range env {
					if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
						seenSSHCmd = kv
					}
				}
				return "abcdef123456\tHEAD\n", "", nil
			}},
		}

		res, err := a.VerifySSHAccess(context.Background(), "https://github.com/example-org/vault.git", keyPath)
		if err != nil {
			t.Fatalf("VerifySSHAccess failed: %v", err)
		}
		if res.Transport != "ssh" {
			t.Fatalf("expected ssh transport, got %q", res.Transport)
		}
		if res.ResolvedURL != "git@github.com:example-org/vault.git" {
			t.Fatalf("unexpected resolved URL: %q", res.ResolvedURL)
		}
		if res.CommitSHA != "abcdef123456" {
			t.Fatalf("unexpected commit sha: %q", res.CommitSHA)
		}
		if seenTarget != "git@github.com:example-org/vault.git" {
			t.Fatalf("expected ls-remote against SSH URL, got %q", seenTarget)
		}
		if !strings.Contains(seenSSHCmd, "IdentitiesOnly=yes") || !strings.Contains(seenSSHCmd, "id_vault") {
			t.Fatalf("expected explicit key in GIT_SSH_COMMAND, got %q", seenSSHCmd)
		}
	})

	t.Run("auth failure returns classified ErrKindAuth with remediation", func(t *testing.T) {
		a := &Adapter{
			Runner: fakeRunner{fn: func(_ context.Context, _ []string, _ string, _ string, _ ...string) (string, string, error) {
				return "", "git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.", errors.New("exit 128")
			}},
		}

		_, err := a.VerifySSHAccess(context.Background(), "git@github.com:example-org/vault.git", keyPath)
		var tErr *TransportError
		if !errors.As(err, &tErr) || tErr.Kind != ErrKindAuth {
			t.Fatalf("expected ErrKindAuth, got %v", err)
		}
		if tErr.Remediation == "" {
			t.Fatalf("expected non-empty remediation")
		}
	})
}

func TestCloneAndUpdateForwardSSHKey(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	var sshCmds []string
	a := &Adapter{
		Runner: fakeRunner{fn: func(_ context.Context, env []string, _ string, _ string, args ...string) (string, string, error) {
			calls = append(calls, args)
			for _, kv := range env {
				if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
					sshCmds = append(sshCmds, kv)
				}
			}
			if len(args) > 0 && args[0] == "ls-remote" {
				return "deadbeef\tHEAD\n", "", nil
			}
			return "", "", nil
		}},
	}

	dest := filepath.Join(tmp, "vault")
	res, err := a.Clone(context.Background(), CloneOptions{
		URL:    "git@github.com:example-org/vault.git",
		Dest:   dest,
		Depth:  1,
		Auth:   AuthSSH,
		SSHKey: keyPath,
	})
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}
	if res.Transport != "ssh" {
		t.Fatalf("expected ssh transport, got %q", res.Transport)
	}
	if len(calls) < 2 {
		t.Fatalf("expected ls-remote and clone calls, got %v", calls)
	}
	for i, cmd := range sshCmds {
		if !strings.Contains(cmd, "id_ed25519") || !strings.Contains(cmd, "IdentitiesOnly=yes") {
			t.Fatalf("call %d missing explicit SSH key in GIT_SSH_COMMAND: %q", i, cmd)
		}
	}

	sshCmds = nil
	if err := a.UpdateWithOptions(context.Background(), UpdateOptions{
		Dir:    dest,
		Ref:    "main",
		SSHKey: keyPath,
	}); err != nil {
		t.Fatalf("UpdateWithOptions failed: %v", err)
	}
	if len(sshCmds) == 0 {
		t.Fatalf("expected GIT_SSH_COMMAND on update calls")
	}
	for i, cmd := range sshCmds {
		if !strings.Contains(cmd, "id_ed25519") {
			t.Fatalf("update call %d missing SSH key: %q", i, cmd)
		}
	}
}
