package editor

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsScriptQuotingAndRejection(t *testing.T) {
	c, err := editorCommand(context.Background(), `C:\Program Files\Code\code.cmd`, []string{"--wait", `C:\Users\Test User\草稿.jsonc`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SysProcAttr.CmdLine, `"C:\Program Files\Code\code.cmd" "--wait" "C:\Users\Test User\草稿.jsonc"`) {
		t.Fatal(c.SysProcAttr.CmdLine)
	}
	for _, arg := range []string{"a&b", "%TEMP%", "a\"b", "a!b"} {
		if _, err := editorCommand(context.Background(), "code.cmd", []string{arg}); err == nil {
			t.Fatalf("accepted %q", arg)
		}
	}
}

func TestWindowsPrivateDraftDACL(t *testing.T) {
	dir, err := privateDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sddl := sd.String()
	if !strings.Contains(sddl, "D:P") || !strings.Contains(sddl, user.User.Sid.String()) || strings.Contains(sddl, ";;;WD") {
		t.Fatalf("not protected: %s", sddl)
	}
}

func TestWindowsScriptRunsAndWaits(t *testing.T) {
	dir := t.TempDir()
	script := dir + `\editor test.cmd`
	output := dir + `\draft file.json`
	if err := os.WriteFile(script, []byte("@echo off\r\necho edited>\"%~1\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := editorCommand(context.Background(), script, []string{output})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if data, err := os.ReadFile(output); err != nil || !strings.Contains(string(data), "edited") {
		t.Fatalf("%s %v", data, err)
	}
	// Ensure executable commands bypass cmd.exe.
	exe, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal(err)
	}
	c, err = editorCommand(context.Background(), exe, []string{"/d", "/c", "exit", "0"})
	if err != nil || c.SysProcAttr != nil {
		t.Fatalf("%v %+v", err, c)
	}
}
