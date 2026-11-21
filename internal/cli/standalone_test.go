package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arturonavax/anx/internal/cli"
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
