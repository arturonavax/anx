package buildversion

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvance(t *testing.T) {
	cases := []struct {
		subject, body, want string
	}{
		{"fix(cli): validate input", "", "1.2.4"},
		{"perf: reduce allocations", "", "1.2.4"},
		{"feat(cli): add filters", "", "1.3.0"},
		{"FEAT: add filters", "", "1.3.0"},
		{"refactor(cli)!: remove legacy flags", "", "2.0.0"},
		{"feat: replace parser", "Description\n\nBREAKING CHANGE: new grammar", "2.0.0"},
		{"fix: change parser", "BREAKING CHANGE: new grammar\n\nMigration details.", "2.0.0"},
		{"fix: change parser", "Description\n\nRefs: #1\nBREAKING-CHANGE: new grammar", "2.0.0"},
		{"fix: parser", "Notes\nBREAKING CHANGE: example\n\nMore notes.", "1.2.4"},
		{"docs: explain flags", "", "1.2.3"},
	}
	for _, tc := range cases {
		t.Run(tc.subject, func(t *testing.T) {
			v := revision{major: 1, minor: 2, patch: 3}
			v.advance(tc.subject, tc.body)
			if got := v.String(); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveReplaysLocalHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("uses local Git")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is unavailable")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_TEMPLATE_DIR", home)
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		} else {
			return strings.TrimSpace(string(out))
		}
		return ""
	}
	run("init", "-q")
	run("config", "user.name", "Fixture")
	run("config", "user.email", "fixture@example.test")
	run("config", "commit.gpgsign", "false")
	commit := func(subject string) { run("commit", "-q", "--allow-empty", "-m", subject) }
	commit("chore: baseline")
	anchor := run("rev-parse", "HEAD")
	initial, err := resolve(root, anchor)
	if err != nil || initial.Next != "1.0.0" || !strings.HasPrefix(initial.Version, "1.0.0-dev.0+g") {
		t.Fatalf("unexpected checkpoint version: %+v, %v", initial, err)
	}
	commit("fix: first patch")
	commit("fix: second patch")
	commit("docs: document patches")

	info, err := resolve(root, anchor)
	if err != nil || info.Next != "1.0.2" ||
		info.Version != "1.0.2-dev.3+g"+info.Commit || info.Date == "" {
		t.Fatalf("unexpected resolved version: %+v, %v", info, err)
	}
	run("tag", "v99.0.0")
	unchanged, err := resolve(root, anchor)
	if err != nil || unchanged != info {
		t.Fatalf("unrelated tag changed version: %+v, %v", unchanged, err)
	}
	if err := os.WriteFile(filepath.Join(root, "dirty"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := resolve(root, anchor)
	if err != nil || dirty.Version != info.Version+".dirty" || dirty.Next != info.Next {
		t.Fatalf("unexpected dirty version: %+v, %v", dirty, err)
	}
	unknown, err := resolve(root, strings.Repeat("0", 40))
	if err == nil || unknown.Next != "" || !strings.Contains(unknown.Version, "-dev") {
		t.Fatalf("missing checkpoint looked like a release: %+v, %v", unknown, err)
	}
}

func TestResolveArchiveDoesNotClaimStableVersion(t *testing.T) {
	info, err := Resolve(t.TempDir())
	if err == nil || info.Next != "" || info.Version != "1.0.0-dev+unknown" {
		t.Fatalf("archive looked like a release: %+v, %v", info, err)
	}
}
