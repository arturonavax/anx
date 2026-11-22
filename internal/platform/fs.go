package platform

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LinkOrCopy creates a symlink at linkPath pointing to target.
// On Windows, if symlink creation fails due to missing symlink privilege (e.g.
// non-elevated shell without Developer Mode), it falls back to os.Link for regular
// files or a safe copy for files and directories.
func LinkOrCopy(target, linkPath string) error {
	return linkOrCopyWithOps(target, linkPath, runtime.GOOS == "windows", os.Symlink, os.Link)
}

func linkOrCopyWithOps(
	target string,
	linkPath string,
	isWindows bool,
	symlinkFn func(oldname, newname string) error,
	hardlinkFn func(oldname, newname string) error,
) error {
	err := symlinkFn(target, linkPath)
	if err == nil {
		return nil
	}
	if !isWindows || !isWindowsSymlinkPrivilegeError(err) {
		return err
	}

	srcPath := target
	if !filepath.IsAbs(srcPath) {
		srcPath = filepath.Join(filepath.Dir(linkPath), target)
	}

	info, statErr := os.Stat(srcPath)
	if statErr != nil {
		return err
	}

	if info.IsDir() {
		return copyDirSafe(srcPath, linkPath)
	}

	if hardlinkFn != nil {
		if linkErr := hardlinkFn(srcPath, linkPath); linkErr == nil {
			return nil
		}
	}

	return copyFileSafe(srcPath, linkPath, info.Mode().Perm())
}

func isWindowsSymlinkPrivilegeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "privilege") ||
		strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "operation not permitted")
}

func copyFileSafe(src, dst string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", dst, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".anx-linkcopy-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", dst, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %s: %w", dst, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", dst, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("renaming temp file to %s: %w", dst, err)
	}
	renamed = true
	return nil
}

func copyDirSafe(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(curr string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, curr)
		if err != nil {
			return err
		}
		target := dstDir
		if rel != "." {
			target = filepath.Join(dstDir, rel)
		}

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(curr)
			if err != nil {
				return err
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFileSafe(resolved, target, info.Mode().Perm())
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFileSafe(curr, target, info.Mode().Perm())
	})
}
