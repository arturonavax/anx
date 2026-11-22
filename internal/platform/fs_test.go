package platform

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLinkOrCopySymlinkFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "link.txt")

	if err := LinkOrCopy(src, dst); err != nil {
		t.Fatalf("LinkOrCopy failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading linked file: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("content = %q, want %q", string(got), "hello world")
	}
}

func TestLinkOrCopyWindowsFallbackHardlinkAndCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.bin")
	if err := os.WriteFile(src, []byte("binary-payload"), 0o755); err != nil {
		t.Fatal(err)
	}

	privErr := &os.LinkError{
		Op:  "symlink",
		Old: src,
		New: filepath.Join(dir, "hardlink.bin"),
		Err: fs.ErrPermission,
	}

	// 1. Windows privilege error falls back to os.Link for regular files.
	dstHardlink := filepath.Join(dir, "hardlink.bin")
	err := linkOrCopyWithOps(
		src,
		dstHardlink,
		true,
		func(string, string) error { return privErr },
		os.Link,
	)
	if err != nil {
		t.Fatalf("Windows hardlink fallback failed: %v", err)
	}
	got, err := os.ReadFile(dstHardlink)
	if err != nil || string(got) != "binary-payload" {
		t.Fatalf("hardlink content = %q, err = %v", string(got), err)
	}

	// 2. When both symlink and hardlink fail on Windows, falls back to safe file copy.
	dstCopy := filepath.Join(dir, "copied.bin")
	err = linkOrCopyWithOps(
		src,
		dstCopy,
		true,
		func(string, string) error { return privErr },
		func(string, string) error { return errors.New("cross-device link") },
	)
	if err != nil {
		t.Fatalf("Windows copy fallback failed: %v", err)
	}
	got, err = os.ReadFile(dstCopy)
	if err != nil || string(got) != "binary-payload" {
		t.Fatalf("copied content = %q, err = %v", string(got), err)
	}

	// 3. On non-Windows, symlink error is returned directly without silent copy.
	dstUnix := filepath.Join(dir, "unix.bin")
	err = linkOrCopyWithOps(
		src,
		dstUnix,
		false,
		func(string, string) error { return privErr },
		os.Link,
	)
	if err == nil {
		t.Fatal("expected error on non-Windows when symlink fails")
	}
}

func TestLinkOrCopyWindowsFallbackDirectoryCopy(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src-dir")
	nested := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "config.toml"), []byte("key = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(dir, "dst-dir")
	privErr := errors.New("A required privilege is not held by the client.")
	err := linkOrCopyWithOps(
		srcDir,
		dstDir,
		true,
		func(string, string) error { return privErr },
		os.Link,
	)
	if err != nil {
		t.Fatalf("Windows directory copy fallback failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dstDir, "sub", "config.toml"))
	if err != nil || string(got) != "key = 1\n" {
		t.Fatalf("copied dir file = %q, err = %v", string(got), err)
	}
}
