package editor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"asoul/internal/model"
)

func TestResolvePreferencesAndWindowsPaths(t *testing.T) {
	look := func(s string) (string, error) {
		if s == "nvim" || s == "code" || s == `C:\Program Files\Neovim\nvim.exe` {
			return s, nil
		}
		return "", errors.New("not found")
	}
	cases := []struct {
		cfg                  *model.EditorConfig
		visual, editor, want string
		args                 []string
		fail                 bool
	}{
		{cfg: &model.EditorConfig{Command: "nvim", Args: []string{"--clean"}}, visual: "code --wait", want: "nvim", args: []string{"--clean"}},
		{visual: `"C:\Program Files\Neovim\nvim.exe" --clean`, want: `C:\Program Files\Neovim\nvim.exe`, args: []string{"--clean"}},
		{visual: `C:\Program Files\Neovim\nvim.exe`, want: `C:\Program Files\Neovim\nvim.exe`},
		{editor: `code --wait "two words"`, want: "code", args: []string{"--wait", "two words"}},
		{want: "nvim"},
		{visual: `"unterminated`, fail: true},
		{cfg: &model.EditorConfig{Command: "missing"}, fail: true},
	}
	for _, c := range cases {
		command, args, err := Resolve(c.cfg, func(key string) string {
			if key == "VISUAL" {
				return c.visual
			}
			return c.editor
		}, look)
		if (err != nil) != c.fail {
			t.Fatalf("%+v: %v", c, err)
		}
		if !c.fail && (command != c.want || !reflect.DeepEqual(args, c.args)) {
			t.Fatalf("got %q %v; want %q %v", command, args, c.want, c.args)
		}
	}
	_, _, err := Resolve(nil, func(string) string { return "" }, func(string) (string, error) { return "", os.ErrNotExist })
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestPrivateDraftRoundTripAndCleanup(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	d, err := Prepare(context.Background(), &model.EditorConfig{Command: exe}, "{\r\n  \"text\": \"中文\"\r\n}\r\n")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Cleanup)
	got, err := d.Read()
	if err != nil || !strings.Contains(got, "中文") || !strings.Contains(got, "\r\n") {
		t.Fatalf("%q %v", got, err)
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{d.Dir, d.Path} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm()&0077 != 0 {
				t.Fatalf("private permissions: %s %v", path, err)
			}
		}
	}
	if err := os.WriteFile(d.Path, []byte("\ufeff{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := d.Read(); err != nil || got != "{}" {
		t.Fatalf("BOM: %q %v", got, err)
	}
	d.Cleanup()
	if _, err := os.Stat(d.Dir); !os.IsNotExist(err) {
		t.Fatal("draft not cleaned")
	}
}

func TestDraftRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary")
	}
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(out, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "draft.jsonc")
	if err := os.Symlink(out, path); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Draft{Dir: dir, Path: path}).Read(); err == nil {
		t.Fatal("escaped draft accepted")
	}
}

func TestDraftRejectsInvalidUTF8AndDirectories(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	d, err := Prepare(context.Background(), &model.EditorConfig{Command: exe}, "{}")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Cleanup()
	if err := os.WriteFile(d.Path, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if err := os.Remove(d.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(d.Path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(); err == nil {
		t.Fatal("directory accepted as draft")
	}
}

func TestExternalEditorHelper(t *testing.T) {
	if os.Getenv("ASOUL_EDITOR_TEST_HELPER") != "1" {
		return
	}
	path := os.Args[len(os.Args)-1]
	if err := os.WriteFile(path, []byte(`{"edited":true}`), 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestEditorProcessEditsOnlyDraft(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	d, err := Prepare(context.Background(), &model.EditorConfig{Command: exe, Args: []string{"-test.run=^TestExternalEditorHelper$", "--"}}, `{"edited":false}`)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Cleanup()
	d.Command.Env = append(os.Environ(), "ASOUL_EDITOR_TEST_HELPER=1")
	if out, err := d.Command.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if got, err := d.Read(); err != nil || got != `{"edited":true}` {
		t.Fatalf("%q %v", got, err)
	}
}
