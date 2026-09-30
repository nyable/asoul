// Package editor prepares private drafts and resolves external editor commands.
// It does not launch a shell or write the original configuration file.
package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"asoul/internal/model"
)

var ErrUnavailable = errors.New("no external editor available")

const maxDraftBytes = 8 << 20

type Draft struct {
	Dir     string
	Path    string
	Command *exec.Cmd
}

// Prepare performs filesystem work before Bubble Tea releases the terminal.
func Prepare(ctx context.Context, cfg *model.EditorConfig, content string) (*Draft, error) {
	command, args, err := Resolve(cfg, os.Getenv, exec.LookPath)
	if err != nil {
		return nil, err
	}
	dir, err := privateDir()
	if err != nil {
		return nil, err
	}
	d := &Draft{Dir: dir, Path: filepath.Join(dir, "draft.jsonc")}
	if err := os.WriteFile(d.Path, []byte(content), 0600); err != nil {
		d.Cleanup()
		return nil, err
	}
	args = append(args, d.Path)
	d.Command, err = editorCommand(ctx, command, args)
	if err != nil {
		d.Cleanup()
		return nil, err
	}
	d.Command.Dir = dir
	return d, nil
}

func (d *Draft) Cleanup() {
	// Only delete the private, randomly allocated draft directory.
	_ = os.RemoveAll(d.Dir)
}

func (d *Draft) Read() (string, error) {
	resolved, err := filepath.EvalSymlinks(d.Path)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(d.Dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel != filepath.Base(d.Path) {
		return "", fmt.Errorf("draft path escaped its private directory")
	}
	entry, err := os.Lstat(d.Path)
	if err != nil || !entry.Mode().IsRegular() {
		return "", fmt.Errorf("draft is not a regular file")
	}
	f, err := os.Open(d.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("draft is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDraftBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxDraftBytes || !utf8.Valid(data) {
		return "", fmt.Errorf("draft must be UTF-8 and at most %d bytes", maxDraftBytes)
	}
	return strings.TrimPrefix(string(data), "\ufeff"), nil
}

// Resolve prioritizes explicit config, VISUAL, EDITOR, then terminal editors.
// An invalid explicit preference is reported, not silently replaced.
func Resolve(cfg *model.EditorConfig, getenv func(string) string, lookPath func(string) (string, error)) (string, []string, error) {
	if cfg != nil && strings.TrimSpace(cfg.Command) != "" {
		path, err := lookPath(cfg.Command)
		return path, append([]string(nil), cfg.Args...), err
	}
	for _, name := range []string{"VISUAL", "EDITOR"} {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			continue
		}
		// A bare executable path with spaces is unambiguous if it exists.
		if path, err := lookPath(value); err == nil {
			return path, nil, nil
		}
		parts, err := splitCommand(value)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", name, err)
		}
		path, err := lookPath(parts[0])
		return path, parts[1:], err
	}
	for _, name := range []string{"nvim", "vim", "vi", "nano"} {
		if path, err := lookPath(name); err == nil {
			return path, nil, nil
		}
	}
	return "", nil, ErrUnavailable
}

// Quotes group arguments; backslashes are literal (including Windows paths).
// This deliberately does not support shell expansion, redirects or pipelines.
func splitCommand(value string) ([]string, error) {
	var parts []string
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range value {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
		} else if unicode.IsSpace(r) {
			if started {
				parts = append(parts, word.String())
				word.Reset()
				started = false
			}
		} else {
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated editor command quote")
	}
	if started {
		parts = append(parts, word.String())
	}
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("empty editor command")
	}
	return parts, nil
}
