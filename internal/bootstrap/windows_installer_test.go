package bootstrap_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
	if os.Getenv("ANX_TEST_WINDOWS_INSTALL_EXE") != "1" {
		return
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("go version go1.27.3 windows/amd64")
	case "run":
		fmt.Println("-X main.version=fixture")
	case "build":
		if len(os.Args) < 4 {
			os.Exit(2)
		}
		for i, arg := range os.Args[2 : len(os.Args)-1] {
			if arg == "-o" {
				self, err := os.Executable()
				if err != nil {
					os.Exit(2)
				}
				binary, err := os.ReadFile(self)
				if err != nil || os.WriteFile(os.Args[i+3], binary, 0o700) != nil {
					os.Exit(2)
				}
				os.Exit(0)
			}
		}
		os.Exit(2)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

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
		"-BootstrapPath", filepath.Join(root, "install.ps1"),
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

func TestWindowsInstallerOneLine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the PowerShell subprocess in short mode")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil && runtime.GOOS == "windows" {
		pwsh, err = exec.LookPath("powershell")
	}
	if err != nil {
		t.Skip("PowerShell is required for the offline Windows installer fixture")
	}
	fixture, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"release", "source-fallback"} {
		t.Run(kind, func(t *testing.T) {
			scratch := t.TempDir()
			args := []string{"-NoProfile", "-NonInteractive"}
			if runtime.GOOS == "windows" {
				args = append(args, "-ExecutionPolicy", "RemoteSigned")
			}
			args = append(args, "-File", filepath.Join(root, "tests", "windows_install_entry.ps1"),
				"-InstallerPath", filepath.Join(root, "install.ps1"), "-ScratchRoot", scratch,
				"-FixtureExecutable", fixture)
			if kind == "source-fallback" {
				args = append(args, "-SourceFallback")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, pwsh, args...)
			cmd.Env = []string{
				"HOME=" + scratch, "USERPROFILE=" + scratch, "LOCALAPPDATA=" + scratch, "APPDATA=" + scratch,
				"TEMP=" + scratch, "TMP=" + scratch, "TMPDIR=" + scratch, "ANX_TEST_WINDOWS_INSTALL_EXE=1",
				"ANX_NO_PATH=1",
			}
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "Installed ANX at") {
				t.Fatalf("offline one-line installer fixture failed: %v\n%s", err, out)
			}
		})
	}
}
