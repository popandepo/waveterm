// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellexec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/util/shellutil"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// startOpts is what a cmd block hands to StartLocalShellProc: a fresh swap token, the data folder of the process moved under the test's temp
// folder (the start copies shell integration files there), and the Direct list.
func startOpts(t *testing.T, direct []string) CommandOptsType {
	t.Helper()
	old := wavebase.DataHome_VarCache
	wavebase.DataHome_VarCache = t.TempDir()
	t.Cleanup(func() { wavebase.DataHome_VarCache = old })
	swap := &shellutil.TokenSwapEntry{Token: uuid.NewString(), Env: map[string]string{}, Exp: time.Now().Add(time.Minute)}
	return CommandOptsType{SwapToken: swap, Direct: direct}
}

// The bundled ConPTY (conpty.dll and OpenConsole.exe) is looked up next to the running executable; a test binary lives in a temp folder, so the
// two files from third_party/conpty are put beside it first.
func putConPTYBesideTheTestBinary(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	for _, name := range []string{"conpty.dll", "OpenConsole.exe"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "third_party", "conpty", "x64", name))
		if err != nil {
			t.Skipf("the bundled ConPTY is not in third_party/conpty/x64: %v", err)
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(exe), name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// Through StartLocalShellProc itself, on a real pseudo terminal: a Direct list starts the program with this process as its parent. With the
// branch turned off the program is started as "<shell> -c <line>" and the parent is the shell, so this is what stands behind the branch.
func TestStartLocalShellProcRunsADirectListWithoutAShell(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not on the PATH")
	}
	putConPTYBesideTheTestBinary(t)
	script := `$p = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId; Write-Output ('PPID=' + $p + '=END')`
	opts := startOpts(t, []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", script})
	// the line the old path would have run; with the Direct list it must be ignored
	line := "pwsh -NoProfile -NonInteractive -Command 'Write-Output PPID=0=END'"
	proc, err := StartLocalShellProc(context.Background(), waveobj.TermSize{Rows: 24, Cols: 300}, line, opts, "local")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var mu sync.Mutex
	var out bytes.Buffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := proc.Cmd.Read(buf)
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() { _ = proc.Cmd.Wait(); close(done) }()
	re := regexp.MustCompile(`PPID=(\d+)=END`)
	deadline := time.After(30 * time.Second)
	for {
		mu.Lock()
		m := re.FindStringSubmatch(out.String())
		mu.Unlock()
		if m != nil {
			parent, _ := strconv.Atoi(m[1])
			if parent != os.Getpid() {
				t.Fatalf("the program's parent is %d, not this process %d: a shell was started in front of it", parent, os.Getpid())
			}
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("no parent id in the output after 30 s: %q", out.String())
		case <-done:
			time.Sleep(300 * time.Millisecond) // the last bytes of the terminal
		case <-time.After(100 * time.Millisecond):
		}
	}
	proc.Close()
}

// A program that is not on the PATH is not started, and the error names it (a block that fails says which program it could not find).
func TestStartLocalShellProcRefusesAProgramThatIsNotFound(t *testing.T) {
	opts := startOpts(t, []string{"no-such-program-for-wave-test", "-x"})
	_, err := StartLocalShellProc(context.Background(), waveobj.TermSize{Rows: 24, Cols: 80}, "ignored", opts, "local")
	if err == nil || !strings.Contains(err.Error(), "no-such-program-for-wave-test") {
		t.Fatalf("expected an error naming the program, got %v", err)
	}
}

// A bare name that exists only in the current folder is not run (exec.ErrDot): a claude.cmd in a repo must not stand in for claude.
func TestStartLocalShellProcRefusesAProgramFoundOnlyInTheCurrentFolder(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran.txt")
	planted := "planted-wave-test-program"
	body := "@echo off\r\necho ran> \"" + marker + "\"\r\n"
	if err := os.WriteFile(filepath.Join(dir, planted+".cmd"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	opts := startOpts(t, []string{planted})
	proc, err := StartLocalShellProc(context.Background(), waveobj.TermSize{Rows: 24, Cols: 80}, "ignored", opts, "local")
	if err == nil {
		proc.Close()
		t.Fatalf("the program found only in the current folder was started")
	}
	if !strings.Contains(err.Error(), planted) {
		t.Fatalf("the error does not name the program: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("the planted program ran")
	}
}
