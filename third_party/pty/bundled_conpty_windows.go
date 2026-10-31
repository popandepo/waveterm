//go:build windows

package pty

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// The pinned files (microsoft/terminal v1.24.12741.0, Microsoft.Windows.Console.ConPTY 1.24.261001001, x64). The same values are in
// third_party/conpty/README.md; tools/ours/Test-BundledConpty.ps1 fails when the two differ or a file in the repo is another one.
const (
	pinnedConptyDLL   = "5afea6c9480e2c7dfd195376dd10996b0ae25cdc6f014e6247fd8aedc8c34852"
	pinnedOpenConsole = "6f8e68dec4e8e5e15a54ecc8afa127b0e62139edc004ffbb4892125ff01d5737"
)

var bundled struct {
	mu     sync.Mutex
	handle windows.Handle
}

// bundledDir is the folder of the running executable. There is no override: nothing in the environment can point this at another folder.
func bundledDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("bundled ConPTY: the folder of the running program cannot be found: %w", err)
	}
	return filepath.Dir(exe), nil
}

// verifyBundled checks that conpty.dll and OpenConsole.exe are in dir and are exactly the pinned files. A missing or different file is an
// error that names it; there is NO fallback to the Windows inbox ConPTY. The check reads the file and the load or start follows it, so a
// program that can write into the folder in between can still swap it: the folder is the application's own install folder, where such a
// program could just as well replace wavesrv itself.
func verifyBundled(dir string) error {
	for _, f := range []struct{ name, pin string }{{"conpty.dll", pinnedConptyDLL}, {"OpenConsole.exe", pinnedOpenConsole}} {
		p := filepath.Join(dir, f.name)
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("bundled ConPTY not found: %s cannot be read (%v); conpty.dll and OpenConsole.exe must sit next to wavesrv; there is no fallback to the Windows inbox ConPTY", p, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != f.pin {
			return fmt.Errorf("bundled ConPTY refused: %s has sha256 %s, not the pinned %s; there is no fallback to the Windows inbox ConPTY", p, got, f.pin)
		}
	}
	return nil
}

// loadBundledConpty loads conpty.dll from the folder of the executable once both files are verified. conpty.dll starts OpenConsole.exe from
// its own folder.
func loadBundledConpty() (windows.Handle, error) {
	bundled.mu.Lock()
	defer bundled.mu.Unlock()
	if bundled.handle != 0 {
		return bundled.handle, nil
	}
	dir, err := bundledDir()
	if err != nil {
		return 0, err
	}
	if err := verifyBundled(dir); err != nil {
		return 0, err
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
	if p.name == "CreatePseudoConsole" {
		// conpty.dll starts OpenConsole.exe now: check both files again right before every new console, not only when the dll was loaded
		dir, err := bundledDir()
		if err == nil {
			err = verifyBundled(dir)
		}
		if err != nil {
			return 0, 0, err
		}
	}
	r1, r2, e := syscall.SyscallN(a, args...)
	return r1, r2, e
}
