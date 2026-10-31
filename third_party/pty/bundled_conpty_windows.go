//go:build windows

package pty

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// ConptyDirEnv names a folder that holds conpty.dll and OpenConsole.exe, used instead of the folder of the running executable (tests).
const ConptyDirEnv = "WAVETERM_CONPTY_DIR"

var bundled struct {
	mu     sync.Mutex
	handle windows.Handle
}

func conptyDir() (string, error) {
	if d := os.Getenv(ConptyDirEnv); d != "" {
		return d, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("bundled ConPTY: the folder of the running program cannot be found: %w", err)
	}
	return filepath.Dir(exe), nil
}

// loadBundledConpty loads conpty.dll from the folder of the executable (or ConptyDirEnv). conpty.dll starts OpenConsole.exe from its own
// folder, so both must be there. There is NO fallback to the inbox ConPTY: a missing or unloadable file is an error that says so.
func loadBundledConpty() (windows.Handle, error) {
	bundled.mu.Lock()
	defer bundled.mu.Unlock()
	if bundled.handle != 0 {
		return bundled.handle, nil
	}
	dir, err := conptyDir()
	if err != nil {
		return 0, err
	}
	for _, name := range []string{"conpty.dll", "OpenConsole.exe"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			return 0, fmt.Errorf("bundled ConPTY not found: %s is missing (conpty.dll and OpenConsole.exe must sit next to wavesrv; there is no fallback to the Windows inbox ConPTY)", p)
		}
	}
	abs, err := filepath.Abs(filepath.Join(dir, "conpty.dll"))
	if err != nil {
		return 0, fmt.Errorf("bundled ConPTY: %w", err)
	}
	h, err := windows.LoadLibraryEx(abs, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return 0, fmt.Errorf("bundled ConPTY: loading %s failed: %w (there is no fallback to the Windows inbox ConPTY)", abs, err)
	}
	bundled.handle = h
	return h, nil
}

// bundledProc is one exported function of the bundled conpty.dll, with the two methods the call sites use on a lazy proc.
type bundledProc struct{ name string }

func (p bundledProc) addr() (uintptr, error) {
	h, err := loadBundledConpty()
	if err != nil {
		return 0, err
	}
	a, err := windows.GetProcAddress(h, p.name)
	if err != nil {
		return 0, fmt.Errorf("bundled ConPTY: %s is not exported by conpty.dll: %w", p.name, err)
	}
	return a, nil
}

func (p bundledProc) Find() error {
	_, err := p.addr()
	return err
}

func (p bundledProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	a, err := p.addr()
	if err != nil {
		return 0, 0, err
	}
	r1, r2, e := syscall.SyscallN(a, args...)
	return r1, r2, e
}
