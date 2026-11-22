package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/arturonavax/anx/internal/cli"
	"github.com/arturonavax/anx/internal/gittransport"
)

func TestInstallerEnrollmentBeforeApply(t *testing.T) {
	home := t.TempDir()
	t.Chdir(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("ANX_INSTALL_ENV", "1")
	for _, name := range []string{"ANX_CONFIG_REPO", "ANX_REPO", "REPO_ROOT", "ANX_BOOTSTRAP_ONLY", "ANX_CONFIG_REF"} {
		t.Setenv(name, "")
	}

	dest := filepath.Join(home, ".local", "share", "anx", "env")
	source := "git@fixture.invalid:team/custom.git"
	clones := 0
	restore := cli.SetGitAdapterFactoryForTest(func() *gittransport.Adapter {
		return &gittransport.Adapter{
			Runner: fakeCLIGitRunner{fn: func(_ context.Context, _ []string, _, _ string, args ...string) (string, string, error) {
				switch args[0] {
				case "ls-remote":
					return "abcdef012345\tHEAD\n", "", nil
				case "config":
					return source + "\n", "", nil
				case "rev-parse":
					if args[len(args)-1] == "HEAD^{commit}" || args[len(args)-1] == "pinned^{commit}" {
						return "abcdef012345\n", "", nil
					}
					return "feedface9876\n", "", nil
				case "clone":
					clones++
					if args[len(args)-2] != source || args[len(args)-1] != dest {
						return "", "", fmt.Errorf("unexpected clone arguments: %v", args)
					}
					manifests := filepath.Join(dest, "manifests")
					if err := os.MkdirAll(manifests, 0o755); err != nil {
						return "", "", err
					}
					return "", "", os.WriteFile(
						filepath.Join(manifests, "profiles.yaml"),
						[]byte("profiles:\n  - name: from-enrollment\n    packages: []\n"),
						0o600,
					)
				case "checkout":
					if len(args) != 2 || args[1] != "pinned" {
						return "", "", fmt.Errorf("unexpected ref selection: %v", args)
					}
					return "", "", nil
				default:
					return "", "", fmt.Errorf("unexpected Git command: %v", args)
				}
			}},
		}
	})
	defer restore()

	for _, tc := range []struct {
		name       string
		command    string
		flags      []string
		enroll     bool
		hasProfile bool
		standalone bool
		otherRepo  bool
		wantError  string
	}{
		{name: "plan", command: "plan"},
		{name: "offline", command: "apply", flags: []string{"--offline"}},
		{name: "dry-run", command: "apply", flags: []string{"--dry-run"}},
		{name: "standalone", command: "apply", standalone: true},
		{name: "apply pinned", command: "apply", flags: []string{"--config-ref", "pinned"}, enroll: true, hasProfile: true},
		{name: "wizard reuses checkout", command: "wizard", flags: []string{"--config-ref", "pinned"}, hasProfile: true},
		{name: "mismatched origin", command: "apply", otherRepo: true, wantError: "does not match"},
		{name: "mismatched ref", command: "apply", flags: []string{"--config-ref", "other"}, wantError: "is not at ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.standalone {
				t.Setenv("ANX_INSTALL_ENV", "")
			}
			appCtx, err := cli.BuildAppContext("test", "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer appCtx.Close()
			root := cli.NewRootCommand(appCtx)
			cmd, _, err := root.Find([]string{tc.command})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			cmd.RunE = func(_ *cobra.Command, _ []string) error {
				called = true
				_, hasProfile := appCtx.Catalog.GetProfile("from-enrollment")
				if hasProfile != tc.hasProfile || (appCtx.RepoRoot == dest) != tc.hasProfile {
					t.Errorf("unexpected plan context: root=%q, profile loaded=%v", appCtx.RepoRoot, hasProfile)
				}
				return nil
			}
			var output bytes.Buffer
			root.SetOut(&output)
			selectedRepo := source
			if tc.otherRepo {
				selectedRepo = "git@fixture.invalid:team/different.git"
			}
			root.SetArgs(append([]string{tc.command, "--config-repo", selectedRepo}, tc.flags...))
			before := clones
			err = root.Execute()
			if tc.wantError == "" && err != nil ||
				tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("wanted error %q, got %v; stdout=%q", tc.wantError, err, output.String())
			}
			if called != (tc.wantError == "") || (clones != before) != tc.enroll ||
				strings.Contains(output.String(), "Cloning") != tc.enroll {
				t.Errorf("called=%v clones=%d stdout=%q", called, clones, output.String())
			}
			if _, err := os.Stat(dest); clones == 0 && !os.IsNotExist(err) {
				t.Errorf("command touched the managed checkout before enrollment: %v", err)
			}
		})
	}
}

func TestConfigRepoCloneDoesNotUseEngineSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("ANX_CONFIG_REPO", "")
	t.Setenv("ANX_DOTFILES_URL", "")
	t.Setenv("ANX_GIT_URL", "git@fixture.invalid:team/engine.git")
	var out, stderr bytes.Buffer
	code := cli.ExecuteCLI([]string{"repo", "clone", "--dry-run"}, &out, &stderr, "test", "", "")
	if code != 0 || !strings.Contains(out.String(), cli.DefaultConfigRepoURL) ||
		strings.Contains(out.String(), "team/engine.git") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
