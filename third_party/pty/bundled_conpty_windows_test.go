//go:build windows

package pty

import (
	"os"
	"path/filepath"
	"testing"
)

// The bundled files are looked for next to the running executable, which for a test is the test binary in a temp folder: the pinned
// files are copied there. The dll stays loaded for the rest of the process, so the copies are left for go to clean up with the folder.
func copyPinnedNextToExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(exe)
	for _, name := range []string{"conpty.dll", "OpenConsole.exe"} {
		data, err := os.ReadFile(filepath.Join("..", "conpty", "x64", name))
		if err != nil {
			t.Fatalf("the pinned %s cannot be read from third_party/conpty/x64: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// If a bundled file changes after conpty.dll was found and loaded, opening a pseudo console must FAIL. (The first version returned the
// verification error with a zero result, which the caller read as success: a pty with a zero handle.)
func TestOpenFailsWhenOpenConsoleChangesAfterTheDllWasLoaded(t *testing.T) {
	dir := copyPinnedNextToExecutable(t)
	if err := createPseudoConsole.Find(); err != nil {
		t.Fatalf("the pinned files next to the executable should load: %v", err)
	}
	// a first console works
	p, tty, err := open()
	if err != nil {
		t.Fatalf("open() with the pinned files failed: %v", err)
	}
	_ = p.Close()
	_ = tty.Close()
	// now OpenConsole.exe is altered
	f, err := os.OpenFile(filepath.Join(dir, "OpenConsole.exe"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("x")
	_ = f.Close()
	p, tty, err = open()
	if err == nil {
		_ = p.Close()
		_ = tty.Close()
		t.Fatalf("open() succeeded although OpenConsole.exe no longer matches its pinned hash")
	}
	if p != nil || tty != nil {
		t.Fatalf("open() returned a pty together with the error %v", err)
	}
}
