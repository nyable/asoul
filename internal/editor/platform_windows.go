package editor

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Create the directory with a protected DACL from the outset, before placing
// any secrets in it. Child files inherit access only for this user and SYSTEM.
func privateDir() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return "", err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("asoul-editor-%x", random))
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return "", err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(path, &sa); err != nil {
		return "", err
	}
	return dir, nil
}

func editorCommand(ctx context.Context, command string, args []string) (*exec.Cmd, error) {
	ext := strings.ToLower(filepath.Ext(command))
	if ext != ".cmd" && ext != ".bat" {
		return exec.CommandContext(ctx, command, args...), nil
	}
	// cmd scripts require cmd.exe. Never permit expansion or shell operators;
	// Windows command-line escaping for executables is not cmd-shell escaping.
	parts := append([]string{command}, args...)
	for i, part := range parts {
		if strings.ContainsAny(part, "\"\r\n&|<>^%!") {
			return nil, fmt.Errorf("unsafe character in Windows editor script argument; configure an .exe instead")
		}
		parts[i] = "\"" + part + "\""
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	c := exec.CommandContext(ctx, shell)
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /v:off /s /c \"" + strings.Join(parts, " ") + "\""}
	return c, nil
}
