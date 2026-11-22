package cli

import (
	"bytes"
	"context"
	"github.com/spf13/viper"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCapturePublishesOnlyToAuthorizedPushDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX gh fixture")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	helpers := t.TempDir()
	if err := os.WriteFile(filepath.Join(helpers, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", helpers+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, scenario := range []string{"offline", "push", "multiple", "denied", "invalid", "mirror"} {
		t.Run(scenario, func(t *testing.T) {
			repo := t.TempDir()
			remote := t.TempDir()
			other := t.TempDir()
			git := func(dir string, args ...string) string {
				t.Helper()
				c := exec.Command("git", args...)
				c.Dir = dir
				o, e := c.CombinedOutput()
				if e != nil {
					t.Fatalf("git %v: %v %s", args, e, o)
				}
				return strings.TrimSpace(string(o))
			}
			git(remote, "init", "--bare", "-q")
			git(other, "init", "--bare", "-q")
			git(repo, "init", "-q")
			git(repo, "config", "user.name", "fixture")
			git(repo, "config", "user.email", "fixture@example.invalid")
			for _, f := range []string{"captured.conf", "unrelated.conf", "literal[1].conf"} {
				if e := os.WriteFile(filepath.Join(repo, f), []byte("old\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			git(repo, "add", ".")
			git(repo, "commit", "-qm", "fixture")
			git(repo, "branch", "private-not-for-push")
			git(repo, "tag", "private-tag")
			git(repo, "remote", "add", "origin", other)
			git(repo, "config", "remote.origin.pushurl", remote)
			if scenario == "multiple" {
				git(repo, "config", "--add", "remote.origin.pushurl", other)
			}
			if scenario == "mirror" {
				git(repo, "config", "remote.origin.mirror", "true")
			}
			git(repo, "config", "push.followTags", "true")
			git(repo, "config", "remote.origin.push", "refs/heads/private-not-for-push:refs/heads/private-not-for-push")
			for _, f := range []string{"captured.conf", "unrelated.conf", "literal[1].conf"} {
				if e := os.WriteFile(filepath.Join(repo, f), []byte("new\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			git(repo, "add", "unrelated.conf")
			indexBefore := git(repo, "diff", "--cached", "--binary")
			headBefore := git(repo, "rev-parse", "HEAD")
			v := viper.New()
			v.Set("repo_owner", "fixture")
			if scenario == "denied" {
				v.Set("repo_owner", "someone-else")
			}
			a := &AppContext{RepoRoot: repo, Viper: v, Flags: GlobalFlagState{Offline: scenario == "offline"}}
			cmd := newCaptureCmd(a)
			cmd.SetContext(context.Background())
			var out bytes.Buffer
			cmd.SetOut(&out)
			branch := "sync/independent"
			if scenario == "invalid" {
				branch = "HEAD~1"
			}
			e := executeGitCommitAndPush(cmd, a, branch, "test: independent", []string{"captured.conf", "literal[1].conf"}, true)
			if scenario == "multiple" || scenario == "invalid" {
				if e == nil {
					t.Fatal("expected refusal")
				}
			} else if scenario == "mirror" {
				if e != nil {
					t.Logf("mirror fails closed: %v", e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if scenario == "invalid" {
				if git(repo, "rev-parse", "HEAD") != headBefore {
					t.Fatal("invalid branch changed HEAD")
				}
			} else {
				if got := git(repo, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); got != "captured.conf\nliteral[1].conf" {
					t.Fatalf("wrong commit paths: %q", got)
				}
			}
			if git(repo, "diff", "--cached", "--binary") != indexBefore {
				t.Fatal("unrelated staged bytes changed")
			}
			refs := git(remote, "for-each-ref", "--format=%(refname)")
			if scenario == "push" {
				if refs != "refs/heads/sync/independent" {
					t.Fatalf("published extra refs %q", refs)
				}
			} else if refs != "" {
				t.Fatalf("unexpected publication %q", refs)
			}
			if got := git(other, "for-each-ref", "--format=%(refname)"); got != "" {
				t.Fatalf("published to fetch/secondary URL: %q", got)
			}
		})
	}
}
