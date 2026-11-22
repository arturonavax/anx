package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/arturonavax/anx/internal/cli"
	"github.com/arturonavax/anx/internal/gittransport"
)

func isolatedCheckoutHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Chdir(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	for _, key := range []string{"ANX_CONFIG_REPO", "ANX_REPO", "ANX_CONFIG_REPO_DIR", "ANX_DOTFILES_URL",
		"REPO_ROOT", "ANX_BOOTSTRAP_ONLY", "ANX_CONFIG_REF", "ANX_SSH_KEY", "ANX_INSTALL_ENV"} {
		t.Setenv(key, "")
	}
	return home
}

func TestInstallerChoosesAlternateEnvironmentCheckout(t *testing.T) {
	for _, scenario := range []string{"existing", "empty", "occupied"} {
		t.Run(scenario, func(t *testing.T) {
			home := isolatedCheckoutHome(t)
			chosen := filepath.Join(home, "custom environment")
			t.Setenv("ANX_CONFIG_REPO_DIR", chosen)
			t.Setenv("ANX_INSTALL_ENV", "1")
			manifests := filepath.Join(chosen, "manifests")
			switch scenario {
			case "existing":
				if err := os.MkdirAll(manifests, 0o755); err != nil {
					t.Fatal(err)
				}
			case "empty", "occupied":
				if err := os.MkdirAll(chosen, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "occupied" {
				if err := os.WriteFile(filepath.Join(chosen, "keep"), []byte("untouched"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			clones := 0
			restore := cli.SetGitAdapterFactoryForTest(func() *gittransport.Adapter {
				return &gittransport.Adapter{Runner: fakeCLIGitRunner{fn: func(
					_ context.Context, _ []string, _, _ string, args ...string,
				) (string, string, error) {
					if args[0] == "ls-remote" {
						return "abcdef012345\tHEAD\n", "", nil
					}
					if args[0] == "clone" && args[len(args)-1] == chosen {
						clones++
						return "", "", os.MkdirAll(manifests, 0o755)
					}
					return "", "", fmt.Errorf("unexpected Git arguments: %q", args)
				}}}
			})
			defer restore()

			appCtx, err := cli.BuildAppContext("test", "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer appCtx.Close()
			root := cli.NewRootCommand(appCtx)
			cmd, _, err := root.Find([]string{"wizard"})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			cmd.RunE = func(*cobra.Command, []string) error {
				called = true
				if appCtx.RepoRoot != chosen {
					t.Errorf("wizard uses %q, want %q", appCtx.RepoRoot, chosen)
				}
				return nil
			}
			root.SetArgs([]string{"wizard"})
			err = root.Execute()
			if scenario == "occupied" {
				data, readErr := os.ReadFile(filepath.Join(chosen, "keep"))
				if err == nil || called || clones != 0 || readErr != nil || string(data) != "untouched" {
					t.Fatalf("occupied path was not protected: err=%v called=%v clones=%d data=%q", err, called, clones, data)
				}
				return
			}
			if err != nil || !called || (clones == 1) != (scenario == "empty") {
				t.Fatalf("unexpected checkout: err=%v called=%v clones=%d", err, called, clones)
			}
			reloaded, err := cli.BuildAppContext("test", "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer reloaded.Close()
			if reloaded.RepoRoot != chosen {
				t.Errorf("chosen checkout was not persisted: got %q, want %q", reloaded.RepoRoot, chosen)
			}
		})
	}
}

func TestAuxiliarySSHCloneDoesNotLinkActiveEnvironment(t *testing.T) {
	home := isolatedCheckoutHome(t)
	env := filepath.Join(home, "environment")
	for _, dir := range []string{"manifests", "files"} {
		if err := os.MkdirAll(filepath.Join(env, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	destination := filepath.Join(home, "private checkout")
	const source = "git@private.example.invalid:owner/extra.git"
	clones := 0
	restore := cli.SetGitAdapterFactoryForTest(func() *gittransport.Adapter {
		return &gittransport.Adapter{Runner: fakeCLIGitRunner{fn: func(
			_ context.Context, _ []string, _, _ string, args ...string,
		) (string, string, error) {
			switch args[0] {
			case "ls-remote":
				return "abcdef012345\tHEAD\n", "", nil
			case "clone":
				if args[len(args)-2] == source && args[len(args)-1] == destination {
					clones++
					return "", "", os.MkdirAll(filepath.Join(destination, ".git"), 0o755)
				}
			}
			return "", "", fmt.Errorf("unexpected Git arguments: %q", args)
		}}}
	})
	defer restore()
	run := func(args ...string) (int, string) {
		t.Helper()
		var out bytes.Buffer
		var stderr bytes.Buffer
		code := cli.ExecuteCLI(args, &out, &stderr, "test", "", "")
		if code != 0 {
			return code, stderr.String()
		}
		return code, out.String()
	}
	if code, detail := run("repo", "set", env); code != 0 {
		t.Fatalf("linking environment: %s", detail)
	}
	t.Setenv("ANX_CONFIG_REF", "active-environment-only")
	for _, args := range [][]string{
		{"repo", "clone", "--no-link", "--dir", destination},
		{"repo", "clone", "unknown://fixture.invalid/repo", "--no-link", "--dir", destination},
		{"repo", "clone", source, "--no-link"},
		{"repo", "clone", source, "--no-link", "--dir", destination, "--offline"},
	} {
		if code, detail := run(args...); code == 0 || clones != 0 {
			t.Errorf("unexpected clone for %v: exit=%d clones=%d detail=%q", args, code, clones, detail)
		}
	}
	if code, detail := run("repo", "clone", source, "--auth", "ssh", "--dir", destination, "--no-link"); code != 0 || clones != 1 {
		t.Fatalf("cloning auxiliary checkout: exit=%d clones=%d detail=%q", code, clones, detail)
	}
	code, output := run("repo", "status", "--format", "json")
	var actual struct {
		Root string `json:"repo_root"`
	}
	if err := json.Unmarshal([]byte(output), &actual); code != 0 || err != nil || actual.Root != env {
		t.Fatalf("active environment changed: exit=%d output=%q parse=%v", code, output, err)
	}
	if code, detail := run("repo", "clone", source, "--auth", "ssh", "--dir", destination, "--no-link"); code == 0 ||
		!strings.Contains(detail, "not an empty directory") || clones != 1 {
		t.Errorf("existing auxiliary checkout not protected: exit=%d clones=%d detail=%q", code, clones, detail)
	}
}
