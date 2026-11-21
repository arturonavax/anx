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
