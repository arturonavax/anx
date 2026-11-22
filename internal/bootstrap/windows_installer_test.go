package bootstrap_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWindowsInstallerReleaseVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the PowerShell subprocess in short mode")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil && runtime.GOOS == "windows" {
		pwsh, err = exec.LookPath("powershell")
	}
	if err != nil {
		t.Skip("PowerShell is required for the offline Windows release fixture")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	root := filepath.Join("..", "..")
	scratch := t.TempDir()
	args := []string{"-NoProfile", "-NonInteractive"}
	if runtime.GOOS == "windows" {
		args = append(args, "-ExecutionPolicy", "Bypass")
	}
	args = append(args,
		"-File", filepath.Join(root, "tests", "bootstrap_security.ps1"),
		"-BootstrapPath", filepath.Join(root, "scripts", "bootstrap_anx.ps1"),
		"-ScratchRoot", scratch,
	)
	cmd := exec.CommandContext(ctx, pwsh, args...)
	cmd.Env = []string{
		"HOME=" + scratch,
		"USERPROFILE=" + scratch,
		"LOCALAPPDATA=" + scratch,
		"APPDATA=" + scratch,
		"TEMP=" + scratch,
		"TMP=" + scratch,
		"TMPDIR=" + scratch,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("offline Windows release fixture failed: %v\n%s", err, out)
	}
}
