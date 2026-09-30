//go:build !windows

package editor

import (
	"context"
	"os"
	"os/exec"
)

func privateDir() (string, error) {
	return os.MkdirTemp("", "asoul-editor-*")
}

func editorCommand(ctx context.Context, command string, args []string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, command, args...), nil
}
