package gitx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"asoul/internal/progress"
)

// Client wraps safe system git executions.
type Client struct {
	gitPath string
}

// NewClient returns a new Git Client.
func NewClient() *Client {
	return &Client{gitPath: "git"}
}

// CheckInstalled verifies that git is available and executable.
func (c *Client) CheckInstalled(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git is not installed or not in PATH: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CloneMirror clones a remote repository as a bare repository into destDir.
func (c *Client) CloneMirror(ctx context.Context, remoteURL, destDir string, onProgress ...progress.Func) error {
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir for git cache: %w", err)
	}
	cb := progress.First(onProgress...)
	// Use --bare --progress instead of --mirror to avoid fetching heavy internal/PR refs.
	if err := c.runWithProgress(ctx, "", cb, "clone", "--bare", "--progress", remoteURL, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return err
	}
	// Set remote.origin.fetch so subsequent git fetches update branch heads.
	_ = c.run(ctx, "", "--git-dir", destDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/heads/*")
	return nil
}

// Fetch runs git fetch inside a bare repository.
func (c *Client) Fetch(ctx context.Context, repoDir string, onProgress ...progress.Func) error {
	return c.runWithProgress(ctx, "", progress.First(onProgress...), "--git-dir", repoDir, "fetch", "origin", "+refs/heads/*:refs/heads/*", "--tags", "--prune", "--progress")
}

// ShowFile outputs the contents of a file at a specific commit from a bare repo.
func (c *Client) ShowFile(ctx context.Context, repoDir, commit, filePath string) ([]byte, error) {
	spec := fmt.Sprintf("%s:%s", commit, filepath.ToSlash(filePath))
	cmd := exec.CommandContext(ctx, c.gitPath, "--git-dir", repoDir, "show", spec)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show failed for %s: %w", spec, err)
	}
	return out, nil
}

// ResolveRef resolves a branch, tag, or commit ref to a 40-character hex commit ID.
func (c *Client) ResolveRef(ctx context.Context, repoDir, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	// Try rev-parse ref^{commit}
	cmd := exec.CommandContext(ctx, c.gitPath, "--git-dir", repoDir, "rev-parse", "--verify", ref+"^{commit}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Fallback: try raw ref
		cmd2 := exec.CommandContext(ctx, c.gitPath, "--git-dir", repoDir, "rev-parse", "--verify", ref)
		var out2, err2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Stderr = &err2
		if err2Run := cmd2.Run(); err2Run != nil {
			return "", fmt.Errorf("failed to resolve git ref %q in %s: %s", ref, repoDir, strings.TrimSpace(stderr.String()))
		}
		return strings.TrimSpace(out2.String()), nil
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ArchiveExtract extracts a path at a specific commit from a bare repo to destDir using git archive.
func (c *Client) ArchiveExtract(ctx context.Context, repoDir, commit, subpath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destDir: %w", err)
	}

	treeIsh := commit
	if subpath != "" && subpath != "." {
		cleanSub := strings.Trim(filepath.ToSlash(subpath), "/")
		treeIsh = fmt.Sprintf("%s:%s", commit, cleanSub)
	}

	// git --git-dir <repoDir> archive <treeIsh> | tar -x -C <destDir>
	archiveCmd := exec.CommandContext(ctx, c.gitPath, "--git-dir", repoDir, "archive", treeIsh)
	tarCmd := exec.CommandContext(ctx, "tar", "-x", "-C", destDir)

	pr, pw := io.Pipe()
	archiveCmd.Stdout = pw
	var archiveErr bytes.Buffer
	archiveCmd.Stderr = &archiveErr

	tarCmd.Stdin = pr
	var tarErr bytes.Buffer
	tarCmd.Stderr = &tarErr

	if err := archiveCmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("failed to start git archive: %w", err)
	}

	if err := tarCmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("failed to start tar: %w", err)
	}

	errArchive := archiveCmd.Wait()
	pw.Close() // notify tar that archive is done
	errTar := tarCmd.Wait()

	if errArchive != nil {
		return fmt.Errorf("git archive failed (%s): %s", errArchive, strings.TrimSpace(archiveErr.String()))
	}
	if errTar != nil {
		return fmt.Errorf("tar extraction failed (%s): %s", errTar, strings.TrimSpace(tarErr.String()))
	}

	return nil
}

// ArchiveExtractPaths extracts multiple paths at a specific commit from a bare repo to destDir using git archive.
// If subpaths is empty or contains root ("." or ""), the whole repo tree is extracted.
func (c *Client) ArchiveExtractPaths(ctx context.Context, repoDir, commit string, subpaths []string, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destDir: %w", err)
	}

	args := []string{"--git-dir", repoDir, "archive", commit}
	hasRoot := false
	var cleanPaths []string
	seen := make(map[string]bool)

	for _, p := range subpaths {
		cleaned := strings.Trim(filepath.ToSlash(p), "/")
		if cleaned == "" || cleaned == "." {
			hasRoot = true
			break
		}
		if !seen[cleaned] {
			seen[cleaned] = true
			cleanPaths = append(cleanPaths, cleaned)
		}
	}

	if !hasRoot && len(cleanPaths) > 0 {
		args = append(args, cleanPaths...)
	}

	archiveCmd := exec.CommandContext(ctx, c.gitPath, args...)
	tarCmd := exec.CommandContext(ctx, "tar", "-x", "-C", destDir)

	pr, pw := io.Pipe()
	archiveCmd.Stdout = pw
	var archiveErr bytes.Buffer
	archiveCmd.Stderr = &archiveErr

	tarCmd.Stdin = pr
	var tarErr bytes.Buffer
	tarCmd.Stderr = &tarErr

	if err := archiveCmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("failed to start git archive: %w", err)
	}

	if err := tarCmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("failed to start tar: %w", err)
	}

	errArchive := archiveCmd.Wait()
	pw.Close()
	errTar := tarCmd.Wait()

	if errArchive != nil {
		return fmt.Errorf("git archive failed (%s): %s", errArchive, strings.TrimSpace(archiveErr.String()))
	}
	if errTar != nil {
		return fmt.Errorf("tar extraction failed (%s): %s", errTar, strings.TrimSpace(tarErr.String()))
	}

	return nil
}

// ListTreeLists files in a tree path at a commit.
func (c *Client) ListTree(ctx context.Context, repoDir, commit, subpath string) ([]string, error) {
	args := []string{"--git-dir", repoDir, "ls-tree", "-r", "--name-only", commit}
	if subpath != "" && subpath != "." {
		cleanSub := strings.Trim(filepath.ToSlash(subpath), "/")
		args = append(args, cleanSub)
	}
	cmd := exec.CommandContext(ctx, c.gitPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree failed: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

// DiffNoIndex generates unified diff between two directories using git diff --no-index.
func (c *Client) DiffNoIndex(ctx context.Context, dirA, dirB string) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath, "diff", "--no-index", dirA, dirB)
	out, err := cmd.CombinedOutput()
	// git diff returns 0 for identical, 1 for differences, >1 for error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return string(out), nil
			}
		}
		return "", fmt.Errorf("git diff --no-index failed: %s: %w", string(out), err)
	}
	return string(out), nil
}

func (c *Client) run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, c.gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git command failed (%s): %s", strings.Join(args, " "), msg)
	}
	return nil
}

func (c *Client) runWithProgress(ctx context.Context, dir string, onProgress progress.Func, args ...string) error {
	cmd := exec.CommandContext(ctx, c.gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Force a stable, parseable English locale so progress lines can be
	// translated into structured counts regardless of the user's environment.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "LANG=C")

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return c.run(ctx, dir, args...)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start git command (%s): %w", strings.Join(args, " "), err)
	}

	var fullStderr bytes.Buffer
	buf := make([]byte, 512)
	var currentLine bytes.Buffer
	lastEmitted := ""

	emit := func(line string) {
		if line == "" || onProgress == nil || line == lastEmitted {
			return
		}
		lastEmitted = line
		if u, ok := progress.ParseGitLine(line); ok {
			onProgress(u)
			return
		}
		onProgress(progress.Update{Phase: progress.PhaseUnknown, Text: line})
	}

	for {
		n, readErr := stderrPipe.Read(buf)
		if n > 0 {
			fullStderr.Write(buf[:n])
			for _, b := range buf[:n] {
				if b == '\r' || b == '\n' {
					emit(strings.TrimSpace(currentLine.String()))
					currentLine.Reset()
				} else {
					currentLine.WriteByte(b)
				}
			}
		}
		if readErr != nil {
			break
		}
	}
	emit(strings.TrimSpace(currentLine.String()))

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(fullStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git command failed (%s): %s", strings.Join(args, " "), msg)
	}
	return nil
}
