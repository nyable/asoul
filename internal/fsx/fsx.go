package fsx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExpandUser expands the leading ~ in a path to the user's home directory.
func ExpandUser(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Clean(path), nil
}

// AtomicWriteFilePreservePerm writes data atomically, keeping the permission
// bits of an existing file (or 0644 when the file does not exist). This avoids
// widening a private file (e.g. 0600) during a rewrite or rollback.
func AtomicWriteFilePreservePerm(filename string, data []byte) error {
	perm := os.FileMode(0644)
	if info, err := os.Stat(filename); err == nil {
		perm = info.Mode().Perm()
	}
	return AtomicWriteFile(filename, data, perm)
}

// AtomicWriteFile writes data to a temporary file in the same directory and renames it atomically.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode, permissionSource ...string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName) // clean up on error
	source := filename
	if len(permissionSource) > 0 {
		source = permissionSource[0]
	}
	if err := preserveFileAccess(tmpName, source); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to preserve file access: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("failed to replace target file atomically: %w", err)
	}

	return nil
}

// CopyFile copies a single regular file from src to dst.
func CopyFile(src, dst string) error {
	sFi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !sFi.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file (mode: %s)", src, sFi.Mode())
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, sFi.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

// CopyDir recursively copies directory src to dst, validating that no symlinks or special files exist.
func CopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		fi, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("symbolic link forbidden: %s", srcPath)
		case fi.IsDir():
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		case fi.Mode().IsRegular():
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("special file forbidden: %s (mode: %s)", srcPath, fi.Mode())
		}
	}

	return nil
}

// ValidateNoSymlinks verifies that a directory contains only regular files and subdirectories.
func ValidateNoSymlinks(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are forbidden: %s", path)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("special files are forbidden: %s (mode: %s)", path, info.Mode())
		}
		return nil
	})
}

// DirExists reports whether dir exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// FileExists reports whether path exists and is a regular file.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// CanonicalPath returns the cleaned absolute path, expanding leading ~ if present.
func CanonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	exp, err := ExpandUser(p)
	if err != nil {
		exp = p
	}
	abs, err := filepath.Abs(exp)
	if err != nil {
		return filepath.Clean(exp)
	}
	return filepath.Clean(abs)
}

// CompactUser replaces the user's home directory prefix with ~ if applicable.
func CompactUser(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	if strings.HasPrefix(cleanPath, cleanHome+string(filepath.Separator)) {
		return "~" + cleanPath[len(cleanHome):]
	}
	return path
}

// NormalizeWorkspacePath converts a path into a canonical, compact representation (~/... for home paths, absolute for others).
func NormalizeWorkspacePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	canon := CanonicalPath(p)
	if canon == "" {
		return p
	}
	return CompactUser(canon)
}

// SamePath checks whether two path strings refer to the same physical canonical path on disk.
func SamePath(p1, p2 string) bool {
	p1 = strings.TrimSpace(p1)
	p2 = strings.TrimSpace(p2)
	if p1 == p2 && p1 != "" {
		return true
	}
	c1 := CanonicalPath(p1)
	c2 := CanonicalPath(p2)
	return c1 != "" && c1 == c2
}
